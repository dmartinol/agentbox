package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeleteProtectsActiveAndReviewableRunData(t *testing.T) {
	for _, tc := range []struct {
		name   string
		active bool
		edit   bool
		want   string
	}{
		{name: "active", active: true},
		{name: "unsynced", active: false, want: "unsynced"},
		{name: "review changes", active: false, edit: true, want: "review changes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, run, _ := createSyncFixture(t)
			run.Synced = true
			if tc.name == "unsynced" {
				run.Synced = false
			}
			if !tc.active {
				run.Status = StatusStopped
			} else {
				run.Status = StatusWorking
			}
			if tc.edit {
				if err := os.WriteFile(filepath.Join(run.Worktree, "tracked.txt"), []byte("review me\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := app.Store.SaveRun(run); err != nil {
				t.Fatal(err)
			}
			runner := newSyncRunner(t, nil)
			app.Runner = runner
			err := app.Delete(context.Background(), run.ID, false)
			if err == nil {
				t.Fatal("run with protected data was deleted")
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected delete refusal: %v", err)
			}
			if tc.active && !strings.Contains(err.Error(), "run is Working") {
				t.Fatalf("active run refusal was unclear: %v", err)
			}
			for _, call := range runner.outputs {
				if call.name == "openshell" {
					t.Fatalf("sandbox delete attempted before guard: %#v", call)
				}
			}
		})
	}
}

func TestStopRetainsSandboxWhenSyncRefusesLocalConflict(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	if err := os.WriteFile(filepath.Join(run.Worktree, "tracked.txt"), []byte("local edit during run\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := newSyncRunner(t, nil)
	app.Runner = runner
	err := app.Stop(context.Background(), run.ID)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected sync refusal, got %v", err)
	}
	for _, call := range runner.outputs {
		if call.name == "openshell" && commandHasArg(call, "stop") {
			t.Fatal("sandbox stopped despite sync failure")
		}
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || stored.Status != StatusWorking {
		t.Fatalf("run state did not retain recovery status: %+v, %v", stored, err)
	}
}

func TestStopSyncsBeforeStoppingSandbox(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts := createAgentExport(t, run, t.TempDir())
	runner := newSyncRunner(t, artifacts)
	app.Runner = runner
	if err := app.Stop(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || stored.Status != StatusStopped || !stored.Synced {
		t.Fatalf("run did not record successful stop/sync: %+v, %v", stored, err)
	}
	if stored.StatusDetail != "stopped by user" {
		t.Fatalf("stop detail = %q", stored.StatusDetail)
	}
	stopIndex, syncDownloadIndex := -1, -1
	for i, call := range runner.outputs {
		if call.name == "openshell" && commandHasArg(call, "stop") {
			stopIndex = i
		}
		if call.name == "openshell" && commandHasArg(call, "download") {
			syncDownloadIndex = i
		}
	}
	if syncDownloadIndex < 0 || stopIndex < syncDownloadIndex {
		t.Fatalf("sandbox stop did not follow sync: download=%d stop=%d", syncDownloadIndex, stopIndex)
	}
}

func TestListAndReconcileAttemptsSyncWhenCompletedMarkerExists(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	store := app.Store
	run.Synced = false
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "git" {
			return (OSRunner{}).Output(context.Background(), "git", call.args...)
		}
		if call.name == "openshell" && commandHasArg(call, "get") {
			return []byte(`{"phase":"running"}`), nil
		}
		if call.name == "openshell" && strings.Contains(strings.Join(call.args, " "), "tmux has-session -t agent") {
			return []byte("session=0\ndead=1\nfinished=1\nexit=0\n"), nil
		}
		return nil, nil
	}
	app.Runner = runner
	got, err := app.ListAndReconcile(context.Background())
	if err != nil || len(got) != 1 || got[0].Status != StatusCompleted || !strings.Contains(got[0].StatusDetail, "exit code") {
		t.Fatalf("reconcile result = %#v, %v", got, err)
	}
	foundExport := false
	for _, call := range runner.outputs {
		if call.name == "openshell" && strings.Contains(strings.Join(call.args, " "), "/workspace/export.bundle") {
			foundExport = true
		}
	}
	if !foundExport {
		t.Fatal("completed run was not offered to the sync path")
	}
}

func TestStartReportsMissingProfileAndPolicy(t *testing.T) {
	ctx := context.Background()
	store := Store{Base: t.TempDir()}
	app := App{Store: store, Runner: &fakeRunner{}}

	if _, err := app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell"}); err == nil || !strings.Contains(err.Error(), `profile "mycodex" not found`) {
		t.Fatalf("missing profile error = %v", err)
	}

	policy := filepath.Join(store.PolicyDir(), "missing.yaml")
	if err := store.SaveProfile(Profile{
		Version:  1,
		Name:     "mycodex",
		Agent:    "codex",
		Image:    "localhost/openshell-codex:1.2.3-r3",
		Mode:     "standard",
		Provider: "provider",
		Policy:   policy,
		Approval: "prompted",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell"}); err == nil || !strings.Contains(err.Error(), "references missing policy file") {
		t.Fatalf("missing policy error = %v", err)
	}
}

func TestBootstrapKillsAgentSessionWhenAgentExits(t *testing.T) {
	for _, want := range []string{"/workspace/state/session.sh", "touch /workspace/finished", "tmux kill-session -t agent", "exec sleep infinity", "export LANG=C.UTF-8", "export LC_ALL=C.UTF-8"} {
		if !strings.Contains(bootstrapScript, want) {
			t.Fatalf("bootstrap script missing %q:\n%s", want, bootstrapScript)
		}
	}
}

func TestAttachArgsRequestUTF8Locale(t *testing.T) {
	args := attachArgs(Run{Sandbox: "run-test"}, agentSessionName)
	joined := strings.Join(args, " ")
	for _, want := range []string{"--tty", "--env LC_ALL=C.UTF-8", "--env LANG=C.UTF-8", "-- tmux attach -t agent"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("attach args missing %q: %v", want, args)
		}
	}
	if strings.Index(joined, "--env LC_ALL") > strings.Index(joined, "-- tmux") {
		t.Fatalf("locale env must precede the command separator: %v", args)
	}
}

func TestSandboxExecRequestsUTF8Locale(t *testing.T) {
	var recorded recordedCommand
	app := App{Runner: &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		recorded = call
		return nil, nil
	}}}
	if _, err := app.sandboxExec(context.Background(), Run{Sandbox: "run-test"}, "true"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(recorded.args, " ")
	if !strings.Contains(joined, "--env LC_ALL=C.UTF-8 --env LANG=C.UTF-8 -- true") {
		t.Fatalf("sandbox exec args lack UTF-8 locale: %v", recorded.args)
	}
}

func TestWaitForSandboxDeletedPollsUntilGone(t *testing.T) {
	getCalls := 0
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && commandHasArg(call, "get") {
			getCalls++
			if getCalls < 3 {
				return []byte(`{"phase":"Deleting"}`), nil
			}
			return nil, errors.New("sandbox not found")
		}
		return nil, nil
	}}
	app := App{Runner: runner}
	run := Run{ID: "run-unit123", Sandbox: "sandbox-unit123", Gateway: "gateway-one"}
	if err := app.waitForSandboxDeletedWith(context.Background(), run, time.Second, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if getCalls != 3 {
		t.Fatalf("get calls = %d; want 3", getCalls)
	}
}

