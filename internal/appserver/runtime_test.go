package appserver

import (
	"context"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"imtty/internal/session"
	"imtty/internal/stream"
	"imtty/internal/tmux"

	"github.com/gorilla/websocket"
)

func TestRuntimeSubmitTextStartsTypingAndStopsAfterFinalAnswer(t *testing.T) {
	server := newFakeAppServer(t)
	defer server.Close()

	server.onRequest("initialize", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{"protocolVersion": 1})
	})
	server.onRequest("thread/start", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{
			"thread": map[string]any{"id": "thread-123"},
		})
	})
	server.onRequest("model/list", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{
			"data": []map[string]any{{
				"id":                     "gpt-5.5",
				"model":                  "gpt-5.5",
				"displayName":            "GPT-5.5",
				"defaultReasoningEffort": "high",
				"supportedReasoningEfforts": []string{
					"medium", "high", "xhigh",
				},
				"description": "test",
				"hidden":      false,
				"isDefault":   true,
			}},
		})
	})
	server.onRequest("turn/start", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{
			"turn": map[string]any{"id": "turn-1"},
		})
	})

	wsURL, err := url.Parse(server.wsURL())
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	port, err := strconv.Atoi(wsURL.Port())
	if err != nil {
		t.Fatalf("Atoi(port) error = %v", err)
	}

	host := &fakeSessionHost{meta: tmux.SessionRuntimeInfo{Port: port}}
	sender := &fakeRuntimeSender{}
	runtime := NewRuntime(host, stream.NewFormatter(3500), sender, "codex", "xhigh")
	view := session.View{
		Name:    "codex-project-a",
		Project: "project-a",
		Root:    "/tmp/project-a",
	}

	if err := runtime.OpenSession(context.Background(), 42, view); err != nil {
		t.Fatalf("OpenSession() error = %v", err)
	}
	defer runtime.CloseSession(view.Name)

	if err := runtime.SubmitText(context.Background(), 42, view, "hi"); err != nil {
		t.Fatalf("SubmitText() error = %v", err)
	}

	waitForCondition(t, func() bool {
		return sender.chatActionCount("typing") > 0
	})

	server.withConn(func(conn *websocket.Conn) {
		server.notify(conn, "item/completed", map[string]any{
			"item": map[string]any{
				"type":  "agentMessage",
				"text":  "最终回复",
				"phase": "final_answer",
			},
		})
	})

	waitForCondition(t, func() bool {
		return sender.messageCount() == 1
	})

	initialActions := sender.chatActionCount("typing")
	time.Sleep(200 * time.Millisecond)
	if sender.chatActionCount("typing") != initialActions {
		t.Fatalf("typing chat actions kept increasing after final answer")
	}
}

