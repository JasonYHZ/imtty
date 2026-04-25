package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreSaveImageWritesTempFileAndCleansExpiredFiles(t *testing.T) {
	baseDir := t.TempDir()
	store := NewStore(baseDir, 24*time.Hour)

	sessionDir := filepath.Join(baseDir, "codex-project-a")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	expiredPath := filepath.Join(sessionDir, "expired.png")
	if err := os.WriteFile(expiredPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(expired) error = %v", err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(expiredPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	savedPath, err := store.SaveImage("codex-project-a", "telegram-file-1", ".png", strings.NewReader("png-bytes"))
	if err != nil {
		t.Fatalf("SaveImage() error = %v", err)
	}

	if !strings.HasPrefix(savedPath, sessionDir) {
		t.Fatalf("savedPath = %q, want prefix %q", savedPath, sessionDir)
	}

	body, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("ReadFile(savedPath) error = %v", err)
	}
	if string(body) != "png-bytes" {
		t.Fatalf("saved content = %q, want %q", string(body), "png-bytes")
	}

	if _, err := os.Stat(expiredPath); !os.IsNotExist(err) {
		t.Fatalf("expired file still exists, stat err = %v", err)
	}
}
