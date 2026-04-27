package miniapp

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"

	"imtty/internal/appserver"
	"imtty/internal/config"
	"imtty/internal/session"
	"imtty/internal/telegram"
)

type Options struct {
	BotToken    string
	OwnerID     int64
	Registry    *session.Registry
	Adapter     *telegram.Adapter
	Runtime     telegram.SessionRuntime
	BrowseRoots map[string]string
	StaticFS    fs.FS
}

func NewHandler(options Options) http.Handler {
	browser := NewBrowser(options.BrowseRoots)
	mux := http.NewServeMux()
	mux.Handle("/mini-app/api/bootstrap", withViewer(options, func(writer http.ResponseWriter, request *http.Request, viewer Viewer) {
		writeJSON(writer, http.StatusOK, buildBootstrap(request.Context(), options.Registry, options.Runtime, browser, viewer))
	}))
	mux.Handle("/mini-app/api/project-browse", withViewer(options, func(writer http.ResponseWriter, request *http.Request, _ Viewer) {
		if request.Method != http.MethodGet {
			http.Error(writer, "请求方法不被允许", http.StatusMethodNotAllowed)
			return
		}

		response, err := browser.Browse(request.URL.Query().Get("path"))
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(writer, http.StatusOK, response)
	}))
	mux.Handle("/mini-app/api/open", withViewer(options, commandAction(options, func(payload commandPayload) string {
		return "/open " + payload.Project
	})))
	mux.Handle("/mini-app/api/close", withViewer(options, commandAction(options, func(commandPayload) string {
		return "/close"
	})))
	mux.Handle("/mini-app/api/kill", withViewer(options, commandAction(options, func(commandPayload) string {
		return "/kill"
	})))
	mux.Handle("/mini-app/api/clear", withViewer(options, commandAction(options, func(commandPayload) string {
		return "/clear"
	})))
	mux.Handle("/mini-app/api/project-add", withViewer(options, commandAction(options, func(payload commandPayload) string {
		return "/project_add " + payload.Name + " " + payload.Root
	})))
	mux.Handle("/mini-app/api/project-remove", withViewer(options, commandAction(options, func(payload commandPayload) string {
		return "/project_remove " + payload.Name
	})))
	mux.Handle("/mini-app/api/model", withViewer(options, commandAction(options, func(payload commandPayload) string {
		return "/model " + payload.Model
	})))
	mux.Handle("/mini-app/api/reasoning", withViewer(options, commandAction(options, func(payload commandPayload) string {
		return "/reasoning " + payload.Reasoning
	})))
	mux.Handle("/mini-app/api/plan-mode", withViewer(options, commandAction(options, func(payload commandPayload) string {
		return "/plan_mode " + payload.Mode
	})))
	var staticHandler http.Handler
	if options.StaticFS != nil {
		staticHandler = http.StripPrefix("/mini-app/", http.FileServer(http.FS(options.StaticFS)))
	}
	pageHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/mini-app" {
			http.Redirect(writer, request, "/mini-app/", http.StatusMovedPermanently)
			return
		}

		if staticHandler == nil {
			http.Error(writer, "Mini App 页面未构建", http.StatusServiceUnavailable)
			return
		}

		staticHandler.ServeHTTP(writer, request)
	})
	mux.Handle("/mini-app", pageHandler)
	mux.Handle("/mini-app/", pageHandler)
	return mux
}

func StaticAssetsFromDir(path string) (fs.FS, error) {
	staticFS := os.DirFS(path)
	if _, err := fs.Stat(staticFS, "index.html"); err != nil {
		return nil, err
	}
	return staticFS, nil
}

func StaticAssetsFromEmbed(assets embed.FS, root string) (fs.FS, error) {
	staticFS, err := fs.Sub(assets, root)
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(staticFS, "index.html"); err != nil {
		return nil, err
	}
	return staticFS, nil
}

type commandPayload struct {
	Project   string `json:"project"`
	Name      string `json:"name"`
	Root      string `json:"root"`
	Model     string `json:"model"`
	Reasoning string `json:"reasoning"`
	Mode      string `json:"mode"`
}

