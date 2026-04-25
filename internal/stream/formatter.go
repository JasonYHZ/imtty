package stream

import (
	"regexp"
	"strings"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

type Formatter struct {
	chunkBytes int
}

func NewFormatter(chunkBytes int) Formatter {
	if chunkBytes <= 0 {
		chunkBytes = 3500
	}

	return Formatter{chunkBytes: chunkBytes}
}

func (f Formatter) Format(text string) []string {
	sanitized := sanitize(text)
	if sanitized == "" {
		return nil
	}

	chunks := make([]string, 0, (len(sanitized)/f.chunkBytes)+1)
	for len(sanitized) > 0 {
		if len(sanitized) <= f.chunkBytes {
			chunks = append(chunks, sanitized)
			break
		}

		chunks = append(chunks, sanitized[:f.chunkBytes])
		sanitized = sanitized[f.chunkBytes:]
	}

	return chunks
}

func sanitize(text string) string {
	text = ansiPattern.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.TrimSpace(text)
	return text
}
