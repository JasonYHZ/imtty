package fileinput

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var (
	ErrUnsupportedFileType   = errors.New("unsupported file type")
	ErrFileTooLarge          = errors.New("file too large")
	ErrPDFTextExtractFailure = errors.New("pdf text extraction failed")
)

const (
	defaultMaxFileBytes = 1 << 20
	defaultMaxTextBytes = 256 << 10
)

type PDFExtractor interface {
	ExtractText(ctx context.Context, path string) (string, error)
}

type Analyzer struct {
	pdfExtractor PDFExtractor
	maxFileBytes int64
	maxTextBytes int
}

func NewAnalyzer(pdfExtractor PDFExtractor, maxFileBytes int64, maxTextBytes int) *Analyzer {
	if maxFileBytes <= 0 {
		maxFileBytes = defaultMaxFileBytes
	}
	if maxTextBytes <= 0 {
		maxTextBytes = defaultMaxTextBytes
	}
	if pdfExtractor == nil {
		pdfExtractor = SwiftPDFExtractor{}
	}
	return &Analyzer{
		pdfExtractor: pdfExtractor,
		maxFileBytes: maxFileBytes,
		maxTextBytes: maxTextBytes,
	}
}

func (a *Analyzer) BuildTurnText(ctx context.Context, path string, fileName string, mimeType string, caption string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > a.maxFileBytes {
		return "", ErrFileTooLarge
	}

	var (
		content   string
		truncated bool
	)
	if isPDF(fileName, mimeType) {
		content, err = a.pdfExtractor.ExtractText(ctx, path)
		if err != nil {
			return "", ErrPDFTextExtractFailure
		}
	} else if isTextLike(fileName, mimeType) {
		content, truncated, err = a.readText(path)
		if err != nil {
			return "", err
		}
	} else {
		return "", ErrUnsupportedFileType
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return "", ErrUnsupportedFileType
	}

	var builder strings.Builder
	builder.WriteString("用户通过 Telegram 发送了一个文件。\n")
	builder.WriteString("文件名：")
	builder.WriteString(fileName)
	if strings.TrimSpace(mimeType) != "" {
		builder.WriteString("\n媒体类型：")
		builder.WriteString(mimeType)
	}
	if strings.TrimSpace(caption) != "" {
		builder.WriteString("\n用户说明：")
		builder.WriteString(strings.TrimSpace(caption))
	}
	builder.WriteString("\n\n文件内容如下：\n")
	builder.WriteString(content)
	if truncated {
		builder.WriteString("\n\n[文件内容已截断，仅发送前 262144 字节文本]")
	}
	return builder.String(), nil
}

func (a *Analyzer) readText(path string) (string, bool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	if !utf8.Valid(body) {
		return "", false, ErrUnsupportedFileType
	}
	truncated := false
	if len(body) > a.maxTextBytes {
		body = body[:a.maxTextBytes]
		truncated = true
	}
	return string(body), truncated, nil
}

func isPDF(fileName string, mimeType string) bool {
	return strings.EqualFold(strings.TrimSpace(mimeType), "application/pdf") || strings.EqualFold(filepath.Ext(fileName), ".pdf")
}

func isTextLike(fileName string, mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	switch mimeType {
	case "application/json", "application/yaml", "application/x-yaml", "application/xml":
		return true
	}

	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".txt", ".md", ".json", ".yaml", ".yml", ".log", ".csv", ".go", ".ts", ".js", ".tsx", ".jsx", ".py", ".java", ".kt", ".kts", ".sql", ".sh":
		return true
	default:
		return false
	}
}

type SwiftPDFExtractor struct{}

func (SwiftPDFExtractor) ExtractText(ctx context.Context, path string) (string, error) {
	script := `
import Foundation
import PDFKit

let path = CommandLine.arguments[1]
guard let document = PDFDocument(url: URL(fileURLWithPath: path)) else {
    fputs("failed to open pdf\n", stderr)
    exit(1)
}
let text = document.string ?? ""
FileHandle.standardOutput.write(Data(text.utf8))
`
	command := exec.CommandContext(ctx, "/usr/bin/swift", "-e", script, path)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", ErrPDFTextExtractFailure, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
