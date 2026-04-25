package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	defaultListenAddr       = ":8080"
	defaultTmuxPrefix       = "codex-"
	defaultCodexBin         = "codex"
	defaultMessageChunkSize = 3500
	defaultFlushInterval    = 500 * time.Millisecond
)

type Config struct {
	ListenAddr            string
	TelegramBotToken      string
	TelegramWebhookSecret string
	TelegramOwnerID       int64
	MiniAppBaseURL        string
	ProjectRoots          map[string]string
	ProjectBrowseRoots    map[string]string
	ProjectStorePath      string
	TmuxPrefix            string
	CodexBin              string
	MessageChunkBytes     int
	FlushInterval         time.Duration
}

type fileConfig struct {
	ListenAddr            string            `toml:"listen"`
	TelegramBotToken      string            `toml:"telegram_bot_token"`
	TelegramWebhookSecret string            `toml:"telegram_webhook_secret"`
	TelegramOwnerID       int64             `toml:"telegram_owner_id"`
	MiniAppBaseURL        string            `toml:"mini_app_base_url"`
	ProjectRoots          map[string]string `toml:"projects"`
	ProjectBrowseRoots    map[string]string `toml:"project_browse_roots"`
	ProjectStorePath      string            `toml:"project_store_path"`
	TmuxPrefix            string            `toml:"tmux_prefix"`
	CodexBin              string            `toml:"codex_bin"`
	MessageChunkBytes     int               `toml:"message_chunk_bytes"`
	FlushIntervalMS       int               `toml:"flush_interval_ms"`
}

func Load(configPath string) (Config, error) {
	return load(configPath, os.Getenv, os.Getwd)
}

func LoadFromEnv() (Config, error) {
	return Load("")
}

