package appserver

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"imtty/internal/session"
	"imtty/internal/stream"
	"imtty/internal/tmux"
)

var ErrApprovalReplyRequired = errors.New("当前等待审批，请先回复 是 或 否")

var ErrUserInputReplyRequired = errors.New("当前等待 Codex 输入请求，请回复选项文字或答案")

type SessionHost interface {
	EnsureSession(ctx context.Context, sessionName string, root string) error
	SessionMetadata(ctx context.Context, sessionName string) (tmux.SessionRuntimeInfo, error)
	SetThreadID(ctx context.Context, sessionName string, threadID string) error
	SetRuntimeMetadata(ctx context.Context, sessionName string, metadata session.RuntimeMetadata) error
	HasWritableAttachedClients(ctx context.Context, sessionName string) (bool, error)
	KillSession(ctx context.Context, sessionName string) error
}

type Sender interface {
	SendMessage(ctx context.Context, chatID int64, message stream.OutboundMessage) error
	SendChatAction(ctx context.Context, chatID int64, action string) error
}

type Runtime struct {
	host                SessionHost
	formatter           stream.Formatter
	sender              Sender
	codexBin            string
	planModeReasoning   string
	resolveBranch       func(context.Context, string) string
	resolveCodexVersion func(context.Context, string) string

	mu       sync.Mutex
	sessions map[string]*liveSession

	versionOnce sync.Once
	version     string
}

type liveSession struct {
	chatID       int64
	client       *Client
	stopCh       chan struct{}
	typingStopCh chan struct{}

	mu            sync.Mutex
	effective     session.ControlSelection
	pending       session.ControlSelection
	threadID      string
	cwd           string
	models        []ModelInfo
	tokenUsage    session.TokenUsage
	hasTokenUsage bool
}

func NewRuntime(host SessionHost, formatter stream.Formatter, sender Sender, codexBin string, planModeReasoning string) *Runtime {
	if strings.TrimSpace(planModeReasoning) == "" {
		planModeReasoning = "xhigh"
	}
	return &Runtime{
		host:                host,
		formatter:           formatter,
		sender:              sender,
		codexBin:            codexBin,
		planModeReasoning:   planModeReasoning,
		resolveBranch:       gitBranch,
		resolveCodexVersion: codexVersion,
		sessions:            make(map[string]*liveSession),
	}
}

func (r *Runtime) OpenSession(ctx context.Context, chatID int64, view session.View) error {
	return r.openSession(ctx, chatID, view, "", false)
}

func (r *Runtime) OpenSessionWithThreadID(ctx context.Context, chatID int64, view session.View, threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return errors.New("thread id is required")
	}
	return r.openSession(ctx, chatID, view, threadID, true)
}

