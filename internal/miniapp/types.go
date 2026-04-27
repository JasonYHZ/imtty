package miniapp

import "imtty/internal/session"

type Viewer struct {
	ID       int64  `json:"id"`
	Username string `json:"username,omitempty"`
}

type SessionView struct {
	Name    string        `json:"name"`
	Project string        `json:"project"`
	Root    string        `json:"root"`
	State   session.State `json:"state"`
}

type ProjectView struct {
	Name    string `json:"name"`
	Root    string `json:"root"`
	Dynamic bool   `json:"dynamic"`
}

type BrowseShortcutView struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type DirectoryEntryView struct {
	Name         string `json:"name"`
	AbsolutePath string `json:"absolute_path"`
}

type BootstrapResponse struct {
	Viewer            Viewer               `json:"viewer"`
	ActiveSession     *SessionView         `json:"active_session,omitempty"`
	ActiveStatus      *StatusView          `json:"active_status,omitempty"`
	Models            []ModelView          `json:"models,omitempty"`
	Sessions          []SessionView        `json:"sessions"`
	Projects          []ProjectView        `json:"projects"`
	BrowseDefaultPath string               `json:"browse_default_path"`
	BrowseShortcuts   []BrowseShortcutView `json:"browse_shortcuts"`
}

type actionResponse struct {
	OK        bool     `json:"ok"`
	Responses []string `json:"responses,omitempty"`
}

type BrowseResponse struct {
	CurrentAbsolutePath string               `json:"current_absolute_path"`
	ParentAbsolutePath  string               `json:"parent_absolute_path,omitempty"`
	Directories         []DirectoryEntryView `json:"directories"`
	Shortcuts           []BrowseShortcutView `json:"shortcuts"`
}

type ControlSelectionView struct {
	Model     string `json:"model,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
	PlanMode  string `json:"plan_mode,omitempty"`
}

type TokenUsageView struct {
	ContextWindow int64 `json:"context_window"`
	TotalTokens   int64 `json:"total_tokens"`
}

type StatusView struct {
	ThreadID            string               `json:"thread_id,omitempty"`
	Cwd                 string               `json:"cwd,omitempty"`
	Branch              string               `json:"branch,omitempty"`
	CodexVersion        string               `json:"codex_version,omitempty"`
	Effective           ControlSelectionView `json:"effective"`
	Pending             ControlSelectionView `json:"pending"`
	Target              ControlSelectionView `json:"target"`
	HasPendingControls  bool                 `json:"has_pending_controls"`
	TokenUsage          TokenUsageView       `json:"token_usage"`
	HasTokenUsage       bool                 `json:"has_token_usage"`
	LocalWritableAttach bool                 `json:"local_writable_attach"`
}

type ModelView struct {
	ID               string   `json:"id"`
	Model            string   `json:"model"`
	DefaultReasoning string   `json:"default_reasoning"`
	Supported        []string `json:"supported"`
}
