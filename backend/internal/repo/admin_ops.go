package repo

// Расширенные операции администратора (in-memory). Postgres — postgres_admin_ops.go.

import (
	"sort"
	"strings"
	"time"
)

// AdminCertificateItem — строка списка сертификатов в админке.
type AdminCertificateItem struct {
	Number      string    `json:"number"`
	IssuedAt    time.Time `json:"issuedAt"`
	UserID      string    `json:"userId"`
	Email       string    `json:"email"`
	StudentName string    `json:"studentName"`
	CourseID    string    `json:"courseId"`
	CourseTitle string    `json:"courseTitle"`
}

// SiteSettingKeys — редактируемые из админки тексты и контакты сайта.
var SiteSettingKeys = []string{
	"promo_text", "promo_link_text", "promo_link_url",
	"contact_phone", "contact_email", "contact_city", "footer_tagline",
}

// SetUserEmail меняет email пользователя. ErrNotFound — нет пользователя,
// ErrAlreadyExists — email занят другим пользователем.
func (s *Store) SetUserEmail(id, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return ErrNotFound
	}
	key := normEmail(email)
	if other, taken := s.usersByEmail[key]; taken && other.ID != id {
		return ErrAlreadyExists
	}
	delete(s.usersByEmail, normEmail(u.Email))
	u.Email = key
	u.EmailVerifiedAt = nil
	s.usersByEmail[key] = u
	// Письма, выданные на прежний адрес, больше не действуют (как в Postgres).
	for k, token := range s.accountTokens {
		if token.UserID == id {
			delete(s.accountTokens, k)
		}
	}
	return nil
}

// DeleteLessonProgress удаляет прогресс ученика по указанным урокам; возвращает число удалённых записей.
func (s *Store) DeleteLessonProgress(studentID string, lessonIDs []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, id := range lessonIDs {
		key := studentID + "|" + id
		if _, ok := s.progress[key]; ok {
			delete(s.progress, key)
			n++
		}
	}
	return n
}

// ListAllCertificates — сертификаты всех учеников, новые первыми; q — поиск
// по email, имени, номеру или названию курса. limit<=0 — без ограничения.
func (s *Store) ListAllCertificates(q string, limit int) []*AdminCertificateItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q = strings.ToLower(strings.TrimSpace(q))
	out := make([]*AdminCertificateItem, 0, len(s.certificates))
	for _, c := range s.certificates {
		it := &AdminCertificateItem{Number: c.Number, IssuedAt: c.IssuedAt, UserID: c.UserID, CourseID: c.CourseID}
		if u, ok := s.users[c.UserID]; ok {
			it.Email, it.StudentName = u.Email, u.DisplayName
		}
		if co, ok := s.courses[c.CourseID]; ok {
			it.CourseTitle = co.Title
		}
		if q != "" && !strings.Contains(strings.ToLower(it.Email+" "+it.StudentName+" "+it.Number+" "+it.CourseTitle), q) {
			continue
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// DeleteCertificate отзывает (удаляет) сертификат по номеру; false — нет такого.
func (s *Store) DeleteCertificate(number string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.certByNumber[number]
	if !ok {
		return false
	}
	c := s.certificates[id]
	delete(s.certByNumber, number)
	delete(s.certificates, id)
	if c != nil {
		delete(s.certIdx, c.UserID+"|"+c.CourseID)
	}
	return true
}

// SetCourseTeacher передаёт курс другому преподавателю; false — нет курса.
func (s *Store) SetCourseTeacher(courseID, teacherID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.courses[courseID]
	if !ok {
		return false
	}
	c.TeacherID = teacherID
	c.UpdatedAt = time.Now().UTC()
	return true
}

// GetSiteSettings — редактируемые тексты сайта (только заданные ключи).
func (s *Store) GetSiteSettings() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.siteSettings))
	for k, v := range s.siteSettings {
		out[k] = v
	}
	return out
}

// SetSiteSettings сохраняет тексты сайта; пустое значение удаляет ключ (вернётся значение по умолчанию).
func (s *Store) SetSiteSettings(kv map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.siteSettings == nil {
		s.siteSettings = make(map[string]string)
	}
	for k, v := range kv {
		if v == "" {
			delete(s.siteSettings, k)
		} else {
			s.siteSettings[k] = v
		}
	}
	return nil
}