func (r *Runtime) openSession(ctx context.Context, chatID int64, view session.View, requestedThreadID string, strictResume bool) error {
	if err := r.host.EnsureSession(ctx, view.Name, view.Root); err != nil {
		return err
	}

	meta, err := r.host.SessionMetadata(ctx, view.Name)
	if err != nil {
		return err
	}

	client := NewClient(fmt.Sprintf("ws://127.0.0.1:%d", meta.Port), view.Root)
	if err := connectWithRetry(ctx, client); err != nil {
		return err
	}

	resumeThreadID := strings.TrimSpace(meta.ThreadID)
	if strictResume {
		resumeThreadID = requestedThreadID
	}
	threadState, err := client.EnsureThread(ctx, resumeThreadID)
	if err != nil && !strictResume && strings.TrimSpace(meta.ThreadID) != "" {
		threadState, err = client.EnsureThread(ctx, "")
	}
	if err != nil {
		_ = client.Close()
		if strictResume {
			return fmt.Errorf("resume thread %s failed: %w", requestedThreadID, err)
		}
		return err
	}
	if strictResume && threadState.ThreadID != requestedThreadID {
		_ = client.Close()
		return fmt.Errorf("resume thread %s returned thread %s", requestedThreadID, threadState.ThreadID)
	}

	if threadState.ThreadID != meta.ThreadID {
		if err := r.host.SetThreadID(ctx, view.Name, threadState.ThreadID); err != nil {
			_ = client.Close()
			return err
		}
	}

	models, _ := client.ModelList(ctx)
	effectivePlanMode := normalizePlanMode(meta.Metadata.EffectivePlanMode)
	if effectivePlanMode == "" {
		effectivePlanMode = derivePlanMode(threadState.Model, threadState.Reasoning, models, r.planModeReasoning)
	}

	live := &liveSession{
		chatID: chatID,
		client: client,
		stopCh: make(chan struct{}),
		effective: session.ControlSelection{
			Model:     threadState.Model,
			Reasoning: threadState.Reasoning,
			PlanMode:  effectivePlanMode,
		},
		pending:       normalizePendingSelection(meta.Metadata.Pending),
		threadID:      threadState.ThreadID,
		cwd:           coalesce(threadState.Cwd, view.Root),
		models:        models,
		tokenUsage:    meta.Metadata.TokenUsage,
		hasTokenUsage: meta.Metadata.TokenUsage.ContextWindow > 0,
	}

	live.pending = r.normalizePending(live, live.pending)

	r.mu.Lock()
	if existing := r.sessions[view.Name]; existing != nil {
		close(existing.stopCh)
		r.stopTyping(existing)
		_ = existing.client.Close()
	}
	r.sessions[view.Name] = live
	r.mu.Unlock()

	if err := r.syncRuntimeMetadata(ctx, view.Name, live); err != nil {
		close(live.stopCh)
		_ = live.client.Close()
		return err
	}

	go r.forwardEvents(view.Name, live)
	return nil
}

func (r *Runtime) CloseSession(sessionName string) {
	r.mu.Lock()
	live := r.sessions[sessionName]
	delete(r.sessions, sessionName)
	r.mu.Unlock()

	if live == nil {
		return
	}
	close(live.stopCh)
	r.stopTyping(live)
	_ = live.client.Close()
}

func (r *Runtime) SubmitText(ctx context.Context, _ int64, view session.View, text string) error {
	live, err := r.session(view.Name)
	if err != nil {
		return err
	}
	options := r.turnOptions(live)
	r.startTyping(live)
	if err := live.client.StartTurnWithOptions(ctx, text, options); err != nil {
		r.stopTyping(live)
		return err
	}
	r.applyPendingAfterTurn(ctx, view.Name, live)
	return nil
}

func (r *Runtime) SubmitImage(ctx context.Context, _ int64, view session.View, imagePath string, caption string) error {
	live, err := r.session(view.Name)
	if err != nil {
		return err
	}

	inputs := []TurnInput{{
		Type: "localImage",
		Path: imagePath,
	}}
	if strings.TrimSpace(caption) != "" {
		inputs = append(inputs, TurnInput{
			Type: "text",
			Text: caption,
		})
	}
	options := r.turnOptions(live)
	r.startTyping(live)
	if err := live.client.StartTurnInputs(ctx, inputs, options); err != nil {
		r.stopTyping(live)
		return err
	}
	r.applyPendingAfterTurn(ctx, view.Name, live)
	return nil
}

func (r *Runtime) SubmitApproval(ctx context.Context, _ int64, view session.View, text string) (bool, error) {
	live, err := r.session(view.Name)
	if err != nil {
		return false, err
	}
	if !live.client.HasPendingApproval() && !live.client.HasPendingUserInput() {
		return false, nil
	}

	if live.client.HasPendingUserInput() {
		if strings.TrimSpace(text) == "" {
			return true, ErrUserInputReplyRequired
		}
		return true, live.client.ResolveUserInput(ctx, text)
	}

	decision, ok := normalizeDecision(text)
	if !ok {
		return true, ErrApprovalReplyRequired
	}
	return true, live.client.ResolveApproval(ctx, decision)
}

