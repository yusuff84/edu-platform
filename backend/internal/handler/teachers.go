package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// teacherProfileResponse — публичный профиль преподавателя (T81):
// карточка со статистикой и published-курсы в форме CourseItem.
type teacherProfileResponse struct {
	Teacher repo.TeacherPublic `json:"teacher"`
	Courses []courseItem       `json:"courses"`
}

// GetTeacherProfile — GET /teachers/{id}: публичный профиль преподавателя
// (без авторизации). 404 «Преподаватель не найден» — пользователя нет или
// его роль не teacher/admin. Агрегаты и курсы готовит хранилище
// (StoreAPI.GetTeacherProfile): только published-курсы, порядок — активные
// записи desc, затем createdAt desc.
func (a *App) GetTeacherProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	data, ok := a.Store.GetTeacherProfile(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "Преподаватель не найден")
		return
	}
	courses := make([]courseItem, 0, len(data.Courses))
	for _, c := range data.Courses {
		courses = append(courses, toCourseItem(c))
	}
	writeJSON(w, http.StatusOK, teacherProfileResponse{
		Teacher: data.Teacher,
		Courses: courses,
	})
}
