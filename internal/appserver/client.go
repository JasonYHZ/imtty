package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"imtty/internal/session"

	"github.com/gorilla/websocket"
)

type Client struct {
	url        string
	cwd        string
	httpClient *http.Client

	mu               sync.Mutex
	conn             *websocket.Conn
	nextID           int
	pending          map[string]chan rpcEnvelope
	events           chan Event
	threadID         string
	pendingApproval  *approvalRequest
	pendingUserInput *userInputRequest
	closed           bool
}

type approvalRequest struct {
	ID     string
	Method string
	Params map[string]any
}

type userInputRequest struct {
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

func (c *Client) EnsureThread(ctx context.Context, threadID string) (ThreadState, error) {
	var (
		method string
		params map[string]any
	)
	if strings.TrimSpace(threadID) == "" {
		method = "thread/start"
		params = map[string]any{
			"cwd":                    c.cwd,
			"approvalPolicy":         "on-request",
			"experimentalRawEvents":  false,
			"persistExtendedHistory": false,
		}
	} else {
		method = "thread/resume"
		params = map[string]any{
			"threadId":               threadID,
			"cwd":                    c.cwd,
			"approvalPolicy":         "on-request",
			"persistExtendedHistory": false,
		}
	}

	result, err := c.call(ctx, method, params)
	if err != nil {
		return ThreadState{}, err
	}

	threadMap, ok := objectField(result.Result, "thread")
	if !ok {
		return ThreadState{}, errors.New("missing thread in response")
	}
	newThreadID, _ := threadMap["id"].(string)
	if strings.TrimSpace(newThreadID) == "" {
		return ThreadState{}, errors.New("missing thread id in response")
	}

	c.mu.Lock()
	c.threadID = newThreadID
	c.pendingApproval = nil
	c.pendingUserInput = nil
	c.mu.Unlock()
	return ThreadState{
		ThreadID:  newThreadID,
		Cwd:       stringValue(result.Result["cwd"]),
		Model:     stringValue(result.Result["model"]),
		Reasoning: stringValue(result.Result["reasoningEffort"]),
	}, nil
}

func (c *Client) StartFreshThread(ctx context.Context) (ThreadState, error) {
	return c.EnsureThread(ctx, "")
}

func (c *Client) StartTurn(ctx context.Context, text string) error {
	return c.StartTurnWithOptions(ctx, text, TurnOptions{})
}

func (c *Client) StartTurnWithOptions(ctx context.Context, text string, options TurnOptions) error {
	return c.StartTurnInputs(ctx, []TurnInput{{
		Type: "text",
		Text: text,
	}}, options)
}

func (c *Client) StartTurnInputs(ctx context.Context, inputs []TurnInput, options TurnOptions) error {
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

	params := map[string]any{
		"threadId": threadID,
		"input":    payloadInputs,
	}
	if strings.TrimSpace(options.Model) != "" {
		params["model"] = strings.TrimSpace(options.Model)
	}
	if strings.TrimSpace(options.Reasoning) != "" {
		params["effort"] = strings.TrimSpace(options.Reasoning)
	}

	_, err := c.call(ctx, "turn/start", params)
	return err
}

func (c *Client) ModelList(ctx context.Context) ([]ModelInfo, error) {
	cursor := ""
	models := make([]ModelInfo, 0, 16)
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		result, err := c.call(ctx, "model/list", params)
		if err != nil {
			return nil, err
		}

		data, _ := result.Result["data"].([]any)
		for _, item := range data {
			payload, ok := item.(map[string]any)
			if !ok {
				continue
			}
			models = append(models, ModelInfo{
				ID:               stringValue(payload["id"]),
				Model:            stringValue(payload["model"]),
				DisplayName:      stringValue(payload["displayName"]),
				DefaultReasoning: stringValue(payload["defaultReasoningEffort"]),
				Supported:        stringArray(payload["supportedReasoningEfforts"]),
			})
		}

		cursor = stringValue(result.Result["nextCursor"])
		if cursor == "" {
			break
		}
	}
	return models, nil
}

func (c *Client) ResolveApproval(ctx context.Context, decision Decision) error {
	c.mu.Lock()
	pending := c.pendingApproval
	c.mu.Unlock()

	if pending == nil {
		return errors.New("no pending approval")
	}

	result := approvalResponsePayload(pending, decision)
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

func (c *Client) ResolveUserInput(ctx context.Context, text string) error {
	c.mu.Lock()
	pending := c.pendingUserInput
	c.mu.Unlock()

	if pending == nil {
		return errors.New("no pending user input request")
	}

	result, err := userInputResponsePayload(pending.Params, text)
	if err != nil {
		return err
	}

	if err := c.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      pending.ID,
		"result":  result,
	}); err != nil {
		return err
	}

	c.mu.Lock()
	c.pendingUserInput = nil
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

