package stream

import (
	"reflect"
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
