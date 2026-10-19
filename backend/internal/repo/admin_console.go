package repo

// T25 — хранилище «супер-админской» сводки: сводные агрегаты (AdminOverview),
// список курсов всех статусов, удаление отзывов с пересчётом рейтинга, все
// записи на курсы и счётчики содержимого курсов.
//
// Файл содержит in-memory-реализации методов на *Store и объявления типов
// (CourseFilter, EnrollmentFilter, EnrollmentRow, AdminTotals) — модели
// (models.go) не трогаются. SQL-реализации — в postgres_admin_console.go.

import (
	"math"
	"sort"
	"strings"
	"time"
)

// CourseFilter — параметры списка курсов для админки (T25/T48): статус,
// поиск по названию, сортировка, фильтр по преподавателю и периоду создания,
// пагинация. Пагинация 1-based: Limit по умолчанию 20, максимум 100; Page
// по умолчанию 1. Status "" — любой статус, Q "" — без поиска,
// TeacherEmail "" — без фильтра; фильтры комбинируются через AND.
//
// T48: Sort — createdAt|updatedAt|title|price|students ("" = createdAt),
// Dir — asc|desc ("" = desc); при равенстве первичного ключа порядок —
// createdAt по убыванию. From/To — границы по дате создания включительно
// (нулевое значение — границы нет; To — «до конца суток»), как в
// PaymentFilter (T31).
type CourseFilter struct {
	Page, Limit  int
	Status       CourseStatus // "" — любой
	Q            string       // подстрока по title, регистронезависимо
	Sort         string       // createdAt|updatedAt|title|price|students ("" = createdAt)
	Dir          string       // asc|desc ("" = desc)
	TeacherEmail string       // подстрока по email преподавателя, регистронезависимо
	From, To     time.Time    // границы по createdAt включительно; нулевое — без границы
}

// CatalogFilter — параметры публичного каталога GET /courses (T65).
// Пустые значения означают «без фильтра»; Sort "" — popular. Значения
// category/price/minRating/sort валидирует handler (400 на неизвестные),
// хранилище получает только допустимые.
type CatalogFilter struct {
	Q         string  // подстрока по title/description, регистронезависимо
	Category  string  // слаг категории; "" — любая
	Price     string  // free|paid|"" ("" — любая цена)
	MinRating float64 // 0 — без фильтра; иначе минимальный avgRating
	Sort      string  // popular|new|rating|price_asc|price_desc ("" = popular)
}

// EnrollmentFilter — параметры списка записей на курсы (T25/T48).
// CourseID/StringID "" означают «любой»; Q — подстрока без учёта регистра
// по email ИЛИ displayName ученика ("" — без фильтра); Status "" — любой,
// иначе точное совпадение статуса; фильтры комбинируются через AND.
// Пагинация 1-based: Limit по умолчанию 20, максимум 100.
type EnrollmentFilter struct {
	Page, Limit int
	CourseID    string           // "" — любой
	StudentID   string           // "" — любые
	Q           string           // подстрока по email/displayName ученика (T48)
	Status      EnrollmentStatus // "" — любой (T48)
}

// EnrollmentRow — строка списка записей на курсы: запись + данные ученика
// и курса + прогресс ученика по курсу (T25). Completed/TotalLessons —
// пройденные и всего уроков курса; Percent считает handler.
type EnrollmentRow struct {
	Enrollment
	StudentEmail string
	StudentName  string
	CourseTitle  string
	Completed    int
	TotalLessons int
}

