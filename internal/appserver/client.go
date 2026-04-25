package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type Client struct {
	url        string
	cwd        string
	httpClient *http.Client

	mu              sync.Mutex
	conn            *websocket.Conn
	nextID          int
	pending         map[string]chan rpcEnvelope
	events          chan Event
	threadID        string
	pendingApproval *approvalRequest
	closed          bool
}

type approvalRequest struct {
	ID     string
	Method string
	Params map[string]any
}

func NewClient(url string, cwd string) *Client {
	return &Client{
		url:     url,
		cwd:     cwd,
		pending: make(map[string]chan rpcEnvelope),
		events:  make(chan Event, 16),
	}
}

func (c *Client) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, c.url, nil)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	go c.readLoop()

	_, err = c.call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "imtty",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": false,
		},
	})
	return err
}

func (c *Client) EnsureThread(ctx context.Context, threadID string) (string, error) {
	var (
		method string
		params map[string]any
	)
	if strings.TrimSpace(threadID) == "" {
		method = "thread/start"
		params = map[string]any{
			"cwd":                  c.cwd,
			"approvalPolicy":       "on-request",
			"experimentalRawEvents": false,
			"persistExtendedHistory": false,
		}
	} else {
		method = "thread/resume"
		params = map[string]any{
			"threadId":             threadID,
			"cwd":                  c.cwd,
			"approvalPolicy":       "on-request",
			"persistExtendedHistory": false,
		}
	}

	result, err := c.call(ctx, method, params)
	if err != nil {
		return "", err
	}

	threadMap, ok := objectField(result.Result, "thread")
	if !ok {
		return "", errors.New("missing thread in response")
	}
	newThreadID, _ := threadMap["id"].(string)
	if strings.TrimSpace(newThreadID) == "" {
		return "", errors.New("missing thread id in response")
	}

	c.mu.Lock()
	c.threadID = newThreadID
	c.mu.Unlock()
	return newThreadID, nil
}

func (c *Client) StartTurn(ctx context.Context, text string) error {
	return c.StartTurnInputs(ctx, []TurnInput{{
		Type: "text",
		Text: text,
	}})
}

func (c *Client) StartTurnInputs(ctx context.Context, inputs []TurnInput) error {
	c.mu.Lock()
	threadID := c.threadID
	c.mu.Unlock()

	if strings.TrimSpace(threadID) == "" {
		return errors.New("thread is not initialized")
	}

	payloadInputs := make([]map[string]any, 0, len(inputs))
	for _, input := range inputs {
		switch input.Type {
		case "text":
			payloadInputs = append(payloadInputs, map[string]any{
				"type":          "text",
				"text":          input.Text,
				"text_elements": []any{},
			})
		case "localImage":
			payloadInputs = append(payloadInputs, map[string]any{
				"type": "localImage",
				"path": input.Path,
			})
		default:
			return fmt.Errorf("unsupported turn input type %q", input.Type)
		}
	}
	if len(payloadInputs) == 0 {
		return errors.New("turn inputs must not be empty")
	}

	_, err := c.call(ctx, "turn/start", map[string]any{
		"threadId": threadID,
		"input":    payloadInputs,
	})
	return err
}

func (c *Client) ResolveApproval(ctx context.Context, decision Decision) error {
	c.mu.Lock()
	pending := c.pendingApproval
	c.mu.Unlock()

	if pending == nil {
		return errors.New("no pending approval")
	}

	result := map[string]any{
		"decision": string(decision),
	}
	if pending.Method == "item/permissions/requestApproval" {
		result = map[string]any{
			"permissions": pending.Params["permissions"],
			"scope":       "turn",
		}
	}

	if err := c.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      pending.ID,
		"result":  result,
	}); err != nil {
		return err
	}

	c.mu.Lock()
	c.pendingApproval = nil
	c.mu.Unlock()
	return nil
}

func (c *Client) Events() <-chan Event {
	return c.events
}

func (c *Client) HasPendingApproval() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pendingApproval != nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()

	if conn != nil {
		return conn.Close()
	}
	return nil
}

func (c *Client) call(ctx context.Context, method string, params any) (rpcEnvelope, error) {
	id, responseCh, err := c.preparePending()
	if err != nil {
		return rpcEnvelope{}, err
	}
	defer c.clearPending(id)

	if err := c.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}); err != nil {
		return rpcEnvelope{}, err
	}

	select {
	case response := <-responseCh:
		if response.Error != nil {
			return rpcEnvelope{}, fmt.Errorf("rpc %s failed: %v", method, response.Error)
		}
		return response, nil
	case <-ctx.Done():
		return rpcEnvelope{}, ctx.Err()
	}
}

