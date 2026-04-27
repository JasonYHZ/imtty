package appserver

import "imtty/internal/session"

type EventKind string

const (
	EventFinalAnswer        EventKind = "final_answer"
	EventApprovalRequested  EventKind = "approval_requested"
	EventUserInputRequested EventKind = "user_input_requested"
	EventTokenUsageUpdated  EventKind = "token_usage_updated"
	EventModelRerouted      EventKind = "model_rerouted"
	EventTurnError          EventKind = "turn_error"
	EventConnectionClosed   EventKind = "connection_closed"
)

type Event struct {
	Kind       EventKind
	Text       string
	Model      string
	TokenUsage session.TokenUsage
}

type TurnInput struct {
	Type string
	Text string
	Path string
}

type TurnOptions struct {
	Model     string
	Reasoning string
}

type ThreadState struct {
	ThreadID  string
	Cwd       string
	Model     string
	Reasoning string
}

type ModelInfo struct {
	ID               string
	Model            string
	DisplayName      string
	DefaultReasoning string
	Supported        []string
}

type Decision string

const (
	DecisionApprove Decision = "accept"
	DecisionDecline Decision = "decline"
)
