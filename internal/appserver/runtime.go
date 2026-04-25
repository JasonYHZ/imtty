package appserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"imtty/internal/session"
	"imtty/internal/stream"
	"imtty/internal/tmux"
)

var ErrApprovalReplyRequired = errors.New("当前等待审批，请先回复 是/否")

type SessionHost interface {
	EnsureSession(ctx context.Context, sessionName string, root string) error
	SessionMetadata(ctx context.Context, sessionName string) (tmux.SessionRuntimeInfo, error)
	SetThreadID(ctx context.Context, sessionName string, threadID string) error
	HasWritableAttachedClients(ctx context.Context, sessionName string) (bool, error)
	KillSession(ctx context.Context, sessionName string) error
}

type Sender interface {
	SendMessage(ctx context.Context, chatID int64, message stream.OutboundMessage) error
	SendChatAction(ctx context.Context, chatID int64, action string) error
}

type Runtime struct {
	host      SessionHost
	formatter stream.Formatter
	sender    Sender

	mu       sync.Mutex
	sessions map[string]*liveSession
}

type liveSession struct {
	chatID       int64
	client       *Client
	stopCh       chan struct{}
	typingStopCh chan struct{}
}

func NewRuntime(host SessionHost, formatter stream.Formatter, sender Sender) *Runtime {
	return &Runtime{
		host:      host,
		formatter: formatter,
		sender:    sender,
		sessions:  make(map[string]*liveSession),
	}
}

func (r *Runtime) OpenSession(ctx context.Context, chatID int64, view session.View) error {
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

	threadID, err := client.EnsureThread(ctx, meta.ThreadID)
	if err != nil && strings.TrimSpace(meta.ThreadID) != "" {
		threadID, err = client.EnsureThread(ctx, "")
	}
	if err != nil {
		_ = client.Close()
		return err
	}

	if threadID != meta.ThreadID {
		if err := r.host.SetThreadID(ctx, view.Name, threadID); err != nil {
			_ = client.Close()
			return err
		}
	}

	live := &liveSession{
		chatID:       chatID,
		client:       client,
		stopCh:       make(chan struct{}),
		typingStopCh: nil,
	}

	r.mu.Lock()
	if existing := r.sessions[view.Name]; existing != nil {
		close(existing.stopCh)
		_ = existing.client.Close()
	}
	r.sessions[view.Name] = live
	r.mu.Unlock()

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
	r.startTyping(live)
	if err := live.client.StartTurn(ctx, text); err != nil {
		r.stopTyping(live)
		return err
	}
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
	r.startTyping(live)
	if err := live.client.StartTurnInputs(ctx, inputs); err != nil {
		r.stopTyping(live)
		return err
	}
	return nil
}

func (r *Runtime) SubmitApproval(ctx context.Context, _ int64, view session.View, text string) (bool, error) {
	live, err := r.session(view.Name)
	if err != nil {
		return false, err
	}
	if !live.client.HasPendingApproval() {
		return false, nil
	}

	decision, ok := normalizeDecision(text)
	if !ok {
		return true, ErrApprovalReplyRequired
	}
	return true, live.client.ResolveApproval(ctx, decision)
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

func (r *Runtime) forwardEvents(sessionName string, live *liveSession) {
	for {
		select {
		case <-live.stopCh:
			return
		case event := <-live.client.Events():
			r.sendEvent(sessionName, live, event)
		}
	}
}

func (r *Runtime) sendEvent(_ string, live *liveSession, event Event) {
	r.stopTyping(live)
	switch event.Kind {
	case EventFinalAnswer:
		for _, chunk := range r.formatter.Format(event.Text) {
			if strings.TrimSpace(chunk) == "" {
				continue
			}
			_ = r.sender.SendMessage(context.Background(), live.chatID, stream.OutboundMessage{Text: chunk})
		}
	case EventApprovalRequested:
		for _, chunk := range r.formatter.Format(event.Text) {
			if strings.TrimSpace(chunk) == "" {
				continue
			}
			_ = r.sender.SendMessage(context.Background(), live.chatID, stream.OutboundMessage{
				Text:         chunk,
				QuickReplies: []string{"是", "否"},
			})
		}
	}
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
