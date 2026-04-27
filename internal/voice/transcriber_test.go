package voice

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhisperCPPTranscriberConvertsAudioAndReturnsTurnText(t *testing.T) {
	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "voice.oga")
	if err := os.WriteFile(audioPath, []byte("opus"), 0o600); err != nil {
		t.Fatalf("WriteFile(audio) error = %v", err)
	}
	runner := &fakeRunner{}
	transcriber, err := NewWhisperCPPTranscriberWithRunner(Config{
		FFmpegBin:  "ffmpeg",
		WhisperBin: "whisper-cli",
		ModelPath:  "/models/ggml-large-v3-turbo.bin",
		Language:   "zh",
		TempDir:    tempDir,
	}, runner)
	if err != nil {
		t.Fatalf("NewWhisperCPPTranscriberWithRunner() error = %v", err)
	}

	text, err := transcriber.BuildTurnText(context.Background(), audioPath, "audio/ogg")
	if err != nil {
		t.Fatalf("BuildTurnText() error = %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("len(calls) = %d, want ffmpeg + whisper", len(runner.calls))
	}
	if runner.calls[0].name != "ffmpeg" {
		t.Fatalf("first command = %q, want ffmpeg", runner.calls[0].name)
	}
	if runner.calls[1].name != "whisper-cli" {
		t.Fatalf("second command = %q, want whisper-cli", runner.calls[1].name)
	}
	if !containsArg(runner.calls[1].args, "/models/ggml-large-v3-turbo.bin") {
		t.Fatalf("whisper args = %#v, want model path", runner.calls[1].args)
	}
	if !containsArg(runner.calls[1].args, "zh") {
		t.Fatalf("whisper args = %#v, want language zh", runner.calls[1].args)
	}
	if !strings.Contains(text, "[Telegram voice transcript]") || !strings.Contains(text, "帮我检查当前项目") {
		t.Fatalf("text = %q, want transcript turn text", text)
	}
}

func TestWhisperCPPTranscriberRejectsEmptyTranscript(t *testing.T) {
	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "voice.oga")
	if err := os.WriteFile(audioPath, []byte("opus"), 0o600); err != nil {
		t.Fatalf("WriteFile(audio) error = %v", err)
	}
	runner := &fakeRunner{transcript: "   \n"}
	transcriber, err := NewWhisperCPPTranscriberWithRunner(Config{
		FFmpegBin:  "ffmpeg",
		WhisperBin: "whisper-cli",
		ModelPath:  "/models/ggml-large-v3-turbo.bin",
		Language:   "zh",
		TempDir:    tempDir,
	}, runner)
	if err != nil {
		t.Fatalf("NewWhisperCPPTranscriberWithRunner() error = %v", err)
	}

	_, err = transcriber.BuildTurnText(context.Background(), audioPath, "audio/ogg")
	if err == nil {
		t.Fatal("BuildTurnText() error = nil, want empty transcript error")
	}
}

type fakeRunner struct {
	calls      []runnerCall
	transcript string
}

type runnerCall struct {
	name string
	args []string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	if name != "whisper-cli" {
		return nil, nil
	}
	outBase := argAfter(args, "-of")
	if outBase == "" {
		return nil, nil
	}
	transcript := f.transcript
	if transcript == "" {
		transcript = "帮我检查当前项目"
	}
	if err := os.WriteFile(outBase+".txt", []byte(transcript), 0o600); err != nil {
		return nil, err
	}
	return nil, nil
}

func argAfter(args []string, key string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}

func containsArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}
