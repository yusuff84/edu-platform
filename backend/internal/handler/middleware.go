// Package handler — HTTP-слой: роутер, middleware и обработчики API.
package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"edu-platform/backend/internal/auth"
	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/payments"
	"edu-platform/backend/internal/repo"
	"edu-platform/backend/internal/storage"
)

// App — общие зависимости обработчиков.
// Store — интерфейс хранилища: in-memory (dev без БД) или Postgres (pgxpool).
type App struct {
	Store  repo.StoreAPI
	Config *config.Config

	// YandexAuth — OAuth-провайдер для входа через Яндекс ID (T13).
	// Создаётся один раз в NewRouter из Config; тесты подменяют поле до
	// сборки роутера.
	YandexAuth auth.AuthProvider

	// Storage — хранилище файлов для обложек (T54). nil → LocalStore в
	// каталоге ./uploads — тот же каталог, что раздаёт маршрут /uploads/*.
	// Тесты подставляют свою реализацию, чтобы не писать на диск.
	Storage storage.ObjectStore

	// Лениво создаваемые rate-limiter'ы по имени группы (T10).
	limitersMu   sync.Mutex
	limitersOnce sync.Once
	limiters     map[string]*rateLimiter

	// webhookMu сериализует применение вебхуков оплаты (находка приёмки P5):
	// проверка «статус уже терминальный → duplicate» и запись нового статуса
	// должны быть одной критической секцией, иначе одновременные доставки
	// одного события обе отвечают processed (тест T35 под нагрузкой видел
	// processed=2). Вебхуки редки — общий замок на приложение достаточен.
	webhookMu sync.Mutex
}

type ctxKey string

const userCtxKey ctxKey = "user"

const sessionCookie = "session_id"

// sessionTTL — время жизни сессии (7 дней).
const sessionTTL = 7 * 24 * time.Hour

