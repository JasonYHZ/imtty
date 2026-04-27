package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadParsesRequiredEnvAndDefaults(t *testing.T) {
	t.Setenv("IMTTY_TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("IMTTY_TELEGRAM_WEBHOOK_SECRET", "secret-token")
	t.Setenv("IMTTY_PROJECT_ROOTS", "project-a=/tmp/project-a,project-b=/tmp/project-b")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.TelegramBotToken != "bot-token" {
		t.Fatalf("TelegramBotToken = %q, want %q", cfg.TelegramBotToken, "bot-token")
	}

	if cfg.TelegramWebhookSecret != "secret-token" {
		t.Fatalf("TelegramWebhookSecret = %q, want %q", cfg.TelegramWebhookSecret, "secret-token")
	}

	if got := cfg.ProjectRoots["project-a"]; got != "/tmp/project-a" {
		t.Fatalf("ProjectRoots[project-a] = %q, want %q", got, "/tmp/project-a")
	}

	if cfg.TmuxPrefix != "codex-" {
		t.Fatalf("TmuxPrefix = %q, want %q", cfg.TmuxPrefix, "codex-")
	}

	if cfg.CodexBin != "codex" {
		t.Fatalf("CodexBin = %q, want %q", cfg.CodexBin, "codex")
	}
	if cfg.PlanModeReasoning != "xhigh" {
		t.Fatalf("PlanModeReasoning = %q, want %q", cfg.PlanModeReasoning, "xhigh")
	}

	if cfg.MessageChunkBytes <= 0 {
		t.Fatalf("MessageChunkBytes = %d, want positive", cfg.MessageChunkBytes)
	}

	if cfg.FlushInterval != 500*time.Millisecond {
		t.Fatalf("FlushInterval = %s, want %s", cfg.FlushInterval, 500*time.Millisecond)
	}

	if got := cfg.ProjectStorePath; got == "" {
		t.Fatal("ProjectStorePath = empty, want default path")
	}

	if cfg.TelegramOwnerID != 0 {
		t.Fatalf("TelegramOwnerID = %d, want 0 by default", cfg.TelegramOwnerID)
	}

	if cfg.MiniAppBaseURL != "" {
		t.Fatalf("MiniAppBaseURL = %q, want empty by default", cfg.MiniAppBaseURL)
	}
}

func TestLoadRejectsRelativeProjectRoot(t *testing.T) {
	t.Setenv("IMTTY_TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("IMTTY_TELEGRAM_WEBHOOK_SECRET", "secret-token")
	t.Setenv("IMTTY_PROJECT_ROOTS", "project-a=relative/path")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("LoadFromEnv() error = nil, want error")
	}
}

func TestLoadParsesCustomProjectStorePath(t *testing.T) {
	t.Setenv("IMTTY_TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("IMTTY_TELEGRAM_WEBHOOK_SECRET", "secret-token")
	t.Setenv("IMTTY_PROJECT_ROOTS", "project-a=/tmp/project-a")
	t.Setenv("IMTTY_PROJECT_STORE_PATH", "/tmp/imtty-projects.json")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if got := cfg.ProjectStorePath; got != "/tmp/imtty-projects.json" {
		t.Fatalf("ProjectStorePath = %q, want %q", got, "/tmp/imtty-projects.json")
	}
}

func TestLoadParsesMiniAppConfig(t *testing.T) {
	t.Setenv("IMTTY_TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("IMTTY_TELEGRAM_WEBHOOK_SECRET", "secret-token")
	t.Setenv("IMTTY_PROJECT_ROOTS", "project-a=/tmp/project-a")
	t.Setenv("IMTTY_TELEGRAM_OWNER_ID", "42")
	t.Setenv("IMTTY_MINI_APP_BASE_URL", "https://example.com")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if got := cfg.TelegramOwnerID; got != 42 {
		t.Fatalf("TelegramOwnerID = %d, want %d", got, 42)
	}

	if got := cfg.MiniAppBaseURL; got != "https://example.com" {
		t.Fatalf("MiniAppBaseURL = %q, want %q", got, "https://example.com")
	}
}

func TestLoadParsesVoiceConfigFromEnv(t *testing.T) {
	t.Setenv("IMTTY_TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("IMTTY_TELEGRAM_WEBHOOK_SECRET", "secret-token")
	t.Setenv("IMTTY_PROJECT_ROOTS", "project-a=/tmp/project-a")
	t.Setenv("IMTTY_VOICE_ENABLED", "true")
	t.Setenv("IMTTY_VOICE_FFMPEG_BIN", "/opt/homebrew/bin/ffmpeg")
	t.Setenv("IMTTY_VOICE_WHISPER_BIN", "/opt/whisper.cpp/build/bin/whisper-cli")
	t.Setenv("IMTTY_VOICE_MODEL_PATH", "/opt/whisper.cpp/models/ggml-large-v3-turbo.bin")
	t.Setenv("IMTTY_VOICE_LANGUAGE", "zh")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if !cfg.Voice.Enabled {
		t.Fatal("Voice.Enabled = false, want true")
	}
	if got := cfg.Voice.FFmpegBin; got != "/opt/homebrew/bin/ffmpeg" {
		t.Fatalf("Voice.FFmpegBin = %q, want configured ffmpeg", got)
	}
	if got := cfg.Voice.WhisperBin; got != "/opt/whisper.cpp/build/bin/whisper-cli" {
		t.Fatalf("Voice.WhisperBin = %q, want configured whisper", got)
	}
	if got := cfg.Voice.ModelPath; got != "/opt/whisper.cpp/models/ggml-large-v3-turbo.bin" {
		t.Fatalf("Voice.ModelPath = %q, want configured model", got)
	}
	if got := cfg.Voice.Language; got != "zh" {
		t.Fatalf("Voice.Language = %q, want zh", got)
	}
}

func TestLoadRejectsInvalidOwnerID(t *testing.T) {
	t.Setenv("IMTTY_TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("IMTTY_TELEGRAM_WEBHOOK_SECRET", "secret-token")
	t.Setenv("IMTTY_PROJECT_ROOTS", "project-a=/tmp/project-a")
	t.Setenv("IMTTY_TELEGRAM_OWNER_ID", "not-a-number")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("LoadFromEnv() error = nil, want error")
	}
}

func TestLoadParsesConfigTomlAndDefaults(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(`
listen = ":9090"
telegram_bot_token = "bot-token"
telegram_webhook_secret = "secret"
telegram_owner_id = 42
mini_app_base_url = "https://imtty.example.com"
project_store_path = "state/projects.json"
plan_mode_reasoning_effort = "xhigh"

[projects]
imtty = "/tmp/imtty"

[project_browse_roots]
personal = "/tmp/personal"
`), 0o644); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", configPath, err)
	}

	if got := cfg.ListenAddr; got != ":9090" {
		t.Fatalf("ListenAddr = %q, want %q", got, ":9090")
	}

	if got := cfg.TelegramOwnerID; got != 42 {
		t.Fatalf("TelegramOwnerID = %d, want %d", got, 42)
	}

	if got := cfg.ProjectRoots["imtty"]; got != "/tmp/imtty" {
		t.Fatalf("ProjectRoots[imtty] = %q, want %q", got, "/tmp/imtty")
	}

	if got := cfg.ProjectStorePath; got != filepath.Join(tempDir, "state/projects.json") {
		t.Fatalf("ProjectStorePath = %q, want %q", got, filepath.Join(tempDir, "state/projects.json"))
	}

	if got := cfg.ProjectBrowseRoots["personal"]; got != "/tmp/personal" {
		t.Fatalf("ProjectBrowseRoots[personal] = %q, want %q", got, "/tmp/personal")
	}
	if got := cfg.PlanModeReasoning; got != "xhigh" {
		t.Fatalf("PlanModeReasoning = %q, want %q", got, "xhigh")
	}
}

func TestLoadEnvOverridesConfigToml(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(`
telegram_bot_token = "file-token"
telegram_webhook_secret = "file-secret"

[projects]
imtty = "/tmp/imtty"
`), 0o644); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}

	t.Setenv("IMTTY_TELEGRAM_BOT_TOKEN", "env-token")
	t.Setenv("IMTTY_TELEGRAM_WEBHOOK_SECRET", "env-secret")
	t.Setenv("IMTTY_MINI_APP_BASE_URL", "https://env.example.com")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", configPath, err)
	}

	if got := cfg.TelegramBotToken; got != "env-token" {
		t.Fatalf("TelegramBotToken = %q, want %q", got, "env-token")
	}

	if got := cfg.TelegramWebhookSecret; got != "env-secret" {
		t.Fatalf("TelegramWebhookSecret = %q, want %q", got, "env-secret")
	}

	if got := cfg.MiniAppBaseURL; got != "https://env.example.com" {
		t.Fatalf("MiniAppBaseURL = %q, want %q", got, "https://env.example.com")
	}
}

func TestLoadRejectsRelativeProjectBrowseRoot(t *testing.T) {
	t.Setenv("IMTTY_TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("IMTTY_TELEGRAM_WEBHOOK_SECRET", "secret-token")
	t.Setenv("IMTTY_PROJECT_ROOTS", "project-a=/tmp/project-a")
	t.Setenv("IMTTY_PROJECT_BROWSE_ROOTS", "personal=relative/path")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("LoadFromEnv() error = nil, want error")
	}
}
