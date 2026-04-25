package fileinput

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzerBuildTurnTextFromTextFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(path, []byte("# title\nhello"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	analyzer := NewAnalyzer(nil, 1<<20, 4096)
	text, err := analyzer.BuildTurnText(context.Background(), path, "notes.md", "text/markdown", "帮我总结")
	if err != nil {
		t.Fatalf("BuildTurnText() error = %v", err)
	}

	if !strings.Contains(text, "文件名：notes.md") {
		t.Fatalf("text = %q, want file name", text)
	}
	if !strings.Contains(text, "用户说明：帮我总结") {
		t.Fatalf("text = %q, want caption", text)
	}
	if !strings.Contains(text, "# title\nhello") {
		t.Fatalf("text = %q, want file content", text)
	}
}

func TestAnalyzerBuildTurnTextFromPDFUsesExtractor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paper.pdf")
	if err := os.WriteFile(path, []byte("%PDF"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	extractor := &fakePDFExtractor{text: "pdf body"}
	analyzer := NewAnalyzer(extractor, 1<<20, 4096)
	text, err := analyzer.BuildTurnText(context.Background(), path, "paper.pdf", "application/pdf", "")
	if err != nil {
		t.Fatalf("BuildTurnText() error = %v", err)
	}

	if extractor.paths[0] != path {
		t.Fatalf("paths = %#v, want %q", extractor.paths, path)
	}
	if !strings.Contains(text, "pdf body") {
		t.Fatalf("text = %q, want extracted pdf text", text)
	}
}

func TestAnalyzerRejectsUnsupportedBinaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.zip")
	if err := os.WriteFile(path, []byte("PK"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	analyzer := NewAnalyzer(nil, 1<<20, 4096)
	_, err := analyzer.BuildTurnText(context.Background(), path, "archive.zip", "application/zip", "")
	if !errors.Is(err, ErrUnsupportedFileType) {
		t.Fatalf("err = %v, want ErrUnsupportedFileType", err)
	}
}

type fakePDFExtractor struct {
	text  string
	err   error
	paths []string
}

func (f *fakePDFExtractor) ExtractText(_ context.Context, path string) (string, error) {
	f.paths = append(f.paths, path)
	if f.err != nil {
		return "", f.err
	}
	return f.text, nil
}
