package tmux

import (
	"context"
	"reflect"
	"testing"

	"imtty/internal/session"
)

func TestManagerEnsureSessionCreatesAndStartsAppServerWhenMissing(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]runResult{
			"has-session -t codex-project-a": {err: errExit},
		},
	}

	manager := NewManager(runner, "codex-", "codex")
	manager.portAllocator = func() (int, error) { return 42001, nil }
	err := manager.EnsureSession(context.Background(), "codex-project-a", "/tmp/project-a")
	if err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}

	want := []string{
		"has-session -t codex-project-a",
		"new-session -d -s codex-project-a -c /tmp/project-a codex app-server --listen ws://127.0.0.1:42001",
		"set-option -q -t codex-project-a @imtty_port 42001",
	}

	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestManagerEnsureSessionSkipsCreateWhenSessionExists(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]runResult{
			"has-session -t codex-project-a":                 {},
			"show-options -v -t codex-project-a @imtty_port": {output: []byte("42001\n")},
		},
	}

	manager := NewManager(runner, "codex-", "codex")
	err := manager.EnsureSession(context.Background(), "codex-project-a", "/tmp/project-a")
	if err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}

	want := []string{
		"has-session -t codex-project-a",
		"show-options -v -t codex-project-a @imtty_port",
	}

	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestManagerEnsureSessionRecreatesLegacySessionWithoutMetadata(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]runResult{
			"has-session -t codex-project-a":                 {},
			"show-options -v -t codex-project-a @imtty_port": {err: errExit},
		},
	}

	manager := NewManager(runner, "codex-", "codex")
	manager.portAllocator = func() (int, error) { return 42001, nil }
	err := manager.EnsureSession(context.Background(), "codex-project-a", "/tmp/project-a")
	if err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}

	want := []string{
		"has-session -t codex-project-a",
		"show-options -v -t codex-project-a @imtty_port",
		"kill-session -t codex-project-a",
		"new-session -d -s codex-project-a -c /tmp/project-a codex app-server --listen ws://127.0.0.1:42001",
		"set-option -q -t codex-project-a @imtty_port 42001",
	}

	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestManagerSessionMetadataReadsPortAndThreadID(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]runResult{
			"show-options -v -t codex-project-a @imtty_port":                {output: []byte("42001\n")},
			"show-options -v -t codex-project-a @imtty_thread_id":           {output: []byte("thread-123\n")},
			"show-options -v -t codex-project-a @imtty_pending_model":       {output: []byte("gpt-5.5\n")},
			"show-options -v -t codex-project-a @imtty_pending_reasoning":   {output: []byte("xhigh\n")},
			"show-options -v -t codex-project-a @imtty_pending_plan_mode":   {output: []byte("plan\n")},
			"show-options -v -t codex-project-a @imtty_effective_plan_mode": {output: []byte("default\n")},
			"show-options -v -t codex-project-a @imtty_context_window":      {output: []byte("258000\n")},
			"show-options -v -t codex-project-a @imtty_total_tokens":        {output: []byte("188000\n")},
		},
	}

	manager := NewManager(runner, "codex-", "codex")
	info, err := manager.SessionMetadata(context.Background(), "codex-project-a")
	if err != nil {
		t.Fatalf("SessionMetadata() error = %v", err)
	}

	if info.Port != 42001 {
		t.Fatalf("Port = %d, want %d", info.Port, 42001)
	}
	if info.ThreadID != "thread-123" {
		t.Fatalf("ThreadID = %q, want %q", info.ThreadID, "thread-123")
	}
	if info.Metadata.Pending.Model != "gpt-5.5" {
		t.Fatalf("Pending.Model = %q, want %q", info.Metadata.Pending.Model, "gpt-5.5")
	}
	if info.Metadata.Pending.Reasoning != "xhigh" {
		t.Fatalf("Pending.Reasoning = %q, want %q", info.Metadata.Pending.Reasoning, "xhigh")
	}
	if info.Metadata.Pending.PlanMode != session.PlanModePlan {
		t.Fatalf("Pending.PlanMode = %q, want %q", info.Metadata.Pending.PlanMode, session.PlanModePlan)
	}
	if info.Metadata.EffectivePlanMode != session.PlanModeDefault {
		t.Fatalf("EffectivePlanMode = %q, want %q", info.Metadata.EffectivePlanMode, session.PlanModeDefault)
	}
	if info.Metadata.TokenUsage.ContextWindow != 258000 || info.Metadata.TokenUsage.TotalTokens != 188000 {
		t.Fatalf("TokenUsage = %#v, want context 258000 total 188000", info.Metadata.TokenUsage)
	}
}

