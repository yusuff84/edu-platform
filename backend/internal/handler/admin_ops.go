package handler

// Расширенная админка: всё, что раньше требовало SQL или было недоступно.
//
//	POST   /admin/users                                   — создать пользователя (пароль задан или сгенерирован)
//	PATCH  /admin/users/{id}/profile                      — изменить email и/или имя
//	POST   /admin/users/{id}/password                     — задать/сгенерировать новый пароль (+ выход со всех устройств)
//	POST   /admin/users/{id}/logout-all                   — завершить все сеансы пользователя
//	GET    /admin/users/{id}/courses                      — курсы пользователя: статус, прогресс, сертификат
//	POST   /admin/enrollments/{studentId}/courses/{courseId}/reset-progress — сбросить прогресс
//	POST   /admin/enrollments/{studentId}/courses/{courseId}/complete       — засчитать курс целиком + сертификат
//	GET    /admin/certificates?q=                         — все сертификаты
//	POST   /admin/certificates                            — выдать сертификат вручную
//	DELETE /admin/certificates/{number}                   — отозвать сертификат
//	POST   /admin/courses/{id}/publish                    — опубликовать курс без очереди модерации
//	POST   /admin/courses/{id}/owner                      — передать курс другому преподавателю
//	GET    /admin/site-settings, PUT /admin/site-settings — тексты промо-полосы и контакты в футере

import (
	"crypto/rand"
	"errors"
	"math/big"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"edu-platform/backend/internal/repo"
)

// RegisterAdminOpsRoutes — абсолютные пути, admin-only (как RegisterAdminPaymentsRoutes).
func (a *App) RegisterAdminOpsRoutes(r chi.Router) {
	adm := r.With(a.requireAuth, a.requireRole(repo.RoleAdmin))
	adm.Post("/admin/users", a.AdminCreateUser)
	adm.Patch("/admin/users/{id}/profile", a.AdminUpdateUserProfile)
	adm.Post("/admin/users/{id}/password", a.AdminSetUserPassword)
	adm.Post("/admin/users/{id}/logout-all", a.AdminLogoutUser)
	adm.Get("/admin/users/{id}/courses", a.AdminUserCourses)
	adm.Post("/admin/enrollments/{studentId}/courses/{courseId}/reset-progress", a.AdminResetProgress)
	adm.Post("/admin/enrollments/{studentId}/courses/{courseId}/complete", a.AdminCompleteCourse)
	adm.Get("/admin/certificates", a.AdminListCertificates)
	adm.Post("/admin/certificates", a.AdminIssueCertificate)
	adm.Delete("/admin/certificates/{number}", a.AdminRevokeCertificate)
	adm.Post("/admin/courses/{id}/publish", a.AdminPublishCourse)
	adm.Post("/admin/courses/{id}/owner", a.AdminSetCourseOwner)
	adm.Get("/admin/site-settings", a.AdminGetSiteSettings)
	adm.Put("/admin/site-settings", a.AdminPutSiteSettings)
}

// ---- пользователи ----

const passwordAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// generatePassword — случайный пароль из 14 символов без похожих букв (l/1, O/0).
func generatePassword() string {
	b := make([]byte, 14)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(passwordAlphabet))))
		b[i] = passwordAlphabet[n.Int64()]
	}
	return string(b)
}

// validNewPassword — те же правила, что при регистрации: 8..72 байта.
func validNewPassword(p string) string {
	if utf8.RuneCountInString(p) < 8 {
		return "Пароль не короче 8 символов"
	}
	if len(p) > 72 {
		return "Пароль слишком длинный"
	}
	return ""
}

func validEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email && strings.Contains(email, "@") && len(email) <= 254
}

// adminUserOf — пользователь из {id} пути; пишет 404/400 сам.
func (a *App) adminUserOf(w http.ResponseWriter, r *http.Request) (*repo.User, bool) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if _, err := uuid.Parse(id); err != nil {
		writeErr(w, http.StatusBadRequest, "Некорректный id пользователя")
		return nil, false
	}
	u, ok := a.Store.GetUserByID(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "Пользователь не найден")
		return nil, false
	}
	return u, true
}

