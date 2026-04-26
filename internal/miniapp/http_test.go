package miniapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"imtty/internal/appserver"
	"imtty/internal/session"
	"imtty/internal/telegram"
)

func TestHandlerBootstrapReturnsSessionsAndProjects(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	mustMkdirAll(t, filepath.Join(homeDir, "workspace", "Personal"))
	mustMkdirAll(t, filepath.Join(homeDir, "workspace", "Playground"))

	registry := session.NewRegistryWithDynamic(map[string]string{
		"project-a": "/tmp/project-a",
	}, map[string]string{
		"project-b": "/tmp/project-b",
	})
	runtime := &fakeSessionRuntime{}
	store := &fakeProjectStore{}
	adapter := telegram.NewAdapter(registry, runtime, store, nil, nil, nil)
	view, err := registry.Open("project-a")
	if err != nil {
		t.Fatalf("registry.Open(project-a) error = %v", err)
	}
	if _, err := registry.SetState(view.Project, session.StateRunning); err != nil {
		t.Fatalf("registry.SetState(project-a) error = %v", err)
	}

	handler := NewHandler(Options{
		BotToken: "bot-token",
		OwnerID:  42,
		Registry: registry,
		Adapter:  adapter,
		BrowseRoots: map[string]string{
			"dotfiles": filepath.Join(homeDir, ".dotfiles"),
		},
		StaticFS:  nil,
		IndexHTML: nil,
	})

	request := httptest.NewRequest(http.MethodGet, "/mini-app/api/bootstrap", nil)
	request.Header.Set("X-Telegram-Init-Data", signedInitData(t, "bot-token", 42, "jason", time.Unix(1_700_000_000, 0)))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response BootstrapResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if response.ActiveSession == nil || response.ActiveSession.Project != "project-a" {
		t.Fatalf("ActiveSession = %#v, want project-a", response.ActiveSession)
	}

	if len(response.Sessions) != 1 || response.Sessions[0].Project != "project-a" {
		t.Fatalf("Sessions = %#v, want one project-a session", response.Sessions)
	}

	if len(response.Projects) != 2 {
		t.Fatalf("Projects = %#v, want 2 projects", response.Projects)
	}

	if !hasDynamicProject(response.Projects, "project-b") {
		t.Fatalf("Projects = %#v, want dynamic project-b", response.Projects)
	}

	if got := response.BrowseDefaultPath; got != homeDir {
		t.Fatalf("BrowseDefaultPath = %q, want %q", got, homeDir)
	}

	if !hasShortcut(response.BrowseShortcuts, "workspace", filepath.Join(homeDir, "workspace")) {
		t.Fatalf("BrowseShortcuts = %#v, want workspace shortcut", response.BrowseShortcuts)
	}
	if !hasShortcut(response.BrowseShortcuts, "Home", homeDir) {
		t.Fatalf("BrowseShortcuts = %#v, want Home shortcut", response.BrowseShortcuts)
	}
	if !hasShortcut(response.BrowseShortcuts, "Root", "/") {
		t.Fatalf("BrowseShortcuts = %#v, want Root shortcut", response.BrowseShortcuts)
	}
	if !hasShortcut(response.BrowseShortcuts, "dotfiles", filepath.Join(homeDir, ".dotfiles")) {
		t.Fatalf("BrowseShortcuts = %#v, want custom shortcut", response.BrowseShortcuts)
	}
}

func TestHandlerRejectsInvalidInitData(t *testing.T) {
	registry := session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	})
	adapter := telegram.NewAdapter(registry, &fakeSessionRuntime{}, &fakeProjectStore{}, nil, nil, nil)
	handler := NewHandler(Options{
		BotToken: "bot-token",
		OwnerID:  42,
		Registry: registry,
		Adapter:  adapter,
	})

	request := httptest.NewRequest(http.MethodGet, "/mini-app/api/bootstrap", nil)
	request.Header.Set("X-Telegram-Init-Data", "bad=data")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestHandlerProjectAddAndRemoveUseAdapterCommands(t *testing.T) {
	registry := session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	})
	runtime := &fakeSessionRuntime{}
	store := &fakeProjectStore{}
	adapter := telegram.NewAdapter(registry, runtime, store, nil, nil, nil)
	handler := NewHandler(Options{
		BotToken: "bot-token",
		OwnerID:  42,
		Registry: registry,
		Adapter:  adapter,
	})

	initData := signedInitData(t, "bot-token", 42, "jason", time.Unix(1_700_000_000, 0))
	addRequest := httptest.NewRequest(http.MethodPost, "/mini-app/api/project-add", strings.NewReader(`{"name":"project-b","root":"/tmp/project-b"}`))
	addRequest.Header.Set("Content-Type", "application/json")
	addRequest.Header.Set("X-Telegram-Init-Data", initData)
	addRecorder := httptest.NewRecorder()
	handler.ServeHTTP(addRecorder, addRequest)

	if addRecorder.Code != http.StatusOK {
		t.Fatalf("add status = %d, want %d body=%s", addRecorder.Code, http.StatusOK, addRecorder.Body.String())
	}

	if got := store.added["project-b"]; got != "/tmp/project-b" {
		t.Fatalf("store.added[project-b] = %q, want %q", got, "/tmp/project-b")
	}

	removeRequest := httptest.NewRequest(http.MethodPost, "/mini-app/api/project-remove", strings.NewReader(`{"name":"project-b"}`))
	removeRequest.Header.Set("Content-Type", "application/json")
	removeRequest.Header.Set("X-Telegram-Init-Data", initData)
	removeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(removeRecorder, removeRequest)

	if removeRecorder.Code != http.StatusOK {
		t.Fatalf("remove status = %d, want %d body=%s", removeRecorder.Code, http.StatusOK, removeRecorder.Body.String())
	}

	if len(store.removed) != 1 || store.removed[0] != "project-b" {
		t.Fatalf("store.removed = %#v, want project-b", store.removed)
	}
}