func (r *Runtime) Status(ctx context.Context, view session.View) (session.Status, error) {
	live, err := r.session(view.Name)
	if err != nil {
		return session.Status{}, err
	}

	live.mu.Lock()
	effective := live.effective
	pending := live.pending
	threadID := live.threadID
	cwd := live.cwd
	tokenUsage := live.tokenUsage
	hasTokenUsage := live.hasTokenUsage
	live.mu.Unlock()

	status := session.Status{
		View:          view,
		ThreadID:      threadID,
		Cwd:           cwd,
		CodexVersion:  r.codexVersion(ctx),
		Effective:     effective,
		Pending:       pending,
		TokenUsage:    tokenUsage,
		HasTokenUsage: hasTokenUsage,
		Branch:        r.resolveBranch(ctx, cwd),
	}

	attached, err := r.host.HasWritableAttachedClients(ctx, view.Name)
	if err == nil {
		status.LocalWritableAttach = attached
	}

	return status, nil
}

func (r *Runtime) ListModels(ctx context.Context, view session.View) ([]ModelInfo, error) {
	live, err := r.session(view.Name)
	if err != nil {
		return nil, err
	}
	return r.ensureModels(ctx, live)
}

func (r *Runtime) SetModel(ctx context.Context, view session.View, model string) (session.Status, string, error) {
	live, err := r.session(view.Name)
	if err != nil {
		return session.Status{}, "", err
	}
	models, err := r.ensureModels(ctx, live)
	if err != nil {
		return session.Status{}, "", err
	}
	target, ok := findModel(models, model)
	if !ok {
		return session.Status{}, "", fmt.Errorf("不支持的模型：%s", model)
	}

	var note string
	live.mu.Lock()
	targetReasoning := effectiveOrPendingReasoning(live)
	if !supportsReasoning(target, targetReasoning) {
		targetReasoning = target.DefaultReasoning
		note = fmt.Sprintf("当前 reasoning 已自动调整为 %s", targetReasoning)
	}
	live.pending.Model = pendingValue(target.Model, live.effective.Model)
	live.pending.Reasoning = pendingValue(targetReasoning, live.effective.Reasoning)
	live.pending = r.normalizePendingLocked(live)
	live.mu.Unlock()

	if err := r.syncRuntimeMetadata(ctx, view.Name, live); err != nil {
		return session.Status{}, "", err
	}
	status, err := r.Status(ctx, view)
	return status, note, err
}

func (r *Runtime) SetReasoning(ctx context.Context, view session.View, reasoning string) (session.Status, error) {
	live, err := r.session(view.Name)
	if err != nil {
		return session.Status{}, err
	}
	models, err := r.ensureModels(ctx, live)
	if err != nil {
		return session.Status{}, err
	}

	live.mu.Lock()
	targetModel := effectiveOrPendingModel(live)
	modelInfo, ok := findModel(models, targetModel)
	if !ok {
		live.mu.Unlock()
		return session.Status{}, fmt.Errorf("当前模型不可用：%s", targetModel)
	}
	if !supportsReasoning(modelInfo, reasoning) {
		live.mu.Unlock()
		return session.Status{}, fmt.Errorf("模型 %s 不支持 reasoning %s", targetModel, reasoning)
	}
	live.pending.Reasoning = pendingValue(reasoning, live.effective.Reasoning)
	live.pending = r.normalizePendingLocked(live)
	live.mu.Unlock()

	if err := r.syncRuntimeMetadata(ctx, view.Name, live); err != nil {
		return session.Status{}, err
	}
	return r.Status(ctx, view)
}

