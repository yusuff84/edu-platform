package handler

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

var slugRe = regexp.MustCompile(`^[a-z0-9-]{3,120}$`)

// makeSlug строит уникальный slug; кириллицу не транслитерируем —
// используем префикс course- + короткий id, кастомный slug валидируем.
func (a *App) makeSlug(custom string) (string, bool) {
	if strings.TrimSpace(custom) != "" {
		s := strings.ToLower(strings.TrimSpace(custom))
		if !slugRe.MatchString(s) || a.Store.SlugTaken(s) {
			return "", false
		}
		return s, true
	}
	base := "course-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	for a.Store.SlugTaken(base) {
		base = "course-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	}
	return base, true
}

type createCourseRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Slug        string `json:"slug"`
	PriceCents  int    `json:"priceCents"`
	Currency    string `json:"currency"`
}

// TeacherCreateCourse — POST /teacher/courses: черновик курса.
func (a *App) TeacherCreateCourse(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	var req createCourseRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeErr(w, http.StatusBadRequest, "Название курса обязательно")
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		writeErr(w, http.StatusBadRequest, "Описание курса обязательно")
		return
	}
	if req.PriceCents < 0 {
		writeErr(w, http.StatusBadRequest, "Цена не может быть отрицательной")
		return
	}
	slug, valid := a.makeSlug(req.Slug)
	if !valid {
		writeErr(w, http.StatusBadRequest, "Некорректный или занятый slug (латиница, цифры, дефис, 3–120 символов)")
		return
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "RUB"
	}
	c := a.Store.CreateCourse(&repo.Course{
		TeacherID:   u.ID,
		Slug:        slug,
		Title:       strings.TrimSpace(req.Title),
		Description: strings.TrimSpace(req.Description),
		PriceCents:  req.PriceCents,
		Currency:    currency,
	})
	writeJSON(w, http.StatusCreated, c)
}

// TeacherListCourses — GET /teacher/courses: свои курсы.
func (a *App) TeacherListCourses(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	list := a.Store.ListCoursesByTeacher(u.ID)
	if list == nil {
		list = []*repo.Course{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "total": len(list)})
}

// ownCourse проверяет владение курсом (админу разрешены все).
func (a *App) ownCourse(w http.ResponseWriter, r *http.Request) (*repo.User, *repo.Course, bool) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return nil, nil, false
	}
	c, found := a.Store.GetCourseByID(chi.URLParam(r, "id"))
	if !found {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return nil, nil, false
	}
	if c.TeacherID != u.ID && u.Role != repo.RoleAdmin {
		writeErr(w, http.StatusForbidden, "Это чужой курс")
		return nil, nil, false
	}
	return u, c, true
}

type createModuleRequest struct {
	Title string `json:"title"`
}

// TeacherCreateModule — POST /teacher/courses/{id}/modules.
func (a *App) TeacherCreateModule(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	var req createModuleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeErr(w, http.StatusBadRequest, "Название модуля обязательно")
		return
	}
	m := a.Store.CreateModule(c.ID, strings.TrimSpace(req.Title))
	a.Store.TouchCourse(c.ID)
	writeJSON(w, http.StatusCreated, m)
}

type createLessonRequest struct {
	ModuleID string          `json:"moduleId"`
	Title    string          `json:"title"`
	Kind     repo.LessonKind `json:"kind"`
	BodyMD   string          `json:"bodyMd"`
	VideoURL string          `json:"videoUrl"`
	LiveURL  string          `json:"liveUrl"`
	StartsAt *string         `json:"startsAt"`
	Quiz     *repo.QuizData  `json:"quiz"`
	CodeTask *repo.CodeTask  `json:"codeTask"`
}

