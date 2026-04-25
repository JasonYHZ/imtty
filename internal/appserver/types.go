package appserver

type EventKind string

const (
	EventFinalAnswer       EventKind = "final_answer"
	EventApprovalRequested EventKind = "approval_requested"
)

type Event struct {
	Kind EventKind
	Text string
}

type TurnInput struct {
	Type string
	Text string
	Path string
}

type Decision string

const (
	DecisionApprove Decision = "accept"
	DecisionDecline Decision = "decline"
)