// NewRouter собирает chi-роутер: /healthz, /pay/mock, /api/v1, логгер, recover и CORS.
func NewRouter(app *App) http.Handler {
	// T13: OAuth-клиент Яндекса создаётся один раз при сборке роутера
	// (в тестах поле App.YandexAuth заполняется заранее — не перезаписываем).
	if app.YandexAuth == nil {
		app.YandexAuth = &auth.YandexClient{
			ClientID:     app.Config.YandexClientID,
			ClientSecret: app.Config.YandexClientSecret,
			RedirectURI:  app.Config.YandexRedirectURI,
		}
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(app.realIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// T42: cors получает сам роутер — для preflight Allow-Methods считаются
	// по реально зарегистрированным маршрутам через Mux.Match (Match есть
	// только у *chi.Mux, не у интерфейса chi.Router).
	r.Use(app.cors(r))

	r.Get("/healthz", healthz)
	// T09: mock-страница оплаты. Регистрируется НА ВЕРХНЕМ УРОВНЕ роутера,
	// а не под /api/v1 — redirectUrl, который возвращает провайдер, равен
	// "/pay/mock/{id}" (payments.MockConfirmPath) и браузер пойдёт именно туда.
	// Страница сама шлёт POST /api/v1/payments/webhook/mock, поэтому доступ
	// к курсу активируется тем же кодом, что и для реальных провайдеров.
	// Без этого маршрута mock-покупка не работает вообще.
	// chi r.Get требует http.HandlerFunc — оборачиваем Handler через ServeHTTP.
	r.Get("/pay/mock/{id}", payments.MockConfirmHandler().ServeHTTP)
	// T13: обслуживание уже загруженных файлов (обложки курсов и т.п.,
	// см. internal/storage). Загрузочного эндпоинта в MVP нет — раздаются
	// только файлы, которые уже лежат в каталоге uploads/.
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))
	r.Route("/api/v1", func(r chi.Router) {
		// T10: глобальный мягкий лимит от флуда + CSRF-проверка меняющих методов.
		r.Use(app.rateLimit("global", app.Config.RateLimitGlobalPerMin))
		r.Use(app.csrfProtect)

		r.Get("/healthz", healthz)

		// T10: жёсткий лимит на перебор паролей и спам регистрациями (10/мин на IP).
		r.With(app.rateLimit("auth", app.Config.RateLimitAuthPerMin)).Post("/auth/register", app.Register)
		r.With(app.rateLimit("auth", app.Config.RateLimitAuthPerMin)).Post("/auth/login", app.Login)
		r.Post("/auth/logout", app.Logout)
		r.Get("/auth/me", app.Me)

		// T13: вход через Яндекс ID (хендлеры — auth_yandex.go, клиент —
		// auth/yandex.go от T10). Это GET-редиректы, поэтому csrfProtect их
		// не трогает, и отдельный лимитер auth НЕ навешивается (10/мин на IP
		// уже стоит на POST /auth/login и /auth/register); действует
		// общий глобальный лимит /api/v1.
		r.Get("/auth/yandex/start", app.YandexAuthStart)
		r.Get("/auth/yandex/callback", app.YandexAuthCallback)

		r.Get("/courses", app.ListCourses)
		// T65: публичный список категорий — фиксированные шесть слагов
		// и число published-курсов (регистрируется до /courses/{slug}).
		r.Get("/categories", app.ListCategories)
		r.Get("/courses/{slug}", app.GetCourse)
		// T67: публичная проверка сертификата по номеру (без авторизации).
		// Статический сегмент /certificates не пересекается с /courses.
		app.RegisterCertificateRoutes(r)
		r.Get("/courses/{id}/reviews", app.ListReviews)
		r.With(app.requireAuth).Post("/courses/{id}/reviews", app.CreateReview)
		// T63: проверка промокода на странице курса — только вошедший.
		r.With(app.requireAuth).Post("/courses/{id}/promo/validate", app.ValidatePromoCode)
		// T71: вопросы и ответы по урокам (handler/lesson_qa.go). Лента
		// публичная; создание/удаление — только вошедшим (права проверяет
		// хендлер: активная запись/владение курсом/admin).
		r.Get("/courses/{id}/questions", app.ListLessonQuestions)
		r.With(app.requireAuth).Post("/courses/{id}/questions", app.CreateLessonQuestion)
		r.With(app.requireAuth).Post("/courses/{id}/questions/{questionId}/answers", app.CreateLessonAnswer)
		r.With(app.requireAuth).Delete("/courses/{id}/questions/{questionId}", app.DeleteLessonQuestion)
		r.With(app.requireAuth).Delete("/courses/{id}/questions/{questionId}/answers/{answerId}", app.DeleteLessonAnswer)

		// Пост-P7: публичные настройки интерфейса (тогл промо-баннера) —
		// читаются шапкой без авторизации, поэтому живут вне /admin
		// (см. public_settings.go).
		r.Get("/settings/public", app.GetPublicSettings)

		r.Route("/teacher", func(r chi.Router) {
			r.Use(app.requireAuth, app.requireRole(repo.RoleTeacher, repo.RoleAdmin))
			r.Post("/courses", app.TeacherCreateCourse)
			// GET /teacher/courses регистрируется одна — в
			// RegisterTeacherCRUDRoutes (teacher_crud.go, T19): мёртвый дубль
			// регистрации сюда больше не возвращается (чистка T53).
			r.Post("/courses/{id}/modules", app.TeacherCreateModule)
			r.Post("/courses/{id}/lessons", app.TeacherCreateLesson)
			r.Post("/courses/{id}/submit", app.TeacherSubmitCourse)
			// T13: бэклог T06 — редактирование (GET/PATCH по id, переименование
			// модуля). Монтируется внутрь этого блока, чтобы пути были
			// относительными (/courses/{id}), а requireAuth/requireRole — уже навешены.
			app.RegisterTeacherEditRoutes(r)
			// T19: CRUD конструктора (правка урока, удаление, порядок, счётчики
			// в списке) — та же точка монтирования, что у RegisterTeacherEditRoutes.
			app.RegisterTeacherCRUDRoutes(r)
			// T21: жизненный цикл курса (копирование, архив, отзыв с модерации,
			// перенос урока). T22: ученики курса и статистика для учителя.
			app.RegisterTeacherLifecycleRoutes(r)
			app.RegisterTeacherStudentsRoutes(r)
			// T69: ответ преподавателя на отзыв —
			// PUT/DELETE /courses/{id}/reviews/{reviewId}/reply.
			app.RegisterTeacherReviewReplyRoutes(r)
			// T54: обложка курса — POST/DELETE /courses/{id}/cover (multipart).
			// Лимит тела 1 МБ из decodeJSON multipart не задевает (он действует
			// только на JSON-декодер); у хендлеров обложки свой лимит ≥6 МБ
			// через http.MaxBytesReader — см. teacher_cover.go.
			app.RegisterTeacherCoverRoutes(r)
			// T63: промокоды курса — POST/GET /courses/{id}/promo-codes,
			// DELETE /courses/{id}/promo-codes/{promoId} (handler/promo_codes.go).
			app.RegisterTeacherPromoRoutes(r)
		})

		r.Route("/admin", func(r chi.Router) {
			r.Use(app.requireAuth, app.requireRole(repo.RoleAdmin))
			r.Post("/invites", app.AdminCreateInvite)
			// T47: список приглашений и отзыв (handler/admin_invites.go).
			// requireAuth/requireRole(admin) уже навешены на группу.
			r.Get("/invites", app.AdminListInvites)
			r.Post("/invites/{id}/revoke", app.AdminRevokeInvite)
			r.Get("/courses/review", app.AdminReviewQueue)
			r.Post("/courses/{id}/moderate", app.AdminModerateCourse)
			app.RegisterAdminUsersRoutes(r) // T24: /admin/users — управление пользователями
			app.RegisterAdminConsoleRoutes(r)
			// T55: детали курса GET /admin/courses/{id} — программа, статистика
			// и карточка «одним экраном» (handler/admin_course_detail.go).
			// Регистрируется после очереди модерации: статический сегмент
			// "review" в chi приоритетнее {id}, очередь не перехватывается.
			r.Get("/courses/{id}", app.AdminCourseDetail)
			// T07 (журнал платежей и настройки) сюда НЕ подключать: метод
			// RegisterAdminPaymentsRoutes регистрирует АБСОЛЮТНЫЕ пути
			// /admin/payments и /admin/settings (см. handler/admin_payments.go),
			// монтирование внутрь дало бы /admin/admin/payments.
			// Вызов — на уровне /api/v1 ниже.
		})

		r.With(app.requireAuth).Post("/enrollments", app.CreateEnrollment)

		// T07 (волна 3): журнал платежей и комиссия US6. Метод регистрирует
		// АБСОЛЮТНЫЕ пути /admin/payments и /admin/settings, поэтому вызывается
		// на уровне /api/v1, а НЕ внутри r.Route("/admin", ...) — иначе
		// получилось бы /admin/admin/payments. requireAuth + requireRole(admin)
		// вешаются внутри метода через .With, двойная обёртка не нужна.
		app.RegisterAdminPaymentsRoutes(r)
		app.RegisterAdminEnrollmentRoutes(r) // T16: /admin/enrollments/{studentId}/courses/{courseId}/revoke|unrevoke

		// Webhook-и провайдеров оплачивают CSRF-проверку подписью (T09),
		// поэтому исключены из csrfProtect — платёжки шлют их без Origin.
		r.With(app.rateLimit("webhook", app.Config.RateLimitWebhookPerMin)).Post("/payments/webhook/{provider}", app.PaymentWebhook)

		r.With(app.requireAuth).Get("/me/courses", app.MyCourses)
		r.With(app.requireAuth).Post("/me/courses/{id}/lessons/{lessonId}/complete", app.CompleteLesson)
		app.RegisterMeExtraRoutes(r)
		// T73: избранные курсы — PUT/DELETE /me/favorites/{courseId},
		// GET /me/favorites (handler/favorites.go).
		app.RegisterFavoritesRoutes(r)
	})
	return r
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---- JSON envelope ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr пишет русскую ошибку в envelope {"error": "..."}.
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeJSON читает JSON-тело с лимитом 1 МБ и переводит ошибки
// декодера на понятный русский текст (без технических подробностей).
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &typeErr):
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("Некорректное поле «%s»: неверный тип значения", typeErr.Field))
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			name := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("Некорректное тело запроса: неизвестное поле %s", name))
		default:
			writeErr(w, http.StatusBadRequest, "Некорректное тело запроса")
		}
		return false
	}
	return true
}