func (c *Client) preparePending() (string, chan rpcEnvelope, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return "", nil, errors.New("client is not connected")
	}

	c.nextID++
	id := fmt.Sprintf("%d", c.nextID)
	responseCh := make(chan rpcEnvelope, 1)
	c.pending[id] = responseCh
	return id, responseCh, nil
}

func (c *Client) clearPending(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, id)
}

func (c *Client) writeJSON(payload any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return errors.New("client is not connected")
	}
	return c.conn.WriteJSON(payload)
}

func (c *Client) readLoop() {
	for {
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			return
		}

		var envelope rpcEnvelope
		if err := conn.ReadJSON(&envelope); err != nil {
			return
		}

		if envelope.Method != "" {
			c.handleInboundRequestOrNotification(envelope)
			continue
		}

		id := fmt.Sprint(envelope.ID)
		c.mu.Lock()
		responseCh := c.pending[id]
		c.mu.Unlock()
		if responseCh != nil {
			responseCh <- envelope
		}
	}
}

func (c *Client) handleInboundRequestOrNotification(envelope rpcEnvelope) {
	if envelope.ID != nil && isApprovalMethod(envelope.Method) {
		c.mu.Lock()
		c.pendingApproval = &approvalRequest{
			ID:     fmt.Sprint(envelope.ID),
			Method: envelope.Method,
			Params: envelope.Params,
		}
		c.mu.Unlock()

		c.events <- Event{
			Kind: EventApprovalRequested,
			Text: approvalText(envelope.Method, envelope.Params),
		}
		return
	}

	if envelope.Method != "item/completed" {
		return
	}

	item, ok := objectField(envelope.Params, "item")
	if !ok {
		return
	}
	itemType, _ := item["type"].(string)
	phase, _ := item["phase"].(string)
	text, _ := item["text"].(string)
	if itemType == "agentMessage" && phase == string(EventFinalAnswer) {
		c.events <- Event{
			Kind: EventFinalAnswer,
			Text: text,
		}
	}
}

func isApprovalMethod(method string) bool {
	switch method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval":
		return true
	default:
		return false
	}
}

func approvalText(method string, params map[string]any) string {
	switch method {
	case "item/commandExecution/requestApproval":
		if command, _ := params["command"].(string); strings.TrimSpace(command) != "" {
			return "需要审批的命令：\n" + command
		}
	case "item/fileChange/requestApproval":
		if reason, _ := params["reason"].(string); strings.TrimSpace(reason) != "" {
			return "需要确认文件变更：\n" + reason
		}
	case "item/permissions/requestApproval":
		if reason, _ := params["reason"].(string); strings.TrimSpace(reason) != "" {
			return "需要确认权限请求：\n" + reason
		}
	}
	return "当前操作需要审批，请回复 是 或 否"
}

func objectField(payload map[string]any, key string) (map[string]any, bool) {
	value, ok := payload[key]
	if !ok {
		return nil, false
	}
	result, ok := value.(map[string]any)
	return result, ok
}

type rpcEnvelope struct {
	JSONRPC string         `json:"jsonrpc,omitempty"`
	ID      any            `json:"id,omitempty"`
	Method  string         `json:"method,omitempty"`
	Params  map[string]any `json:"params,omitempty"`
	Result  map[string]any `json:"result,omitempty"`
	Error   any            `json:"error,omitempty"`
}

func (e *rpcEnvelope) UnmarshalJSON(data []byte) error {
	type alias rpcEnvelope
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	var target alias
	if value, ok := raw["jsonrpc"]; ok {
		if err := json.Unmarshal(value, &target.JSONRPC); err != nil {
			return err
		}
	}
	if value, ok := raw["id"]; ok {
		var id any
		if err := json.Unmarshal(value, &id); err != nil {
			return err
		}
		target.ID = id
	}
	if value, ok := raw["method"]; ok {
		if err := json.Unmarshal(value, &target.Method); err != nil {
			return err
		}
	}
	if value, ok := raw["params"]; ok {
		if err := json.Unmarshal(value, &target.Params); err != nil {
			return err
		}
	}
	if value, ok := raw["result"]; ok {
		if err := json.Unmarshal(value, &target.Result); err != nil {
			return err
		}
	}
	if value, ok := raw["error"]; ok {
		var errObj any
		if err := json.Unmarshal(value, &errObj); err != nil {
			return err
		}
		target.Error = errObj
	}
	*e = rpcEnvelope(target)
	return nil
}
