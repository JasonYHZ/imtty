package miniapp

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"

	"imtty/internal/config"
	"imtty/internal/session"
	"imtty/internal/telegram"
)

type Options struct {
	BotToken    string
	OwnerID     int64
	Registry    *session.Registry
	Adapter     *telegram.Adapter
	BrowseRoots map[string]string
	StaticFS    fs.FS
}

func NewHandler(options Options) http.Handler {
	browser := NewBrowser(options.BrowseRoots)
	mux := http.NewServeMux()
	mux.Handle("/mini-app/api/bootstrap", withViewer(options, func(writer http.ResponseWriter, _ *http.Request, viewer Viewer) {
		writeJSON(writer, http.StatusOK, buildBootstrap(options.Registry, browser, viewer))
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
	mux.Handle("/mini-app/api/project-add", withViewer(options, commandAction(options, func(payload commandPayload) string {
		return "/project_add " + payload.Name + " " + payload.Root
	})))
	mux.Handle("/mini-app/api/project-remove", withViewer(options, commandAction(options, func(payload commandPayload) string {
		return "/project_remove " + payload.Name
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
	Project string `json:"project"`
	Name    string `json:"name"`
	Root    string `json:"root"`
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

func buildBootstrap(registry *session.Registry, browser *Browser, viewer Viewer) BootstrapResponse {
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