// TeacherCreateLesson — POST /teacher/courses/{id}/lessons с валидацией по kind.
func (a *App) TeacherCreateLesson(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	var req createLessonRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeErr(w, http.StatusBadRequest, "Название урока обязательно")
		return
	}
	mod, found := a.Store.GetModule(req.ModuleID)
	if !found || mod.CourseID != c.ID {
		writeErr(w, http.StatusBadRequest, "Модуль не принадлежит этому курсу")
		return
	}
	// T16 (п.1, расхождение №4): startsAt раньше принимался, но не копировался
	// в repo.Lesson — планирование живых эфиров не работало. Разбираем строку
	// здесь: мусор в поле — ошибка запроса, а не тихо потерянный эфир.
	var startsAt *time.Time
	if req.StartsAt != nil && strings.TrimSpace(*req.StartsAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.StartsAt))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Некорректный startsAt: ожидается дата в формате RFC3339 (например 2030-01-01T12:00:00Z)")
			return
		}
		startsAt = &t
	}
	l := &repo.Lesson{
		ModuleID: mod.ID,
		Title:    strings.TrimSpace(req.Title),
		Kind:     req.Kind,
		BodyMD:   req.BodyMD,
		VideoURL: strings.TrimSpace(req.VideoURL),
		LiveURL:  strings.TrimSpace(req.LiveURL),
		StartsAt: startsAt,
		Quiz:     req.Quiz,
		CodeTask: req.CodeTask,
	}
	switch req.Kind {
	case repo.LessonVideoText:
		if strings.TrimSpace(l.BodyMD) == "" && l.VideoURL == "" {
			writeErr(w, http.StatusBadRequest, "Уроку нужен текст или ссылка на видео")
			return
		}
	case repo.LessonLive:
		if l.LiveURL == "" {
			writeErr(w, http.StatusBadRequest, "Для live-урока нужна ссылка на трансляцию")
			return
		}
	case repo.LessonQuiz:
		if l.Quiz == nil || len(l.Quiz.Questions) == 0 {
			writeErr(w, http.StatusBadRequest, "Квиз должен содержать минимум 1 вопрос")
			return
		}
		for i := range l.Quiz.Questions {
			q := &l.Quiz.Questions[i]
			if strings.TrimSpace(q.Text) == "" || len(q.Options) < 2 {
				writeErr(w, http.StatusBadRequest, "Каждый вопрос: текст и минимум 2 варианта ответа")
				return
			}
			if q.Correct < 0 || q.Correct >= len(q.Options) {
				writeErr(w, http.StatusBadRequest, "Индекс правильного ответа вне диапазона вариантов")
				return
			}
			if strings.TrimSpace(q.ID) == "" {
				q.ID = uuid.NewString()
			}
		}
	case repo.LessonCode:
		if l.CodeTask == nil {
			writeErr(w, http.StatusBadRequest, "Для code-урока нужно задание")
			return
		}
		switch strings.ToLower(l.CodeTask.Language) {
		case "python", "js", "htmlcss":
		default:
			writeErr(w, http.StatusBadRequest, "Язык задания: python, js или htmlcss")
			return
		}
		if len(l.CodeTask.Tests) == 0 {
			writeErr(w, http.StatusBadRequest, "Задание должно содержать минимум 1 тест")
			return
		}
	default:
		writeErr(w, http.StatusBadRequest, "Неизвестный тип урока (video_text, live, quiz, code)")
		return
	}
	l = a.Store.CreateLesson(l)
	a.Store.TouchCourse(c.ID)
	writeJSON(w, http.StatusCreated, l)
}

// TeacherSubmitCourse — POST /teacher/courses/{id}/submit: guard минимум 1 модуль и 1 урок.
func (a *App) TeacherSubmitCourse(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if c.Status != repo.CourseDraft {
		writeErr(w, http.StatusBadRequest, "На модерацию можно отправить только черновик")
		return
	}
	mods := a.Store.ListModulesByCourse(c.ID)
	if len(mods) == 0 || a.Store.CountLessonsByCourse(c.ID) == 0 {
		writeErr(w, http.StatusBadRequest, "Курс пуст: добавьте минимум 1 модуль и 1 урок")
		return
	}
	a.Store.SetCourseStatus(c.ID, repo.CourseReview, "")
	updated, _ := a.Store.GetCourseByID(c.ID)
	writeJSON(w, http.StatusOK, updated)
}