func TestManagerSetThreadIDStoresTmuxOption(t *testing.T) {
	runner := &fakeRunner{}
	manager := NewManager(runner, "codex-", "codex")

	if err := manager.SetThreadID(context.Background(), "codex-project-a", "thread-123"); err != nil {
		t.Fatalf("SetThreadID() error = %v", err)
	}

	want := []string{
		"set-option -q -t codex-project-a @imtty_thread_id thread-123",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestManagerSetRuntimeMetadataStoresAndClearsTmuxOptions(t *testing.T) {
	runner := &fakeRunner{}
	manager := NewManager(runner, "codex-", "codex")

	err := manager.SetRuntimeMetadata(context.Background(), "codex-project-a", session.RuntimeMetadata{
		Pending: session.ControlSelection{
			Model:     "gpt-5.5",
			Reasoning: "xhigh",
			PlanMode:  session.PlanModePlan,
		},
		EffectivePlanMode: session.PlanModeDefault,
		TokenUsage: session.TokenUsage{
			ContextWindow: 258000,
			TotalTokens:   188000,
		},
	})
	if err != nil {
		t.Fatalf("SetRuntimeMetadata() error = %v", err)
	}

	err = manager.SetRuntimeMetadata(context.Background(), "codex-project-a", session.RuntimeMetadata{})
	if err != nil {
		t.Fatalf("SetRuntimeMetadata(clear) error = %v", err)
	}

	want := []string{
		"set-option -q -t codex-project-a @imtty_pending_model gpt-5.5",
		"set-option -q -t codex-project-a @imtty_pending_reasoning xhigh",
		"set-option -q -t codex-project-a @imtty_pending_plan_mode plan",
		"set-option -q -t codex-project-a @imtty_effective_plan_mode default",
		"set-option -q -t codex-project-a @imtty_context_window 258000",
		"set-option -q -t codex-project-a @imtty_total_tokens 188000",
		"set-option -qu -t codex-project-a @imtty_pending_model",
		"set-option -qu -t codex-project-a @imtty_pending_reasoning",
		"set-option -qu -t codex-project-a @imtty_pending_plan_mode",
		"set-option -qu -t codex-project-a @imtty_effective_plan_mode",
		"set-option -qu -t codex-project-a @imtty_context_window",
		"set-option -qu -t codex-project-a @imtty_total_tokens",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestManagerHasWritableAttachedClients(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]runResult{
			"list-clients -t codex-project-a -F #{client_name} #{client_readonly}": {output: []byte("/dev/ttys001 0\n/dev/ttys002 1\n")},
		},
	}
	manager := NewManager(runner, "codex-", "codex")

	attached, err := manager.HasWritableAttachedClients(context.Background(), "codex-project-a")
	if err != nil {
		t.Fatalf("HasWritableAttachedClients() error = %v", err)
	}
	if !attached {
		t.Fatalf("attached = false, want true")
	}
}

func TestManagerHasWritableAttachedClientsIgnoresReadonlyClients(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]runResult{
			"list-clients -t codex-project-a -F #{client_name} #{client_readonly}": {output: []byte("/dev/ttys001 1\n/dev/ttys002 1\n")},
		},
	}
	manager := NewManager(runner, "codex-", "codex")

	attached, err := manager.HasWritableAttachedClients(context.Background(), "codex-project-a")
	if err != nil {
		t.Fatalf("HasWritableAttachedClients() error = %v", err)
	}
	if attached {
		t.Fatalf("attached = true, want false for readonly-only clients")
	}
}

func TestManagerSendTextInjectsEnterTerminatedMessage(t *testing.T) {
	runner := &fakeRunner{}
	manager := NewManager(runner, "codex-", "codex")

	err := manager.SendText(context.Background(), "codex-project-a", "hello from telegram")
	if err != nil {
		t.Fatalf("SendText() error = %v", err)
	}

	want := []string{
		"send-keys -l -t codex-project-a -- hello from telegram",
		"send-keys -t codex-project-a Enter",
	}

	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestManagerKillSessionTerminatesTmuxSession(t *testing.T) {
	runner := &fakeRunner{}
	manager := NewManager(runner, "codex-", "codex")

	err := manager.KillSession(context.Background(), "codex-project-a")
	if err != nil {
		t.Fatalf("KillSession() error = %v", err)
	}

	want := []string{
		"kill-session -t codex-project-a",
	}

	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestManagerListSessionsFiltersByPrefix(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]runResult{
			"list-sessions -F #{session_name}": {output: []byte("codex-project-a\nother\ncodex-project-b\n")},
		},
	}
	manager := NewManager(runner, "codex-", "codex")

	sessions, err := manager.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}

	want := []string{"codex-project-a", "codex-project-b"}
	if !reflect.DeepEqual(sessions, want) {
		t.Fatalf("sessions = %#v, want %#v", sessions, want)
	}
}

type fakeRunner struct {
	commands []string
	outputs  map[string]runResult
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	command := joinArgs(args)
	f.commands = append(f.commands, command)

	if f.outputs == nil {
		return nil, nil
	}

	result, ok := f.outputs[command]
	if !ok {
		return nil, nil
	}

	return result.output, result.err
}

type runResult struct {
	output []byte
	err    error
}
