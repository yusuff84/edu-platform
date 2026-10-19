// Package storage — сохранение загружаемых файлов (обложки курсов и т.п.).
//
// Статус (T54): пакет используется хендлером обложки (teacher_cover.go);
// каталог uploads/ раздаётся как статика по /uploads/* (см. NewRouter в
// handler/middleware.go). В образе api каталог /uploads принадлежит appuser
// (backend/Dockerfile) — иначе запись падает с 500.
package storage

import (
	"os"
	"path/filepath"
)

// ObjectStore сохраняет загружаемые файлы (обложки курсов и т.п.).
// Текущая реализация — локальный диск под ./uploads.
type ObjectStore interface {
	// SaveLocal сохраняет data под относительным path и возвращает публичный путь.
	SaveLocal(path string, data []byte) (string, error)
}

type LocalStore struct {
	Dir string // default "uploads"
}

func (s LocalStore) SaveLocal(path string, data []byte) (string, error) {
	dir := s.Dir
	if dir == "" {
		dir = "uploads"
	}
	// clean — «/covers/<файл>»: уже с ведущим слэшем, без «..» и двойных слэшей.
	clean := filepath.Clean("/" + path)
	full := filepath.Join(dir, clean)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return "", err
	}
	// Публичный путь — «/uploads/covers/<файл>» (контракт P7): раньше здесь
	// собирался лишний слэш («/uploads//covers/…»), пойман сквозным QA на стенде.
	return "/uploads" + filepath.ToSlash(clean), nil
}