func TestRuntimeAppliesPendingControlsOnNextTurnAndTracksTokenUsage(t *testing.T) {
	server := newFakeAppServer(t)
	defer server.Close()

	server.onRequest("initialize", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{"protocolVersion": 1})
	})
	server.onRequest("thread/start", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{
			"thread":          map[string]any{"id": "thread-123"},
			"cwd":             "/tmp/project-a",
			"model":           "gpt-5.5",
			"reasoningEffort": "high",
		})
	})
	server.onRequest("model/list", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{
			"data": []map[string]any{{
				"id":                     "gpt-5.5",
				"model":                  "gpt-5.5",
				"displayName":            "GPT-5.5",
				"defaultReasoningEffort": "high",
				"supportedReasoningEfforts": []string{
					"medium", "high", "xhigh",
				},
				"description": "test",
				"hidden":      false,
				"isDefault":   true,
			}, {
				"id":                     "gpt-5.4",
				"model":                  "gpt-5.4",
				"displayName":            "GPT-5.4",
				"defaultReasoningEffort": "high",
				"supportedReasoningEfforts": []string{
					"medium", "high", "xhigh",
				},
				"description": "test",
				"hidden":      false,
				"isDefault":   false,
			}},
		})
	})
	server.onRequest("turn/start", func(conn *websocket.Conn, request rpcRequest) {
		if got := request.Params["model"]; got != "gpt-5.4" {
			t.Fatalf("turn model = %#v, want gpt-5.4", got)
		}
		if got := request.Params["effort"]; got != "xhigh" {
			t.Fatalf("turn effort = %#v, want xhigh", got)
		}
		server.reply(conn, request.ID, map[string]any{
			"turn": map[string]any{"id": "turn-1"},
		})
		server.notify(conn, "thread/tokenUsage/updated", map[string]any{
			"threadId": "thread-123",
			"turnId":   "turn-1",
			"tokenUsage": map[string]any{
				"modelContextWindow": 258000,
				"total": map[string]any{
					"totalTokens": 188000,
				},
			},
		})
	})

	wsURL, err := url.Parse(server.wsURL())
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	port, err := strconv.Atoi(wsURL.Port())
	if err != nil {
		t.Fatalf("Atoi(port) error = %v", err)
	}

	host := &fakeSessionHost{meta: tmux.SessionRuntimeInfo{Port: port}}
	sender := &fakeRuntimeSender{}
	runtime := NewRuntime(host, stream.NewFormatter(3500), sender, "codex", "xhigh")
	runtime.resolveBranch = func(context.Context, string) string { return "main" }
	runtime.resolveCodexVersion = func(context.Context, string) string { return "0.125.0" }

	view := session.View{Name: "codex-project-a", Project: "project-a", Root: "/tmp/project-a", State: session.StateRunning}
	if err := runtime.OpenSession(context.Background(), 42, view); err != nil {
		t.Fatalf("OpenSession() error = %v", err)
	}
	defer runtime.CloseSession(view.Name)

	if _, _, err := runtime.SetModel(context.Background(), view, "gpt-5.4"); err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if _, err := runtime.SetReasoning(context.Background(), view, "xhigh"); err != nil {
		t.Fatalf("SetReasoning() error = %v", err)
	}
	if err := runtime.SubmitText(context.Background(), 42, view, "hello"); err != nil {
		t.Fatalf("SubmitText() error = %v", err)
	}

	waitForCondition(t, func() bool {
		host.mu.Lock()
		defer host.mu.Unlock()
		return len(host.metadataWrites) > 0
	})

	status, err := runtime.Status(context.Background(), view)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Effective.Model != "gpt-5.4" || status.Effective.Reasoning != "xhigh" {
		t.Fatalf("Effective = %#v, want gpt-5.4/xhigh", status.Effective)
	}
	if status.Pending.Model != "" || status.Pending.Reasoning != "" {
		t.Fatalf("Pending = %#v, want cleared after turn", status.Pending)
	}

	waitForCondition(t, func() bool {
		status, _ := runtime.Status(context.Background(), view)
		return status.HasTokenUsage
	})
	status, _ = runtime.Status(context.Background(), view)
	if status.TokenUsage.ContextWindow != 258000 || status.TokenUsage.TotalTokens != 188000 {
		t.Fatalf("TokenUsage = %#v, want 258000/188000", status.TokenUsage)
	}
}

