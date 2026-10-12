// Package handler — HTTP-слой: роутер, middleware и обработчики API.
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/repo"
)

// App — общие зависимости обработчиков.
// Store — интерфейс хранилища: in-memory (dev без БД) или Postgres (pgxpool).
type App struct {
	Store  repo.StoreAPI
	Config *config.Config
}

type ctxKey string

const userCtxKey ctxKey = "user"

const sessionCookie = "session_id"

// sessionTTL — время жизни сессии (7 дней).
const sessionTTL = 7 * 24 * time.Hour

// NewRouter собирает chi-роутер: /healthz, /api/v1, логгер, recover и CORS.
func NewRouter(app *App) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(app.cors)

	r.Get("/healthz", healthz)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/healthz", healthz)

		r.Post("/auth/register", app.Register)
		r.Post("/auth/login", app.Login)
		r.Post("/auth/logout", app.Logout)
		r.Get("/auth/me", app.Me)

		r.Get("/courses", app.ListCourses)
		r.Get("/courses/{slug}", app.GetCourse)
		r.Get("/courses/{id}/reviews", app.ListReviews)
		r.With(app.requireAuth).Post("/courses/{id}/reviews", app.CreateReview)

		r.Route("/teacher", func(r chi.Router) {
			r.Use(app.requireAuth, app.requireRole(repo.RoleTeacher, repo.RoleAdmin))
			r.Post("/courses", app.TeacherCreateCourse)
			r.Get("/courses", app.TeacherListCourses)
			r.Post("/courses/{id}/modules", app.TeacherCreateModule)
			r.Post("/courses/{id}/lessons", app.TeacherCreateLesson)
			r.Post("/courses/{id}/submit", app.TeacherSubmitCourse)
		})

		r.Route("/admin", func(r chi.Router) {
			r.Use(app.requireAuth, app.requireRole(repo.RoleAdmin))
			r.Post("/invites", app.AdminCreateInvite)
			r.Get("/courses/review", app.AdminReviewQueue)
			r.Post("/courses/{id}/moderate", app.AdminModerateCourse)
		})

		r.With(app.requireAuth).Post("/enrollments", app.CreateEnrollment)
		r.Post("/payments/webhook/{provider}", app.PaymentWebhook)

		r.With(app.requireAuth).Get("/me/courses", app.MyCourses)
		r.With(app.requireAuth).Post("/me/courses/{id}/lessons/{lessonId}/complete", app.CompleteLesson)
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

// decodeJSON читает JSON-тело с лимитом 1 МБ.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "Некорректное тело запроса")
		return false
	}
	return true
}

// ---- Sessions / auth ----

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

// startSession создаёт сессию и ставит httpOnly cookie (SameSite=Lax — CSRF-минимум).
func (a *App) startSession(w http.ResponseWriter, userID string) {
	sess := a.Store.CreateSession(userID, sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func (a *App) clearSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.Store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// requireAuth требует валидную сессию и кладёт пользователя в контекст.
func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
			return
		}
		sess, ok := a.Store.GetSession(c.Value)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "Сессия истекла, войдите снова")
			return
		}
		u, ok := a.Store.GetUserByID(sess.UserID)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "Пользователь не найден")
			return
		}
		// CSRF-минимум: для меняющих методов проверяем Origin/Referer.
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if !a.allowedOrigin(r) {
				writeErr(w, http.StatusForbidden, "Недопустимый источник запроса")
				return
			}
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
	sess, ok := a.Store.GetSession(c.Value)
	if !ok {
		return nil
	}
	u, ok := a.Store.GetUserByID(sess.UserID)
	if !ok {
		return nil
	}
	return u
}

// ---- CORS / CSRF ----

func originOf(r *http.Request) string {
	if o := strings.TrimSpace(r.Header.Get("Origin")); o != "" {
		return o
	}
	return strings.TrimSpace(r.Header.Get("Referer"))
}

// allowedOrigin: без заголовков — разрешаем (curl, тесты, same-origin);
// с заголовком — только из allowlist.
func (a *App) allowedOrigin(r *http.Request) bool {
	o := originOf(r)
	if o == "" {
		return true
	}
	for _, allow := range a.Config.AllowedOrigins {
		if o == allow || strings.HasPrefix(o, strings.TrimRight(allow, "/")+"/") {
			return true
		}
	}
	return false
}

// cors разрешает запросы с фронтенда (localhost:3000) с cookie.
func (a *App) cors(next http.Handler) http.Handler {
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
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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