func load(configPath string, getenv func(string) string, getwd func() (string, error)) (Config, error) {
	cwd, err := getwd()
	if err != nil {
		return Config{}, fmt.Errorf("resolve working directory: %w", err)
	}

	fileDir := cwd
	fileCfg, err := readFileConfig(configPath, cwd)
	if err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(configPath) != "" {
		fileDir = filepath.Dir(configPath)
	} else {
		fileDir = cwd
	}

	cfg := Config{
		ListenAddr:        defaultListenAddr,
		ProjectStorePath:  filepath.Join(cwd, ".imtty-projects.json"),
		TmuxPrefix:        defaultTmuxPrefix,
		CodexBin:          defaultCodexBin,
		MessageChunkBytes: defaultMessageChunkSize,
		FlushInterval:     defaultFlushInterval,
	}

	if fileCfg != nil {
		if raw := strings.TrimSpace(fileCfg.ListenAddr); raw != "" {
			cfg.ListenAddr = raw
		}
		if raw := strings.TrimSpace(fileCfg.TelegramBotToken); raw != "" {
			cfg.TelegramBotToken = raw
		}
		if raw := strings.TrimSpace(fileCfg.TelegramWebhookSecret); raw != "" {
			cfg.TelegramWebhookSecret = raw
		}
		if fileCfg.TelegramOwnerID > 0 {
			cfg.TelegramOwnerID = fileCfg.TelegramOwnerID
		}
		if raw := strings.TrimRight(strings.TrimSpace(fileCfg.MiniAppBaseURL), "/"); raw != "" {
			cfg.MiniAppBaseURL = raw
		}
		if len(fileCfg.ProjectRoots) > 0 {
			cfg.ProjectRoots = copyProjectRoots(fileCfg.ProjectRoots)
		}
		if len(fileCfg.ProjectBrowseRoots) > 0 {
			cfg.ProjectBrowseRoots = copyProjectRoots(fileCfg.ProjectBrowseRoots)
		}
		if raw := strings.TrimSpace(fileCfg.ProjectStorePath); raw != "" {
			cfg.ProjectStorePath = resolvePath(fileDir, raw)
		}
		if raw := strings.TrimSpace(fileCfg.TmuxPrefix); raw != "" {
			cfg.TmuxPrefix = raw
		}
		if raw := strings.TrimSpace(fileCfg.CodexBin); raw != "" {
			cfg.CodexBin = raw
		}
		if fileCfg.MessageChunkBytes > 0 {
			cfg.MessageChunkBytes = fileCfg.MessageChunkBytes
		}
		if fileCfg.FlushIntervalMS > 0 {
			cfg.FlushInterval = time.Duration(fileCfg.FlushIntervalMS) * time.Millisecond
		}
	}

	if raw := strings.TrimSpace(getenv("IMTTY_TELEGRAM_BOT_TOKEN")); raw != "" {
		cfg.TelegramBotToken = raw
	}
	if raw := strings.TrimSpace(getenv("IMTTY_TELEGRAM_WEBHOOK_SECRET")); raw != "" {
		cfg.TelegramWebhookSecret = raw
	}
	if raw := strings.TrimSpace(getenv("IMTTY_TELEGRAM_OWNER_ID")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("IMTTY_TELEGRAM_OWNER_ID must be a positive integer")
		}
		cfg.TelegramOwnerID = value
	}
	if raw := strings.TrimSpace(getenv("IMTTY_MINI_APP_BASE_URL")); raw != "" {
		cfg.MiniAppBaseURL = strings.TrimRight(raw, "/")
	}
	if raw := strings.TrimSpace(getenv("IMTTY_PROJECT_ROOTS")); raw != "" {
		projectRoots, err := parseNamedPaths(raw, "IMTTY_PROJECT_ROOTS")
		if err != nil {
			return Config{}, err
		}
		cfg.ProjectRoots = projectRoots
	}
	if raw := strings.TrimSpace(getenv("IMTTY_PROJECT_BROWSE_ROOTS")); raw != "" {
		projectBrowseRoots, err := parseNamedPaths(raw, "IMTTY_PROJECT_BROWSE_ROOTS")
		if err != nil {
			return Config{}, err
		}
		cfg.ProjectBrowseRoots = projectBrowseRoots
	}
	if raw := strings.TrimSpace(getenv("IMTTY_PROJECT_STORE_PATH")); raw != "" {
		cfg.ProjectStorePath = resolvePath(cwd, raw)
	}
	if raw := strings.TrimSpace(getenv("IMTTY_TMUX_PREFIX")); raw != "" {
		cfg.TmuxPrefix = raw
	}
	if raw := strings.TrimSpace(getenv("IMTTY_CODEX_BIN")); raw != "" {
		cfg.CodexBin = raw
	}
	if raw := strings.TrimSpace(getenv("IMTTY_MESSAGE_CHUNK_BYTES")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("IMTTY_MESSAGE_CHUNK_BYTES must be a positive integer")
		}
		cfg.MessageChunkBytes = value
	}
	if raw := strings.TrimSpace(getenv("IMTTY_FLUSH_INTERVAL_MS")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("IMTTY_FLUSH_INTERVAL_MS must be a positive integer")
		}
		cfg.FlushInterval = time.Duration(value) * time.Millisecond
	}

	if cfg.TelegramBotToken == "" {
		return Config{}, errors.New("IMTTY_TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.TelegramWebhookSecret == "" {
		return Config{}, errors.New("IMTTY_TELEGRAM_WEBHOOK_SECRET is required")
	}
	if len(cfg.ProjectRoots) == 0 {
		return Config{}, errors.New("IMTTY_PROJECT_ROOTS is required")
	}
	if err := validateProjectRoots(cfg.ProjectRoots); err != nil {
		return Config{}, err
	}
	if err := validateProjectRoots(cfg.ProjectBrowseRoots); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func readFileConfig(configPath string, cwd string) (*fileConfig, error) {
	if strings.TrimSpace(configPath) == "" {
		configPath = filepath.Join(cwd, "config.toml")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg fileConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	return &cfg, nil
}

func parseProjectRoots(raw string) (map[string]string, error) {
	return parseNamedPaths(raw, "IMTTY_PROJECT_ROOTS")
}

func parseNamedPaths(raw string, sourceName string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%s is required", sourceName)
	}

	projectRoots := make(map[string]string)
	entries := strings.Split(raw, ",")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		name, root, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("invalid %s entry %q", sourceName, entry)
		}

		name = strings.TrimSpace(name)
		root = strings.TrimSpace(root)
		if name == "" || root == "" {
			return nil, fmt.Errorf("invalid %s entry %q", sourceName, entry)
		}

		projectRoots[name] = root
	}

	if len(projectRoots) == 0 {
		return nil, fmt.Errorf("%s must contain at least one entry", sourceName)
	}
	if err := validateProjectRoots(projectRoots); err != nil {
		return nil, err
	}

	return projectRoots, nil
}

func validateProjectRoots(projectRoots map[string]string) error {
	for name, root := range projectRoots {
		if !filepath.IsAbs(root) {
			return fmt.Errorf("project root for %q must be absolute", name)
		}
	}
	return nil
}

func resolvePath(baseDir string, raw string) string {
	if filepath.IsAbs(raw) {
		return raw
	}
	return filepath.Join(baseDir, raw)
}

func copyProjectRoots(projectRoots map[string]string) map[string]string {
	projects := make(map[string]string, len(projectRoots))
	for name, root := range projectRoots {
		projects[name] = root
	}
	return projects
}

func SortedProjectNames(projectRoots map[string]string) []string {
	names := make([]string, 0, len(projectRoots))
	for name := range projectRoots {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
