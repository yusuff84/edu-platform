package repo

// T55 — SQL-реализация деталей курса для админа (см. store_iface.go:
// описание AdminCourseDetailData и AdminCourseDetailStats, и store.go —
// in-memory-вариант).
//
// Два запроса + склейка в Go (приёмы переиспользованы из
// postgres_admin_console.go: скалярные подзапросы-агрегаты вместо выгрузки
// таблиц, LEFT JOIN users, COUNT/SUM ::int, ROUND(AVG) до 1 знака):
//  1) курс (courseColsJoined — тот же порядок колонок, что в списке T48)
//     + email автора + счётчики программы/записей + агрегаты платежей;
//  2) программа — учебные модули курса с уроками (LEFT JOIN), порядок
//     задан в SQL (m.position, l.position) и продублирован в Go.
//
// Ошибка скана (в том числе не-UUID id в пути) трактована как «курса нет» —
// так же, как в GetCourseByID (postgres_store.go: WHERE id=$1::uuid).

import (
	"log"
	"sort"
	"strings"
	"time"
)

// AdminCourseDetail собирает карточку курса админа (T55): запрос 1 — курс,
// автор и вся статистика скалярными подзапросами (одна строка); запрос 2 —
// программа. Рейтинг и число отзывов берутся из полей курса — это тот же
// источник, что у публичной карточки (RecalcCourseRating пересчитывает их
// при каждом изменении отзывов), поэтому AvgRating/ReviewsCount в stats и
// в course совпадают по определению.
func (s *PostgresStore) AdminCourseDetail(courseID string) (*AdminCourseDetailData, bool) {
	ctx, cancel := s.ctx()
	defer cancel()

	d := &AdminCourseDetailData{
		Modules: []*Module{},
		Lessons: map[string][]*Lesson{},
	}

	// Запрос 1: колонки courseColsJoined (та же форма, что у scanCourse
	// postgres_store.go) сканируются вручную, дальше идут дополнительные.
	var teacherID, category, currency, status, comment string
	var cover *string
	var avg float64
	var ratings int
	var created, updated time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT `+courseColsJoined+`,
		       COALESCE(u.email, ''),
		       (SELECT COUNT(*)::int FROM modules m WHERE m.course_id = c.id),
		       (SELECT COUNT(*)::int FROM lessons l
		          JOIN modules m ON m.id = l.module_id
		         WHERE m.course_id = c.id),
		       (SELECT COUNT(*)::int FROM enrollments e WHERE e.course_id = c.id),
		       (SELECT COUNT(*)::int FROM enrollments e
		         WHERE e.course_id = c.id AND e.status = 'active'),
		       (SELECT COUNT(*)::int FROM payments p
		         WHERE p.course_id = c.id AND p.status = 'succeeded'),
		       (SELECT COALESCE(SUM(p.amount_cents), 0)::int FROM payments p
		         WHERE p.course_id = c.id AND p.status = 'succeeded')
		FROM courses c
		LEFT JOIN users u ON u.id = c.teacher_id
		WHERE c.id = $1::uuid`, courseID).Scan(
		&d.Course.ID, &teacherID, &d.Course.Slug, &d.Course.Title, &d.Course.Description,
		&category, &cover, &d.Course.PriceCents, &currency, &status, &avg, &ratings, &comment,
		&created, &updated,
		&d.TeacherEmail,
		&d.Counts.Modules, &d.Counts.Lessons,
		&d.Stats.StudentsCount, &d.Stats.ActiveCount,
		&d.Stats.PaymentsCount, &d.Stats.PaidTotalCents,
	)
	if err != nil {
		// Не-UUID путь и «курса нет» — одна картина: pgx вернёт ошибку
		// разбора/пустую строку, наружу это единый 404 хендлера.
		log.Printf("postgres AdminCourseDetail course: %v", err)
		return nil, false
	}

	// Нормализация полей курса — зеркало scanCourse (postgres_store.go).
	d.Course.TeacherID = teacherID
	d.Course.Category = category
	if strings.TrimSpace(d.Course.Category) == "" {
		d.Course.Category = "other"
	}
	d.Course.CoverPath = cover
	d.Course.Currency = strings.TrimSpace(currency)
	if d.Course.Currency == "" {
		d.Course.Currency = "RUB"
	}
	d.Course.Status = CourseStatus(status)
	d.Course.AvgRating = avg
	d.Stats.ReviewsCount = ratings
	d.Stats.AvgRating = avg
	d.Course.ModerationComment = comment
	d.Course.CreatedAt = created
	d.Course.UpdatedAt = updated

	// Запрос 2: программа курса. LEFT JOIN: модуль без уроков даёт одну
	// строку с NULL-уроком (модуль попадает в выдачу без lessons).
	rows, err := s.pool.Query(ctx, `
		SELECT m.id::text, m.title, m.position,
		       l.id::text, l.title, l.kind, l.position
		FROM modules m
		LEFT JOIN lessons l ON l.module_id = m.id
		WHERE m.course_id = $1::uuid
		ORDER BY m.position, l.position`, courseID)
	if err != nil {
		log.Printf("postgres AdminCourseDetail program: %v", err)
		return nil, false
	}
	defer rows.Close()

	modByID := make(map[string]*Module)
	for rows.Next() {
		var modID, modTitle string
		var modPos int
		// Урок может отсутствовать (LEFT JOIN): все пять полей — указатели,
		// нулевой lID — «урока в этой строке нет».
		var lID, lTitle, lKind *string
		var lPos *int
		if err := rows.Scan(&modID, &modTitle, &modPos, &lID, &lTitle, &lKind, &lPos); err != nil {
			log.Printf("postgres AdminCourseDetail scan: %v", err)
			return nil, false
		}
		m, ok := modByID[modID]
		if !ok {
			// courseID не сканируем заново: он равен аргументу запроса.
			m = &Module{ID: modID, CourseID: courseID, Title: modTitle, Position: modPos}
			modByID[modID] = m
			d.Modules = append(d.Modules, m)
		}
		if lID == nil {
			continue
		}
		d.Lessons[modID] = append(d.Lessons[modID], &Lesson{
			ID: *lID, ModuleID: modID, Title: *lTitle,
			Kind: LessonKind(*lKind), Position: *lPos,
		})
	}
	if err := rows.Err(); err != nil {
		log.Printf("postgres AdminCourseDetail rows: %v", err)
		return nil, false
	}

	// Страховка порядка: SQL уже сортировал (m.position, l.position),
	// но при равных позициях строки могли перемежаться — выравниваем в Go.
	sort.Slice(d.Modules, func(i, j int) bool {
		return d.Modules[i].Position < d.Modules[j].Position
	})
	for _, ls := range d.Lessons {
		sort.Slice(ls, func(i, j int) bool { return ls[i].Position < ls[j].Position })
	}
	return d, true
}