// AdminTotals — все агрегаты сводки супер-админа одним вызовом (T25).
//
// Денежные суммы (PaidCents, PlatformFeeCents, TeacherCents, PaidLast30Cents)
// и PaymentsCount считаются ТОЛЬКО по платежам в статусе succeeded: сводка
// показывает фактическую выручку, а не выставленные счёта. Комиссия берётся
// из полей платежа (platform_fee_cents/teacher_amount_cents), а не
// пересчитывается от текущей настройки.
//
// UsersBlocked — число пользователей со статусом blocked: блокировка появилась
// в T24 (поле Status у пользователя, миграция 006_user_status.sql). Счётчик
// считается и в памяти, и в Postgres; до T34 в обеих реализациях он всегда
// оставался нулевым.
type AdminTotals struct {
	UsersTotal, UsersStudents, UsersTeachers, UsersAdmins, UsersBlocked int
	UsersNew7                                                          int
	CoursesTotal, CoursesDraft, CoursesReview, CoursesPublished        int
	CoursesArchived                                                    int
	PaymentsCount, PaidCents, PlatformFeeCents, TeacherCents           int
	PaidLast30Cents                                                    int
	EnrollmentsTotal, EnrollmentsActive                                int
	ReviewsTotal                                                       int
	AvgRating                                                          float64
}

// clampPageLimit приводит пагинацию к безопасным границам 1-based
// (Limit 1..100, по умолчанию 20) — та же семантика, что в
// ListPaymentsJournal (store.go) и parseJournalPagination (handler).
func clampPageLimit(page, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit
}

// normalizeCourseSort сводит Sort к известному ключу: неизвестное или пустое
// значение — createdAt (дефолт контракта P6). Хендлер валидирует параметр и
// отвечает 400 на мусор; нормализация здесь — страховка для прямых вызовов
// хранилища.
func normalizeCourseSort(s string) string {
	switch s {
	case "updatedAt", "title", "price", "students":
		return s
	default:
		return "createdAt"
	}
}

// normalizeCourseDir сводит Dir к asc|desc: всё, кроме asc, — desc (дефолт).
func normalizeCourseDir(d string) string {
	if d == "asc" {
		return "asc"
	}
	return "desc"
}

// cmpTime сравнивает моменты времени трёхзначным результатом: -1 — a раньше
// b, 0 — равны, 1 — позже.
func cmpTime(a, b time.Time) int {
	switch {
	case a.Before(b):
		return -1
	case a.After(b):
		return 1
	}
	return 0
}

// courseSortCmp сравнивает курсы по первичному ключу сортировки (T48):
// <0 — a раньше b при возрастании, 0 — значения равны. counts — снимок
// CourseContentCounts для ключа students (число активных учеников).
func courseSortCmp(a, b *Course, sortKey string, counts map[string]ContentCounts) int {
	switch sortKey {
	case "updatedAt":
		return cmpTime(a.UpdatedAt, b.UpdatedAt)
	case "title":
		// Без учёта регистра — контракт P6.
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	case "price":
		switch {
		case a.PriceCents < b.PriceCents:
			return -1
		case a.PriceCents > b.PriceCents:
			return 1
		}
		return 0
	case "students":
		as, bs := counts[a.ID].Students, counts[b.ID].Students
		switch {
		case as < bs:
			return -1
		case as > bs:
			return 1
		}
		return 0
	default: // createdAt
		return cmpTime(a.CreatedAt, b.CreatedAt)
	}
}

// AdminOverview собирает все агрегаты сводки одним проходом по картам
// (in-memory; в Postgres — SQL-агрегатами, см. postgres_admin_console.go).
func (s *Store) AdminOverview() AdminTotals {
	now := time.Now().UTC()
	cutoff7 := now.AddDate(0, 0, -7)
	cutoff30 := now.AddDate(0, 0, -30)

	var t AdminTotals
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, u := range s.users {
		t.UsersTotal++
		switch u.Role {
		case RoleStudent:
			t.UsersStudents++
		case RoleTeacher:
			t.UsersTeachers++
		case RoleAdmin:
			t.UsersAdmins++
		}
		if !u.CreatedAt.Before(cutoff7) {
			t.UsersNew7++
		}
		if u.Status == UserBlocked {
			t.UsersBlocked++
		}
	}

	for _, c := range s.courses {
		t.CoursesTotal++
		switch c.Status {
		case CourseDraft:
			t.CoursesDraft++
		case CourseReview:
			t.CoursesReview++
		case CoursePublished:
			t.CoursesPublished++
		case CourseArchived:
			t.CoursesArchived++
		}
	}

	for _, p := range s.payments {
		if p.Status != PaymentSucceeded {
			continue
		}
		t.PaymentsCount++
		t.PaidCents += p.AmountCents
		t.PlatformFeeCents += p.PlatformFeeCents
		t.TeacherCents += p.TeacherAmountCents
		if !p.CreatedAt.Before(cutoff30) {
			t.PaidLast30Cents += p.AmountCents
		}
	}

	for _, e := range s.enrollments {
		t.EnrollmentsTotal++
		if e.Status == EnrollmentActive {
			t.EnrollmentsActive++
		}
	}

	sum := 0
	for _, r := range s.reviews {
		t.ReviewsTotal++
		sum += r.Rating
	}
	if t.ReviewsTotal > 0 {
		// Среднее до 1 знака — как RecalcCourseRating, чтобы avgRating
		// сводки и карточек курсов совпадали.
		t.AvgRating = math.Round(float64(sum)/float64(t.ReviewsTotal)*10) / 10
	}
	return t
}