func TestRuntimeClearThreadStartsFreshThreadAndResetsSnapshot(t *testing.T) {
	server := newFakeAppServer(t)
	defer server.Close()

	threadStarts := 0
	server.onRequest("initialize", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{"protocolVersion": 1})
	})
	server.onRequest("thread/start", func(conn *websocket.Conn, request rpcRequest) {
		threadStarts++
		threadID := "thread-123"
		if threadStarts == 2 {
			threadID = "thread-456"
		}
		server.reply(conn, request.ID, map[string]any{
			"thread":          map[string]any{"id": threadID},
			"cwd":             "/tmp/project-a",
			"model":           "gpt-5.5",
			"reasoningEffort": "high",
		})
	})
	server.onRequest("model/list", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{
			"data": []map[string]any{{
				"id":                     "gpt-5.5",
				"model":                  "gpt-5.5",
				"displayName":            "GPT-5.5",
				"defaultReasoningEffort": "high",
				"supportedReasoningEfforts": []string{
					"medium", "high", "xhigh",
				},
			}},
		})
	})

	wsURL, err := url.Parse(server.wsURL())
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	port, err := strconv.Atoi(wsURL.Port())
	if err != nil {
		t.Fatalf("Atoi(port) error = %v", err)
	}

	host := &fakeSessionHost{
		meta: tmux.SessionRuntimeInfo{
			Port: port,
			Metadata: session.RuntimeMetadata{
				Pending: session.ControlSelection{
					Model:     "gpt-5.4",
					Reasoning: "xhigh",
					PlanMode:  session.PlanModePlan,
				},
				TokenUsage: session.TokenUsage{
					ContextWindow: 258000,
					TotalTokens:   188000,
				},
			},
		},
	}
	sender := &fakeRuntimeSender{}
	runtime := NewRuntime(host, stream.NewFormatter(3500), sender, "codex", "xhigh")
	view := session.View{Name: "codex-project-a", Project: "project-a", Root: "/tmp/project-a", State: session.StateRunning}

	if err := runtime.OpenSession(context.Background(), 42, view); err != nil {
		t.Fatalf("OpenSession() error = %v", err)
	}
	defer runtime.CloseSession(view.Name)

	status, err := runtime.ClearThread(context.Background(), view)
	if err != nil {
		t.Fatalf("ClearThread() error = %v", err)
	}
	if status.ThreadID != "thread-456" {
		t.Fatalf("ThreadID = %q, want thread-456", status.ThreadID)
	}
	if status.HasTokenUsage {
		t.Fatalf("HasTokenUsage = true, want false after clear")
	}
	if status.Effective.Model != "gpt-5.5" || status.Effective.Reasoning != "high" || status.Effective.PlanMode != session.PlanModeDefault {
		t.Fatalf("Effective = %#v, want preserved effective controls", status.Effective)
	}
	if status.Pending.Model != "gpt-5.4" || status.Pending.Reasoning != "xhigh" || status.Pending.PlanMode != session.PlanModePlan {
		t.Fatalf("Pending = %#v, want preserved pending controls", status.Pending)
	}

	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.threadIDs) == 0 || host.threadIDs[len(host.threadIDs)-1] != "thread-456" {
		t.Fatalf("threadIDs = %#v, want thread-456 persisted", host.threadIDs)
	}
	if len(host.metadataWrites) == 0 {
		t.Fatalf("metadataWrites = %#v, want clear metadata write", host.metadataWrites)
	}
	got := host.metadataWrites[len(host.metadataWrites)-1]
	if got.TokenUsage.ContextWindow != 0 || got.TokenUsage.TotalTokens != 0 {
		t.Fatalf("last metadata TokenUsage = %#v, want zero snapshot", got.TokenUsage)
	}
	if got.Pending.Model != "gpt-5.4" || got.Pending.Reasoning != "xhigh" || got.Pending.PlanMode != session.PlanModePlan {
		t.Fatalf("last metadata Pending = %#v, want preserved pending", got.Pending)
	}
}

type fakeSessionHost struct {
	meta           tmux.SessionRuntimeInfo
	mu             sync.Mutex
	metadataWrites []session.RuntimeMetadata
	threadIDs      []string
}

func (f *fakeSessionHost) EnsureSession(context.Context, string, string) error { return nil }
func (f *fakeSessionHost) SessionMetadata(context.Context, string) (tmux.SessionRuntimeInfo, error) {
	return f.meta, nil
}
func (f *fakeSessionHost) SetThreadID(_ context.Context, _ string, threadID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.threadIDs = append(f.threadIDs, threadID)
	f.meta.ThreadID = threadID
	return nil
}
func (f *fakeSessionHost) SetRuntimeMetadata(_ context.Context, _ string, metadata session.RuntimeMetadata) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.metadataWrites = append(f.metadataWrites, metadata)
	return nil
}
func (f *fakeSessionHost) HasWritableAttachedClients(context.Context, string) (bool, error) {
	return false, nil
}
func (f *fakeSessionHost) KillSession(context.Context, string) error { return nil }

type fakeRuntimeSender struct {
	mu          sync.Mutex
	messages    []stream.OutboundMessage
	chatActions []string
}

func (f *fakeRuntimeSender) SendMessage(_ context.Context, _ int64, message stream.OutboundMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, message)
	return nil
}

func (f *fakeRuntimeSender) SendChatAction(_ context.Context, _ int64, action string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chatActions = append(f.chatActions, action)
	return nil
}

func (f *fakeRuntimeSender) chatActionCount(action string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, item := range f.chatActions {
		if item == action {
			count++
		}
	}
	return count
}

func (f *fakeRuntimeSender) messageCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

func waitForCondition(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
