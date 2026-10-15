package handler

// Бэклог T06 (реализация в T13): эндпоинты редактирования конструктора курсов,
// которых не хватало фронтенду для переименования/правки карточки.
// Зарегистрированы в NewRouter внутри r.Route("/teacher", ...) — см.
// RegisterTeacherEditRoutes в middleware.go.
//
// Реализованы три приоритетных эндпоинта из бэклога T06:
//   - GET   /teacher/courses/{id}                 — редактор по id без обхода через slug;
//   - PATCH /teacher/courses/{id}                 — правка карточки курса;
//   - PATCH /teacher/courses/{id}/modules/{moduleID} — переименование модуля.
//
// Остальные пункты бэклога (DELETE, reorder, PATCH урока, счётчики в списке)
// не реализованы: фронтенд T06 на них не рассчитан, а расширение интерфейса
// StoreAPI без клиентов раздувает scope. См. отчёт T13.

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// RegisterTeacherEditRoutes вешает маршруты редактирования на /teacher-роутер
// (requireAuth + requireRole(teacher, admin) уже навешены родителем).
func (a *App) RegisterTeacherEditRoutes(r chi.Router) {
	r.Get("/courses/{id}", a.TeacherGetCourse)
	r.Patch("/courses/{id}", a.TeacherUpdateCourse)
	r.Patch("/courses/{id}/modules/{moduleID}", a.TeacherUpdateModule)
}

// TeacherGetCourse — GET /teacher/courses/{id}: курс + модули с полными
// уроками (владелец видит quiz/без скрытых тестов — та же форма, что
// GetCourse для владельца).
func (a *App) TeacherGetCourse(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, a.courseDetail(c))
}

// courseDetail собирает конверт { course, modules } с полными уроками —
// витринный метод для GET /teacher/courses/{id}.
func (a *App) courseDetail(c *repo.Course) map[string]any {
	mods := a.Store.ListModulesByCourse(c.ID)
	mv := make([]moduleView, 0, len(mods))
	for _, m := range mods {
		lessons := a.Store.ListLessonsByModule(m.ID)
		lv := make([]lessonView, 0, len(lessons))
		for _, l := range lessons {
			lv = append(lv, lessonToView(l, true))
		}
		mv = append(mv, moduleView{ID: m.ID, Title: m.Title, Position: m.Position, Lessons: lv})
	}
	return map[string]any{"course": c, "modules": mv}
}

type updateCourseRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Slug        *string `json:"slug"`
	PriceCents  *int    `json:"priceCents"`
}

// TeacherUpdateCourse — PATCH /teacher/courses/{id}: частичное обновление
// карточки. Пустые строки, отрицательная цена и занятый slug — 400.
// Статус курса этим эндпоинтом не меняется (модерация — через submit).
func (a *App) TeacherUpdateCourse(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	var req updateCourseRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	upd := repo.CourseUpdate{}
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if t == "" {
			writeErr(w, http.StatusBadRequest, "Название курса обязательно")
			return
		}
		upd.Title = &t
	}
	if req.Description != nil {
		d := strings.TrimSpace(*req.Description)
		if d == "" {
			writeErr(w, http.StatusBadRequest, "Описание курса обязательно")
			return
		}
		upd.Description = &d
	}
	if req.PriceCents != nil {
		if *req.PriceCents < 0 {
			writeErr(w, http.StatusBadRequest, "Цена не может быть отрицательной")
			return
		}
		upd.PriceCents = req.PriceCents
	}
	if req.Slug != nil {
		s := strings.ToLower(strings.TrimSpace(*req.Slug))
		if !slugRe.MatchString(s) {
			writeErr(w, http.StatusBadRequest, "Некорректный slug (латиница, цифры, дефис, 3–120 символов)")
			return
		}
		// Свой текущий slug не считается занятым.
		if s != strings.ToLower(c.Slug) && a.Store.SlugTaken(s) {
			writeErr(w, http.StatusBadRequest, "Такой slug уже занят")
			return
		}
		upd.Slug = &s
	}
	if upd.Title == nil && upd.Description == nil && upd.Slug == nil && upd.PriceCents == nil {
		writeErr(w, http.StatusBadRequest, "Укажите хотя бы одно поле для обновления")
		return
	}
	if !a.Store.UpdateCourse(c.ID, upd) {
		writeErr(w, http.StatusInternalServerError, "Не удалось обновить курс")
		return
	}
	updated, _ := a.Store.GetCourseByID(c.ID)
	writeJSON(w, http.StatusOK, updated)
}

type updateModuleRequest struct {
	Title string `json:"title"`
}

// TeacherUpdateModule — PATCH /teacher/courses/{id}/modules/{moduleID}:
// переименование модуля. Повторный POST создал бы новый модуль, поэтому
// для правки названия нужен именно PATCH.
func (a *App) TeacherUpdateModule(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	var req updateModuleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeErr(w, http.StatusBadRequest, "Название модуля обязательно")
		return
	}
	mod, found := a.Store.GetModule(chi.URLParam(r, "moduleID"))
	if !found || mod.CourseID != c.ID {
		writeErr(w, http.StatusNotFound, "Модуль не найден")
		return
	}
	if !a.Store.UpdateModuleTitle(mod.ID, title) {
		writeErr(w, http.StatusInternalServerError, "Не удалось переименовать модуль")
		return
	}
	updated, _ := a.Store.GetModule(mod.ID)
	writeJSON(w, http.StatusOK, updated)
}