// ListAllCourses возвращает курсы всех статусов для админки (T25/T48),
// total — до пагинации. Фильтры комбинируются через AND; сортировка — по
// Sort/Dir, вторичный ключ — createdAt по убыванию. Курсы копируются:
// наружу уходит снимок, а не живой указатель на карту хранилища.
//
// T48: счётчики учеников берутся снимком CourseContentCounts ДО основного
// RLock (вызов под локом взял бы RLock повторно), email преподавателя и
// границы периода проверяются прямо по картам под тем же локом — по той же
// причине (GetUserByID тоже взял бы лок повторно).
func (s *Store) ListAllCourses(f CourseFilter) ([]*Course, int) {
	page, limit := clampPageLimit(f.Page, f.Limit)
	q := strings.ToLower(strings.TrimSpace(f.Q))
	emailQ := strings.ToLower(strings.TrimSpace(f.TeacherEmail))
	sortKey := normalizeCourseSort(f.Sort)
	asc := normalizeCourseDir(f.Dir) == "asc"
	studentCounts := s.CourseContentCounts()

	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*Course, 0, len(s.courses))
	for _, c := range s.courses {
		if f.Status != "" && c.Status != f.Status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(c.Title), q) {
			continue
		}
		// T48: подстрока по email преподавателя без учёта регистра;
		// курс без найденного автора под фильтр не попадает.
		if emailQ != "" {
			u, ok := s.users[c.TeacherID]
			if !ok || !strings.Contains(strings.ToLower(u.Email), emailQ) {
				continue
			}
		}
		// T48: период по createdAt включительно; To — «до конца суток»
		// (та же семантика, что у ListPaymentsJournal).
		if !f.From.IsZero() && c.CreatedAt.Before(f.From) {
			continue
		}
		if !f.To.IsZero() && !c.CreatedAt.Before(f.To.AddDate(0, 0, 1)) {
			continue
		}
		matched = append(matched, c)
	}
	sort.Slice(matched, func(i, j int) bool {
		cmp := courseSortCmp(matched[i], matched[j], sortKey, studentCounts)
		if cmp == 0 {
			// Вторичный ключ — createdAt по убыванию (контракт P6).
			return matched[i].CreatedAt.After(matched[j].CreatedAt)
		}
		if asc {
			return cmp < 0
		}
		return cmp > 0
	})

	total := len(matched)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	items := make([]*Course, 0, end-start)
	for _, c := range matched[start:end] {
		snapshot := *c
		items = append(items, &snapshot)
	}
	return items, total
}

// DeleteReview удаляет отзыв и пересчитывает рейтинг курса (T25).
// ErrNotFound — отзыва нет. Пересчёт делается инлайн под тем же локом,
// чтобы не брать RWMutex повторно (это привело бы к самоблокировке).
func (s *Store) DeleteReview(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.reviews[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.reviews, id)
	delete(s.reviewIdx, enrollKey(r.StudentID, r.CourseID))

	sum, n := 0, 0
	for _, rv := range s.reviews {
		if rv.CourseID == r.CourseID {
			sum += rv.Rating
			n++
		}
	}
	if c, ok := s.courses[r.CourseID]; ok {
		c.RatingsCount = n
		if n == 0 {
			c.AvgRating = 0
		} else {
			c.AvgRating = math.Round(float64(sum)/float64(n)*10) / 10
		}
	}
	return nil
}

