package appserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestClientConnectAndStartThread(t *testing.T) {
	server := newFakeAppServer(t)
	defer server.Close()

	server.onRequest("initialize", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{
			"protocolVersion": 1,
		})
	})
	server.onRequest("thread/start", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{
			"thread": map[string]any{
				"id": "thread-123",
			},
		})
	})

	client := NewClient(server.wsURL(), "/tmp/project-a")
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	threadState, err := client.EnsureThread(context.Background(), "")
	if err != nil {
		t.Fatalf("EnsureThread() error = %v", err)
	}

	if threadState.ThreadID != "thread-123" {
		t.Fatalf("threadID = %q, want %q", threadState.ThreadID, "thread-123")
	}

	if got := server.methodLog(); !strings.Contains(got, "initialize") || !strings.Contains(got, "thread/start") {
		t.Fatalf("method log = %q, want initialize and thread/start", got)
	}
}

func TestClientResumeThreadWhenThreadIDExists(t *testing.T) {
	server := newFakeAppServer(t)
	defer server.Close()

	server.onRequest("initialize", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{
			"protocolVersion": 1,
		})
	})
	server.onRequest("thread/resume", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{
			"thread": map[string]any{
				"id": "thread-123",
			},
		})
	})

	client := NewClient(server.wsURL(), "/tmp/project-a")
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	threadState, err := client.EnsureThread(context.Background(), "thread-123")
	if err != nil {
		t.Fatalf("EnsureThread() error = %v", err)
	}

	if threadState.ThreadID != "thread-123" {
		t.Fatalf("threadID = %q, want %q", threadState.ThreadID, "thread-123")
	}

	if got := server.methodLog(); strings.Contains(got, "thread/start") || !strings.Contains(got, "thread/resume") {
		t.Fatalf("method log = %q, want resume only", got)
	}
}

func TestClientEmitsFinalAnswerOnly(t *testing.T) {
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
		server.notify(conn, "item/completed", map[string]any{
			"item": map[string]any{
				"type":  "agentMessage",
				"text":  "中间 commentary",
				"phase": "commentary",
			},
		})
		server.notify(conn, "item/completed", map[string]any{
			"item": map[string]any{
				"type":  "agentMessage",
				"text":  "最终回复",
				"phase": "final_answer",
			},
		})
	})

	client := NewClient(server.wsURL(), "/tmp/project-a")
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	if _, err := client.EnsureThread(context.Background(), ""); err != nil {
		t.Fatalf("EnsureThread() error = %v", err)
	}
	if err := client.StartTurn(context.Background(), "hi"); err != nil {
		t.Fatalf("StartTurn() error = %v", err)
	}

	event := waitForEvent(t, client.Events())
	if event.Kind != EventFinalAnswer {
		t.Fatalf("Event.Kind = %v, want final answer", event.Kind)
	}
	if event.Text != "最终回复" {
		t.Fatalf("Event.Text = %q, want %q", event.Text, "最终回复")
	}
}

func TestClientSuppressesMemoryMaintenanceFinalAnswer(t *testing.T) {
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
		server.notify(conn, "item/completed", map[string]any{
			"item": map[string]any{
				"type": "agentMessage",
				"text": "Updated [MEMORY.md](/Users/jasonyu/.codex/memories/MEMORY.md:4435) and " +
					"[memory_summary.md](/Users/jasonyu/.codex/memories/memory_summary.md:344) incrementally for the one new thread `019dbf38-d9d3-7fa1-9c70-55666ae6674c`.",
				"phase": "final_answer",
			},
		})
	})

	client := NewClient(server.wsURL(), "/tmp/project-a")
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	if _, err := client.EnsureThread(context.Background(), ""); err != nil {
		t.Fatalf("EnsureThread() error = %v", err)
	}
	if err := client.StartTurn(context.Background(), "hi"); err != nil {
		t.Fatalf("StartTurn() error = %v", err)
	}

	waitForNoEvent(t, client.Events())
}

func TestClientStartTurnInputsSendsLocalImageAndCaption(t *testing.T) {
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
		inputs, ok := request.Params["input"].([]any)
		if !ok {
			t.Fatalf("input = %#v, want []any", request.Params["input"])
		}
		if len(inputs) != 2 {
			t.Fatalf("len(input) = %d, want 2", len(inputs))
		}

		imageInput, ok := inputs[0].(map[string]any)
		if !ok {
			t.Fatalf("image input = %#v, want object", inputs[0])
		}
		if imageInput["type"] != "localImage" || imageInput["path"] != "/tmp/test-image.jpg" {
			t.Fatalf("image input = %#v, want localImage /tmp/test-image.jpg", imageInput)
		}

		textInput, ok := inputs[1].(map[string]any)
		if !ok {
			t.Fatalf("text input = %#v, want object", inputs[1])
		}
		if textInput["type"] != "text" || textInput["text"] != "看看这张图" {
			t.Fatalf("text input = %#v, want caption", textInput)
		}

		server.reply(conn, request.ID, map[string]any{
			"turn": map[string]any{"id": "turn-1"},
		})
	})

	client := NewClient(server.wsURL(), "/tmp/project-a")
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	if _, err := client.EnsureThread(context.Background(), ""); err != nil {
		t.Fatalf("EnsureThread() error = %v", err)
	}
	if err := client.StartTurnInputs(context.Background(), []TurnInput{
		{Type: "localImage", Path: "/tmp/test-image.jpg"},
		{Type: "text", Text: "看看这张图"},
	}, TurnOptions{}); err != nil {
		t.Fatalf("StartTurnInputs() error = %v", err)
	}
}