// ---- Sessions / auth (T10: cookie со HMAC-SHA256-подписью) ----

// signSessionID считает подпись sessionID: base64url(HMAC-SHA256(secret, sessionID)).
func signSessionID(secret, sessionID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sessionID))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// sessionValue возвращает значение cookie: "<sessionID>.<подпись>",
// либо голый sessionID, если подпись отключена (dev без SESSION_SECRET).
func (a *App) sessionValue(sessionID string) string {
	secret := strings.TrimSpace(a.Config.SessionSecret)
	if secret == "" {
		// Dev-режим без секрета: подпись не используется (предупреждение пишет config.Load).
		return sessionID
	}
	return sessionID + "." + signSessionID(secret, sessionID)
}

// parseSessionValue разбирает "<sessionID>.<подпись>" и проверяет подпись
// через hmac.Equal (защита от тайминг-атак). Подделка значения → не ok.
// Голый sessionID принимается только когда подпись отключена (dev без секрета).
func (a *App) parseSessionValue(v string) (string, bool) {
	secret := strings.TrimSpace(a.Config.SessionSecret)
	if secret == "" {
		return v, v != ""
	}
	id, sig, ok := strings.Cut(v, ".")
	if !ok || id == "" || sig == "" {
		return "", false
	}
	if !hmac.Equal([]byte(sig), []byte(signSessionID(secret, id))) {
		return "", false
	}
	return id, true
}

