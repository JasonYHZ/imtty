package stream

import (
	"reflect"
	"strings"
	"testing"
)

func TestFormatterSanitizesAnsiAndSplitsIntoChunks(t *testing.T) {
	formatter := NewFormatter(12)

	chunks := formatter.Format("\x1b[32mhello\x1b[0m world")
	want := []string{"hello world"}

	if !reflect.DeepEqual(chunks, want) {
		t.Fatalf("chunks = %#v, want %#v", chunks, want)
	}
}

func TestFormatterSplitsLongOutputInOrder(t *testing.T) {
	formatter := NewFormatter(5)

	chunks := formatter.Format("abcdefghij")
	want := []string{"abcde", "fghij"}

	if !reflect.DeepEqual(chunks, want) {
		t.Fatalf("chunks = %#v, want %#v", chunks, want)
	}
}

func TestFormatterDropsEmptyOutputAfterSanitizing(t *testing.T) {
	formatter := NewFormatter(10)

	chunks := formatter.Format("\x1b[31m\x1b[0m\r")
	if len(chunks) != 0 {
		t.Fatalf("chunks = %#v, want empty", chunks)
	}
}

func TestFormatterRendersFencedCodeBlocksAsTelegramHTML(t *testing.T) {
	formatter := NewFormatter(4000)

	messages := formatter.FormatTelegramHTML("看这里：\n```go\nif a < b {\n\treturn \"ok\"\n}\n```")
	want := []OutboundMessage{{
		Text:      "看这里：\n<pre><code class=\"language-go\">if a &lt; b {\n\treturn &#34;ok&#34;\n}</code></pre>",
		ParseMode: ParseModeHTML,
	}}

	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("messages = %#v, want %#v", messages, want)
	}
}

func TestFormatterRendersInlineCodeAndEscapesPlainText(t *testing.T) {
	formatter := NewFormatter(4000)

	messages := formatter.FormatTelegramHTML("路径 `<root>/a_b.go` & 保持安全")
	want := []OutboundMessage{{
		Text:      "路径 <code>&lt;root&gt;/a_b.go</code> &amp; 保持安全",
		ParseMode: ParseModeHTML,
	}}

	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("messages = %#v, want %#v", messages, want)
	}
}

func TestFormatterSplitsLongFencedCodeBlocksAsValidTelegramHTML(t *testing.T) {
	formatter := NewFormatter(70)

	messages := formatter.FormatTelegramHTML("```go\nalpha < beta\nreturn \"one\"\nreturn \"two\"\n```")
	if len(messages) < 2 {
		t.Fatalf("len(messages) = %d, want split code block", len(messages))
	}

	for _, message := range messages {
		if message.ParseMode != ParseModeHTML {
			t.Fatalf("ParseMode = %q, want %q", message.ParseMode, ParseModeHTML)
		}
		if len(message.Text) > 70 {
			t.Fatalf("message length = %d, want <= 70: %q", len(message.Text), message.Text)
		}
		if !strings.HasPrefix(message.Text, `<pre><code class="language-go">`) || !strings.HasSuffix(message.Text, "</code></pre>") {
			t.Fatalf("message.Text = %q, want complete pre/code wrapper", message.Text)
		}
	}
}