func TestClientStartTurnWithOptionsSendsModelAndReasoningOverride(t *testing.T) {
	server := newFakeAppServer(t)
	defer server.Close()

	server.onRequest("initialize", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{"protocolVersion": 1})
	})
	server.onRequest("thread/start", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{
			"thread": map[string]any{"id": "thread-123"},
			"model":  "gpt-5.5",
		})
	})
	server.onRequest("turn/start", func(conn *websocket.Conn, request rpcRequest) {
		if got := request.Params["model"]; got != "gpt-5.4" {
			t.Fatalf("model = %#v, want gpt-5.4", got)
		}
		if got := request.Params["effort"]; got != "xhigh" {
			t.Fatalf("effort = %#v, want xhigh", got)
		}
		server.reply(conn, request.ID, map[string]any{"turn": map[string]any{"id": "turn-1"}})
	})

	client := NewClient(server.wsURL(), "/tmp/project-a")
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	if _, err := client.EnsureThread(context.Background(), ""); err != nil {
		t.Fatalf("EnsureThread() error = %v", err)
	}
	if err := client.StartTurnWithOptions(context.Background(), "hi", TurnOptions{
		Model:     "gpt-5.4",
		Reasoning: "xhigh",
	}); err != nil {
		t.Fatalf("StartTurnWithOptions() error = %v", err)
	}
}

func TestClientModelListAndProtocolEvents(t *testing.T) {
	server := newFakeAppServer(t)
	defer server.Close()

	server.onRequest("initialize", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{"protocolVersion": 1})
	})
	server.onRequest("model/list", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{
			"data": []map[string]any{{
				"id":                     "gpt-5.4",
				"model":                  "gpt-5.4",
				"displayName":            "GPT-5.4",
				"defaultReasoningEffort": "high",
				"supportedReasoningEfforts": []string{
					"medium",
					"high",
					"xhigh",
				},
				"description": "test",
				"hidden":      false,
				"isDefault":   true,
			}},
		})
	})

	client := NewClient(server.wsURL(), "/tmp/project-a")
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	models, err := client.ModelList(context.Background())
	if err != nil {
		t.Fatalf("ModelList() error = %v", err)
	}
	if len(models) != 1 || models[0].Model != "gpt-5.4" || models[0].DefaultReasoning != "high" {
		t.Fatalf("models = %#v, want one gpt-5.4 default high", models)
	}

	server.withConn(func(conn *websocket.Conn) {
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
	event := waitForEvent(t, client.Events())
	if event.Kind != EventTokenUsageUpdated || event.TokenUsage.ContextWindow != 258000 || event.TokenUsage.TotalTokens != 188000 {
		t.Fatalf("event = %#v, want token usage update", event)
	}

	server.withConn(func(conn *websocket.Conn) {
		server.notify(conn, "model/rerouted", map[string]any{
			"threadId":  "thread-123",
			"turnId":    "turn-1",
			"fromModel": "gpt-5.5",
			"toModel":   "gpt-5.4",
			"reason":    "policy",
		})
	})
	event = waitForEvent(t, client.Events())
	if event.Kind != EventModelRerouted || event.Model != "gpt-5.4" {
		t.Fatalf("event = %#v, want reroute to gpt-5.4", event)
	}
}

func TestClientTracksPendingApprovalRequests(t *testing.T) {
	server := newFakeAppServer(t)
	defer server.Close()

	server.onRequest("initialize", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{"protocolVersion": 1})
	})

	client := NewClient(server.wsURL(), "/tmp/project-a")
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	server.withConn(func(conn *websocket.Conn) {
		server.request(conn, "approval-1", "item/commandExecution/requestApproval", map[string]any{
			"threadId": "thread-123",
			"turnId":   "turn-1",
			"itemId":   "item-1",
			"command":  "brew upgrade --cask codex",
			"reason":   "update codex",
		})
	})

	event := waitForEvent(t, client.Events())
	if event.Kind != EventApprovalRequested {
		t.Fatalf("Event.Kind = %v, want approval request", event.Kind)
	}
	if !strings.Contains(event.Text, "brew upgrade --cask codex") {
		t.Fatalf("Event.Text = %q, want command text", event.Text)
	}
}