// isHTTPS определяет HTTPS по TLS-соединению или заголовку балансировщика.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

// sessionCookieFor собирает httpOnly cookie сессии: Secure — по HTTPS-запросу
// (и всегда при SameSite=None — браузеры такое требуют), SameSite — из конфига.
func (a *App) sessionCookieFor(r *http.Request, value string, maxAge int) *http.Cookie {
	samesite := a.Config.CookieSameSite
	if samesite == 0 {
		samesite = http.SameSiteLaxMode
	}
	secure := isHTTPS(r) || samesite == http.SameSiteNoneMode
	return &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: samesite,
		MaxAge:   maxAge,
	}
}

// startSession создаёт сессию и ставит подписанную httpOnly cookie.
func (a *App) startSession(w http.ResponseWriter, r *http.Request, userID string) {
	sess := a.Store.CreateSession(userID, sessionTTL)
	http.SetCookie(w, a.sessionCookieFor(r, a.sessionValue(sess.ID), int(sessionTTL.Seconds())))
}

func (a *App) clearSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		// Удаляем сессию по разобранному sessionID, а не по сырым "<id>.<подпись>".
		if id, ok := a.parseSessionValue(c.Value); ok {
			a.Store.DeleteSession(id)
		}
	}
	http.SetCookie(w, a.sessionCookieFor(r, "", -1))
}

