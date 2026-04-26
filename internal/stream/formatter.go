package stream

import (
	"html"
	"regexp"
	"strings"
	"unicode"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

type Formatter struct {
	chunkBytes int
}

type telegramHTMLPart struct {
	kind     string
	text     string
	language string
}

const (
	telegramHTMLPartText = "text"
	telegramHTMLPartCode = "code"
)

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

func (f Formatter) FormatTelegramHTML(text string) []OutboundMessage {
	sanitized := sanitize(text)
	if sanitized == "" {
		return nil
	}

	chunks := renderTelegramHTMLChunks(sanitized, f.chunkBytes)
	messages := make([]OutboundMessage, 0, len(chunks))
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		messages = append(messages, OutboundMessage{
			Text:      chunk,
			ParseMode: ParseModeHTML,
		})
	}
	return messages
}

func sanitize(text string) string {
	text = ansiPattern.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.TrimSpace(text)
	return text
}

func renderTelegramHTMLChunks(text string, chunkBytes int) []string {
	if chunkBytes <= 0 {
		chunkBytes = 3500
	}

	parts := parseTelegramHTMLParts(text)
	chunks := make([]string, 0, len(parts))
	current := ""

	flush := func() {
		if current == "" {
			return
		}
		chunks = append(chunks, current)
		current = ""
	}

	appendRendered := func(rendered string) {
		if rendered == "" {
			return
		}
		if len(rendered) <= chunkBytes {
			if current != "" && len(current)+len(rendered) > chunkBytes {
				flush()
			}
			current += rendered
			return
		}

		flush()
		chunks = append(chunks, splitRenderedHTML(rendered, chunkBytes)...)
	}

	for _, part := range parts {
		switch part.kind {
		case telegramHTMLPartCode:
			for _, rendered := range splitCodeBlockHTML(part.language, part.text, chunkBytes) {
				appendRendered(rendered)
			}
		default:
			appendRendered(renderInlineCodeHTML(part.text))
		}
	}
	flush()
	return chunks
}

func parseTelegramHTMLParts(text string) []telegramHTMLPart {
	lines := strings.SplitAfter(text, "\n")
	parts := make([]telegramHTMLPart, 0, 4)
	var textBuffer strings.Builder
	var codeBuffer strings.Builder
	inFence := false
	language := ""

	flushText := func() {
		if textBuffer.Len() == 0 {
			return
		}
		parts = append(parts, telegramHTMLPart{kind: telegramHTMLPartText, text: textBuffer.String()})
		textBuffer.Reset()
	}
	flushCode := func() {
		code := strings.TrimSuffix(codeBuffer.String(), "\n")
		parts = append(parts, telegramHTMLPart{kind: telegramHTMLPartCode, text: code, language: language})
		codeBuffer.Reset()
		language = ""
	}

	for _, line := range lines {
		lineWithoutNewline := strings.TrimSuffix(line, "\n")
		trimmed := strings.TrimLeftFunc(lineWithoutNewline, unicode.IsSpace)
		if strings.HasPrefix(trimmed, "```") {
			if inFence {
				flushCode()
				inFence = false
				continue
			}
			flushText()
			language = sanitizeCodeLanguage(strings.TrimSpace(strings.TrimPrefix(trimmed, "```")))
			inFence = true
			continue
		}

		if inFence {
			codeBuffer.WriteString(line)
			continue
		}
		textBuffer.WriteString(line)
	}

	if inFence {
		flushCode()
	} else {
		flushText()
	}
	return parts
}

func renderInlineCodeHTML(text string) string {
	var builder strings.Builder
	for {
		start := strings.IndexByte(text, '`')
		if start < 0 {
			builder.WriteString(html.EscapeString(text))
			return builder.String()
		}

		end := strings.IndexByte(text[start+1:], '`')
		if end < 0 {
			builder.WriteString(html.EscapeString(text))
			return builder.String()
		}

		builder.WriteString(html.EscapeString(text[:start]))
		code := text[start+1 : start+1+end]
		builder.WriteString("<code>")
		builder.WriteString(html.EscapeString(code))
		builder.WriteString("</code>")
		text = text[start+1+end+1:]
	}
}

func renderCodeBlockHTML(language string, code string) string {
	var builder strings.Builder
	builder.WriteString(codeBlockPrefix(language))
	builder.WriteString(html.EscapeString(code))
	builder.WriteString("</code></pre>")
	return builder.String()
}

func splitCodeBlockHTML(language string, code string, chunkBytes int) []string {
	prefix := codeBlockPrefix(language)
	suffix := "</code></pre>"
	maxCodeBytes := chunkBytes - len(prefix) - len(suffix)
	if maxCodeBytes <= 0 || len(prefix)+len(suffix) > chunkBytes {
		return []string{renderCodeBlockHTML(language, code)}
	}
	if code == "" {
		return []string{prefix + suffix}
	}

	chunks := make([]string, 0, (len(code)/maxCodeBytes)+1)
	for len(code) > 0 {
		cut := escapedPrefixCut(code, maxCodeBytes)
		if cut <= 0 {
			cut = len(code)
		}
		chunks = append(chunks, prefix+html.EscapeString(code[:cut])+suffix)
		code = code[cut:]
	}
	return chunks
}

func codeBlockPrefix(language string) string {
	if language == "" {
		return "<pre><code>"
	}
	return `<pre><code class="language-` + html.EscapeString(language) + `">`
}

func escapedPrefixCut(text string, maxEscapedBytes int) int {
	used := 0
	lastNewlineCut := 0
	for index, r := range text {
		escapedLen := len(html.EscapeString(string(r)))
		if used+escapedLen > maxEscapedBytes {
			if lastNewlineCut > 0 {
				return lastNewlineCut
			}
			if index > 0 {
				return index
			}
			return index + len(string(r))
		}
		used += escapedLen
		if r == '\n' {
			lastNewlineCut = index + len(string(r))
		}
	}
	return len(text)
}

func sanitizeCodeLanguage(language string) string {
	fields := strings.Fields(language)
	if len(fields) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, r := range fields[0] {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '+' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func splitRenderedHTML(text string, chunkBytes int) []string {
	if chunkBytes <= 0 {
		chunkBytes = 3500
	}

	chunks := make([]string, 0, (len(text)/chunkBytes)+1)
	for len(text) > 0 {
		if len(text) <= chunkBytes {
			chunks = append(chunks, text)
			break
		}

		cut := safeHTMLCut(text, chunkBytes)
		chunks = append(chunks, text[:cut])
		text = text[cut:]
	}
	return chunks
}

func safeHTMLCut(text string, limit int) int {
	if len(text) <= limit {
		return len(text)
	}

	cut := strings.LastIndex(text[:limit], "\n")
	if cut > limit/2 {
		return cut + 1
	}
	cut = strings.LastIndex(text[:limit], " ")
	if cut > limit/2 {
		return cut + 1
	}

	for cut = limit; cut > 0 && !utf8RuneBoundary(text, cut); cut-- {
	}
	if cut == 0 {
		return limit
	}
	return cut
}

func utf8RuneBoundary(text string, index int) bool {
	if index <= 0 || index >= len(text) {
		return true
	}
	return text[index]&0xc0 != 0x80
}
