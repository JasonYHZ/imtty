package config

import (
	"path/filepath"
	"testing"
)

func TestProjectStorePersistsAddedAndRemovedProjects(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "imtty-projects.json")
	store := NewProjectStore(storePath)

	if err := store.AddProject("project-a", "/tmp/project-a"); err != nil {
		t.Fatalf("AddProject(project-a) error = %v", err)
	}

	projects, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := projects["project-a"]; got != "/tmp/project-a" {
		t.Fatalf("projects[project-a] = %q, want %q", got, "/tmp/project-a")
	}

	if err := store.RemoveProject("project-a"); err != nil {
		t.Fatalf("RemoveProject(project-a) error = %v", err)
	}

	projects, err = store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if _, ok := projects["project-a"]; ok {
		t.Fatalf("projects still contains project-a: %#v", projects)
	}
}