// AdminCreateUser — POST /admin/users {email, password?, role?, displayName?}:
// 201 {user, password} — password возвращается, только если сгенерирован.
func (a *App) AdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		Role        string `json:"role"`
		DisplayName string `json:"displayName"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(email) {
		writeErr(w, http.StatusBadRequest, "Некорректный email")
		return
	}
	role := repo.Role(strings.TrimSpace(req.Role))
	if role == "" {
		role = repo.RoleStudent
	}
	if role != repo.RoleStudent && role != repo.RoleTeacher && role != repo.RoleAdmin {
		writeErr(w, http.StatusBadRequest, "Некорректная роль")
		return
	}
	generated := ""
	password := req.Password
	if password == "" {
		generated = generatePassword()
		password = generated
	} else if msg := validNewPassword(password); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	// Имя — по тем же правилам, что PATCH /admin/users/{id}/profile: пробелы
	// схлопываются, не длиннее 60 символов (раньше при создании длина не
	// проверялась вовсе).
	displayName := strings.Join(strings.Fields(req.DisplayName), " ")
	if utf8.RuneCountInString(displayName) > 60 {
		writeErr(w, http.StatusBadRequest, "Имя не длиннее 60 символов")
		return
	}
	if _, exists := a.Store.GetUserByEmail(email); exists {
		writeErr(w, http.StatusConflict, "Пользователь с таким email уже есть")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось создать пользователя")
		return
	}
	u, err := a.Store.CreateUser(email, string(hash), role)
	if err != nil {
		if errors.Is(err, repo.ErrAlreadyExists) {
			writeErr(w, http.StatusConflict, "Пользователь с таким email уже есть")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось создать пользователя")
		return
	}
	if displayName != "" {
		a.Store.SetUserDisplayName(u.ID, displayName)
		u.DisplayName = displayName
	}
	a.auditAdmin(r, "Создан пользователь %s (роль %s)", u.Email, role)
	writeJSON(w, http.StatusCreated, map[string]any{"user": a.adminUserItemOf(u), "password": generated})
}

// AdminUpdateUserProfile — PATCH /admin/users/{id}/profile {email?, displayName?}.
func (a *App) AdminUpdateUserProfile(w http.ResponseWriter, r *http.Request) {
	u, ok := a.adminUserOf(w, r)
	if !ok {
		return
	}
	var req struct {
		Email       *string `json:"email"`
		DisplayName *string `json:"displayName"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Email == nil && req.DisplayName == nil {
		writeErr(w, http.StatusBadRequest, "Укажите email или имя")
		return
	}
	if req.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*req.Email))
		if !validEmail(email) {
			writeErr(w, http.StatusBadRequest, "Некорректный email")
			return
		}
		if email != u.Email {
			if err := a.Store.SetUserEmail(u.ID, email); err != nil {
				if errors.Is(err, repo.ErrAlreadyExists) {
					writeErr(w, http.StatusConflict, "Этот email уже занят другим пользователем")
					return
				}
				writeErr(w, http.StatusInternalServerError, "Не удалось изменить email")
				return
			}
		}
	}
	if req.DisplayName != nil {
		name := strings.Join(strings.Fields(*req.DisplayName), " ")
		if utf8.RuneCountInString(name) > 60 {
			writeErr(w, http.StatusBadRequest, "Имя не длиннее 60 символов")
			return
		}
		a.Store.SetUserDisplayName(u.ID, name)
	}
	a.auditAdmin(r, "Изменён профиль %s", u.Email)
	nu, _ := a.Store.GetUserByID(u.ID)
	writeJSON(w, http.StatusOK, a.adminUserItemOf(nu))
}

