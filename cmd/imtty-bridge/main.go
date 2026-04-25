package main

import (
	"context"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"imtty/internal/appserver"
	"imtty/internal/config"
	"imtty/internal/fileinput"
	"imtty/internal/media"
	"imtty/internal/miniapp"
	"imtty/internal/session"
	"imtty/internal/stream"
	"imtty/internal/telegram"
	"imtty/internal/tmux"
)

func main() {
	configPath := flag.String("config", "", "config.toml path")
	listenAddr := flag.String("listen", "", "HTTP listen address")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if strings.TrimSpace(*listenAddr) == "" {
		*listenAddr = cfg.ListenAddr
	}

	projectStore := config.NewProjectStore(cfg.ProjectStorePath)
	dynamicProjects, err := projectStore.Load()
	if err != nil {
		log.Fatalf("load dynamic projects: %v", err)
	}

	registry := session.NewRegistryWithDynamic(cfg.ProjectRoots, dynamicProjects)
	sessionManager := tmux.NewManager(tmux.NewCommandRunner("tmux"), cfg.TmuxPrefix, cfg.CodexBin)
	restored, err := restoreDetachedSessions(context.Background(), registry, sessionManager, cfg.TmuxPrefix)
	if err != nil {
		log.Printf("reattach existing tmux sessions: %v", err)
	} else if len(restored) > 0 {
		log.Printf("reattached %d tmux sessions", len(restored))
	}
	botClient := telegram.NewBotClient("", cfg.TelegramBotToken, nil)
	mediaStore := media.NewStore("", 0)
	documentAnalyzer := fileinput.NewAnalyzer(nil, 0, 0)
	runtime := appserver.NewRuntime(sessionManager, stream.NewFormatter(cfg.MessageChunkBytes), botClient)
	adapter := telegram.NewAdapter(registry, runtime, projectStore, botClient, mediaStore, documentAnalyzer)

	var (
		miniAppFS    fs.FS
		miniAppIndex []byte
	)
	staticFS, indexHTML, err := miniapp.StaticAssetsFromDir("web/mini-app/dist")
	if err != nil {
		log.Printf("mini app static assets unavailable: %v", err)
	} else {
		miniAppFS = staticFS
		miniAppIndex = indexHTML
	}

	mux := newMux(
		telegram.NewWebhookHandler(cfg.TelegramWebhookSecret, adapter, botClient, log.Default()),
		miniapp.NewHandler(miniapp.Options{
			BotToken:    cfg.TelegramBotToken,
			OwnerID:     cfg.TelegramOwnerID,
			Registry:    registry,
			Adapter:     adapter,
			BrowseRoots: cfg.ProjectBrowseRoots,
			StaticFS:    miniAppFS,
			IndexHTML:   miniAppIndex,
		}),
	)

	if cfg.MiniAppBaseURL != "" {
		menuURL := cfg.MiniAppBaseURL + "/mini-app"
		if err := botClient.SetMenuButton(context.Background(), "控制面板", menuURL); err != nil {
			log.Printf("set telegram menu button: %v", err)
		}
	}

	log.Printf("imtty bridge listening on %s", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func newMux(webhookHandler http.Handler, miniAppHandler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/telegram/webhook", webhookHandler)
	mux.Handle("/mini-app", miniAppHandler)
	mux.Handle("/mini-app/", miniAppHandler)
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("ok"))
	})
	return mux
}
