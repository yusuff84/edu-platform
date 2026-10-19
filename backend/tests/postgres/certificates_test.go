package postgres_test

// T67 — parity-тесты Postgres-реализации сертификатов: идемпотентная выдача,
// номер EDU-<ГГГГ>-<8 hex>, публичная карточка по номеру, список ученика
// (новые первыми), каскадное удаление вместе с курсом/пользователем.
// Схему создаёт 010_certificates.sql, её прогоняет testStore вместе с
// остальными миграциями.

import (
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// certNumberRe — формат номера из контракта T67.
var certNumberRe = regexp.MustCompile(`^EDU-\d{4}-[0-9A-F]{8}$`)

// TestPostgresCertificateIssueAndLookup — выдача, повторная выдача (та же
// запись), чтение по номеру с displayName студента и список ученика.
func TestPostgresCertificateIssueAndLookup(t *testing.T) {
	s := testStore(t)
	_, course, _ := seedCourseWithLessons(t, s, 1)
	student := createUser(t, s, "cert-student", repo.RoleStudent)
	enroll(t, s, student.ID, course.ID)
	if !s.SetUserDisplayName(student.ID, "Иван Петров") {
		t.Fatalf("SetUserDisplayName(%s) = false", student.ID)
	}

	cert, err := s.IssueCertificate(student.ID, course.ID)
	if err != nil {
		t.Fatalf("IssueCertificate: %v", err)
	}
	if cert.ID == "" || cert.UserID != student.ID || cert.CourseID != course.ID {
		t.Fatalf("сертификат заполнен неверно: %+v", cert)
	}
	if !certNumberRe.MatchString(cert.Number) {
		t.Fatalf("номер %q не соответствует EDU-<ГГГГ>-<8 hex>", cert.Number)
	}
	if cert.IssuedAt.IsZero() {
		t.Fatalf("issued_at не заполнен: %+v", cert)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM certificates WHERE user_id=$1::uuid AND course_id=$2::uuid`,
		student.ID, course.ID); n != 1 {
		t.Fatalf("строк сертификата после выдачи: %d, ожидали 1", n)
	}

	// Идемпотентность: повторная выдача возвращает ту же запись.
	again, err := s.IssueCertificate(student.ID, course.ID)
	if err != nil {
		t.Fatalf("повторный IssueCertificate: %v", err)
	}
	if again.ID != cert.ID || again.Number != cert.Number {
		t.Fatalf("повторная выдача создала другой сертификат: %+v → %+v", cert, again)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM certificates WHERE user_id=$1::uuid AND course_id=$2::uuid`,
		student.ID, course.ID); n != 1 {
		t.Fatalf("строк после повторной выдачи: %d, ожидали 1", n)
	}

	// Публичная карточка по номеру.
	got, ok := s.GetCertificateByNumber(cert.Number)
	if !ok {
		t.Fatalf("GetCertificateByNumber(%s) = false", cert.Number)
	}
	if got.Number != cert.Number || !sameMicro(got.IssuedAt, cert.IssuedAt) {
		t.Fatalf("карточка не совпала с сертификатом: got=%+v cert=%+v", got, cert)
	}
	if got.StudentName != "Иван Петров" {
		t.Fatalf("studentName=%q, ожидали displayName", got.StudentName)
	}
	if got.CourseTitle != course.Title || got.CourseSlug != course.Slug {
		t.Fatalf("данные курса в карточке: got=%+v course=%+v", got, course)
	}
	if _, ok := s.GetCertificateByNumber("EDU-2026-00000000"); ok {
		t.Fatal("GetCertificateByNumber для несуществующего номера вернул true")
	}

	// Список ученика.
	items := s.ListCertificatesByStudent(student.ID)
	if len(items) != 1 || items[0].Number != cert.Number ||
		items[0].CourseID != course.ID || items[0].CourseTitle != course.Title ||
		items[0].CourseSlug != course.Slug {
		t.Fatalf("ListCertificatesByStudent: %+v", items)
	}
	if empty := s.ListCertificatesByStudent(uuid.NewString()); empty == nil || len(empty) != 0 {
		t.Fatalf("пустой список сертификатов должен быть [] (не nil): %#v", empty)
	}

	// Нет ученика/курса — ErrNotFound (паритет с in-memory Store).
	if _, err := s.IssueCertificate(student.ID, uuid.NewString()); err != repo.ErrNotFound {
		t.Fatalf("IssueCertificate с несуществующим курсом: err=%v, ожидали ErrNotFound", err)
	}
	if _, err := s.IssueCertificate(uuid.NewString(), course.ID); err != repo.ErrNotFound {
		t.Fatalf("IssueCertificate с несуществующим учеником: err=%v, ожидали ErrNotFound", err)
	}
}

// TestPostgresCertificatesOrderAndCascade — два сертификата: новые первыми;
// удаление курса и ученика уносит сертификаты каскадом.
func TestPostgresCertificatesOrderAndCascade(t *testing.T) {
	s := testStore(t)
	teacher, first, _ := seedCourseWithLessons(t, s, 1)
	student := createUser(t, s, "cert-order", repo.RoleStudent)
	enroll(t, s, student.ID, first.ID)

	// Второй курс того же преподавателя.
	second := createCourse(t, s, teacher.ID, "cert-second", 0)
	m := s.CreateModule(second.ID, "Модуль")
	s.CreateLesson(&repo.Lesson{ModuleID: m.ID, Title: "Урок", Kind: repo.LessonVideoText, BodyMD: "текст"})
	enroll(t, s, student.ID, second.ID)

	firstCert, err := s.IssueCertificate(student.ID, first.ID)
	if err != nil {
		t.Fatalf("IssueCertificate(first): %v", err)
	}
	// issued_at в Postgres — микросекунды; разносим выдачи, чтобы порядок
	// «новые первыми» был детерминированным.
	time.Sleep(2 * time.Millisecond)
	secondCert, err := s.IssueCertificate(student.ID, second.ID)
	if err != nil {
		t.Fatalf("IssueCertificate(second): %v", err)
	}

	items := s.ListCertificatesByStudent(student.ID)
	if len(items) != 2 {
		t.Fatalf("сертификатов: %d, ожидали 2", len(items))
	}
	if items[0].Number != secondCert.Number || items[1].Number != firstCert.Number {
		t.Fatalf("порядок сертификатов (новые первыми) нарушен: %+v", items)
	}

	// Удаление курса: FK ON DELETE CASCADE уносит сертификат.
	if err := s.DeleteCourse(second.ID); err != nil {
		t.Fatalf("DeleteCourse(second): %v", err)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM certificates WHERE course_id=$1::uuid`, second.ID); n != 0 {
		t.Fatalf("сертификатов удалённого курса: %d, ожидали 0", n)
	}
	if _, ok := s.GetCertificateByNumber(secondCert.Number); ok {
		t.Fatal("сертификат удалённого курса всё ещё читается по номеру")
	}

	// Удаление ученика: сертификаты уходят каскадом, остальных не задевает.
	if err := s.DeleteUser(student.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM certificates WHERE user_id=$1::uuid`, student.ID); n != 0 {
		t.Fatalf("сертификатов удалённого ученика: %d, ожидали 0", n)
	}
}
