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

// makeSlug строит уникальный slug (T41): кастомный транслитерируется
// (translitSlug) и обязан попасть в slugRe, пустой — собирается из названия
// курса (slugFromTitle). Занятость авто-slug разрешается суффиксами -2…-9,
// затем случайным course-<hex8> — как раньше.
func (a *App) makeSlug(custom, title string) (string, bool) {
	if strings.TrimSpace(custom) != "" {
		s := translitSlug(custom)
		if !slugRe.MatchString(s) || a.Store.SlugTaken(s) {
			return "", false
		}
		return s, true
	}
	// Короткая/пустая основа (slugFromTitle вернула "") — сразу course-<hex8>.
	if auto := slugFromTitle(title); auto != "" {
		if !a.Store.SlugTaken(auto) {
			return auto, true
		}
		for i := 2; i <= 9; i++ {
			if cand := slugWithSuffix(auto, i); !a.Store.SlugTaken(cand) {
				return cand, true
			}
		}
	}
	fallback := "course-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	for a.Store.SlugTaken(fallback) {
		fallback = "course-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	}
	return fallback, true
}

type createCourseRequest struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Slug        string  `json:"slug"`
	PriceCents  int     `json:"priceCents"`
	Currency    string  `json:"currency"`
	Category    *string `json:"category"` // T65: nil — other
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
	slug, valid := a.makeSlug(req.Slug, req.Title)
	if !valid {
		writeErr(w, http.StatusBadRequest, "Некорректный или занятый slug (латиница, цифры, дефис, 3–120 символов)")
		return
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "RUB"
	}
	// T65: категория необязательна; без поля — other. Пустое/неизвестное
	// значение — 400: API принимает только фиксированные слаги.
	category := "other"
	if req.Category != nil {
		category = strings.TrimSpace(*req.Category)
		if !validCourseCategory(category) {
			writeErr(w, http.StatusBadRequest, "Неизвестная категория курса")
			return
		}
	}
	c := a.Store.CreateCourse(&repo.Course{
		TeacherID:   u.ID,
		Slug:        slug,
		Title:       strings.TrimSpace(req.Title),
		Description: strings.TrimSpace(req.Description),
		Category:    category,
		PriceCents:  req.PriceCents,
		Currency:    currency,
	})
	writeJSON(w, http.StatusCreated, c)
}