func (c *Client) HasPendingUserInput() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pendingUserInput != nil
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
			if !c.isClosed() {
				c.markDisconnected(conn)
				c.emitEvent(Event{
					Kind: EventConnectionClosed,
					Text: "Codex app-server 连接已断开",
				})
			}
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

func (c *Client) markDisconnected(conn *websocket.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == conn {
		c.conn = nil
	}
}

func (c *Client) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *Client) handleInboundRequestOrNotification(envelope rpcEnvelope) {
	if envelope.ID != nil && isApprovalMethod(envelope.Method) {
		c.mu.Lock()
		c.pendingApproval = &approvalRequest{
			ID:     fmt.Sprint(envelope.ID),
			Method: envelope.Method,
			Params: envelope.Params,
		}
		c.pendingUserInput = nil
		c.mu.Unlock()

		c.events <- Event{
			Kind: EventApprovalRequested,
			Text: approvalText(envelope.Method, envelope.Params),
		}
		return
	}

	if envelope.ID != nil && envelope.Method == "item/tool/requestUserInput" {
		c.mu.Lock()
		c.pendingUserInput = &userInputRequest{
			ID:     fmt.Sprint(envelope.ID),
			Method: envelope.Method,
			Params: envelope.Params,
		}
		c.pendingApproval = nil
		c.mu.Unlock()

		c.events <- Event{
			Kind: EventUserInputRequested,
			Text: userInputText(envelope.Params),
		}
		return
	}

	switch envelope.Method {
	case "thread/tokenUsage/updated":
		c.events <- Event{
			Kind: EventTokenUsageUpdated,
			TokenUsage: session.TokenUsage{
				ContextWindow: nestedInt64(envelope.Params, "tokenUsage", "modelContextWindow"),
				TotalTokens:   nestedInt64(envelope.Params, "tokenUsage", "total", "totalTokens"),
			},
		}
		return
	case "model/rerouted":
		c.events <- Event{
			Kind:  EventModelRerouted,
			Model: stringValue(envelope.Params["toModel"]),
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
	if (itemType == "error" || phase == "error") && strings.TrimSpace(turnErrorText(item)) != "" {
		c.emitEvent(Event{
			Kind: EventTurnError,
			Text: turnErrorText(item),
		})
		return
	}
	if itemType == "agentMessage" && phase == string(EventFinalAnswer) {
		if isMemoryMaintenanceReport(text) {
			return
		}
		c.emitEvent(Event{
			Kind: EventFinalAnswer,
			Text: text,
		})
	}
}

func (c *Client) emitEvent(event Event) {
	select {
	case c.events <- event:
	default:
	}
}

func turnErrorText(item map[string]any) string {
	if text := stringValue(item["text"]); text != "" {
		return text
	}
	if message := stringValue(item["message"]); message != "" {
		return message
	}
	if errorMap, ok := objectField(item, "error"); ok {
		if message := stringValue(errorMap["message"]); message != "" {
			return message
		}
	}
	return "Codex 会话异常中断"
}

func isApprovalMethod(method string) bool {
	switch method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval", "execCommandApproval", "applyPatchApproval":
		return true
	default:
		return false
	}
}

func isMemoryMaintenanceReport(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasPrefix(trimmed, "Updated [MEMORY.md](") &&
		strings.Contains(trimmed, ".codex/memories/MEMORY.md") &&
		strings.Contains(trimmed, "memory_summary.md")
}

func approvalText(method string, params map[string]any) string {
	var lines []string
	switch method {
	case "item/commandExecution/requestApproval":
		if command, _ := params["command"].(string); strings.TrimSpace(command) != "" {
			lines = append(lines, "需要审批的命令：", command)
		}
	case "execCommandApproval":
		if command := commandArrayText(params["command"]); command != "" {
			lines = append(lines, "需要审批的命令：", command)
		}
		if reason, _ := params["reason"].(string); strings.TrimSpace(reason) != "" {
			lines = append(lines, "原因："+strings.TrimSpace(reason))
		}
	case "item/fileChange/requestApproval", "applyPatchApproval":
		lines = append(lines, "需要确认文件变更。")
		if reason, _ := params["reason"].(string); strings.TrimSpace(reason) != "" {
			lines = append(lines, "原因："+strings.TrimSpace(reason))
		}
		if grantRoot, _ := params["grantRoot"].(string); strings.TrimSpace(grantRoot) != "" {
			lines = append(lines, "授权范围："+strings.TrimSpace(grantRoot))
		}
		if files := fileChangePaths(params["fileChanges"]); len(files) > 0 {
			lines = append(lines, "文件："+strings.Join(files, ", "))
		}
	case "item/permissions/requestApproval":
		lines = append(lines, "需要确认权限请求。")
		if reason, _ := params["reason"].(string); strings.TrimSpace(reason) != "" {
			lines = append(lines, "原因："+strings.TrimSpace(reason))
		}
	}
	if cwd, _ := params["cwd"].(string); strings.TrimSpace(cwd) != "" {
		lines = append(lines, "工作目录："+strings.TrimSpace(cwd))
	}
	lines = append(lines, "请回复 是 或 否。")
	if len(lines) == 1 {
		return "当前操作需要审批，请回复 是 或 否。"
	}
	return strings.Join(lines, "\n")
}

func approvalResponsePayload(pending *approvalRequest, decision Decision) map[string]any {
	value := string(decision)
	if pending.Method == "execCommandApproval" || pending.Method == "applyPatchApproval" {
		if decision == DecisionApprove {
			value = "approved"
		} else {
			value = "denied"
		}
	}
	return map[string]any{
		"decision": value,
	}
}

func userInputText(params map[string]any) string {
	questions := questionList(params)
	if len(questions) == 0 {
		return "Codex 需要补充输入。请直接回复文本。"
	}

	lines := []string{"Codex 需要你补充输入："}
	for _, question := range questions {
		header := stringValue(question["header"])
		text := stringValue(question["question"])
		id := stringValue(question["id"])
		title := strings.TrimSpace(text)
		if header != "" {
			title = header + "：" + title
		}
		if id != "" && len(questions) > 1 {
			title += "（" + id + "）"
		}
		lines = append(lines, title)
		for _, option := range optionList(question["options"]) {
			label := stringValue(option["label"])
			if label == "" {
				continue
			}
			description := stringValue(option["description"])
			if description != "" {
				lines = append(lines, "- "+label+"："+description)
			} else {
				lines = append(lines, "- "+label)
			}
		}
	}
	if len(questions) == 1 {
		lines = append(lines, "请直接回复选项文字或你的答案。")
	} else {
		lines = append(lines, "请每行按 question_id=答案 回复。")
	}
	return strings.Join(lines, "\n")
}

func userInputResponsePayload(params map[string]any, text string) (map[string]any, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, errors.New("当前等待 Codex 输入请求，请回复选项文字或答案")
	}

	questions := questionList(params)
	if len(questions) == 0 {
		return nil, errors.New("当前 Codex 输入请求缺少问题定义")
	}

	answers := make(map[string]any, len(questions))
	if len(questions) == 1 {
		id := stringValue(questions[0]["id"])
		if id == "" {
			return nil, errors.New("当前 Codex 输入请求缺少问题 ID")
		}
		answers[id] = map[string]any{"answers": []string{trimmed}}
		return map[string]any{"answers": answers}, nil
	}

	parsed := parseQuestionAnswers(trimmed)
	for _, question := range questions {
		id := stringValue(question["id"])
		if id == "" {
			return nil, errors.New("当前 Codex 输入请求缺少问题 ID")
		}
		answer := strings.TrimSpace(parsed[id])
		if answer == "" {
			return nil, errors.New("当前等待多个 Codex 输入问题，请每行按 question_id=答案 回复")
		}
		answers[id] = map[string]any{"answers": []string{answer}}
	}
	return map[string]any{"answers": answers}, nil
}