// requireAuth требует валидную (и корректно подписанную) сессию
// и кладёт пользователя в контекст. CSRF-проверку см. csrfProtect.
func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
			return
		}
		id, ok := a.parseSessionValue(c.Value)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "Некорректная сессия, войдите снова")
			return
		}
		sess, ok := a.Store.GetSession(id)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "Сессия истекла, войдите снова")
			return
		}
		u, ok := a.Store.GetUserByID(sess.UserID)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "Пользователь не найден")
			return
		}
		// T24: заблокированный пользователь теряет доступ даже с живой сессией.
		if u.Status == repo.UserBlocked {
			writeErr(w, http.StatusForbidden, "Учётная запись заблокирована")
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireRole требует одну из ролей (после requireAuth).
func (a *App) requireRole(roles ...repo.Role) func(http.Handler) http.Handler {
	allowed := make(map[repo.Role]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := a.currentUser(r)
			if !ok {
				writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
				return
			}
			if !allowed[u.Role] {
				writeErr(w, http.StatusForbidden, "Недостаточно прав для этого действия")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// optionalUser достаёт пользователя из cookie, если она есть (не требует auth).
func (a *App) optionalUser(r *http.Request) *repo.User {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	id, ok := a.parseSessionValue(c.Value)
	if !ok {
		return nil
	}
	sess, ok := a.Store.GetSession(id)
	if !ok {
		return nil
	}
	u, ok := a.Store.GetUserByID(sess.UserID)
	if !ok {
		return nil
	}
	return u
}

// currentUser возвращает пользователя из контекста (после requireAuth).
func (a *App) currentUser(r *http.Request) (*repo.User, bool) {
	u, ok := r.Context().Value(userCtxKey).(*repo.User)
	if !ok || u == nil {
		return nil, false
	}
	fresh, ok := a.Store.GetUserByID(u.ID)
	if !ok {
		return nil, false
	}
	return fresh, true
}

// ---- Rate-limit (T10) ----
//
// In-memory token bucket без внешних зависимостей: у каждой IP и именованной
// группы (auth, global, webhook) — своё ведро ёмкостью perMin с непрерывным
// пополнением perMin/мин. Протухшие ведра вычищает фоновая горутина,
// чтобы карта не росла по памяти.

// rlBucket — состояние одного IP внутри лимитера.
type rlBucket struct {
	tokens float64
	seen   time.Time // последняя активность: по ней считаем пополнение и TTL
}

// rateLimiter — потокобезопасный лимитер token bucket.
// Поле now инжектируется в тестах для детерминированности.
type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*rlBucket
	capacity float64       // максимум токенов (= perMin)
	refill   float64       // токенов в секунду
	ttl      time.Duration // неактивность ведра после которой удаляем
	now      func() time.Time
}

func newRateLimiter(perMin int, window time.Duration, now func() time.Time) *rateLimiter {
	if now == nil {
		now = time.Now
	}
	if window <= 0 {
		window = time.Minute
	}
	return &rateLimiter{
		buckets:  make(map[string]*rlBucket),
		capacity: float64(perMin),
		refill:   float64(perMin) / window.Seconds(),
		ttl:      2 * window,
		now:      now,
	}
}

// allow списывает один токен по ключу; при пустом ведре возвращает
// false и паузу до следующего токена (для заголовка Retry-After).
func (l *rateLimiter) allow(key string) (bool, time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = &rlBucket{tokens: l.capacity, seen: now}
		l.buckets[key] = b
	}
	if elapsed := now.Sub(b.seen).Seconds(); elapsed > 0 {
		b.tokens += elapsed * l.refill
		if b.tokens > l.capacity {
			b.tokens = l.capacity
		}
	}
	b.seen = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1-b.tokens)/l.refill) * time.Second
	if wait < time.Second {
		wait = time.Second
	}
	return false, wait
}

// cleanup удаляет ведра, неактивные дольше ttl (защита от утечки памяти).
func (l *rateLimiter) cleanup() {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.buckets {
		if now.Sub(b.seen) > l.ttl {
			delete(l.buckets, k)
		}
	}
}

// startJanitor запускает периодическую очистку карты ведёр
// (горутина живёт вместе с процессом — лимитер создаётся один раз).
func (l *rateLimiter) startJanitor(interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			l.cleanup()
		}
	}()
}