// AdminSetUserPassword — POST /admin/users/{id}/password {password?}: задаёт
// пароль (или генерирует) и завершает все сеансы пользователя. Нужен, пока на
// сервере не настроена почта: иначе забывший пароль не может войти.
func (a *App) AdminSetUserPassword(w http.ResponseWriter, r *http.Request) {
	u, ok := a.adminUserOf(w, r)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if r.ContentLength != 0 && !decodeJSON(w, r, &req) {
		return
	}
	generated := ""
	password := req.Password
	if password == "" {
		generated = generatePassword()
		password = generated
	} else if msg := validNewPassword(password); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil || !a.Store.SetUserPasswordHash(u.ID, string(hash)) {
		writeErr(w, http.StatusInternalServerError, "Не удалось сменить пароль")
		return
	}
	_ = a.Store.RevokeOtherAccountSessions(u.ID, uuid.Nil.String())
	a.auditAdmin(r, "Задан новый пароль для %s", u.Email)
	writeJSON(w, http.StatusOK, map[string]string{"password": generated})
}

// AdminLogoutUser — POST /admin/users/{id}/logout-all: завершает все сеансы.
func (a *App) AdminLogoutUser(w http.ResponseWriter, r *http.Request) {
	u, ok := a.adminUserOf(w, r)
	if !ok {
		return
	}
	if err := a.Store.RevokeOtherAccountSessions(u.ID, uuid.Nil.String()); err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось завершить сеансы")
		return
	}
	a.auditAdmin(r, "Завершены все сеансы %s", u.Email)
	w.WriteHeader(http.StatusNoContent)
}

type adminUserCourse struct {
	CourseID          string                `json:"courseId"`
	CourseTitle       string                `json:"courseTitle"`
	Status            repo.EnrollmentStatus `json:"status"`
	EnrolledAt        string                `json:"enrolledAt"`
	CompletedLessons  int                   `json:"completedLessons"`
	TotalLessons      int                   `json:"totalLessons"`
	CertificateNumber string                `json:"certificateNumber"`
}