func TestClientRespondsToApprovalDecision(t *testing.T) {
	server := newFakeAppServer(t)
	defer server.Close()

	server.onRequest("initialize", func(conn *websocket.Conn, request rpcRequest) {
		server.reply(conn, request.ID, map[string]any{"protocolVersion": 1})
	})

	client := NewClient(server.wsURL(), "/tmp/project-a")
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	server.withConn(func(conn *websocket.Conn) {
		server.request(conn, "approval-1", "item/commandExecution/requestApproval", map[string]any{
			"threadId": "thread-123",
			"turnId":   "turn-1",
			"itemId":   "item-1",
			"command":  "brew upgrade --cask codex",
		})
	})

	event := waitForEvent(t, client.Events())
	if event.Kind != EventApprovalRequested {
		t.Fatalf("Event.Kind = %v, want approval request", event.Kind)
	}

	if err := client.ResolveApproval(context.Background(), DecisionApprove); err != nil {
		t.Fatalf("ResolveApproval() error = %v", err)
	}

	response := server.waitForResponse(t)
	if response.ID != "approval-1" {
		t.Fatalf("response.ID = %#v, want %q", response.ID, "approval-1")
	}

	resultMap, ok := response.Result.(map[string]any)
	if !ok {
		t.Fatalf("response.Result = %#v, want object", response.Result)
	}
	if resultMap["decision"] != "accept" {
		t.Fatalf("decision = %#v, want %q", resultMap["decision"], "accept")
	}
}

func waitForEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()

	select {
	case event := <-events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

func waitForNoEvent(t *testing.T, events <-chan Event) {
	t.Helper()

	select {
	case event := <-events:
		t.Fatalf("unexpected event = %#v", event)
	case <-time.After(200 * time.Millisecond):
	}
}

type fakeAppServer struct {
	t         *testing.T
	upgrader  websocket.Upgrader
	server    *httptest.Server
	conn      *websocket.Conn
	methods   []string
	handlers  map[string]func(*websocket.Conn, rpcRequest)
	responses chan rpcResponse
}

func newFakeAppServer(t *testing.T) *fakeAppServer {
	t.Helper()

	fake := &fakeAppServer{
		t: t,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
		handlers:  make(map[string]func(*websocket.Conn, rpcRequest)),
		responses: make(chan rpcResponse, 8),
	}

	fake.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		conn, err := fake.upgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		fake.conn = conn

		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var envelope map[string]any
			if err := json.Unmarshal(payload, &envelope); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			if method, ok := envelope["method"].(string); ok {
				request := rpcRequest{
					ID:     envelope["id"],
					Method: method,
				}
				if params, ok := envelope["params"].(map[string]any); ok {
					request.Params = params
				}
				fake.methods = append(fake.methods, method)
				if handler := fake.handlers[method]; handler != nil {
					handler(conn, request)
				}
				continue
			}

			fake.responses <- rpcResponse{
				ID:     envelope["id"],
				Result: envelope["result"],
			}
		}
	}))

	return fake
}

func (f *fakeAppServer) Close() {
	if f.conn != nil {
		_ = f.conn.Close()
	}
	f.server.Close()
}

func (f *fakeAppServer) wsURL() string {
	return "ws" + strings.TrimPrefix(f.server.URL, "http")
}

func (f *fakeAppServer) onRequest(method string, handler func(*websocket.Conn, rpcRequest)) {
	f.handlers[method] = handler
}

func (f *fakeAppServer) reply(conn *websocket.Conn, id any, result any) {
	f.write(conn, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func (f *fakeAppServer) notify(conn *websocket.Conn, method string, params any) {
	f.write(conn, map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

func (f *fakeAppServer) request(conn *websocket.Conn, id any, method string, params any) {
	f.write(conn, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
}

func (f *fakeAppServer) write(conn *websocket.Conn, payload any) {
	if err := conn.WriteJSON(payload); err != nil {
		f.t.Fatalf("WriteJSON() error = %v", err)
	}
}

func (f *fakeAppServer) withConn(fn func(*websocket.Conn)) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f.conn != nil {
			fn(f.conn)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.t.Fatal("timed out waiting for websocket connection")
}

func (f *fakeAppServer) methodLog() string {
	return strings.Join(f.methods, ",")
}

func (f *fakeAppServer) waitForResponse(t *testing.T) rpcResponse {
	t.Helper()
	select {
	case response := <-f.responses:
		return response
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rpc response")
		return rpcResponse{}
	}
}

type rpcRequest struct {
	ID     any
	Method string
	Params map[string]any
}

type rpcResponse struct {
	ID     any
	Result any
}