func TestSandboxCreateArgsUsesExactlyOneProvider(t *testing.T) {
	run := Run{ID: "run-unit", Sandbox: "sandbox-unit", Policy: "/tmp/policy.yaml"}
	profile := testProfile("codex", "standard", "prompted")
	args := sandboxCreateArgs(run, profile, "gateway-one")
	if countArg(args, "--provider") != 1 {
		t.Fatalf("provider flags = %v; want exactly one", args)
	}
	for i, arg := range args {
		if arg != "--provider" {
			continue
		}
		if i+1 >= len(args) || args[i+1] != profile.Provider {
			t.Fatalf("provider value = %v; want %q", args, profile.Provider)
		}
	}
	for _, arg := range args {
		if strings.Contains(arg, "git-github") {
			t.Fatalf("sandbox args carry a source-control provider: %v", args)
		}
	}
}

func countArg(args []string, want string) int {
	count := 0
	for _, arg := range args {
		if arg == want {
			count++
		}
	}
	return count
}

// Attaching immediately after start used to fail with "no sessions" because the
// sandbox bootstrap creates the tmux session just after the run reports running.
func TestAttachSessionWaitsForTmuxSession(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	run := Run{ID: "run-unit123", Sandbox: "sandbox-unit123", Gateway: "gateway-one", Status: "running"}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	probes := 0
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if commandHasArg(call, "has-session") {
			probes++
			if probes < 3 {
				return nil, errors.New("no sessions")
			}
			return nil, nil
		}
		return nil, errors.New("missing")
	}}
	app := App{Store: store, Runner: runner}
	if err := app.AttachSessionStream(context.Background(), run.ID, "agent", strings.NewReader(""), io.Discard, io.Discard, DefaultTerminalSize, nil); err != nil {
		t.Fatal(err)
	}
	if probes != 3 {
		t.Fatalf("session probes = %d, want 3", probes)
	}
	if len(runner.streams) != 1 || !commandHasArg(runner.streams[0], "attach") {
		t.Fatalf("attach stream not issued: %#v", runner.streams)
	}
}

func TestAttachSessionFailsFastWhenAgentEnded(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	run := Run{ID: "run-unit123", Sandbox: "sandbox-unit123", Status: "running"}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if commandHasArg(call, "has-session") {
			return nil, errors.New("no sessions")
		}
		return nil, nil
	}}
	app := App{Store: store, Runner: runner}
	err := app.AttachSessionStream(context.Background(), run.ID, "agent", strings.NewReader(""), io.Discard, io.Discard, DefaultTerminalSize, nil)
	if err == nil || !strings.Contains(err.Error(), "has ended") {
		t.Fatalf("attach error = %v", err)
	}
	if len(runner.streams) != 0 {
		t.Fatalf("attach stream issued for an ended session: %#v", runner.streams)
	}
}
