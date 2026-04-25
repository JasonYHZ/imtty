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
	runtime := NewRuntime(host, stream.NewFormatter(3500), sender)
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

type fakeSessionHost struct {
	meta tmux.SessionRuntimeInfo
}

func (f *fakeSessionHost) EnsureSession(context.Context, string, string) error { return nil }
func (f *fakeSessionHost) SessionMetadata(context.Context, string) (tmux.SessionRuntimeInfo, error) {
	return f.meta, nil
}
func (f *fakeSessionHost) SetThreadID(context.Context, string, string) error { return nil }
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