// rateLimit возвращает middleware «name» с лимитом perMin запросов в минуту
// на IP клиента. perMin <= 0 — лимит выключен (pass-through): так тесты
// и dev не зависят от rate-limit'ов, а прод получает дефолты из config.Load.
func (a *App) rateLimit(name string, perMin int) func(http.Handler) http.Handler {
	if perMin <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	a.limitersOnce.Do(func() { a.limiters = make(map[string]*rateLimiter) })
	a.limitersMu.Lock()
	l, ok := a.limiters[name]
	if !ok {
		l = newRateLimiter(perMin, time.Minute, time.Now)
		l.startJanitor(time.Minute)
		a.limiters[name] = l
	}
	a.limitersMu.Unlock()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, wait := l.allow(clientIP(r))
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
				writeErr(w, http.StatusTooManyRequests, "Слишком много запросов, повторите позже")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP извлекает IP из RemoteAddr (после realIP).
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// realIP подставляет клиента из заголовков балансировщика (X-Real-IP /
// X-Forwarded-For), но только когда реальный RemoteAddr — loopback или
// приватная сеть (доверенный прокси). Иначе заголовкам доверять нельзя:
// браузер умеет подставлять X-Forwarded-For сам и обходил бы rate-limit по IP.
func (a *App) realIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
				if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
					r.RemoteAddr = v
				} else if v := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); v != "" {
					if first, _, ok := strings.Cut(v, ","); ok {
						v = first
					}
					r.RemoteAddr = strings.TrimSpace(v)
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ---- CORS / CSRF ----

func originOf(r *http.Request) string {
	if o := strings.TrimSpace(r.Header.Get("Origin")); o != "" {
		return o
	}
	return strings.TrimSpace(r.Header.Get("Referer"))
}

// allowedOrigin: с заголовком Origin/Referer — только из allowlist;
// без заголовков — см. allowMissingOrigin.
func (a *App) allowedOrigin(r *http.Request) bool {
	o := originOf(r)
	if o == "" {
		return a.allowMissingOrigin(r)
	}
	for _, allow := range a.Config.AllowedOrigins {
		if o == allow || strings.HasPrefix(o, strings.TrimRight(allow, "/")+"/") {
			return true
		}
	}
	return false
}

// allowMissingOrigin решает судьбу меняющих запросов без Origin/Referer:
//   - CSRF_ALLOW_MISSING_ORIGIN=true — пропускаем (локальные qa-скрипты, curl);
//   - dev-режим — пропускаем (контрактные тесты, локальная отладка);
//   - прод — пропускаем только «заведомо безопасные» запросы: с bearer-заголовком
//     или multipart/form-data (см. Task/T10-auth-security.md §3).
func (a *App) allowMissingOrigin(r *http.Request) bool {
	if a.Config.CSRFAllowMissingOrigin {
		return true
	}
	if a.Config.IsDev() {
		return true
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		return true
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "multipart/form-data") {
		return true
	}
	return false
}

// csrfProtect проверяет Origin/Referer всех меняющих методов /api/v1.
// Браузеры всегда шлют Origin на cross-site POST, поэтому в проде отсутствие
// обоих заголовков означает не-браузерный запрос — отклоняем. Проверка была
// перенесена из requireAuth сюда, чтобы накрыть и неавторизованные маршруты
// (login, register, enrollments). Webhook-и платежек исключены: их шлют
// серверы провайдеров без Origin, а подпись проверяет T09.
func (a *App) csrfProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		// Webhook-и платежек исключены: их шлют серверы провайдеров без Origin,
		// подпись проверяет T09. Проверяем по пути запроса — RoutePattern
		// в chi-middleware ещё не заполнен.
		if strings.Contains(r.URL.Path, "/payments/webhook/") {
			next.ServeHTTP(w, r)
			return
		}
		if !a.allowedOrigin(r) {
			writeErr(w, http.StatusForbidden, "Недопустимый источник запроса: нет Origin/Referer")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// cors разрешает запросы с фронтенда (localhost:3000) с cookie.
// T42: для preflight-запроса (OPTIONS) Access-Control-Allow-Methods собирается
// по реальным методам запрошенного пути: перебираем GET, POST, PUT, PATCH,
// DELETE и проверяем их через mux.Match (роутер chi v5 сам знает свои
// маршруты, включая вложенные подроутеры). Методы, зарегистрированные для
// пути, перечисляются в порядке перебора + OPTIONS; если для пути нет ни
// одного маршрута — заголовок не ставится. Раньше заголовок был жёсткой
// константой для всех путей, включая несуществующие.
func (a *App) cors(mux *chi.Mux) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			allowed := ""
			if origin != "" {
				for _, o := range a.Config.AllowedOrigins {
					if origin == o {
						allowed = o
						break
					}
				}
			}
			if allowed != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowed)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				if allowed != "" {
					// Новый RouteContext на каждый Match: Match мутирует
					// состояние контекста при обходе дерева маршрутов.
					methods := make([]string, 0, 6)
					for _, m := range []string{
						http.MethodGet, http.MethodPost, http.MethodPut,
						http.MethodPatch, http.MethodDelete,
					} {
						if mux.Match(chi.NewRouteContext(), m, r.URL.Path) {
							methods = append(methods, m)
						}
					}
					if len(methods) > 0 {
						w.Header().Set("Access-Control-Allow-Methods",
							strings.Join(append(methods, http.MethodOptions), ", "))
					}
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
					w.Header().Set("Access-Control-Max-Age", "86400")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---- pagination helper ----

func paginate(total, page, limit int) (items int, safePage, safeLimit int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return start, page, limit
}