// GET /teacher/courses — живая регистрация одна: в RegisterTeacherCRUDRoutes
// (teacher_crud.go, T19), список сразу со счётчиками modulesCount/lessonsCount.
// Мёртвый дубль обработки из teacher.go удалён (T53): его регистрация в
// /teacher-роуте middleware.go всё равно перекрывалась CRUD-хендлером.

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
// T87: у teacher здесь исторически нет draft-guard (контрактные тесты
// добавляют уроки в published-курс) — правило не меняется. Для admin
// архивный курс закрыт: 409 до unarchive.
func (a *App) TeacherCreateModule(w http.ResponseWriter, r *http.Request) {
	u, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if u != nil && u.Role == repo.RoleAdmin && !requireEditable(w, u, c) {
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
	Quiz     *quizRequest    `json:"quiz"`
	CodeTask *repo.CodeTask  `json:"codeTask"`
	// IsPreview — необязательный флаг бесплатного превью (T75): указатель,
	// чтобы отличить «поле не передано» (создание без него — false) от
	// явного false.
	IsPreview *bool `json:"isPreview"`
}

// ---- quiz (T53, контракт P7: валидация теста на бэкенде) ----

// quizQuestionRequest — вопрос теста в теле запроса. Индекс правильного
// ответа принимается под обоими именами: correct (как хранится и отдаётся
// репозиторием — форма фронтенда) и correctIndex (как названо в контракте
// P7 и qa-скрипте). Для вопроса с multi правильные варианты перечислены в
// correctIndexes; correct при этом остаётся первым из них для старых клиентов.
type quizQuestionRequest struct {
	ID             string   `json:"id"`
	Text           string   `json:"text"`
	Options        []string `json:"options"`
	Correct        *int     `json:"correct"`
	CorrectIndex   *int     `json:"correctIndex"`
	Multi          bool     `json:"multi"`
	CorrectIndexes []int    `json:"correctIndexes"`
	Explanation    string   `json:"explanation,omitempty"`
}

// quizRequest — тест из вопросов: на POST/PATCH его проверяет validate
// (полный набор правил P7 в форме запроса), затем слитый урок проходит
// validateLessonContent → validateQuiz (форма хранилища).
type quizRequest struct {
	Questions []quizQuestionRequest `json:"questions"`
}

// correctAnswerIndex возвращает индекс правильного ответа: correct (как
// хранится и отдаётся репозиторием — форма существующего фронтенда) имеет
// приоритет над correctIndex (имя из контракта P7 и qa-скрипта). Вызывать
// после validate: тест без явно указанного ответа отклоняется.
func (q quizQuestionRequest) correctAnswerIndex() int {
	if q.Correct != nil {
		return *q.Correct
	}
	if q.CorrectIndex != nil {
		return *q.CorrectIndex
	}
	return 0
}

// validate проверяет тест в форме запроса по контракту P7. Дополнительно к
// правилам validateQuiz (они не видны в форме хранилища) требует, чтобы
// правильный ответ был указан явно: «неопределённый» индекс (ни correct,
// ни correctIndex) — тоже 400 «Некорректный тест: …». Возвращает текст
// ошибки или пустую строку.
func (q *quizRequest) validate() string {
	if q == nil || len(q.Questions) == 0 {
		return "Некорректный тест: должен быть хотя бы один вопрос"
	}
	for _, in := range q.Questions {
		if in.Multi {
			if len(in.CorrectIndexes) < 2 {
				return "Некорректный тест: у вопроса с несколькими ответами должно быть не менее двух правильных вариантов"
			}
			continue
		}
		if in.Correct == nil && in.CorrectIndex == nil {
			return "Некорректный тест: не указан правильный вариант ответа"
		}
	}
	return validateQuiz(q.toRepo())
}

// toRepo переводит запросную форму теста в форму хранилища (repo.QuizData).
// Вызывать после validate — она гарантирует непустой тест с указанным
// правильным ответом.
func (q *quizRequest) toRepo() *repo.QuizData {
	if q == nil {
		return nil
	}
	out := &repo.QuizData{Questions: make([]repo.QuizQuestion, len(q.Questions))}
	for i, in := range q.Questions {
		correct := in.correctAnswerIndex()
		var correctIndexes []int
		if in.Multi {
			correctIndexes = append([]int(nil), in.CorrectIndexes...)
			if len(correctIndexes) > 0 {
				correct = correctIndexes[0]
			}
		}
		out.Questions[i] = repo.QuizQuestion{
			ID:             in.ID,
			Text:           in.Text,
			Options:        in.Options,
			Correct:        correct,
			Multi:          in.Multi,
			CorrectIndexes: correctIndexes,
			Explanation:    in.Explanation,
		}
	}
	return out
}

// validateQuiz проверяет тест по контракту P7 (тексты не заморожены):
// у каждого вопроса есть непустой текст, не менее двух непустых вариантов
// и правильный ответ внутри границ списка. Возвращает текст для 400
// «Некорректный тест: …» или пустую строку, если тест корректен.
// Старые уроки не рестриктурируются — валидация работает только на POST/PATCH.
func validateQuiz(qd *repo.QuizData) string {
	if qd == nil || len(qd.Questions) == 0 {
		return "Некорректный тест: должен быть хотя бы один вопрос"
	}
	for _, q := range qd.Questions {
		if strings.TrimSpace(q.Text) == "" {
			return "Некорректный тест: у вопроса нет текста"
		}
		if len(q.Options) < 2 {
			return "Некорректный тест: у вопроса должно быть не менее двух вариантов"
		}
		for _, opt := range q.Options {
			if strings.TrimSpace(opt) == "" {
				return "Некорректный тест: у вопроса есть пустой вариант ответа"
			}
		}
		if q.Multi {
			if len(q.CorrectIndexes) < 2 {
				return "Некорректный тест: у вопроса с несколькими ответами должно быть не менее двух правильных вариантов"
			}
			seen := make(map[int]struct{}, len(q.CorrectIndexes))
			for _, idx := range q.CorrectIndexes {
				if idx < 0 || idx >= len(q.Options) {
					return "Некорректный тест: индекс правильного ответа вне диапазона вариантов"
				}
				if _, ok := seen[idx]; ok {
					return "Некорректный тест: повторяющийся правильный вариант ответа"
				}
				seen[idx] = struct{}{}
			}
			continue
		}
		if q.Correct < 0 || q.Correct >= len(q.Options) {
			return "Некорректный тест: индекс правильного ответа вне диапазона вариантов"
		}
	}
	return ""
}

// parseLessonStartsAt разбирает дату начала урока (контракт P7): RFC3339 или
// datetime-local «ГГГГ-ММ-ДДТЧЧ:ММ» без зоны — такая строка трактуется как UTC
// (форма datetime-local не несёт информации о поясе). Оставлена и форма «ГГГГ-ММ-ДД»
// (полночь UTC) — так раньше разбирались даты, чтобы старые клиенты не сломались.
func parseLessonStartsAt(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", raw, time.UTC); err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02", raw, time.UTC)
}

// TeacherCreateLesson — POST /teacher/courses/{id}/lessons с валидацией по kind.
// T87: у teacher draft-guard на создание не было и нет (поведение не меняем);
// admin не может добавлять уроки в архивный курс — 409 до unarchive.
func (a *App) TeacherCreateLesson(w http.ResponseWriter, r *http.Request) {
	u, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if u != nil && u.Role == repo.RoleAdmin && !requireEditable(w, u, c) {
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
	// Расхождение №4 (закрыто T16): startsAt раньше принимался, но не копировался
	// в repo.Lesson — планирование живых эфиров не работало. Разбираем строку
	// здесь: мусор в поле — ошибка запроса, а не тихо потерянный эфир.
	// Формат разбирает parseLessonStartsAt: RFC3339 или datetime-local
	// (контракт P7: строка без зоны считается UTC).
	l := &repo.Lesson{
		ModuleID:  mod.ID,
		Title:     strings.TrimSpace(req.Title),
		Kind:      req.Kind,
		BodyMD:    req.BodyMD,
		VideoURL:  strings.TrimSpace(req.VideoURL),
		LiveURL:   strings.TrimSpace(req.LiveURL),
		Quiz:      req.Quiz.toRepo(),
		CodeTask:  req.CodeTask,
		IsPreview: req.IsPreview != nil && *req.IsPreview,
	}
	if req.StartsAt != nil {
		raw := strings.TrimSpace(*req.StartsAt)
		if raw != "" {
			parsed, err := parseLessonStartsAt(raw)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "Некорректная дата начала (нужен RFC3339, например 2026-10-09T19:00:00+03:00)")
				return
			}
			l.StartsAt = &parsed
		}
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
		// Контракт P7: тест проверяется в форме запроса (validate — явный
		// правильный ответ + правила validateQuiz), текст ошибки начинается
		// с «Некорректный тест:», иначе — по валидации T16.
		if msg := req.Quiz.validate(); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		for i := range l.Quiz.Questions {
			if strings.TrimSpace(l.Quiz.Questions[i].ID) == "" {
				l.Quiz.Questions[i].ID = uuid.NewString()
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
