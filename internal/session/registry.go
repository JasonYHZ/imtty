package session

import (
	"fmt"
	"sort"
	"sync"
)

type State string

const (
	StateIdle     State = "idle"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateDetached State = "detached"
	StateExited   State = "exited"
	StateLost     State = "lost"
)

type View struct {
	Project string
	Name    string
	Root    string
	State   State
}

type Registry struct {
	mu       sync.RWMutex
	allowed  map[string]string
	mutable  map[string]bool
	sessions map[string]View
	active   string
}

func NewRegistry(allowed map[string]string) *Registry {
	return NewRegistryWithDynamic(allowed, nil)
}

func NewRegistryWithDynamic(allowed map[string]string, dynamic map[string]string) *Registry {
	allowedCopy := make(map[string]string, len(allowed))
	for name, root := range allowed {
		allowedCopy[name] = root
	}

	mutable := make(map[string]bool, len(dynamic))
	for name, root := range dynamic {
		if _, exists := allowedCopy[name]; exists {
			continue
		}
		allowedCopy[name] = root
		mutable[name] = true
	}

	return &Registry{
		allowed:  allowedCopy,
		mutable:  mutable,
		sessions: make(map[string]View),
	}
}

func (r *Registry) Open(project string) (View, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	root, ok := r.allowed[project]
	if !ok {
		return View{}, fmt.Errorf("unknown project %q", project)
	}

	view, exists := r.sessions[project]
	if !exists {
		view = View{
			Project: project,
			Name:    "codex-" + project,
			Root:    root,
			State:   StateStarting,
		}
	} else if view.State == StateDetached || view.State == StateExited || view.State == StateLost || view.State == StateIdle {
		view.State = StateStarting
	}

	r.sessions[project] = view
	r.active = project
	return view, nil
}

func (r *Registry) Resolve(project string) (View, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	root, ok := r.allowed[project]
	if !ok {
		return View{}, fmt.Errorf("unknown project %q", project)
	}

	view, exists := r.sessions[project]
	if !exists {
		return View{
			Project: project,
			Name:    "codex-" + project,
			Root:    root,
			State:   StateStarting,
		}, nil
	}

	if view.State == StateDetached || view.State == StateExited || view.State == StateLost || view.State == StateIdle {
		view.State = StateStarting
	}
	return view, nil
}

func (r *Registry) SetState(project string, state State) (View, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	view, ok := r.sessions[project]
	if !ok {
		return View{}, fmt.Errorf("unknown project %q", project)
	}

	view.State = state
	r.sessions[project] = view
	return view, nil
}

func (r *Registry) Reattach(project string) (View, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	root, ok := r.allowed[project]
	if !ok {
		return View{}, fmt.Errorf("unknown project %q", project)
	}

	view := View{
		Project: project,
		Name:    "codex-" + project,
		Root:    root,
		State:   StateDetached,
	}

	r.sessions[project] = view
	return view, nil
}

func (r *Registry) Active() (View, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.active == "" {
		return View{}, false
	}

	view, ok := r.sessions[r.active]
	return view, ok
}

func (r *Registry) CloseActive() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.active == "" {
		return nil
	}

	view, ok := r.sessions[r.active]
	if ok && view.State != StateExited && view.State != StateLost {
		view.State = StateDetached
		r.sessions[r.active] = view
	}

	r.active = ""
	return nil
}

func (r *Registry) Kill(project string) (View, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	view, ok := r.sessions[project]
	if !ok {
		root, allowed := r.allowed[project]
		if !allowed {
			return View{}, fmt.Errorf("unknown project %q", project)
		}
		view = View{
			Project: project,
			Name:    "codex-" + project,
			Root:    root,
		}
	}

	delete(r.sessions, project)
	if r.active == project {
		r.active = ""
	}

	return view, nil
}

func (r *Registry) List() []View {
	r.mu.RLock()
	defer r.mu.RUnlock()

	projects := make([]string, 0, len(r.sessions))
	for project := range r.sessions {
		projects = append(projects, project)
	}
	sort.Strings(projects)

	views := make([]View, 0, len(projects))
	for _, project := range projects {
		views = append(views, r.sessions[project])
	}

	return views
}

func (r *Registry) AllowedProjects() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	projects := make(map[string]string, len(r.allowed))
	for name, root := range r.allowed {
		projects[name] = root
	}
	return projects
}

func (r *Registry) IsDynamicProject(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.mutable[name]
}

func (r *Registry) AddAllowedProject(name string, root string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.allowed[name]; exists {
		return fmt.Errorf("project %q already exists", name)
	}

	r.allowed[name] = root
	r.mutable[name] = true
	return nil
}

func (r *Registry) RemoveAllowedProject(name string) (View, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	root, exists := r.allowed[name]
	if !exists {
		return View{}, false, fmt.Errorf("unknown project %q", name)
	}

	if !r.mutable[name] {
		return View{}, false, fmt.Errorf("project %q is configured by environment and cannot be removed here", name)
	}

	view, ok := r.sessions[name]
	if !ok {
		view = View{
			Project: name,
			Name:    "codex-" + name,
			Root:    root,
		}
	}

	delete(r.allowed, name)
	delete(r.mutable, name)
	delete(r.sessions, name)
	if r.active == name {
		r.active = ""
	}

	return view, ok, nil
}
