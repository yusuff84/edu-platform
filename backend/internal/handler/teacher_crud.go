package handler

// T19 — CRUD конструктора: правка урока, удаление, порядок, счётчики
// (бэклог T06; карточку курса и переименование модуля уже сделал T13 в
// teacher_edit.go — они остались там).
//
// Маршруты регистрируются RegisterTeacherCRUDRoutes внутри r.Route("/teacher")
// в middleware.go: requireAuth + requireRole(teacher, admin) навешаны родителем,
// пути относительные — так же, как RegisterTeacherEditRoutes (T13).
//
// GET /teacher/courses — единственная регистрация (T53 убрал мёртвый дубль
// регистрации из middleware.go и обработку-дубль из teacher.go): список
// моих курсов отдаётся с modulesCount/lessonsCount — фронт больше не делает
// N+1 запросов.
//
// Права: только владелец курса или админ (ownCourse).
// Статус: контент (уроки, порядок, удаление) — только draft (409 иначе);
// правка карточки и переименование модуля работают в любом статусе — это
// закреплено контрактными тестами teacher_test.go:TestTeacherEditEndpoints
// (PATCH опубликованного курса и модуля → 200).

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// RegisterTeacherCRUDRoutes вешает маршруты T19 на /teacher-роутер
// (см. middleware.go: вызов внутри r.Route("/teacher", ...)).
func (a *App) RegisterTeacherCRUDRoutes(r chi.Router) {
	r.Get("/courses", a.TeacherListCoursesWithCounts)
	r.Patch("/courses/{id}/lessons/{lessonID}", a.TeacherUpdateLesson)
	r.Delete("/courses/{id}/modules/{moduleID}", a.TeacherDeleteModule)
	r.Delete("/courses/{id}/lessons/{lessonID}", a.TeacherDeleteLesson)
	r.Post("/courses/{id}/modules/reorder", a.TeacherReorderModules)
	r.Post("/courses/{id}/lessons/reorder", a.TeacherReorderLessons)
	r.Delete("/courses/{id}", a.TeacherDeleteCourse)
}

// requireDraft — контент курса правится только в черновике (T19, правило 2):
// после отправки на модерацию и после публикации программа заморожена.
// Пишет 409 и возвращает false, если курс уже не draft.
func requireDraft(w http.ResponseWriter, c *repo.Course) bool {
	if c.Status != repo.CourseDraft {
		writeErr(w, http.StatusConflict,
			"Контент курса можно редактировать только в режиме черновика")
		return false
	}
	return true
}

// ---- GET /teacher/courses: свои курсы + счётчики ----

// teacherCourseItem — строка списка моих курсов: полная форма Course
// (встраивание указателя разворачивает все JSON-поля) + счётчики программы.
type teacherCourseItem struct {
	*repo.Course
	ModulesCount int `json:"modulesCount"`
	LessonsCount int `json:"lessonsCount"`
}

