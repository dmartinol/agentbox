package app

import (
	"context"
	"strings"
	"testing"
)

func shellFixture(t *testing.T, sessions string) (App, Run, *fakeRunner) {
	t.Helper()
	store := Store{Base: t.TempDir()}
	run := Run{ID: "run-1", Sandbox: "sandbox-1", Gateway: "gw-1", Status: "running"}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if commandHasArg(call, "list-sessions") {
			return []byte(sessions), nil
		}
		return nil, nil
	}}
	return App{Store: store, Runner: runner}, run, runner
}

func findCommand(calls []recordedCommand, arg string) *recordedCommand {
	for index := range calls {
		if commandHasArg(calls[index], arg) {
			return &calls[index]
		}
	}
	return nil
}

func TestCreateShellUsesNextFreeSessionName(t *testing.T) {
	app, run, runner := shellFixture(t, "agent\nshell-1\nshell-3\n")
	name, err := app.CreateShell(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if name != "shell-2" {
		t.Fatalf("name = %q, want shell-2", name)
	}
	call := findCommand(runner.outputs, "new-session")
	if call == nil {
		t.Fatal("no new-session command recorded")
	}
	if !commandHasArg(*call, "shell-2") || !commandHasArg(*call, "/workspace/project") {
		t.Fatalf("new-session should start shell-2 in the run worktree: %v", call.args)
	}
	joined := strings.Join(call.args, " ")
	for _, want := range []string{"HOME=/workspace/state/home", "cd /workspace/project", "exec /bin/sh"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("shell command missing %q: %v", want, call.args)
		}
	}
	if call.args[1] != "sandbox" && !commandHasArg(*call, "sandbox-1") {
		t.Fatalf("shell should run in the run sandbox: %v", call.args)
	}
}

func TestCreateShellStartsAtShellOneWhenEmpty(t *testing.T) {
	app, run, _ := shellFixture(t, "agent\n")
	name, err := app.CreateShell(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if name != "shell-1" {
		t.Fatalf("name = %q, want shell-1", name)
	}
}

func TestDeleteShellRejectsAgentSession(t *testing.T) {
	app, run, runner := shellFixture(t, "agent\nshell-1\n")
	if err := app.DeleteShell(context.Background(), run.ID, "agent"); err == nil {
		t.Fatal("deleting the agent session should fail")
	}
	if findCommand(runner.outputs, "kill-session") != nil {
		t.Fatal("agent session must never be killed")
	}
}

func TestDeleteShellKillsNamedSession(t *testing.T) {
	app, run, runner := shellFixture(t, "agent\nshell-1\n")
	if err := app.DeleteShell(context.Background(), run.ID, "shell-1"); err != nil {
		t.Fatal(err)
	}
	call := findCommand(runner.outputs, "kill-session")
	if call == nil || !commandHasArg(*call, "shell-1") {
		t.Fatalf("expected kill-session for shell-1, got %v", call)
	}
}

func TestAttachSessionStreamRejectsUnknownSession(t *testing.T) {
	app, run, runner := shellFixture(t, "agent\n")
	err := app.AttachSessionStream(context.Background(), run.ID, "tmux; rm -rf /", strings.NewReader(""), nil, nil, TerminalSize{}, nil)
	if err == nil {
		t.Fatal("unknown session should be rejected")
	}
	if len(runner.streams) != 0 {
		t.Fatal("rejected session must not spawn a process")
	}
}

func TestAttachSessionStreamTargetsRequestedSession(t *testing.T) {
	app, run, runner := shellFixture(t, "agent\nshell-1\n")
	if err := app.AttachSessionStream(context.Background(), run.ID, "shell-1", strings.NewReader(""), nil, nil, TerminalSize{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(runner.streams) != 1 {
		t.Fatalf("streams = %d, want 1", len(runner.streams))
	}
	call := runner.streams[0]
	if !commandHasArg(call, "attach") || !commandHasArg(call, "shell-1") {
		t.Fatalf("attach should target shell-1: %v", call.args)
	}
}

func TestTailLogsForwardsLimit(t *testing.T) {
	app, run, runner := shellFixture(t, "agent\n")
	if _, err := app.TailLogs(context.Background(), run.ID, 250); err != nil {
		t.Fatal(err)
	}
	call := findCommand(runner.outputs, "-c")
	if call == nil {
		t.Fatal("no shell command recorded")
	}
	joined := strings.Join(call.args, " ")
	if !strings.Contains(joined, "tail -n 250 /workspace/state/agent.log") {
		t.Fatalf("expected bounded tail, got %v", call.args)
	}
}
