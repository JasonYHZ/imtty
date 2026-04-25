package session

type PlanMode string

const (
	PlanModeDefault PlanMode = "default"
	PlanModePlan    PlanMode = "plan"
	PlanModeCustom  PlanMode = "custom"
)

type ControlSelection struct {
	Model     string
	Reasoning string
	PlanMode  PlanMode
}

type TokenUsage struct {
	ContextWindow int64
	TotalTokens   int64
}

type RuntimeMetadata struct {
	Pending           ControlSelection
	EffectivePlanMode PlanMode
	TokenUsage        TokenUsage
}

type Status struct {
	View                View
	ThreadID            string
	Cwd                 string
	Branch              string
	CodexVersion        string
	Effective           ControlSelection
	Pending             ControlSelection
	TokenUsage          TokenUsage
	HasTokenUsage       bool
	LocalWritableAttach bool
}
