package handler

// T67 — сертификаты о завершении курса:
//   GET /me/certificates          — сертификаты текущего ученика (requireAuth);
//   GET /certificates/{number}    — публичная проверка сертификата по номеру.
//
// Выдача автоматическая: CompleteLesson после фиксации прогресса вызывает
// issueCertificateIfCourseCompleted — сертификат создаётся, когда все уроки
// курса (их ≥ 1) пройдены. Идемпотентность обеспечивает хранилище
// (UNIQUE (user_id, course_id)), поэтому повторное завершение урока и
// повторное достижение 100% второй сертификат не создают.

import (
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// RegisterCertificateRoutes вешает публичную проверку сертификата на /api/v1.
// Маршрут без requireAuth — по номеру сертификат может проверить любой.
func (a *App) RegisterCertificateRoutes(r chi.Router) {
	r.Get("/certificates/{number}", a.GetPublicCertificate)
}

// issueCertificateIfCourseCompleted выдаёт сертификат, если у ученика
// пройдены ВСЕ уроки курса и уроков хотя бы один. Вызывается после
// CompleteLesson; ошибки выдачи не должны ломать ответ о прогрессе — только
// пишутся в лог.
func (a *App) issueCertificateIfCourseCompleted(studentID string, c *repo.Course) {
	lessons := a.Store.ListLessonsByCourse(c.ID)
	if len(lessons) == 0 {
		return
	}
	for _, l := range lessons {
		if !a.Store.IsLessonCompleted(studentID, l.ID) {
			return
		}
	}
	if _, err := a.Store.IssueCertificate(studentID, c.ID); err != nil {
		log.Printf("T67: не удалось выдать сертификат user=%s course=%s: %v",
			studentID, c.ID, err)
	}
}

// MyCertificates — GET /me/certificates: сертификаты текущего пользователя,
// новые первыми, конверт {"items":[...]} по контракту.
func (a *App) MyCertificates(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	items := a.Store.ListCertificatesByStudent(u.ID)
	if items == nil {
		// Страховка от nil в хранилище: контракт запрещает null в JSON.
		items = []*repo.CertificateListItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// GetPublicCertificate — GET /certificates/{number}: публичная карточка
// сертификата по номеру. 404 — сертификата нет.
func (a *App) GetPublicCertificate(w http.ResponseWriter, r *http.Request) {
	// Номер сертификата печатается в верхнем регистре, но в адресной строке
	// его могут набрать строчными — проверка от регистра не зависит.
	number := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "number")))
	c, ok := a.Store.GetCertificateByNumber(number)
	if !ok {
		writeErr(w, http.StatusNotFound, "Сертификат не найден")
		return
	}
	writeJSON(w, http.StatusOK, c)
}
