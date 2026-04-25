package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ProjectStore struct {
	path string
}

func NewProjectStore(path string) *ProjectStore {
	return &ProjectStore{path: path}
}

func (s *ProjectStore) Load() (map[string]string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read project store: %w", err)
	}

	if len(data) == 0 {
		return map[string]string{}, nil
	}

	projects := make(map[string]string)
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, fmt.Errorf("parse project store: %w", err)
	}

	return projects, nil
}

func (s *ProjectStore) AddProject(name string, root string) error {
	projects, err := s.Load()
	if err != nil {
		return err
	}

	projects[name] = root
	return s.save(projects)
}

func (s *ProjectStore) RemoveProject(name string) error {
	projects, err := s.Load()
	if err != nil {
		return err
	}

	delete(projects, name)
	return s.save(projects)
}

func (s *ProjectStore) save(projects map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create project store dir: %w", err)
	}

	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal project store: %w", err)
	}
	data = append(data, '\n')

	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return fmt.Errorf("write project store: %w", err)
	}

	if err := os.Rename(tempPath, s.path); err != nil {
		return fmt.Errorf("replace project store: %w", err)
	}

	return nil
}