func parseQuestionAnswers(text string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		separator := strings.Index(line, "=")
		if separator < 0 {
			separator = strings.Index(line, ":")
		}
		if separator <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:separator])
		value := strings.TrimSpace(line[separator+1:])
		if key != "" && value != "" {
			result[key] = value
		}
	}
	return result
}

func questionList(params map[string]any) []map[string]any {
	raw, _ := params["questions"].([]any)
	questions := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		question, ok := item.(map[string]any)
		if ok {
			questions = append(questions, question)
		}
	}
	return questions
}

func optionList(value any) []map[string]any {
	raw, _ := value.([]any)
	options := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		option, ok := item.(map[string]any)
		if ok {
			options = append(options, option)
		}
	}
	return options
}

func commandArrayText(value any) string {
	raw, _ := value.([]any)
	parts := make([]string, 0, len(raw))
	for _, item := range raw {
		text := stringValue(item)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

func fileChangePaths(value any) []string {
	changes, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	paths := make([]string, 0, len(changes))
	for path := range changes {
		if strings.TrimSpace(path) != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func objectField(payload map[string]any, key string) (map[string]any, bool) {
	value, ok := payload[key]
	if !ok {
		return nil, false
	}
	result, ok := value.(map[string]any)
	return result, ok
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func stringArray(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text := stringValue(item); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func nestedInt64(payload map[string]any, keys ...string) int64 {
	current := any(payload)
	for _, key := range keys {
		nextMap, ok := current.(map[string]any)
		if !ok {
			return 0
		}
		current, ok = nextMap[key]
		if !ok {
			return 0
		}
	}
	switch value := current.(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	default:
		return 0
	}
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