func TestHandlerServesStaticMiniAppAssets(t *testing.T) {
	staticFS := fstest.MapFS{
		"index.html":     {Data: []byte("<html>mini app</html>")},
		"assets/app.js":  {Data: []byte("console.log('ok')")},
		"assets/app.css": {Data: []byte("body{}")},
	}
	registry := session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	})
	adapter := telegram.NewAdapter(registry, &fakeSessionRuntime{}, &fakeProjectStore{}, nil, nil, nil)
	handler := NewHandler(Options{
		BotToken:  "bot-token",
		OwnerID:   42,
		Registry:  registry,
		Adapter:   adapter,
		StaticFS:  staticFS,
		IndexHTML: []byte("<html>mini app</html>"),
	})

	pageRequest := httptest.NewRequest(http.MethodGet, "/mini-app", nil)
	pageRecorder := httptest.NewRecorder()
	handler.ServeHTTP(pageRecorder, pageRequest)
	if pageRecorder.Code != http.StatusOK {
		t.Fatalf("page status = %d, want %d", pageRecorder.Code, http.StatusOK)
	}

	assetRequest := httptest.NewRequest(http.MethodGet, "/mini-app/assets/app.js", nil)
	assetRecorder := httptest.NewRecorder()
	handler.ServeHTTP(assetRecorder, assetRequest)
	if assetRecorder.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", assetRecorder.Code, http.StatusOK)
	}
	if body := assetRecorder.Body.String(); !strings.Contains(body, "console.log") {
		t.Fatalf("asset body = %q, want js content", body)
	}
}

func TestHandlerProjectBrowseReturnsDirectoriesAndParentPaths(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	targetDir := filepath.Join(homeDir, "workspace", "apps")
	mustMkdirAll(t, filepath.Join(targetDir, "imtty"))
	if err := os.WriteFile(filepath.Join(targetDir, "README.md"), []byte("ignore"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}

	registry := session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	})
	adapter := telegram.NewAdapter(registry, &fakeSessionRuntime{}, &fakeProjectStore{}, nil, nil, nil)
	handler := NewHandler(Options{
		BotToken: "bot-token",
		OwnerID:  42,
		Registry: registry,
		Adapter:  adapter,
	})

	initData := signedInitData(t, "bot-token", 42, "jason", time.Unix(1_700_000_000, 0))

	browseRequest := httptest.NewRequest(http.MethodGet, "/mini-app/api/project-browse?path="+targetDir, nil)
	browseRequest.Header.Set("X-Telegram-Init-Data", initData)
	browseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(browseRecorder, browseRequest)

	if browseRecorder.Code != http.StatusOK {
		t.Fatalf("browse status = %d, want %d body=%s", browseRecorder.Code, http.StatusOK, browseRecorder.Body.String())
	}

	var browseResponse BrowseResponse
	if err := json.Unmarshal(browseRecorder.Body.Bytes(), &browseResponse); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if browseResponse.CurrentAbsolutePath != targetDir {
		t.Fatalf("CurrentAbsolutePath = %q, want %q", browseResponse.CurrentAbsolutePath, targetDir)
	}
	if browseResponse.ParentAbsolutePath != filepath.Dir(targetDir) {
		t.Fatalf("ParentAbsolutePath = %q, want %q", browseResponse.ParentAbsolutePath, filepath.Dir(targetDir))
	}
	if len(browseResponse.Directories) != 1 || browseResponse.Directories[0].Name != "imtty" {
		t.Fatalf("Directories = %#v, want one imtty dir", browseResponse.Directories)
	}
	if browseResponse.Directories[0].AbsolutePath != filepath.Join(targetDir, "imtty") {
		t.Fatalf("Directories[0].AbsolutePath = %q, want %q", browseResponse.Directories[0].AbsolutePath, filepath.Join(targetDir, "imtty"))
	}
}

