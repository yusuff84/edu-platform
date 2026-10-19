package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

type courseItem struct {
	ID           string            `json:"id"`
	Slug         string            `json:"slug"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	Category     string            `json:"category"`
	CoverPath    *string           `json:"coverPath"`
	PriceCents   int               `json:"priceCents"`
	Currency     string            `json:"currency"`
	Status       repo.CourseStatus `json:"status,omitempty"`
	AvgRating    float64           `json:"avgRating"`
	RatingsCount int               `json:"ratingsCount"`
}

func toCourseItem(c *repo.Course) courseItem {
	category := c.Category
	if category == "" {
		category = "other"
	}
	return courseItem{
		ID: c.ID, Slug: c.Slug, Title: c.Title, Description: c.Description,
		Category:   category,
		CoverPath:  c.CoverPath,
		PriceCents: c.PriceCents, Currency: c.Currency,
		AvgRating: c.AvgRating, RatingsCount: c.RatingsCount,
	}
}

// catalogDefaults — значения фильтров GET /courses по умолчанию (T65):
// без параметров каталог сортируется по популярности.
const catalogSortDefault = "popular"

// ListCourses — GET /courses: каталог только опубликованных курсов.
//
// T65: серверные фильтры/сортировка — category, price, minRating, sort
// (комбинируются с q/page/limit через AND). Валидация значений — здесь:
// неизвестный слаг категории → 400 «Неизвестная категория», price вне
// free|paid → «Недопустимый фильтр цены», minRating не из 3.5|4|4.5 →
// «Недопустимый фильтр рейтинга», sort не из списка → «Недопустимая
// сортировка». total считается по отфильтрованному набору до пагинации.
func (a *App) ListCourses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("q")
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))

	category := strings.TrimSpace(q.Get("category"))
	if category != "" && !validCourseCategory(category) {
		writeErr(w, http.StatusBadRequest, "Неизвестная категория")
		return
	}

	price := strings.TrimSpace(q.Get("price"))
	switch price {
	case "", "free", "paid":
	default:
		writeErr(w, http.StatusBadRequest, "Недопустимый фильтр цены")
		return
	}

	var minRating float64
	switch strings.TrimSpace(q.Get("minRating")) {
	case "":
	case "3.5":
		minRating = 3.5
	case "4":
		minRating = 4
	case "4.5":
		minRating = 4.5
	default:
		writeErr(w, http.StatusBadRequest, "Недопустимый фильтр рейтинга")
		return
	}

	sortKey := strings.TrimSpace(q.Get("sort"))
	if sortKey == "" {
		sortKey = catalogSortDefault
	}
	switch sortKey {
	case "popular", "new", "rating", "price_asc", "price_desc":
	default:
		writeErr(w, http.StatusBadRequest, "Недопустимая сортировка")
		return
	}

	all := a.Store.ListCatalogCourses(repo.CatalogFilter{
		Q: search, Category: category, Price: price,
		MinRating: minRating, Sort: sortKey,
	})
	start, safePage, safeLimit := paginate(len(all), page, limit)
	end := start + safeLimit
	if end > len(all) {
		end = len(all)
	}
	items := make([]courseItem, 0, end-start)
	for _, c := range all[start:end] {
		items = append(items, toCourseItem(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": len(all), "page": safePage, "limit": safeLimit,
	})
}

// ---- Публичный вид урока: без правильных ответов и скрытых тестов ----

type quizQuestionPublic struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Options     []string `json:"options"`
	Explanation string   `json:"explanation,omitempty"`
}

type quizPublic struct {
	Questions []quizQuestionPublic `json:"questions"`
}

type codeTaskPublic struct {
	Language string          `json:"language"`
	Template string          `json:"template,omitempty"`
	Tests    []repo.CodeTest `json:"tests"`
}

type lessonView struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Kind     repo.LessonKind `json:"kind"`
	Position int             `json:"position"`
	BodyMD   string          `json:"bodyMd,omitempty"`
	VideoURL string          `json:"videoUrl,omitempty"`
	LiveURL  string          `json:"liveUrl,omitempty"`
	StartsAt *time.Time      `json:"startsAt,omitempty"`
	Quiz     any             `json:"quiz,omitempty"`
	CodeTask any             `json:"codeTask,omitempty"`
}

type moduleView struct {
	ID       string       `json:"id"`
	Title    string       `json:"title"`
	Position int          `json:"position"`
	Lessons  []lessonView `json:"lessons"`
}

// lessonToView строит вид урока; full=true только для владельца/админа.
func lessonToView(l *repo.Lesson, full bool) lessonView {
	v := lessonView{
		ID: l.ID, Title: l.Title, Kind: l.Kind, Position: l.Position,
		BodyMD: l.BodyMD, VideoURL: l.VideoURL, LiveURL: l.LiveURL, StartsAt: l.StartsAt,
	}
	if l.Quiz != nil {
		if full {
			v.Quiz = l.Quiz
		} else {
			qp := quizPublic{}
			for _, q := range l.Quiz.Questions {
				qp.Questions = append(qp.Questions, quizQuestionPublic{
					ID: q.ID, Text: q.Text, Options: q.Options, Explanation: q.Explanation,
				})
			}
			v.Quiz = qp
		}
	}
	if l.CodeTask != nil {
		if full {
			v.CodeTask = l.CodeTask
		} else {
			cp := codeTaskPublic{Language: l.CodeTask.Language, Template: l.CodeTask.Template, Tests: []repo.CodeTest{}}
			for _, t := range l.CodeTask.Tests {
				if !t.Hidden {
					cp.Tests = append(cp.Tests, t)
				}
			}
			v.CodeTask = cp
		}
	}
	return v
}

// GetCourse — GET /courses/{slug}: страница курса с модулями и уроками.
// Неопубликованные курсы видят только владелец и админ.
func (a *App) GetCourse(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	c, ok := a.Store.GetCourseBySlug(slug)
	if !ok {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	viewer := a.optionalUser(r)
	full := viewer != nil && (viewer.ID == c.TeacherID || viewer.Role == repo.RoleAdmin)
	if c.Status != repo.CoursePublished && !full {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	mods := a.Store.ListModulesByCourse(c.ID)
	mv := make([]moduleView, 0, len(mods))
	for _, m := range mods {
		lessons := a.Store.ListLessonsByModule(m.ID)
		lv := make([]lessonView, 0, len(lessons))
		for _, l := range lessons {
			lv = append(lv, lessonToView(l, full))
		}
		mv = append(mv, moduleView{ID: m.ID, Title: m.Title, Position: m.Position, Lessons: lv})
	}
	writeJSON(w, http.StatusOK, map[string]any{"course": c, "modules": mv})
}
