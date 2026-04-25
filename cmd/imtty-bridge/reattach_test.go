package main

import (
	"context"
	"reflect"
	"testing"

	"imtty/internal/session"
)

func TestRestoreDetachedSessionsReattachesKnownProjects(t *testing.T) {
	registry := session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
		"project-b": "/tmp/project-b",
	})
	manager := fakeSessionLister{
		sessions: []string{"codex-project-a", "codex-missing", "codex-project-b"},
	}

	views, err := restoreDetachedSessions(context.Background(), registry, manager, "codex-")
	if err != nil {
		t.Fatalf("restoreDetachedSessions() error = %v", err)
	}

	gotProjects := []string{views[0].Project, views[1].Project}
	wantProjects := []string{"project-a", "project-b"}
	if !reflect.DeepEqual(gotProjects, wantProjects) {
		t.Fatalf("projects = %#v, want %#v", gotProjects, wantProjects)
	}

	list := registry.List()
	if len(list) != 2 || list[0].State != session.StateDetached || list[1].State != session.StateDetached {
		t.Fatalf("registry.List() = %#v, want two detached sessions", list)
	}
}

type fakeSessionLister struct {
	sessions []string
	err      error
}

func (f fakeSessionLister) ListSessions(context.Context) ([]string, error) {
	return f.sessions, f.err
}
