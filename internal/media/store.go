package media

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Store struct {
	baseDir   string
	retention time.Duration
	now       func() time.Time
}

func NewStore(baseDir string, retention time.Duration) *Store {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = filepath.Join(os.TempDir(), "imtty-media")
	}
	if retention <= 0 {
		retention = 24 * time.Hour
	}
	return &Store{
		baseDir:   baseDir,
		retention: retention,
		now:       time.Now,
	}
}

func (s *Store) SaveImage(sessionName string, fileID string, extension string, body io.Reader) (string, error) {
	return s.save(sessionName, fileID, extension, body)
}

func (s *Store) SaveDocument(sessionName string, fileID string, extension string, body io.Reader) (string, error) {
	return s.save(sessionName, fileID, extension, body)
}

func (s *Store) save(sessionName string, fileID string, extension string, body io.Reader) (string, error) {
	if err := s.cleanupExpired(); err != nil {
		return "", err
	}

	if extension == "" || extension[0] != '.' {
		extension = ".bin"
	}

	sessionDir := filepath.Join(s.baseDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("%d-%s%s", s.now().UnixNano(), sanitizeFileID(fileID), extension)
	path := filepath.Join(sessionDir, filename)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, body); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) cleanupExpired() error {
	if _, err := os.Stat(s.baseDir); os.IsNotExist(err) {
		return nil
	}

	cutoff := s.now().Add(-s.retention)
	return filepath.Walk(s.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
}

func sanitizeFileID(fileID string) string {
	fileID = strings.TrimSpace(fileID)
	fileID = strings.ReplaceAll(fileID, "/", "_")
	if fileID == "" {
		return "file"
	}
	return fileID
}
