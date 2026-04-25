package miniapp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"imtty/internal/config"
)

type Browser struct {
	defaultPath string
	shortcuts   []BrowseShortcutView
}

func NewBrowser(extraShortcuts map[string]string) *Browser {
	homeDir := resolveHomeDir()
	shortcuts := buildShortcuts(homeDir, extraShortcuts)

	return &Browser{
		defaultPath: homeDir,
		shortcuts:   shortcuts,
	}
}

func (b *Browser) DefaultPath() string {
	return b.defaultPath
}

func (b *Browser) Shortcuts() []BrowseShortcutView {
	copied := make([]BrowseShortcutView, 0, len(b.shortcuts))
	copied = append(copied, b.shortcuts...)
	return copied
}

func (b *Browser) Browse(path string) (BrowseResponse, error) {
	currentPath, err := resolveBrowsePath(path, b.defaultPath)
	if err != nil {
		return BrowseResponse{}, err
	}

	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return BrowseResponse{}, fmt.Errorf("read browse directory: %w", err)
	}

	directories := make([]DirectoryEntryView, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		directories = append(directories, DirectoryEntryView{
			Name:         entry.Name(),
			AbsolutePath: filepath.Join(currentPath, entry.Name()),
		})
	}

	sort.Slice(directories, func(i, j int) bool {
		return directories[i].Name < directories[j].Name
	})

	response := BrowseResponse{
		CurrentAbsolutePath: currentPath,
		Directories:         directories,
		Shortcuts:           b.Shortcuts(),
	}
	if currentPath != string(filepath.Separator) {
		response.ParentAbsolutePath = filepath.Dir(currentPath)
	}

	return response, nil
}

func resolveBrowsePath(rawPath string, defaultPath string) (string, error) {
	cleanPath := strings.TrimSpace(rawPath)
	if cleanPath == "" {
		cleanPath = defaultPath
	}
	if !filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("browse path must be absolute")
	}

	cleanPath = filepath.Clean(cleanPath)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", fmt.Errorf("stat browse path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("browse path is not a directory")
	}

	return cleanPath, nil
}

func resolveHomeDir() string {
	homeDir, err := os.UserHomeDir()
	if err == nil && filepath.IsAbs(homeDir) {
		return homeDir
	}
	return string(filepath.Separator)
}

func buildShortcuts(homeDir string, extraShortcuts map[string]string) []BrowseShortcutView {
	items := []BrowseShortcutView{
		{Name: "workspace", Path: filepath.Join(homeDir, "workspace")},
		{Name: "Personal", Path: filepath.Join(homeDir, "workspace", "Personal")},
		{Name: "Playground", Path: filepath.Join(homeDir, "workspace", "Playground")},
		{Name: "Home", Path: homeDir},
		{Name: "Root", Path: string(filepath.Separator)},
	}

	names := config.SortedProjectNames(extraShortcuts)
	for _, name := range names {
		path := extraShortcuts[name]
		if !filepath.IsAbs(path) {
			continue
		}
		items = append(items, BrowseShortcutView{Name: name, Path: path})
	}

	deduped := make([]BrowseShortcutView, 0, len(items))
	seenNames := make(map[string]bool, len(items))
	seenPaths := make(map[string]bool, len(items))
	for _, item := range items {
		if item.Path == "" || seenPaths[item.Path] || seenNames[item.Name] {
			continue
		}
		seenNames[item.Name] = true
		seenPaths[item.Path] = true
		deduped = append(deduped, item)
	}
	return deduped
}