// AdminUserCourses — GET /admin/users/{id}/courses: записи пользователя со
// статусом, прогрессом и номером сертификата.
func (a *App) AdminUserCourses(w http.ResponseWriter, r *http.Request) {
	u, ok := a.adminUserOf(w, r)
	if !ok {
		return
	}
	certs := map[string]string{}
	for _, c := range a.Store.ListCertificatesByStudent(u.ID) {
		certs[c.CourseID] = c.Number
	}
	items := []adminUserCourse{}
	for _, e := range a.Store.ListEnrollmentsByStudent(u.ID) {
		c, ok := a.Store.GetCourseByID(e.CourseID)
		if !ok {
			continue
		}
		// Два счётчика вместо IsLessonCompleted на каждый урок курса.
		items = append(items, adminUserCourse{
			CourseID: c.ID, CourseTitle: c.Title, Status: e.Status,
			EnrolledAt:        e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			CompletedLessons:  a.Store.CountCompletedLessons(u.ID, c.ID),
			TotalLessons:      a.Store.CountLessonsByCourse(c.ID),
			CertificateNumber: certs[c.ID],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": a.adminUserItemOf(u), "items": items})
}

// ---- прогресс и сертификаты ----

func (a *App) enrollmentPair(w http.ResponseWriter, r *http.Request) (*repo.User, *repo.Course, bool) {
	sid := strings.TrimSpace(chi.URLParam(r, "studentId"))
	cid := strings.TrimSpace(chi.URLParam(r, "courseId"))
	if _, err := uuid.Parse(sid); err != nil {
		writeErr(w, http.StatusBadRequest, "Параметр studentId должен быть UUID")
		return nil, nil, false
	}
	if _, err := uuid.Parse(cid); err != nil {
		writeErr(w, http.StatusBadRequest, "Параметр courseId должен быть UUID")
		return nil, nil, false
	}
	u, ok := a.Store.GetUserByID(sid)
	if !ok {
		writeErr(w, http.StatusNotFound, "Пользователь не найден")
		return nil, nil, false
	}
	c, ok := a.Store.GetCourseByID(cid)
	if !ok {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return nil, nil, false
	}
	return u, c, true
}

// AdminResetProgress — POST .../reset-progress: удаляет прогресс ученика по курсу
// (сертификат не трогает — его отзывают отдельно). 200 {deleted}.
func (a *App) AdminResetProgress(w http.ResponseWriter, r *http.Request) {
	u, c, ok := a.enrollmentPair(w, r)
	if !ok {
		return
	}
	ids := []string{}
	for _, l := range a.Store.ListLessonsByCourse(c.ID) {
		ids = append(ids, l.ID)
	}
	n := a.Store.DeleteLessonProgress(u.ID, ids)
	a.auditAdmin(r, "Сброшен прогресс %s по курсу «%s» (%d уроков)", u.Email, c.Title, n)
	writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
}

// AdminCompleteCourse — POST .../complete: открывает доступ (если нужно),
// засчитывает все уроки курса и выдаёт сертификат. 200 {certificateNumber}.
func (a *App) AdminCompleteCourse(w http.ResponseWriter, r *http.Request) {
	u, c, ok := a.enrollmentPair(w, r)
	if !ok {
		return
	}
	lessons := a.Store.ListLessonsByCourse(c.ID)
	if len(lessons) == 0 {
		writeErr(w, http.StatusBadRequest, "В курсе нет уроков")
		return
	}
	if e, _ := a.Store.EnsureEnrollment(u.ID, c.ID); e == nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось открыть доступ к курсу")
		return
	}
	for _, l := range lessons {
		a.Store.CompleteLesson(u.ID, l.ID)
	}
	cert, err := a.Store.IssueCertificate(u.ID, c.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Уроки засчитаны, но выдать сертификат не удалось")
		return
	}
	a.auditAdmin(r, "Курс «%s» отмечен пройденным для %s", c.Title, u.Email)
	writeJSON(w, http.StatusOK, map[string]string{"certificateNumber": cert.Number})
}

// AdminListCertificates — GET /admin/certificates?q=: {items}.
func (a *App) AdminListCertificates(w http.ResponseWriter, r *http.Request) {
	items := a.Store.ListAllCertificates(r.URL.Query().Get("q"), 500)
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// AdminIssueCertificate — POST /admin/certificates {email|studentId, courseId}:
// выдаёт сертификат без прохождения (например, курс пройден вне платформы).
// Доступ к курсу открывается, если его не было. 200 {certificateNumber}.
func (a *App) AdminIssueCertificate(w http.ResponseWriter, r *http.Request) {
	var req grantEnrollmentRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	var (
		u  *repo.User
		ok bool
	)
	if id := strings.TrimSpace(req.StudentID); id != "" {
		u, ok = a.Store.GetUserByID(id)
	} else {
		u, ok = a.Store.GetUserByEmail(strings.ToLower(strings.TrimSpace(req.Email)))
	}
	if !ok || u == nil {
		writeErr(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	c, ok := a.Store.GetCourseByID(strings.TrimSpace(req.CourseID))
	if !ok {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	if e, _ := a.Store.EnsureEnrollment(u.ID, c.ID); e == nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось открыть доступ к курсу")
		return
	}
	cert, err := a.Store.IssueCertificate(u.ID, c.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось выдать сертификат")
		return
	}
	a.auditAdmin(r, "Выдан сертификат %s по курсу «%s» для %s", cert.Number, c.Title, u.Email)
	writeJSON(w, http.StatusOK, map[string]string{"certificateNumber": cert.Number})
}

// AdminRevokeCertificate — DELETE /admin/certificates/{number}: 204; 404.
func (a *App) AdminRevokeCertificate(w http.ResponseWriter, r *http.Request) {
	number := strings.TrimSpace(chi.URLParam(r, "number"))
	if !a.Store.DeleteCertificate(number) {
		writeErr(w, http.StatusNotFound, "Сертификат не найден")
		return
	}
	a.auditAdmin(r, "Отозван сертификат %s", number)
	w.WriteHeader(http.StatusNoContent)
}

// ---- курсы ----

func (a *App) adminCourseOf(w http.ResponseWriter, r *http.Request) (*repo.Course, bool) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if _, err := uuid.Parse(id); err != nil {
		writeErr(w, http.StatusBadRequest, "Некорректный id курса")
		return nil, false
	}
	c, ok := a.Store.GetCourseByID(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return nil, false
	}
	return c, true
}

// AdminPublishCourse — POST /admin/courses/{id}/publish: публикует курс сразу
// (черновик, на модерации или из архива). В курсе должен быть хотя бы один урок.
func (a *App) AdminPublishCourse(w http.ResponseWriter, r *http.Request) {
	c, ok := a.adminCourseOf(w, r)
	if !ok {
		return
	}
	if len(a.Store.ListLessonsByCourse(c.ID)) == 0 {
		writeErr(w, http.StatusBadRequest, "Нельзя опубликовать курс без уроков")
		return
	}
	if c.Status != repo.CoursePublished && !a.Store.SetCourseStatus(c.ID, repo.CoursePublished, "") {
		writeErr(w, http.StatusInternalServerError, "Не удалось опубликовать курс")
		return
	}
	a.auditAdmin(r, "Опубликован курс «%s»", c.Title)
	nc, _ := a.Store.GetCourseByID(c.ID)
	writeJSON(w, http.StatusOK, nc)
}

// AdminSetCourseOwner — POST /admin/courses/{id}/owner {email}: передаёт курс
// преподавателю или администратору с этим email.
func (a *App) AdminSetCourseOwner(w http.ResponseWriter, r *http.Request) {
	c, ok := a.adminCourseOf(w, r)
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	u, ok := a.Store.GetUserByEmail(strings.ToLower(strings.TrimSpace(req.Email)))
	if !ok {
		writeErr(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	if u.Role != repo.RoleTeacher && u.Role != repo.RoleAdmin {
		writeErr(w, http.StatusBadRequest, "Владельцем курса может быть только преподаватель или администратор")
		return
	}
	if !a.Store.SetCourseTeacher(c.ID, u.ID) {
		writeErr(w, http.StatusInternalServerError, "Не удалось передать курс")
		return
	}
	a.auditAdmin(r, "Курс «%s» передан %s", c.Title, u.Email)
	writeJSON(w, http.StatusOK, map[string]string{"courseId": c.ID, "teacherId": u.ID, "teacherEmail": u.Email})
}

// ---- тексты сайта ----

var siteSettingLimits = map[string]int{
	"promo_text": 160, "promo_link_text": 40, "promo_link_url": 300,
	"contact_phone": 40, "contact_email": 120, "contact_city": 80, "footer_tagline": 300,
}

// AdminGetSiteSettings — GET /admin/site-settings: {settings}.
func (a *App) AdminGetSiteSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"settings": a.Store.GetSiteSettings()})
}

// AdminPutSiteSettings — PUT /admin/site-settings {settings:{key:value}}: пустое
// значение возвращает текст по умолчанию.
func (a *App) AdminPutSiteSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Settings map[string]string `json:"settings"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	clean := map[string]string{}
	for k, v := range req.Settings {
		limit, known := siteSettingLimits[k]
		if !known {
			writeErr(w, http.StatusBadRequest, "Неизвестная настройка: "+k)
			return
		}
		v = strings.TrimSpace(v)
		if utf8.RuneCountInString(v) > limit {
			writeErr(w, http.StatusBadRequest, "Слишком длинное значение: "+k)
			return
		}
		if k == "promo_link_url" && v != "" && !(strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "//")) &&
			!strings.HasPrefix(v, "https://") {
			writeErr(w, http.StatusBadRequest, "Ссылка должна начинаться с / или https://")
			return
		}
		clean[k] = v
	}
	if err := a.Store.SetSiteSettings(clean); err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить настройки")
		return
	}
	a.auditAdmin(r, "Изменены тексты сайта")
	writeJSON(w, http.StatusOK, map[string]any{"settings": a.Store.GetSiteSettings()})
}