func commandAction(options Options, buildCommand func(commandPayload) string) func(http.ResponseWriter, *http.Request, Viewer) {
	return func(writer http.ResponseWriter, request *http.Request, viewer Viewer) {
		if request.Method != http.MethodPost {
			http.Error(writer, "请求方法不被允许", http.StatusMethodNotAllowed)
			return
		}

		var payload commandPayload
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil && err.Error() != "EOF" {
			http.Error(writer, "无效的请求体", http.StatusBadRequest)
			return
		}

		responses := options.Adapter.HandleUpdate(request.Context(), telegram.Update{
			Message: &telegram.Message{
				Chat: telegram.Chat{ID: viewer.ID},
				From: &telegram.User{ID: viewer.ID, Username: viewer.Username},
				Text: buildCommand(payload),
			},
		})

		writeJSON(writer, http.StatusOK, actionResponse{
			OK:        true,
			Responses: responses,
		})
	}
}

func withViewer(options Options, next func(http.ResponseWriter, *http.Request, Viewer)) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		viewer, err := ValidateInitData(request.Header.Get("X-Telegram-Init-Data"), options.BotToken, options.OwnerID)
		if err != nil {
			http.Error(writer, "Mini App 鉴权失败", http.StatusUnauthorized)
			return
		}
		next(writer, request, viewer)
	})
}

func buildBootstrap(ctx context.Context, registry *session.Registry, runtime telegram.SessionRuntime, browser *Browser, viewer Viewer) BootstrapResponse {
	response := BootstrapResponse{
		Viewer:            viewer,
		Sessions:          make([]SessionView, 0),
		Projects:          make([]ProjectView, 0),
		BrowseDefaultPath: browser.DefaultPath(),
		BrowseShortcuts:   browser.Shortcuts(),
	}

	if active, ok := registry.Active(); ok {
		activeView := toSessionView(active)
		response.ActiveSession = &activeView
		if runtime != nil {
			if status, err := runtime.Status(ctx, active); err == nil {
				statusView := toStatusView(status)
				response.ActiveStatus = &statusView
			}
			if models, err := runtime.ListModels(ctx, active); err == nil {
				response.Models = toModelViews(models)
			}
		}
	}

	for _, item := range registry.List() {
		response.Sessions = append(response.Sessions, toSessionView(item))
	}

	allowed := registry.AllowedProjects()
	names := config.SortedProjectNames(allowed)
	for _, name := range names {
		response.Projects = append(response.Projects, ProjectView{
			Name:    name,
			Root:    allowed[name],
			Dynamic: registry.IsDynamicProject(name),
		})
	}
	return response
}

func toStatusView(status session.Status) StatusView {
	return StatusView{
		ThreadID:     status.ThreadID,
		Cwd:          status.Cwd,
		Branch:       status.Branch,
		CodexVersion: status.CodexVersion,
		Effective: ControlSelectionView{
			Model:     status.Effective.Model,
			Reasoning: status.Effective.Reasoning,
			PlanMode:  string(status.Effective.PlanMode),
		},
		Pending: ControlSelectionView{
			Model:     status.Pending.Model,
			Reasoning: status.Pending.Reasoning,
			PlanMode:  string(status.Pending.PlanMode),
		},
		TokenUsage: TokenUsageView{
			ContextWindow: status.TokenUsage.ContextWindow,
			TotalTokens:   status.TokenUsage.TotalTokens,
		},
		HasTokenUsage:       status.HasTokenUsage,
		LocalWritableAttach: status.LocalWritableAttach,
	}
}

func toModelViews(models []appserver.ModelInfo) []ModelView {
	views := make([]ModelView, 0, len(models))
	for _, model := range models {
		views = append(views, ModelView{
			ID:               model.ID,
			Model:            model.Model,
			DefaultReasoning: model.DefaultReasoning,
			Supported:        append([]string(nil), model.Supported...),
		})
	}
	return views
}

func toSessionView(item session.View) SessionView {
	return SessionView{
		Name:    item.Name,
		Project: item.Project,
		Root:    item.Root,
		State:   item.State,
	}
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}