func (r *Runtime) SetPlanMode(ctx context.Context, view session.View, mode session.PlanMode) (session.Status, error) {
	live, err := r.session(view.Name)
	if err != nil {
		return session.Status{}, err
	}
	models, err := r.ensureModels(ctx, live)
	if err != nil {
		return session.Status{}, err
	}

	live.mu.Lock()
	targetModel := effectiveOrPendingModel(live)
	modelInfo, ok := findModel(models, targetModel)
	if !ok {
		live.mu.Unlock()
		return session.Status{}, fmt.Errorf("当前模型不可用：%s", targetModel)
	}

	targetReasoning := modelInfo.DefaultReasoning
	if mode == session.PlanModePlan {
		targetReasoning = r.planModeReasoning
		if !supportsReasoning(modelInfo, targetReasoning) {
			live.mu.Unlock()
			return session.Status{}, fmt.Errorf("当前模型 %s 不支持计划模式 reasoning %s", targetModel, targetReasoning)
		}
	}

	live.pending.Reasoning = pendingValue(targetReasoning, live.effective.Reasoning)
	live.pending = r.normalizePendingLocked(live)
	if live.pending.Model != "" || live.pending.Reasoning != "" {
		live.pending.PlanMode = mode
	}
	live.mu.Unlock()

	if err := r.syncRuntimeMetadata(ctx, view.Name, live); err != nil {
		return session.Status{}, err
	}
	return r.Status(ctx, view)
}

func (r *Runtime) ClearThread(ctx context.Context, view session.View) (session.Status, error) {
	live, err := r.session(view.Name)
	if err != nil {
		return session.Status{}, err
	}

	r.stopTyping(live)
	threadState, err := live.client.StartFreshThread(ctx)
	if err != nil {
		return session.Status{}, err
	}
	if err := r.host.SetThreadID(ctx, view.Name, threadState.ThreadID); err != nil {
		return session.Status{}, err
	}

	live.mu.Lock()
	live.threadID = threadState.ThreadID
	live.cwd = coalesce(threadState.Cwd, view.Root)
	live.tokenUsage = session.TokenUsage{}
	live.hasTokenUsage = false
	live.mu.Unlock()

	if err := r.syncRuntimeMetadata(ctx, view.Name, live); err != nil {
		return session.Status{}, err
	}
	return r.Status(ctx, view)
}

func (r *Runtime) IsLocallyAttached(ctx context.Context, sessionName string) (bool, error) {
	return r.host.HasWritableAttachedClients(ctx, sessionName)
}

func (r *Runtime) KillSession(ctx context.Context, view session.View) error {
	r.CloseSession(view.Name)
	return r.host.KillSession(ctx, view.Name)
}

func (r *Runtime) session(sessionName string) (*liveSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	live := r.sessions[sessionName]
	if live == nil {
		return nil, errors.New("session runtime is not connected")
	}
	return live, nil
}

func (r *Runtime) ensureModels(ctx context.Context, live *liveSession) ([]ModelInfo, error) {
	live.mu.Lock()
	if len(live.models) > 0 {
		models := append([]ModelInfo(nil), live.models...)
		live.mu.Unlock()
		return models, nil
	}
	client := live.client
	live.mu.Unlock()

	models, err := client.ModelList(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].Model < models[j].Model
	})

	live.mu.Lock()
	live.models = append([]ModelInfo(nil), models...)
	live.mu.Unlock()
	return models, nil
}

func (r *Runtime) forwardEvents(sessionName string, live *liveSession) {
	for {
		select {
		case <-live.stopCh:
			return
		case event := <-live.client.Events():
			switch event.Kind {
			case EventTokenUsageUpdated:
				live.mu.Lock()
				live.tokenUsage = event.TokenUsage
				live.hasTokenUsage = event.TokenUsage.ContextWindow > 0
				live.mu.Unlock()
				_ = r.syncRuntimeMetadata(context.Background(), sessionName, live)
			case EventModelRerouted:
				live.mu.Lock()
				live.effective.Model = event.Model
				live.mu.Unlock()
			default:
				r.sendEvent(live, event)
			}
		}
	}
}

