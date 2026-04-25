package tmux

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"sort"
	"strings"
)

var errExit = errors.New("tmux command exited with non-zero status")

type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

type Manager struct {
	runner        Runner
	prefix        string
	codexBin      string
	portAllocator func() (int, error)
}

func NewManager(runner Runner, prefix string, codexBin string) *Manager {
	return &Manager{
		runner:        runner,
		prefix:        prefix,
		codexBin:      codexBin,
		portAllocator: allocateLoopbackPort,
	}
}

type SessionRuntimeInfo struct {
	Port     int
	ThreadID string
}

func (m *Manager) EnsureSession(ctx context.Context, sessionName string, root string) error {
	if err := m.validateSessionName(sessionName); err != nil {
		return err
	}

	_, err := m.runner.Run(ctx, "has-session", "-t", sessionName)
	if err == nil {
		if _, metaErr := m.showOption(ctx, sessionName, "@imtty_port"); metaErr == nil {
			return nil
		} else if !errors.Is(metaErr, errExit) {
			return metaErr
		}

		if _, killErr := m.runner.Run(ctx, "kill-session", "-t", sessionName); killErr != nil {
			return killErr
		}
	} else if !errors.Is(err, errExit) {
		return err
	}

	port, err := m.portAllocator()
	if err != nil {
		return err
	}

	listenURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
	if _, err := m.runner.Run(ctx, "new-session", "-d", "-s", sessionName, "-c", root, m.codexBin, "app-server", "--listen", listenURL); err != nil {
		return err
	}

	return m.setOption(ctx, sessionName, "@imtty_port", strconv.Itoa(port))
}

func (m *Manager) SendText(ctx context.Context, sessionName string, text string) error {
	if err := m.validateSessionName(sessionName); err != nil {
		return err
	}

	if _, err := m.runner.Run(ctx, "send-keys", "-l", "-t", sessionName, "--", text); err != nil {
		return err
	}

	_, err := m.runner.Run(ctx, "send-keys", "-t", sessionName, "Enter")
	return err
}

func (m *Manager) KillSession(ctx context.Context, sessionName string) error {
	if err := m.validateSessionName(sessionName); err != nil {
		return err
	}

	_, err := m.runner.Run(ctx, "kill-session", "-t", sessionName)
	return err
}

func (m *Manager) SessionMetadata(ctx context.Context, sessionName string) (SessionRuntimeInfo, error) {
	if err := m.validateSessionName(sessionName); err != nil {
		return SessionRuntimeInfo{}, err
	}

	portRaw, err := m.showOption(ctx, sessionName, "@imtty_port")
	if err != nil {
		return SessionRuntimeInfo{}, err
	}

	port, err := strconv.Atoi(strings.TrimSpace(portRaw))
	if err != nil || port <= 0 {
		return SessionRuntimeInfo{}, fmt.Errorf("invalid @imtty_port for %s", sessionName)
	}

	threadID, err := m.showOption(ctx, sessionName, "@imtty_thread_id")
	if err != nil && !errors.Is(err, errExit) {
		return SessionRuntimeInfo{}, err
	}

	return SessionRuntimeInfo{
		Port:     port,
		ThreadID: strings.TrimSpace(threadID),
	}, nil
}

func (m *Manager) SetThreadID(ctx context.Context, sessionName string, threadID string) error {
	if err := m.validateSessionName(sessionName); err != nil {
		return err
	}
	if strings.TrimSpace(threadID) == "" {
		return errors.New("thread id is required")
	}

	return m.setOption(ctx, sessionName, "@imtty_thread_id", threadID)
}

func (m *Manager) HasWritableAttachedClients(ctx context.Context, sessionName string) (bool, error) {
	if err := m.validateSessionName(sessionName); err != nil {
		return false, err
	}

	output, err := m.runner.Run(ctx, "list-clients", "-t", sessionName, "-F", "#{client_name} #{client_readonly}")
	if err != nil {
		if errors.Is(err, errExit) {
			return false, nil
		}
		return false, err
	}

	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) == 1 || fields[len(fields)-1] != "1" {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) ListSessions(ctx context.Context) ([]string, error) {
	output, err := m.runner.Run(ctx, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		if errors.Is(err, errExit) {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	sessions := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m.prefix != "" && !strings.HasPrefix(line, m.prefix) {
			continue
		}
		sessions = append(sessions, line)
	}
	sort.Strings(sessions)
	return sessions, nil
}

func (m *Manager) setOption(ctx context.Context, sessionName string, option string, value string) error {
	_, err := m.runner.Run(ctx, "set-option", "-q", "-t", sessionName, option, value)
	return err
}

func (m *Manager) showOption(ctx context.Context, sessionName string, option string) (string, error) {
	output, err := m.runner.Run(ctx, "show-options", "-v", "-t", sessionName, option)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (m *Manager) validateSessionName(sessionName string) error {
	if sessionName == "" {
		return errors.New("session name is required")
	}

	if m.prefix != "" && !strings.HasPrefix(sessionName, m.prefix) {
		return fmt.Errorf("session %q must use prefix %q", sessionName, m.prefix)
	}

	return nil
}

type CommandRunner struct {
	binary string
}

func NewCommandRunner(binary string) *CommandRunner {
	if strings.TrimSpace(binary) == "" {
		binary = "tmux"
	}
	return &CommandRunner{binary: binary}
}

func (r *CommandRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, r.binary, args...)
	output, err := command.CombinedOutput()
	if err == nil {
		return output, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return output, errExit
		}
		return output, fmt.Errorf("%w: %s", errExit, trimmed)
	}

	return output, err
}

func joinArgs(args []string) string {
	return strings.Join(args, " ")
}

func allocateLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("unexpected listener address type")
	}
	return address.Port, nil
}
