package voice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Config struct {
	FFmpegBin  string
	WhisperBin string
	ModelPath  string
	Language   string
	TempDir    string
}

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type WhisperCPPTranscriber struct {
	config Config
	runner CommandRunner
}

func NewWhisperCPPTranscriber(config Config) (*WhisperCPPTranscriber, error) {
	return NewWhisperCPPTranscriberWithRunner(config, execRunner{})
}

func NewWhisperCPPTranscriberWithRunner(config Config, runner CommandRunner) (*WhisperCPPTranscriber, error) {
	config.FFmpegBin = strings.TrimSpace(config.FFmpegBin)
	config.WhisperBin = strings.TrimSpace(config.WhisperBin)
	config.ModelPath = strings.TrimSpace(config.ModelPath)
	config.Language = strings.TrimSpace(config.Language)
	config.TempDir = strings.TrimSpace(config.TempDir)
	if config.FFmpegBin == "" {
		config.FFmpegBin = "ffmpeg"
	}
	if config.WhisperBin == "" {
		config.WhisperBin = "whisper-cli"
	}
	if config.Language == "" {
		config.Language = "zh"
	}
	if config.TempDir == "" {
		config.TempDir = os.TempDir()
	}
	if config.ModelPath == "" {
		return nil, errors.New("voice model path is required")
	}
	if runner == nil {
		runner = execRunner{}
	}
	return &WhisperCPPTranscriber{config: config, runner: runner}, nil
}

func (t *WhisperCPPTranscriber) BuildTurnText(ctx context.Context, path string, _ string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("voice path is required")
	}
	workDir, err := os.MkdirTemp(t.config.TempDir, "imtty-voice-*")
	if err != nil {
		return "", fmt.Errorf("create voice temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	wavPath := filepath.Join(workDir, "input.wav")
	if output, err := t.runner.Run(ctx, t.config.FFmpegBin,
		"-y",
		"-i", path,
		"-ar", "16000",
		"-ac", "1",
		"-c:a", "pcm_s16le",
		wavPath,
	); err != nil {
		return "", fmt.Errorf("convert voice audio: %w: %s", err, strings.TrimSpace(string(output)))
	}

	outBase := filepath.Join(workDir, "transcript")
	if output, err := t.runner.Run(ctx, t.config.WhisperBin,
		"-m", t.config.ModelPath,
		"-f", wavPath,
		"-l", t.config.Language,
		"-nt",
		"-otxt",
		"-of", outBase,
	); err != nil {
		return "", fmt.Errorf("transcribe voice audio: %w: %s", err, strings.TrimSpace(string(output)))
	}

	raw, err := os.ReadFile(outBase + ".txt")
	if err != nil {
		return "", fmt.Errorf("read transcript: %w", err)
	}
	transcript := strings.TrimSpace(string(raw))
	if transcript == "" {
		return "", errors.New("voice transcript is empty")
	}
	return "[Telegram voice transcript]\n" + transcript, nil
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	return command.CombinedOutput()
}