// ListEnrollmentRows возвращает все записи на курсы с данными ученика и
// курса (T25/T48), новые первыми; total — до пагинации. T48: Q — подстрока
// без учёта регистра по email ИЛИ displayName ученика, Status — точное
// совпадение статуса записи. Completed — число пройденных уроков курса
// (completed_at заполнен), TotalLessons — всего уроков курса. Данные ученика
// и прогресс считаются прямо по картам под одним RLock: вложенные вызовы
// методов Store взяли бы лок повторно.
func (s *Store) ListEnrollmentRows(f EnrollmentFilter) ([]EnrollmentRow, int) {
	page, limit := clampPageLimit(f.Page, f.Limit)
	q := strings.ToLower(strings.TrimSpace(f.Q))

	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*Enrollment, 0, len(s.enrollments))
	for _, e := range s.enrollments {
		if f.CourseID != "" && e.CourseID != f.CourseID {
			continue
		}
		if f.StudentID != "" && e.StudentID != f.StudentID {
			continue
		}
		if f.Status != "" && e.Status != f.Status {
			continue
		}
		// T48: поиск по ученику — подстрока по email ИЛИ displayName.
		// Ученика может не быть в users: тогда оба поля пустые и запись
		// не совпадёт (как в журнале платежей, T31).
		if q != "" {
			email, name := "", ""
			if u, ok := s.users[e.StudentID]; ok {
				email, name = u.Email, u.DisplayName
			}
			if !strings.Contains(strings.ToLower(email), q) &&
				!strings.Contains(strings.ToLower(name), q) {
				continue
			}
		}
		matched = append(matched, e)
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].CreatedAt.After(matched[j].CreatedAt) })

	total := len(matched)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	rows := make([]EnrollmentRow, 0, end-start)
	for _, e := range matched[start:end] {
		row := EnrollmentRow{Enrollment: *e}
		if u, ok := s.users[e.StudentID]; ok {
			row.StudentEmail = u.Email
			row.StudentName = u.DisplayName
		}
		if c, ok := s.courses[e.CourseID]; ok {
			row.CourseTitle = c.Title
		}
		// Уроки курса и прогресс ученика по ним.
		inCourse := make(map[string]bool)
		for _, m := range s.modules {
			if m.CourseID == e.CourseID {
				inCourse[m.ID] = true
			}
		}
		for _, l := range s.lessons {
			if !inCourse[l.ModuleID] {
				continue
			}
			row.TotalLessons++
			if p, ok := s.progress[e.StudentID+"|"+l.ID]; ok && !p.CompletedAt.IsZero() {
				row.Completed++
			}
		}
		rows = append(rows, row)
	}
	return rows, total
}

// CourseContentCounts возвращает по каждому курсу модули, уроки и число
// учеников одним вызовом (T25: без N+1 в списке курсов админки).
// Учеников считаем по АКТИВНЫМ записям: revoked/refunded доступа не имеют,
// поэтому в «учениках курса» им быть не место.
func (s *Store) CourseContentCounts() map[string]ContentCounts {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]ContentCounts, len(s.courses))
	for id := range s.courses {
		out[id] = ContentCounts{}
	}
	for _, m := range s.modules {
		if cc, ok := out[m.CourseID]; ok {
			cc.Modules++
			out[m.CourseID] = cc
		}
	}
	moduleCourse := make(map[string]string, len(s.modules))
	for id, m := range s.modules {
		moduleCourse[id] = m.CourseID
	}
	for _, l := range s.lessons {
		if cid, ok := moduleCourse[l.ModuleID]; ok {
			if cc, ok := out[cid]; ok {
				cc.Lessons++
				out[cid] = cc
			}
		}
	}
	for _, e := range s.enrollments {
		if e.Status != EnrollmentActive {
			continue
		}
		if cc, ok := out[e.CourseID]; ok {
			cc.Students++
			out[e.CourseID] = cc
		}
	}
	return out
}