func (r *Runtime) sendEvent(live *liveSession, event Event) {
	r.stopTyping(live)
	switch event.Kind {
	case EventFinalAnswer:
		for _, message := range r.formatter.FormatTelegramHTML(event.Text) {
			if strings.TrimSpace(message.Text) == "" {
				continue
			}
			_ = r.sender.SendMessage(context.Background(), live.chatID, message)
		}
	case EventApprovalRequested:
		for _, message := range r.formatter.FormatTelegramHTML(event.Text) {
			if strings.TrimSpace(message.Text) == "" {
				continue
			}
			_ = r.sender.SendMessage(context.Background(), live.chatID, stream.OutboundMessage{
				Text:      message.Text,
				ParseMode: message.ParseMode,
			})
		}
	case EventUserInputRequested:
		for _, message := range r.formatter.FormatTelegramHTML(event.Text) {
			if strings.TrimSpace(message.Text) == "" {
				continue
			}
			_ = r.sender.SendMessage(context.Background(), live.chatID, stream.OutboundMessage{
				Text:      message.Text,
				ParseMode: message.ParseMode,
			})
		}
	case EventTurnError:
		text := strings.TrimSpace(event.Text)
		if text == "" {
			text = "Codex 会话异常中断"
		}
		r.sendSystemMessage(live, "Codex 会话异常中断：\n"+text+"\n\n下一步：可以直接发送补充说明继续；如果持续失败，请执行 /clear 或 /kill 后重新 /open。")
	case EventConnectionClosed:
		text := strings.TrimSpace(event.Text)
		if text == "" {
			text = "Codex app-server 连接已断开"
		}
		r.sendSystemMessage(live, text+"\n下一步：请执行 /open <project> 重新接管，或 /status 查看当前状态。")
	}
}

func (r *Runtime) sendSystemMessage(live *liveSession, text string) {
	for _, message := range r.formatter.FormatTelegramHTML(text) {
		if strings.TrimSpace(message.Text) == "" {
			continue
		}
		_ = r.sender.SendMessage(context.Background(), live.chatID, message)
	}
}

func (r *Runtime) turnOptions(live *liveSession) TurnOptions {
	live.mu.Lock()
	defer live.mu.Unlock()

	return TurnOptions{
		Model:     live.pending.Model,
		Reasoning: live.pending.Reasoning,
	}
}

func (r *Runtime) applyPendingAfterTurn(ctx context.Context, sessionName string, live *liveSession) {
	live.mu.Lock()
	if live.pending.Model == "" && live.pending.Reasoning == "" {
		live.mu.Unlock()
		return
	}
	targetModel := effectiveOrPendingModel(live)
	targetReasoning := effectiveOrPendingReasoning(live)
	targetPlanMode := live.pending.PlanMode
	if targetPlanMode == "" {
		targetPlanMode = live.effective.PlanMode
	}
	live.effective.Model = targetModel
	live.effective.Reasoning = targetReasoning
	live.effective.PlanMode = targetPlanMode
	live.pending = session.ControlSelection{}
	live.mu.Unlock()

	_ = r.syncRuntimeMetadata(ctx, sessionName, live)
}

func (r *Runtime) syncRuntimeMetadata(ctx context.Context, sessionName string, live *liveSession) error {
	live.mu.Lock()
	metadata := session.RuntimeMetadata{
		Pending:           live.pending,
		EffectivePlanMode: live.effective.PlanMode,
	}
	if live.hasTokenUsage {
		metadata.TokenUsage = live.tokenUsage
	}
	live.mu.Unlock()
	return r.host.SetRuntimeMetadata(ctx, sessionName, metadata)
}

func (r *Runtime) normalizePending(live *liveSession, pending session.ControlSelection) session.ControlSelection {
	live.mu.Lock()
	defer live.mu.Unlock()
	live.pending = pending
	return r.normalizePendingLocked(live)
}

func (r *Runtime) normalizePendingLocked(live *liveSession) session.ControlSelection {
	pending := normalizePendingSelection(live.pending)
	targetModel := live.effective.Model
	if pending.Model != "" {
		targetModel = pending.Model
	}
	targetReasoning := live.effective.Reasoning
	if pending.Reasoning != "" {
		targetReasoning = pending.Reasoning
	}
	if targetModel == live.effective.Model && targetReasoning == live.effective.Reasoning {
		live.pending = session.ControlSelection{}
		return live.pending
	}
	if pending.Model == live.effective.Model {
		pending.Model = ""
	}
	if pending.Reasoning == live.effective.Reasoning {
		pending.Reasoning = ""
	}
	pending.PlanMode = derivePlanMode(targetModel, targetReasoning, live.models, r.planModeReasoning)
	live.pending = pending
	return pending
}

