package session

import "testing"

func TestRegistryOpenCloseAndKillLifecycle(t *testing.T) {
	registry := NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
		"project-b": "/tmp/project-b",
	})

	sessionView, err := registry.Open("project-a")
	if err != nil {
		t.Fatalf("Open(project-a) error = %v", err)
	}

	if sessionView.Name != "codex-project-a" {
		t.Fatalf("session name = %q, want %q", sessionView.Name, "codex-project-a")
	}

	if sessionView.State != StateStarting {
		t.Fatalf("session state = %q, want %q", sessionView.State, StateStarting)
	}

	active, ok := registry.Active()
	if !ok || active.Project != "project-a" {
		t.Fatalf("Active() = (%+v, %t), want project-a", active, ok)
	}

	if err := registry.CloseActive(); err != nil {
		t.Fatalf("CloseActive() error = %v", err)
	}

	if _, ok := registry.Active(); ok {
		t.Fatal("Active() ok = true, want false after close")
	}

	killed, err := registry.Kill("project-a")
	if err != nil {
		t.Fatalf("Kill(project-a) error = %v", err)
	}

	if killed.Project != "project-a" {
		t.Fatalf("killed project = %q, want %q", killed.Project, "project-a")
	}

	if got := registry.List(); len(got) != 0 {
		t.Fatalf("registry.List() = %#v, want empty after kill", got)
	}
}

func TestRegistryRejectsUnknownProject(t *testing.T) {
	registry := NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	})

	_, err := registry.Open("missing")
	if err == nil {
		t.Fatal("Open(missing) error = nil, want error")
	}
}

func TestRegistryReattachCreatesDetachedSessionWithoutActiveBinding(t *testing.T) {
	registry := NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	})

	view, err := registry.Reattach("project-a")
	if err != nil {
		t.Fatalf("Reattach(project-a) error = %v", err)
	}

	if view.State != StateDetached {
		t.Fatalf("view.State = %q, want %q", view.State, StateDetached)
	}

	if _, ok := registry.Active(); ok {
		t.Fatal("Active() ok = true, want false after reattach")
	}
}

func TestRegistryAllowsDynamicProjectAddAndRemove(t *testing.T) {
	registry := NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	})

	if err := registry.AddAllowedProject("project-b", "/tmp/project-b"); err != nil {
		t.Fatalf("AddAllowedProject(project-b) error = %v", err)
	}

	allowed := registry.AllowedProjects()
	if got := allowed["project-b"]; got != "/tmp/project-b" {
		t.Fatalf("AllowedProjects()[project-b] = %q, want %q", got, "/tmp/project-b")
	}

	removed, hadSession, err := registry.RemoveAllowedProject("project-b")
	if err != nil {
		t.Fatalf("RemoveAllowedProject(project-b) error = %v", err)
	}
	if hadSession {
		t.Fatal("hadSession = true, want false for project-b")
	}

	if removed.Project != "project-b" {
		t.Fatalf("removed.Project = %q, want %q", removed.Project, "project-b")
	}

	allowed = registry.AllowedProjects()
	if _, ok := allowed["project-b"]; ok {
		t.Fatalf("AllowedProjects() still contains project-b: %#v", allowed)
	}
}

func TestRegistryRejectsRemovingStaticProject(t *testing.T) {
	registry := NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	})

	if _, _, err := registry.RemoveAllowedProject("project-a"); err == nil {
		t.Fatal("RemoveAllowedProject(project-a) error = nil, want error")
	}
}