// TeacherListCoursesWithCounts — GET /teacher/courses (единственная
// регистрация с T53): в каждый элемент списка добавлены
// modulesCount/lessonsCount (одним запросом CountContentByCourses вместо
// N+1 на фронте; T19). items никогда не null — make с нулевой ёмкостью.
func (a *App) TeacherListCoursesWithCounts(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	list := a.Store.ListCoursesByTeacher(u.ID)
	counts := a.Store.CountContentByCourses(u.ID)
	items := make([]teacherCourseItem, 0, len(list))
	for _, c := range list {
		cc := counts[c.ID]
		items = append(items, teacherCourseItem{
			Course: c, ModulesCount: cc.Modules, LessonsCount: cc.Lessons,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// ---- PATCH /teacher/courses/{id}/lessons/{lessonID} ----

// updateLessonRequest — тело как в createLessonRequest (DisallowUnknownFields:
// неизвестное поле = 400). moduleId принимаем ради совместимости с формой
// создания, но игнорируем — перенос урока в другой модуль API не поддерживает.
// kind, если передан, обязан совпасть с текущим: смена типа урока после
// создания не предусмотрена (квиз с ответами не может стать видеоуроком).
type updateLessonRequest struct {
	ModuleID string           `json:"moduleId"`
	Title    *string          `json:"title"`
	Kind     *repo.LessonKind `json:"kind"`
	BodyMD   *string          `json:"bodyMd"`
	VideoURL *string          `json:"videoUrl"`
	LiveURL  *string          `json:"liveUrl"`
	StartsAt *string          `json:"startsAt"` // "" — очистить; отсутствует — не менять
	Quiz     *quizRequest     `json:"quiz"`
	CodeTask *repo.CodeTask   `json:"codeTask"`
	// IsPreview — необязательный флаг превью (T75): указатель, чтобы PATCH
	// без поля не менял значение, а явный false снимал флаг.
	IsPreview *bool `json:"isPreview"`
}

// TeacherUpdateLesson — PATCH .../lessons/{lessonID}: переданные поля
// перезаписывают содержимое урока, непереданные остаются; валидация — по kind
// (validateLessonContent, та же валидация, что в TeacherCreateLesson).
// 200; 400 (пустой title, неверный startsAt, контент не прошёл валидацию по
// kind); 404 (урок не из этого курса); 409 (курс не draft).
func (a *App) TeacherUpdateLesson(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if !requireDraft(w, c) {
		return
	}
	var req updateLessonRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	cur, found := a.Store.GetLesson(chi.URLParam(r, "lessonID"))
	if !found || !a.lessonInCourse(cur, c.ID) {
		writeErr(w, http.StatusNotFound, "Урок не найден")
		return
	}
	if req.Kind != nil && *req.Kind != cur.Kind {
		writeErr(w, http.StatusBadRequest, "Тип урока изменить нельзя")
		return
	}
	// Слияние: начинаем с текущего урока, перезаписываем только переданные поля.
	merged := &repo.Lesson{
		ID: cur.ID, ModuleID: cur.ModuleID, Kind: cur.Kind, Position: cur.Position,
		Title: cur.Title, BodyMD: cur.BodyMD, VideoURL: cur.VideoURL,
		LiveURL: cur.LiveURL, StartsAt: cur.StartsAt, Quiz: cur.Quiz, CodeTask: cur.CodeTask,
		IsPreview: cur.IsPreview,
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			writeErr(w, http.StatusBadRequest, "Название урока обязательно")
			return
		}
		merged.Title = title
	}
	if req.BodyMD != nil {
		merged.BodyMD = *req.BodyMD
	}
	if req.VideoURL != nil {
		merged.VideoURL = strings.TrimSpace(*req.VideoURL)
	}
	if req.LiveURL != nil {
		merged.LiveURL = strings.TrimSpace(*req.LiveURL)
	}
	if req.StartsAt != nil {
		raw := strings.TrimSpace(*req.StartsAt)
		if raw == "" {
			merged.StartsAt = nil // явная очистка
		} else {
			// Контракт P7: datetime-local (без зоны) тоже принимается, строка
			// без зоны трактуется как UTC; ответ/ошибки — как было (400).
			ts, err := parseLessonStartsAt(raw)
			if err != nil {
				writeErr(w, http.StatusBadRequest,
					"Некорректный startsAt: ожидается дата в формате RFC3339 (например 2030-01-01T12:00:00Z)")
				return
			}
			merged.StartsAt = &ts
		}
	}
	if req.Quiz != nil {
		// Контракт P7: тест проверяется в форме запроса (validate — явный
		// правильный ответ correct/correctIndex + правила), затем слитый
		// урок повторно проверяет validateLessonContent → validateQuiz.
		if msg := req.Quiz.validate(); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		merged.Quiz = req.Quiz.toRepo()
		for i := range merged.Quiz.Questions {
			if strings.TrimSpace(merged.Quiz.Questions[i].ID) == "" {
				merged.Quiz.Questions[i].ID = uuid.NewString()
			}
		}
	}
	if req.CodeTask != nil {
		merged.CodeTask = req.CodeTask
	}
	if req.IsPreview != nil {
		merged.IsPreview = *req.IsPreview
	}
	if msg := validateLessonContent(merged); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := a.Store.UpdateLesson(merged); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Урок не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось обновить урок")
		return
	}
	a.Store.TouchCourse(c.ID)
	updated, _ := a.Store.GetLesson(merged.ID)
	writeJSON(w, http.StatusOK, updated)
}

// lessonInCourse проверяет, что урок принадлежит одному из модулей курса.
func (a *App) lessonInCourse(l *repo.Lesson, courseID string) bool {
	m, ok := a.Store.GetModule(l.ModuleID)
	return ok && m.CourseID == courseID
}

// validateLessonContent проверяет содержимое урока по kind. Возвращает пустую
// строку, если урок валиден, иначе текст ошибки для 400. Ветки video/live/code
// повторяют валидацию TeacherCreateLesson (teacher.go); с T53 (P7) quiz
// проверяет общий validateQuiz из teacher.go (контракт P7): текст вопроса,
// не менее двух непустых вариантов, правильный ответ в границах.
func validateLessonContent(l *repo.Lesson) string {
	switch l.Kind {
	case repo.LessonVideoText:
		if strings.TrimSpace(l.BodyMD) == "" && strings.TrimSpace(l.VideoURL) == "" {
			return "Уроку нужен текст или ссылка на видео"
		}
	case repo.LessonLive:
		if strings.TrimSpace(l.LiveURL) == "" {
			return "Для live-урока нужна ссылка на трансляцию"
		}
	case repo.LessonQuiz:
		// Контракт P7: тест валидируется и на PATCH, текст ошибки начинается
		// с «Некорректный тест:» (validateQuiz — общий с POST, T53).
		if msg := validateQuiz(l.Quiz); msg != "" {
			return msg
		}
	case repo.LessonCode:
		if l.CodeTask == nil {
			return "Для code-урока нужно задание"
		}
		switch strings.ToLower(l.CodeTask.Language) {
		case "python", "js", "htmlcss":
		default:
			return "Язык задания: python, js или htmlcss"
		}
		if len(l.CodeTask.Tests) == 0 {
			return "Задание должно содержать минимум 1 тест"
		}
	default:
		return "Неизвестный тип урока (video_text, live, quiz, code)"
	}
	return ""
}

// ---- DELETE: модуль, урок, курс ----

// TeacherDeleteModule — DELETE .../modules/{moduleID}: 204. 409 — в модуле
// остались уроки (сначала удалите их); 404 — модуль не из этого курса;
// 409 — курс не draft. Каскад в БД удалил бы уроки молча, поэтому
// непустоту проверяем до удаления.
func (a *App) TeacherDeleteModule(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if !requireDraft(w, c) {
		return
	}
	mod, found := a.Store.GetModule(chi.URLParam(r, "moduleID"))
	if !found || mod.CourseID != c.ID {
		writeErr(w, http.StatusNotFound, "Модуль не найден")
		return
	}
	if len(a.Store.ListLessonsByModule(mod.ID)) > 0 {
		writeErr(w, http.StatusConflict, "В модуле есть уроки: сначала удалите их")
		return
	}
	if err := a.Store.DeleteModule(mod.ID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Модуль не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось удалить модуль")
		return
	}
	a.Store.TouchCourse(c.ID)
	w.WriteHeader(http.StatusNoContent)
}

// TeacherDeleteLesson — DELETE .../lessons/{lessonID}: 204. Прогресс студентов
// по уроку удаляется каскадом. 404 — урок не из этого курса; 409 — не draft.
func (a *App) TeacherDeleteLesson(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if !requireDraft(w, c) {
		return
	}
	l, found := a.Store.GetLesson(chi.URLParam(r, "lessonID"))
	if !found || !a.lessonInCourse(l, c.ID) {
		writeErr(w, http.StatusNotFound, "Урок не найден")
		return
	}
	if err := a.Store.DeleteLesson(l.ID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Урок не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось удалить урок")
		return
	}
	a.Store.TouchCourse(c.ID)
	w.WriteHeader(http.StatusNoContent)
}

// TeacherDeleteCourse — DELETE /teacher/courses/{id}: 204. Каскад в БД удаляет
// записи, платежи и отзывы вместе с курсом, поэтому разрешено удалять только
// черновики: для опубликованных курсов это потеряла бы историю оплат в
// журнале US6 (409). Архивация опубликованных курсов — в бэклоге T20.
func (a *App) TeacherDeleteCourse(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if !requireDraft(w, c) {
		return
	}
	if err := a.Store.DeleteCourse(c.ID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Курс не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось удалить курс")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Порядок: reorder ----

// reorderItem — пункт списка перестановки: id сущности и её новая позиция.
type reorderItem struct {
	ID       string `json:"id"`
	Position int    `json:"position"`
}

// reorderRequest — тело reorder-эндпоинтов: полный список позиций
// (идемпотентность: повтор с тем же списком ничего не меняет).
type reorderRequest struct {
	Items []reorderItem `json:"items"`
}

// TeacherReorderModules — POST .../modules/reorder: 204. Принимает полный
// список модулей курса с позициями 1..N и применяет его одной транзакцией.
// 400 — пустой/неполный список, позиция вне диапазона, дубликат позиции или
// id; 404 — модуль не из этого курса; 409 — курс не draft.
func (a *App) TeacherReorderModules(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if !requireDraft(w, c) {
		return
	}
	var req reorderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	mods := a.Store.ListModulesByCourse(c.ID)
	total := len(mods)
	if len(req.Items) != total {
		writeErr(w, http.StatusBadRequest,
			fmt.Sprintf("Ожидался полный список модулей курса (%d)", total))
		return
	}
	known := make(map[string]bool, total)
	for _, m := range mods {
		known[m.ID] = true
	}
	seenID := make(map[string]bool, total)
	seenPos := make(map[int]bool, total)
	items := make([]repo.PositionItem, 0, total)
	for _, it := range req.Items {
		if !known[it.ID] {
			writeErr(w, http.StatusNotFound, "Модуль не найден в этом курсе")
			return
		}
		if seenID[it.ID] {
			writeErr(w, http.StatusBadRequest, "Позиции модулей не должны дублироваться")
			return
		}
		if it.Position < 1 || it.Position > total {
			writeErr(w, http.StatusBadRequest,
				fmt.Sprintf("Позиция модуля вне диапазона 1..%d", total))
			return
		}
		if seenPos[it.Position] {
			writeErr(w, http.StatusBadRequest,
				fmt.Sprintf("Позиция %d указана дважды", it.Position))
			return
		}
		seenID[it.ID] = true
		seenPos[it.Position] = true
		items = append(items, repo.PositionItem{ID: it.ID, Position: it.Position})
	}
	if err := a.Store.ReorderModules(c.ID, items); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Модуль не найден в этом курсе")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось изменить порядок модулей")
		return
	}
	a.Store.TouchCourse(c.ID)
	w.WriteHeader(http.StatusNoContent)
}

// TeacherReorderLessons — POST .../lessons/reorder: 204. Принимает полный
// список уроков курса; позиция — порядковый номер урока ВНУТРИ своего модуля
// (схема БД: UNIQUE (module_id, position)). Смена модуля урока при этом не
// происходит — меняется только порядок внутри модуля.
// 400 — пустой/неполный список, позиция вне диапазона модуля, дубликат;
// 404 — урок не из этого курса; 409 — курс не draft.
func (a *App) TeacherReorderLessons(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if !requireDraft(w, c) {
		return
	}
	var req reorderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	lessons := a.Store.ListLessonsByCourse(c.ID)
	if len(req.Items) != len(lessons) {
		writeErr(w, http.StatusBadRequest,
			fmt.Sprintf("Ожидался полный список уроков курса (%d)", len(lessons)))
		return
	}
	byID := make(map[string]*repo.Lesson, len(lessons))
	modTotal := make(map[string]int)
	for _, l := range lessons {
		byID[l.ID] = l
		modTotal[l.ModuleID]++
	}
	seenID := make(map[string]bool, len(lessons))
	seenPos := make(map[string]map[int]bool)
	items := make([]repo.PositionItem, 0, len(lessons))
	for _, it := range req.Items {
		l, ok := byID[it.ID]
		if !ok {
			writeErr(w, http.StatusNotFound, "Урок не найден в этом курсе")
			return
		}
		if seenID[it.ID] {
			writeErr(w, http.StatusBadRequest, "Позиции уроков не должны дублироваться")
			return
		}
		size := modTotal[l.ModuleID]
		if it.Position < 1 || it.Position > size {
			writeErr(w, http.StatusBadRequest,
				fmt.Sprintf("Позиция урока вне диапазона 1..%d", size))
			return
		}
		if seenPos[l.ModuleID] == nil {
			seenPos[l.ModuleID] = make(map[int]bool, size)
		}
		if seenPos[l.ModuleID][it.Position] {
			writeErr(w, http.StatusBadRequest,
				fmt.Sprintf("Позиция %d в модуле указана дважды", it.Position))
			return
		}
		seenID[it.ID] = true
		seenPos[l.ModuleID][it.Position] = true
		items = append(items, repo.PositionItem{ID: it.ID, Position: it.Position})
	}
	if err := a.Store.ReorderLessons(c.ID, items); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Урок не найден в этом курсе")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось изменить порядок уроков")
		return
	}
	a.Store.TouchCourse(c.ID)
	w.WriteHeader(http.StatusNoContent)
}
