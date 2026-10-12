package storage

import (
	"fmt"
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
	full := filepath.Join(dir, filepath.Clean("/"+path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("/uploads/%s", filepath.Clean("/"+path)), nil
}
