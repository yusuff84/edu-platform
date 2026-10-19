package handler

// T55 — детали курса «одним экраном» для супер-админа: GET /admin/courses/{id}.
//
// Ответ строго по замороженному разделу T55 контракта (Task/00-api-contract.md):
//   course  — форма элемента GET /admin/courses (T25, adminCourseItem из
//             admin_console.go) плюс coverPath (T54; null, если обложки нет);
//   modules — программа курса: модули по position, уроки внутри каждого
//             модуля по position; урок — ТОЛЬКО метаданные id/title/kind/position:
//             тел уроков (bodyMd, quiz, codeTask) в ответе нет;
//   stats   — studentsCount (ВСЕ записи), activeCount (active), reviewsCount,
//             avgRating, paymentsCount/paidTotalCents (только succeeded).
//
// Права проверяет middleware группы /admin (requireAuth + requireRole(admin),
// middleware.go) — маршрут подключён рядом с RegisterAdminConsoleRoutes.
// 404 — курса нет (в том числе не-UUID id), 403 — не админ, 401 — нет сессии.

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// adminCourseDetailCourse — карточка курса: элемент списка T25 + coverPath.
// Встраивание даёт JSON ровно той же формы, что adminCourseItem (json-теги
// закреплены в admin_console.go), новое поле добавляется уровнем выше.
//
// coverPath — указатель: контракт требует строку|null, а нулевое значение
// должно сериализоваться явным null (строку модели Course с omitempty
// использовать нельзя — поле обложки исчезло бы без неё).
type adminCourseDetailCourse struct {
	adminCourseItem
	CoverPath *string `json:"coverPath"`
}

// adminCourseDetailLesson — урок программы: только метаданные (T55);
// bodyMd/videoUrl/startsAt/quiz/codeTask в ответ не входят.
type adminCourseDetailLesson struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Kind     repo.LessonKind `json:"kind"`
	Position int             `json:"position"`
}

// adminCourseDetailModule — модуль программы c уроками по position;
// Lessons — пустой массив вместо null, если уроков нет.
type adminCourseDetailModule struct {
	ID       string                    `json:"id"`
	Title    string                    `json:"title"`
	Position int                       `json:"position"`
	Lessons  []adminCourseDetailLesson `json:"lessons"`
}

// adminCourseDetailStats — статистика карточки (T55). studentsCount — все
// записи курса (любой статус), activeCount — только active; платежи — только
// succeeded (pending/failed/refunded не считаются ни в числе, ни в деньгах).
type adminCourseDetailStats struct {
	StudentsCount  int     `json:"studentsCount"`
	ActiveCount    int     `json:"activeCount"`
	ReviewsCount   int     `json:"reviewsCount"`
	AvgRating      float64 `json:"avgRating"`
	PaymentsCount  int     `json:"paymentsCount"`
	PaidTotalCents int     `json:"paidTotalCents"`
}

// adminCourseDetailResponse — конверт ответа T55.
type adminCourseDetailResponse struct {
	Course  adminCourseDetailCourse   `json:"course"`
	Modules []adminCourseDetailModule `json:"modules"`
	Stats   adminCourseDetailStats    `json:"stats"`
}

// AdminCourseDetail — GET /admin/courses/{id} (T55). Все данные собирает
// один вызов хранилища AdminCourseDetail (store.go / postgres_course_detail.go):
// хендлер только перекладывает их в контрактные формы.
func (a *App) AdminCourseDetail(w http.ResponseWriter, r *http.Request) {
	d, found := a.Store.AdminCourseDetail(strings.TrimSpace(chi.URLParam(r, "id")))
	if !found {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}

	// coverPath: nil-указатель (обложки нет) → JSON null; строка — когда
	// обложка есть (T54). repo.Course уже отдаёт string|null — копия не нужна.
	course := adminCourseDetailCourse{
		adminCourseItem: adminCourseItem{
			ID:           d.Course.ID,
			Slug:         d.Course.Slug,
			Title:        d.Course.Title,
			Category:     d.Course.Category,
			TeacherID:    d.Course.TeacherID,
			TeacherEmail: d.TeacherEmail,
			Status:       d.Course.Status,
			PriceCents:   d.Course.PriceCents,
			CreatedAt:    d.Course.CreatedAt,
			UpdatedAt:    d.Course.UpdatedAt,
			ModulesCount: d.Counts.Modules,
			LessonsCount: d.Counts.Lessons,
			StudentsCount: d.Counts.Students, // активные записи — как в списке T25
		},
		CoverPath: d.Course.CoverPath,
	}

	// Программа: модули уже отсортированы по position хранилищем; слайсы —
	// make, а не nil: пустая программа => "modules": [].
	mods := make([]adminCourseDetailModule, 0, len(d.Modules))
	for _, m := range d.Modules {
		ls := make([]adminCourseDetailLesson, 0, len(d.Lessons[m.ID]))
		for _, l := range d.Lessons[m.ID] {
			ls = append(ls, adminCourseDetailLesson{
				ID: l.ID, Title: l.Title, Kind: l.Kind, Position: l.Position,
			})
		}
		mods = append(mods, adminCourseDetailModule{
			ID: m.ID, Title: m.Title, Position: m.Position, Lessons: ls,
		})
	}

	writeJSON(w, http.StatusOK, adminCourseDetailResponse{
		Course:  course,
		Modules: mods,
		Stats: adminCourseDetailStats{
			StudentsCount:  d.Stats.StudentsCount,
			ActiveCount:    d.Stats.ActiveCount,
			ReviewsCount:   d.Stats.ReviewsCount,
			AvgRating:      d.Stats.AvgRating,
			PaymentsCount:  d.Stats.PaymentsCount,
			PaidTotalCents: d.Stats.PaidTotalCents,
		},
	})
}
