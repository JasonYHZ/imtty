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
