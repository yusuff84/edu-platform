package repo

// T21 — жизненный цикл курса: глубокая копия курса и перенос урока между
// модулями (in-memory реализация, см. postgres_course_clone.go для SQL).
//
// Дубль копирует всю программу (модули и уроки с новыми id, позиции
// сохраняются) и сбрасывает «репутационную» часть курса: статус → draft,
// комментарий модерации, рейтинг и число отзывов. Записи, платежи, отзывы и
// прогресс не копируются — у нового курса их просто нет.
//
// Правила формирования названия и slug копии живут ТОЛЬКО здесь
// (copyTitle, copySlugCandidate) и переиспользуются Postgres-реализацией,
// чтобы правила не разъехались между хранилищами.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CloneCourse — глубокая копия курса с модулями и уроками (T21).
// Копия создаётся черновиком, slug подбирается по свободному (<slug>-copy-N).
// ErrNotFound — исходного курса нет.
func (s *Store) CloneCourse(courseID string) (*Course, error) {
	return s.CloneCourseAs(courseID, "")
}

// CloneCourseAs — CloneCourse с явным владельцем копии (T21).
// teacherID == "" — владелец копии наследуется от исходного курса.
//
// Отдельный метод нужен хендлеру duplicate: чужой курс может копировать
// админ, а копия по контракту T21 всё равно принадлежит вызывающему, тогда
// как обязательный CloneCourse(courseID) из StoreAPI идентификатора
// пользователя не принимает.
func (s *Store) CloneCourseAs(courseID, teacherID string) (*Course, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src, ok := s.courses[courseID]
	if !ok {
		return nil, ErrNotFound
	}
	now := time.Now().UTC()
	owner := strings.TrimSpace(teacherID)
	if owner == "" {
		owner = src.TeacherID
	}
	c := &Course{
		ID:          uuid.NewString(),
		TeacherID:   owner,
		Slug:        s.freeCopySlugLocked(src.Slug),
		Title:       copyTitle(src.Title),
		Description: src.Description,
		CoverPath:   cloneStringPtr(src.CoverPath),
		PriceCents:  src.PriceCents,
		Currency:    src.Currency,
		Status:      CourseDraft,
		// AvgRating/RatingsCount/ModerationComment — нулевые значения по T21.
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.courses[c.ID] = c
	for _, m := range s.sortedModulesLocked(courseID) {
		nm := &Module{ID: uuid.NewString(), CourseID: c.ID, Title: m.Title, Position: m.Position}
		s.modules[nm.ID] = nm
		for _, l := range s.sortedLessonsLocked(m.ID) {
			nl := cloneLesson(l)
			nl.ModuleID = nm.ID
			s.lessons[nl.ID] = nl
		}
	}
	return c, nil
}

// MoveLessonToModule переносит урок в конец другого модуля (T21).
// ErrNotFound — урока или модуля нет. Проверки «модуль того же курса» и
// «курс в черновике» накладывает handler.
func (s *Store) MoveLessonToModule(lessonID, moduleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.lessons[lessonID]
	if !ok {
		return ErrNotFound
	}
	if _, ok := s.modules[moduleID]; !ok {
		return ErrNotFound
	}
	// Новая позиция — конец целевого модуля (max+1), как CreateLesson.
	pos := 1
	for _, e := range s.lessons {
		if e.ModuleID == moduleID && e.Position >= pos {
			pos = e.Position + 1
		}
	}
	l.ModuleID = moduleID
	l.Position = pos
	return nil
}

// sortedModulesLocked — модули курса по позиции; вызывать под взятым локом
// (ListModulesByCourse берёт RLock — под Lock его звать нельзя).
func (s *Store) sortedModulesLocked(courseID string) []*Module {
	var out []*Module
	for _, m := range s.modules {
		if m.CourseID == courseID {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out
}

// sortedLessonsLocked — уроки модуля по позиции; вызывать под взятым локом.
func (s *Store) sortedLessonsLocked(moduleID string) []*Lesson {
	var out []*Lesson
	for _, l := range s.lessons {
		if l.ModuleID == moduleID {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out
}

// freeCopySlugLocked подбирает свободный slug копии: <slug>-copy-2, -copy-3…
// (нумерация с 2). Проверка занятости — та же, что в SlugTaken (без учёта
// регистра), но под уже взятым локом. Вызывать под взятым локом.
func (s *Store) freeCopySlugLocked(srcSlug string) string {
	for n := 2; n <= 10000; n++ {
		cand := copySlugCandidate(srcSlug, n)
		taken := false
		for _, c := range s.courses {
			if strings.EqualFold(c.Slug, cand) {
				taken = true
				break
			}
		}
		if !taken {
			return cand
		}
	}
	// Практически недостижимо (10 000 копий): uuid-основа гарантирует
	// свободный slug настолько же, насколько уникален uuid.
	return copySlugCandidate(uuid.NewString(), 2)
}

// cloneLesson — глубокая копия урока для дубля курса (T21): новый id и
// created_at, но quiz/codeTask/startsAt копируются значениями — правка копии
// не должна мутировать исходный урок.
func cloneLesson(src *Lesson) *Lesson {
	return &Lesson{
		ID:        uuid.NewString(),
		ModuleID:  src.ModuleID, // CloneCourseAs заменит на модуль копии
		Title:     src.Title,
		Kind:      src.Kind,
		Position:  src.Position,
		BodyMD:    src.BodyMD,
		VideoURL:  src.VideoURL,
		LiveURL:   src.LiveURL,
		StartsAt:  cloneTimePtr(src.StartsAt),
		Quiz:      cloneQuiz(src.Quiz),
		CodeTask:  cloneCodeTask(src.CodeTask),
		CreatedAt: time.Now().UTC(),
	}
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}

// cloneStringPtr копия *string по значению: правка обложки копии курса не
// должна алиасить указатель исходного (и наоборот).
func cloneStringPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	return &v
}

func cloneQuiz(q *QuizData) *QuizData {
	if q == nil {
		return nil
	}
	cp := &QuizData{Questions: make([]QuizQuestion, len(q.Questions))}
	copy(cp.Questions, q.Questions)
	for i := range cp.Questions {
		if q.Questions[i].Options != nil {
			cp.Questions[i].Options = append([]string(nil), q.Questions[i].Options...)
		}
	}
	return cp
}

func cloneCodeTask(ct *CodeTask) *CodeTask {
	if ct == nil {
		return nil
	}
	cp := &CodeTask{Language: ct.Language, Template: ct.Template}
	if ct.Tests != nil {
		cp.Tests = append([]CodeTest(nil), ct.Tests...)
	}
	return cp
}

// copyTitle — название копии курса (T21): единственное место правила.
func copyTitle(title string) string { return title + " (копия)" }

// copySlugCandidate строит очередной вариант slug копии: <base>-copy-N.
// Хвост обрезается, чтобы уложиться в лимит 120 символов из slugRe
// (^[a-z0-9-]{3,120}$ в handler/teacher.go).
func copySlugCandidate(base string, n int) string {
	suffix := fmt.Sprintf("-copy-%d", n)
	if len(base)+len(suffix) > 120 {
		base = strings.TrimRight(base[:120-len(suffix)], "-")
	}
	if strings.TrimSpace(base) == "" {
		base = "course"
	}
	return base + suffix
}