func TestHandlerProjectBrowseRejectsRelativePath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	registry := session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	})
	adapter := telegram.NewAdapter(registry, &fakeSessionRuntime{}, &fakeProjectStore{}, nil, nil, nil)
	handler := NewHandler(Options{
		BotToken: "bot-token",
		OwnerID:  42,
		Registry: registry,
		Adapter:  adapter,
	})

	initData := signedInitData(t, "bot-token", 42, "jason", time.Unix(1_700_000_000, 0))
	request := httptest.NewRequest(http.MethodGet, "/mini-app/api/project-browse?path=workspace/apps", nil)
	request.Header.Set("X-Telegram-Init-Data", initData)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

type fakeSessionRuntime struct {
	opened    []string
	closed    []string
	killed    []string
	submitted map[string][]string
}

func (f *fakeSessionRuntime) OpenSession(_ context.Context, chatID int64, view session.View) error {
	f.opened = append(f.opened, strconv.FormatInt(chatID, 10)+":"+view.Name)
	return nil
}

func (f *fakeSessionRuntime) OpenSessionWithThreadID(_ context.Context, chatID int64, view session.View, threadID string) error {
	f.opened = append(f.opened, strconv.FormatInt(chatID, 10)+":"+view.Name+":"+threadID)
	return nil
}

func (f *fakeSessionRuntime) CloseSession(sessionName string) {
	f.closed = append(f.closed, sessionName)
}

func (f *fakeSessionRuntime) SubmitText(_ context.Context, _ int64, view session.View, text string) error {
	if f.submitted == nil {
		f.submitted = make(map[string][]string)
	}
	f.submitted[view.Name] = append(f.submitted[view.Name], text)
	return nil
}

func (f *fakeSessionRuntime) SubmitImage(_ context.Context, _ int64, view session.View, imagePath string, caption string) error {
	if f.submitted == nil {
		f.submitted = make(map[string][]string)
	}
	f.submitted[view.Name] = append(f.submitted[view.Name], imagePath+"|"+caption)
	return nil
}

func (f *fakeSessionRuntime) SubmitApproval(_ context.Context, _ int64, _ session.View, _ string) (bool, error) {
	return false, nil
}

func (f *fakeSessionRuntime) Status(_ context.Context, view session.View) (session.Status, error) {
	return session.Status{
		View: view,
		Effective: session.ControlSelection{
			Model:     "gpt-5.5",
			Reasoning: "high",
			PlanMode:  session.PlanModeDefault,
		},
		Cwd:          view.Root,
		ThreadID:     "thread-123",
		CodexVersion: "0.125.0",
	}, nil
}

func (f *fakeSessionRuntime) ListModels(_ context.Context, _ session.View) ([]appserver.ModelInfo, error) {
	return []appserver.ModelInfo{{
		ID:               "gpt-5.5",
		Model:            "gpt-5.5",
		DefaultReasoning: "high",
		Supported:        []string{"medium", "high", "xhigh"},
	}}, nil
}

func (f *fakeSessionRuntime) SetModel(ctx context.Context, view session.View, model string) (session.Status, string, error) {
	status, _ := f.Status(ctx, view)
	status.Pending.Model = model
	return status, "", nil
}

func (f *fakeSessionRuntime) SetReasoning(ctx context.Context, view session.View, reasoning string) (session.Status, error) {
	status, _ := f.Status(ctx, view)
	status.Pending.Reasoning = reasoning
	return status, nil
}

func (f *fakeSessionRuntime) SetPlanMode(ctx context.Context, view session.View, mode session.PlanMode) (session.Status, error) {
	status, _ := f.Status(ctx, view)
	status.Pending.PlanMode = mode
	return status, nil
}

func (f *fakeSessionRuntime) ClearThread(ctx context.Context, view session.View) (session.Status, error) {
	status, _ := f.Status(ctx, view)
	status.ThreadID = "thread-cleared"
	status.HasTokenUsage = false
	status.TokenUsage = session.TokenUsage{}
	return status, nil
}

func (f *fakeSessionRuntime) IsLocallyAttached(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (f *fakeSessionRuntime) KillSession(_ context.Context, view session.View) error {
	f.killed = append(f.killed, view.Name)
	return nil
}

type fakeProjectStore struct {
	added   map[string]string
	removed []string
}

func (f *fakeProjectStore) AddProject(name string, root string) error {
	if f.added == nil {
		f.added = make(map[string]string)
	}
	f.added[name] = root
	return nil
}

func (f *fakeProjectStore) RemoveProject(name string) error {
	f.removed = append(f.removed, name)
	return nil
}

func hasDynamicProject(projects []ProjectView, name string) bool {
	for _, project := range projects {
		if project.Name == name && project.Dynamic {
			return true
		}
	}
	return false
}

func hasShortcut(shortcuts []BrowseShortcutView, name string, path string) bool {
	for _, shortcut := range shortcuts {
		if shortcut.Name == name && shortcut.Path == path {
			return true
		}
	}
	return false
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
}
