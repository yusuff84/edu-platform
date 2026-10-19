package repo

// T67 — сертификаты о завершении курса: in-memory реализация и общий
// генератор номера. Postgres-зеркало — postgres_certificates.go; контракт
// методов — StoreAPI в store_iface.go, модели — models.go.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// certificateNumberAttempts — сколько раз пробуем сгенерировать номер при
// коллизии (UNIQUE number). 8 hex-символов дают 2^32 вариантов, поэтому
// практически всегда хватает первой попытки.
const certificateNumberAttempts = 5

// errCertificateNumber — не удалось подобрать свободный номер за отведённые
// попытки или отказал crypto/rand.
var errCertificateNumber = errors.New("не удалось сгенерировать номер сертификата")

// newCertificateNumber генерирует номер «EDU-<ГГГГ>-<8 hex ВЕРХНИМ регистром>»
// (пример EDU-2026-1A2B3C4D). taken (может быть nil) сообщает, занят ли уже
// сгенерированный номер; при коллизии номер генерируется заново.
func newCertificateNumber(taken func(string) bool) (string, error) {
	for attempt := 0; attempt < certificateNumberAttempts; attempt++ {
		var raw [4]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return "", fmt.Errorf("%w: %v", errCertificateNumber, err)
		}
		number := fmt.Sprintf("EDU-%04d-%s",
			time.Now().UTC().Year(), strings.ToUpper(hex.EncodeToString(raw[:])))
		if taken == nil || !taken(number) {
			return number, nil
		}
	}
	return "", errCertificateNumber
}

// certKey — ключ уникальной пары «ученик + курс».
func certKey(studentID, courseID string) string { return studentID + "|" + courseID }

// IssueCertificate идемпотентно выдаёт сертификат: если сертификат за эту
// пару (ученик, курс) уже есть — возвращается он же. ErrNotFound — ученика
// или курса нет (в Postgres то же поведение даёт FK-нарушение).
func (s *Store) IssueCertificate(studentID, courseID string) (*Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.certIdx[certKey(studentID, courseID)]; ok {
		return s.certificates[id], nil
	}
	if _, ok := s.users[studentID]; !ok {
		return nil, ErrNotFound
	}
	if _, ok := s.courses[courseID]; !ok {
		return nil, ErrNotFound
	}
	number, err := newCertificateNumber(func(n string) bool {
		_, exists := s.certByNumber[n]
		return exists
	})
	if err != nil {
		return nil, err
	}
	c := &Certificate{
		ID:       uuid.NewString(),
		UserID:   studentID,
		CourseID: courseID,
		Number:   number,
		IssuedAt: time.Now().UTC(),
	}
	s.certificates[c.ID] = c
	s.certIdx[certKey(c.UserID, c.CourseID)] = c.ID
	s.certByNumber[c.Number] = c.ID
	return c, nil
}

// GetCertificateByNumber — публичная карточка сертификата по номеру.
func (s *Store) GetCertificateByNumber(number string) (*CertificateDetails, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.certByNumber[number]
	if !ok {
		return nil, false
	}
	c := s.certificates[id]
	if c == nil {
		return nil, false
	}
	course, ok := s.courses[c.CourseID]
	if !ok {
		return nil, false
	}
	student, ok := s.users[c.UserID]
	if !ok {
		return nil, false
	}
	return &CertificateDetails{
		Number:      c.Number,
		IssuedAt:    c.IssuedAt,
		StudentName: student.DisplayName,
		CourseTitle: course.Title,
		CourseSlug:  course.Slug,
	}, true
}

// ListCertificatesByStudent — сертификаты ученика, новые первыми; при равной
// issued_at порядок стабилизируется номером по убыванию. Никогда не nil.
func (s *Store) ListCertificatesByStudent(studentID string) []*CertificateListItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]*CertificateListItem, 0)
	for _, c := range s.certificates {
		if c.UserID != studentID {
			continue
		}
		course, ok := s.courses[c.CourseID]
		if !ok {
			continue
		}
		items = append(items, &CertificateListItem{
			Number:      c.Number,
			IssuedAt:    c.IssuedAt,
			CourseID:    c.CourseID,
			CourseTitle: course.Title,
			CourseSlug:  course.Slug,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].IssuedAt.Equal(items[j].IssuedAt) {
			return items[i].IssuedAt.After(items[j].IssuedAt)
		}
		return items[i].Number > items[j].Number
	})
	return items
}

// dropCertificateLocked удаляет сертификат и оба его индекса; вызывается
// каскадом при удалении курса/пользователя (зеркало ON DELETE CASCADE 010).
func (s *Store) dropCertificateLocked(id string) {
	c, ok := s.certificates[id]
	if !ok {
		return
	}
	delete(s.certificates, id)
	delete(s.certIdx, certKey(c.UserID, c.CourseID))
	delete(s.certByNumber, c.Number)
}

// dropCertificatesByCourseLocked — каскад удаления курса.
func (s *Store) dropCertificatesByCourseLocked(courseID string) {
	for id, c := range s.certificates {
		if c.CourseID == courseID {
			s.dropCertificateLocked(id)
		}
	}
}

// dropCertificatesByUserLocked — каскад удаления пользователя.
func (s *Store) dropCertificatesByUserLocked(userID string) {
	for id, c := range s.certificates {
		if c.UserID == userID {
			s.dropCertificateLocked(id)
		}
	}
}