func (r *Runtime) startTyping(live *liveSession) {
	r.stopTyping(live)

	stopCh := make(chan struct{})
	live.typingStopCh = stopCh
	go func(chatID int64, stop <-chan struct{}) {
		_ = r.sender.SendChatAction(context.Background(), chatID, "typing")
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_ = r.sender.SendChatAction(context.Background(), chatID, "typing")
			}
		}
	}(live.chatID, stopCh)
}

func (r *Runtime) stopTyping(live *liveSession) {
	if live == nil || live.typingStopCh == nil {
		return
	}
	close(live.typingStopCh)
	live.typingStopCh = nil
}

func (r *Runtime) codexVersion(ctx context.Context) string {
	r.versionOnce.Do(func() {
		r.version = r.resolveCodexVersion(ctx, r.codexBin)
	})
	return r.version
}

func connectWithRetry(ctx context.Context, client *Client) error {
	var lastErr error
	for i := 0; i < 20; i++ {
		if err := client.Connect(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return lastErr
}

func normalizeDecision(text string) (Decision, bool) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "是", "y", "yes":
		return DecisionApprove, true
	case "否", "n", "no":
		return DecisionDecline, true
	default:
		return "", false
	}
}

func normalizePendingSelection(selection session.ControlSelection) session.ControlSelection {
	selection.Model = strings.TrimSpace(selection.Model)
	selection.Reasoning = strings.TrimSpace(selection.Reasoning)
	selection.PlanMode = normalizePlanMode(selection.PlanMode)
	return selection
}

func normalizePlanMode(mode session.PlanMode) session.PlanMode {
	switch mode {
	case session.PlanModeDefault, session.PlanModePlan, session.PlanModeCustom:
		return mode
	default:
		return ""
	}
}

func derivePlanMode(model string, reasoning string, models []ModelInfo, planReasoning string) session.PlanMode {
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return session.PlanModeCustom
	}
	if modelInfo, ok := findModel(models, model); ok && modelInfo.DefaultReasoning == reasoning {
		return session.PlanModeDefault
	}
	if strings.TrimSpace(planReasoning) != "" && strings.TrimSpace(planReasoning) == reasoning {
		return session.PlanModePlan
	}
	return session.PlanModeCustom
}

func findModel(models []ModelInfo, wanted string) (ModelInfo, bool) {
	wanted = strings.TrimSpace(wanted)
	for _, model := range models {
		if model.Model == wanted || model.ID == wanted {
			return model, true
		}
	}
	return ModelInfo{}, false
}

func supportsReasoning(model ModelInfo, reasoning string) bool {
	if len(model.Supported) == 0 {
		return isKnownReasoningEffort(reasoning)
	}
	for _, option := range model.Supported {
		if option == reasoning {
			return true
		}
	}
	return false
}

func isKnownReasoningEffort(reasoning string) bool {
	switch strings.TrimSpace(reasoning) {
	case "minimal", "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
}

func effectiveOrPendingModel(live *liveSession) string {
	if live.pending.Model != "" {
		return live.pending.Model
	}
	return live.effective.Model
}

func effectiveOrPendingReasoning(live *liveSession) string {
	if live.pending.Reasoning != "" {
		return live.pending.Reasoning
	}
	return live.effective.Reasoning
}

func pendingValue(value string, effective string) string {
	value = strings.TrimSpace(value)
	if value == strings.TrimSpace(effective) {
		return ""
	}
	return value
}

func coalesce(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}

func gitBranch(ctx context.Context, cwd string) string {
	if strings.TrimSpace(cwd) == "" {
		return ""
	}
	output, err := exec.CommandContext(ctx, "git", "-C", cwd, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func codexVersion(ctx context.Context, codexBin string) string {
	if strings.TrimSpace(codexBin) == "" {
		codexBin = "codex"
	}
	output, err := exec.CommandContext(ctx, codexBin, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(output))
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}
