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

func assertNoSandboxRemoval(t *testing.T, runner *fakeRunner) {
	t.Helper()
	for _, call := range runner.outputs {
		if call.name == "openshell" && (containsCommand(call.args, "sandbox", "delete") || containsCommand(call.args, "sandbox", "stop")) {
			t.Fatalf("sandbox removal attempted before the guard: %#v", call)
		}
	}
}

// newDeleteRunner fakes a sandbox that is already gone while running git
// against the real repositories, as a repeated delete would encounter.
func newDeleteRunner() *fakeRunner {
	runner := &fakeRunner{}
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "git" {
			return (OSRunner{}).Output(context.Background(), "git", call.args...)
		}
		if call.name == "openshell" && containsCommand(call.args, "sandbox", "get") {
			return nil, errors.New("sandbox not found")
		}
		return nil, nil
	}
	return runner
}

func seedRunArtifacts(t *testing.T, store Store, id string) []string {
	t.Helper()
	paths := []string{
		filepath.Join(store.RunsDir(), id+"-source.bundle"),
		filepath.Join(store.RunsDir(), id+"-config.tar"),
		filepath.Join(store.RunsDir(), id+"-initial.patch"),
		filepath.Join(store.RunsDir(), id+"-mirror.patch"),
		filepath.Join(store.RunsDir(), id+"-mirror.tar"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("run artifact\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	configDir := filepath.Join(store.RunsDir(), id+"-config")
	if err := os.MkdirAll(filepath.Join(configDir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "nested", "file.txt"), []byte("config\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return append(paths, configDir)
}

func seedRunPolicies(t *testing.T, store Store, run Run) (policy, pending, fragment string) {
	t.Helper()
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	policy = filepath.Join(store.PolicyDir(), run.ID+".yaml")
	pending = filepath.Join(store.PolicyDir(), run.ID+"-pending.yaml")
	fragment = filepath.Join(store.PolicyDir(), run.ID+"-fragment.yaml")
	for path, body := range map[string]string{
		policy:   "policy: run\n",
		pending:  "policy: pending\n",
		fragment: "policy: shared\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return policy, pending, fragment
}

func TestDeleteToleratesWorktreeRemovedOutOfBand(t *testing.T) {
	for _, tc := range []struct {
		name        string
		leftoverDir bool
	}{
		{name: "directory gone"},
		{name: "leftover directory", leftoverDir: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, run, _ := createSyncFixture(t)
			artifactPaths := seedRunArtifacts(t, app.Store, run.ID)
			// Simulate cleanup that happened outside AgentBox: worktree and
			// branch are gone but the run record still references them.
			cmd(t, run.Source, "git", "worktree", "remove", "--force", run.Worktree)
			cmd(t, run.Source, "git", "branch", "-D", run.Branch)
			if tc.leftoverDir {
				// Simulate a stale worktree directory that git no longer
				// tracks, e.g. after the source repository was re-cloned.
				if err := os.MkdirAll(run.Worktree, 0700); err != nil {
					t.Fatal(err)
				}
				gitdir := filepath.Join(run.Source, ".git", "worktrees", run.ID)
				link := "gitdir: " + gitdir + "\n"
				if err := os.WriteFile(filepath.Join(run.Worktree, ".git"), []byte(link), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(run.Worktree, "leftover.txt"), []byte("stale\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			run.Status = StatusStopped
			if err := app.Store.SaveRun(run); err != nil {
				t.Fatal(err)
			}
			app.Runner = newDeleteRunner()
			if err := app.Delete(context.Background(), run.ID, true); err != nil {
				t.Fatalf("delete got stuck on removed worktree: %v", err)
			}
			if _, err := app.Store.LoadRun(run.ID); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("run record still present: %v", err)
			}
			if _, err := os.Stat(run.Worktree); !os.IsNotExist(err) {
				t.Fatalf("worktree directory still present: %v", err)
			}
			if branches := cmd(t, run.Source, "git", "branch", "--list", run.Branch); strings.TrimSpace(branches) != "" {
				t.Fatalf("branch still present: %s", branches)
			}
			for _, path := range artifactPaths {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("run artifact still present after delete: %s (%v)", path, err)
				}
			}
		})
	}
}

func TestDeleteRemovesAllRunArtifactsPoliciesAndRecord(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	policy, pending, fragment := seedRunPolicies(t, app.Store, run)
	run.Status = StatusStopped
	run.Policy = policy
	run.PendingPolicy = pending
	run.PolicyFragment = fragment
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	artifactPaths := seedRunArtifacts(t, app.Store, run.ID)
	unrelated := filepath.Join(app.Store.RunsDir(), "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("keep\n"), 0600); err != nil {
		t.Fatal(err)
	}
	app.Runner = newDeleteRunner()
	if err := app.Delete(context.Background(), run.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range artifactPaths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("run artifact still present: %s (%v)", path, err)
		}
	}
	for _, path := range []string{policy, pending} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("run policy still present: %s (%v)", path, err)
		}
	}
	for _, path := range []string{fragment, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("shared path was removed: %s (%v)", path, err)
		}
	}
	if _, err := app.Store.LoadRun(run.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("run record still present: %v", err)
	}
}

func TestDeleteRetainsRunArtifactsWhenSandboxDeleteFails(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	policy, pending, _ := seedRunPolicies(t, app.Store, run)
	run.Status = StatusStopped
	run.Policy = policy
	run.PendingPolicy = pending
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	artifactPaths := seedRunArtifacts(t, app.Store, run.ID)
	runner := newSyncRunnerWithProbe(t, nil, sandboxProbeJSON(t, run.Worktree))
	base := runner.output
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && containsCommand(call.args, "sandbox", "delete") {
			return nil, errors.New("simulated sandbox delete failure")
		}
		return base(call)
	}
	app.Runner = runner
	err := app.Delete(context.Background(), run.ID, true)
	if err == nil || !strings.Contains(err.Error(), "sandbox cleanup failed") {
		t.Fatalf("sandbox failure was unclear: %v", err)
	}
	if !strings.Contains(err.Error(), "run record retained") || !strings.Contains(err.Error(), "every run artifact retained") {
		t.Fatalf("retention was not named: %v", err)
	}
	for _, path := range append([]string{app.Store.RunPath(run.ID)}, artifactPaths...) {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("path was removed after sandbox failure: %s (%v)", path, statErr)
		}
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("retained path was not named in error: %s", path)
		}
	}
	for _, path := range []string{policy, pending} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("policy was removed after sandbox failure: %s (%v)", path, statErr)
		}
	}
}

func TestDeleteRetainsRunArtifactsWhenWorktreeOrBranchCleanupFails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		first  string
		second string
		stage  string
	}{
		{name: "worktree", first: "worktree", second: "remove", stage: "worktree cleanup failed"},
		{name: "branch", first: "branch", second: "-D", stage: "branch cleanup failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, run, _ := createSyncFixture(t)
			policy, pending, _ := seedRunPolicies(t, app.Store, run)
			run.Status = StatusStopped
			run.Policy = policy
			run.PendingPolicy = pending
			if err := app.Store.SaveRun(run); err != nil {
				t.Fatal(err)
			}
			artifactPaths := seedRunArtifacts(t, app.Store, run.ID)
			runner := newDeleteRunner()
			base := runner.output
			runner.output = func(call recordedCommand) ([]byte, error) {
				if call.name == "git" && containsCommand(call.args, tc.first, tc.second) {
					return nil, errors.New("simulated git cleanup failure")
				}
				return base(call)
			}
			app.Runner = runner
			err := app.Delete(context.Background(), run.ID, true)
			if err == nil || !strings.Contains(err.Error(), tc.stage) {
				t.Fatalf("cleanup failure was unclear: %v", err)
			}
			retained := append([]string{app.Store.RunPath(run.ID), policy, pending}, artifactPaths...)
			for _, path := range retained {
				if _, statErr := os.Stat(path); statErr != nil {
					t.Fatalf("path was removed after %s: %s (%v)", tc.stage, path, statErr)
				}
				if !strings.Contains(err.Error(), path) {
					t.Fatalf("retained path was not named: %s", path)
				}
			}
		})
	}
}

func TestDeleteKeepsUnrelatedDirectoryAtRecordedWorktreePath(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	cmd(t, run.Source, "git", "worktree", "remove", "--force", run.Worktree)
	cmd(t, run.Source, "git", "branch", "-D", run.Branch)
	// An unrelated directory at the recorded path must not be destroyed.
	notes := filepath.Join(run.Worktree, "notes.txt")
	if err := os.MkdirAll(run.Worktree, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notes, []byte("user data\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run.Status = StatusStopped
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	app.Runner = newDeleteRunner()
	if err := app.Delete(context.Background(), run.ID, true); err != nil {
		t.Fatalf("delete got stuck on unrelated directory: %v", err)
	}
	if _, err := os.Stat(notes); err != nil {
		t.Fatalf("unrelated directory was removed: %v", err)
	}
}

func TestDeleteProtectsActiveRun(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	run.Status = StatusWorking
	run.Synced = true
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := newSyncRunner(t, nil)
	app.Runner = runner
	err := app.Delete(context.Background(), run.ID, false)
	if err == nil || !strings.Contains(err.Error(), "run is Working") {
		t.Fatalf("active run refusal was unclear: %v", err)
	}
	assertNoSandboxRemoval(t, runner)
}

func TestDeleteRefusesUnsyncedRun(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	ctx := context.Background()
	sandbox := cloneSandboxRepo(t, run)
	if err := os.WriteFile(filepath.Join(sandbox, "tracked.txt"), []byte("agent edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, sandbox, "git", "commit", "-qam", "agent edit")
	hostFingerprint, err := app.WorktreeFingerprint(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	// A completed run still holds the record, so its sandbox is probeable and
	// the derived state sees sandbox work that was never synced.
	run.Status = StatusCompleted
	run.Synced = true
	run.Fingerprint = hostFingerprint
	run.SyncedSandboxFingerprint = "stale-sandbox-fingerprint"
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := newSyncRunnerWithProbe(t, nil, sandboxProbeJSON(t, sandbox))
	app.Runner = runner
	err = app.Delete(ctx, run.ID, false)
	if err == nil || !strings.Contains(err.Error(), "unsynced") {
		t.Fatalf("unsynced refusal = %v", err)
	}
	assertNoSandboxRemoval(t, runner)
}

func TestDeleteRefusesUnknownSyncState(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	run.Status = StatusStopped
	run.Synced = true
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := newSyncRunner(t, nil)
	app.Runner = runner
	err := app.Delete(context.Background(), run.ID, false)
	if err == nil || !strings.Contains(err.Error(), "cannot verify") {
		t.Fatalf("unknown sync state refusal = %v", err)
	}
	assertNoSandboxRemoval(t, runner)
}

func TestDeleteRefusesReviewChanges(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	run.Status = StatusStopped
	run.Synced = true
	if err := os.WriteFile(filepath.Join(run.Worktree, "tracked.txt"), []byte("review me\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := newSyncRunnerWithProbe(t, nil, sandboxProbeJSON(t, run.Worktree))
	app.Runner = runner
	err := app.Delete(context.Background(), run.ID, false)
	if err == nil || !strings.Contains(err.Error(), "review changes") {
		t.Fatalf("review-changes refusal = %v", err)
	}
	assertNoSandboxRemoval(t, runner)
}

const (
	readyProbe   = "session=1\ndead=0\ncommand=sh\nplan=0\nidle=0\nfinished=0\nexit=\nactivity={\"state\":\"idle\"}\nmtime=\n"
	workingProbe = "session=1\ndead=0\ncommand=codex\nplan=0\nidle=0\nfinished=0\nexit=\nactivity={\"state\":\"working\",\"detail\":\"turn\"}\nmtime=\n"
	exitedProbe  = "session=1\ndead=0\ncommand=sh\nplan=0\nidle=0\nfinished=0\nexit=0\nactivity={\"state\":\"idle\"}\nmtime=\n"
	probeMarker  = "printf 'session=%s"
)

// readyLifecycleRunner answers the status probe with a Ready run and delegates
// everything else to the sync fixture runner.
func readyLifecycleRunner(t *testing.T, artifacts map[string]string) *fakeRunner {
	t.Helper()
	runner := newSyncRunner(t, artifacts)
	base := runner.output
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && strings.Contains(strings.Join(call.args, " "), probeMarker) {
			return []byte(readyProbe), nil
		}
		return base(call)
	}
	return runner
}

func TestSleepRetainsSandboxWhenSyncRefusesLocalConflict(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	if err := os.WriteFile(filepath.Join(run.Worktree, "tracked.txt"), []byte("local edit during run\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := readyLifecycleRunner(t, nil)
	app.Runner = runner
	err := app.Sleep(context.Background(), run.ID, false)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected sync refusal, got %v", err)
	}
	for _, call := range runner.outputs {
		if call.name == "openshell" && containsCommand(call.args, "sandbox", "stop") {
			t.Fatal("sandbox stopped despite sync failure")
		}
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || stored.Status != StatusReady {
		t.Fatalf("run state did not retain the resumable status: %+v, %v", stored, err)
	}
}

func TestSleepRefusesWorkingWithoutForce(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	runner := newSyncRunner(t, nil)
	app.Runner = runner
	err := app.Sleep(context.Background(), run.ID, false)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("working sleep refusal = %v", err)
	}
	for _, call := range runner.outputs {
		if call.name == "openshell" && containsCommand(call.args, "sandbox", "stop") {
			t.Fatal("sandbox stopped despite the Working gate")
		}
	}
}

func TestSleepForceInterruptsWorkingAgent(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts := createAgentExport(t, run, t.TempDir())
	runner := newSyncRunner(t, artifacts)
	base := runner.output
	interrupted := false
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && commandHasArg(call, "send-keys") {
			interrupted = true
			return nil, nil
		}
		if call.name == "openshell" && strings.Contains(strings.Join(call.args, " "), probeMarker) {
			if interrupted {
				return []byte(readyProbe), nil
			}
			return []byte(workingProbe), nil
		}
		return base(call)
	}
	app.Runner = runner
	if err := app.Sleep(context.Background(), run.ID, true); err != nil {
		t.Fatal(err)
	}
	if !interrupted {
		t.Fatal("forced sleep did not interrupt the working turn")
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || stored.Status != StatusStopped || !stored.Synced {
		t.Fatalf("run did not record successful sleep/sync: %+v, %v", stored, err)
	}
}

func TestSleepSyncsBeforeStoppingSandbox(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts := createAgentExport(t, run, t.TempDir())
	runner := readyLifecycleRunner(t, artifacts)
	app.Runner = runner
	if err := app.Sleep(context.Background(), run.ID, false); err != nil {
		t.Fatal(err)
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || stored.Status != StatusStopped || !stored.Synced {
		t.Fatalf("run did not record successful sleep/sync: %+v, %v", stored, err)
	}
	if stored.StatusDetail != "asleep; resume available" {
		t.Fatalf("sleep detail = %q", stored.StatusDetail)
	}
	stopIndex, syncDownloadIndex := -1, -1
	for i, call := range runner.outputs {
		if call.name == "openshell" && containsCommand(call.args, "sandbox", "stop") {
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

func TestResumeRefusesRunningAgent(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	app.Runner = readyLifecycleRunner(t, nil)
	err := app.Resume(context.Background(), run.ID)
	if err == nil || !strings.Contains(err.Error(), "nothing to resume") {
		t.Fatalf("running resume = %v", err)
	}
}

func TestResumeRelaunchesExitedAgentInPlace(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	runner := newSyncRunner(t, nil)
	base := runner.output
	launched := false
	runner.output = func(call recordedCommand) ([]byte, error) {
		joined := strings.Join(call.args, " ")
		if call.name == "openshell" && strings.Contains(joined, probeMarker) {
			if launched {
				return []byte(workingProbe), nil
			}
			return []byte(exitedProbe), nil
		}
		if call.name == "openshell" && strings.Contains(joined, "agent-entry.sh") {
			launched = true
			return nil, nil
		}
		return base(call)
	}
	app.Runner = runner
	if err := app.Resume(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	if !launched {
		t.Fatal("resume did not relaunch the agent entry point")
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || stored.Status != StatusWorking {
		t.Fatalf("resume did not return a working run: %+v, %v", stored, err)
	}
}

func TestResumeStartsStoppedSandbox(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	run.Status = StatusStopped
	run.StatusDetail = "asleep; resume available"
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := newSyncRunner(t, nil)
	base := runner.output
	started := false
	runner.output = func(call recordedCommand) ([]byte, error) {
		joined := strings.Join(call.args, " ")
		if call.name == "openshell" && containsCommand(call.args, "sandbox", "start") {
			started = true
			return nil, nil
		}
		if call.name == "openshell" && containsCommand(call.args, "sandbox", "get") {
			if started {
				return []byte(`{"phase":"running"}`), nil
			}
			return []byte(`{"phase":"stopped"}`), nil
		}
		if call.name == "openshell" && strings.Contains(joined, probeMarker) {
			return []byte(workingProbe), nil
		}
		return base(call)
	}
	app.Runner = runner
	if err := app.Resume(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	if !started {
		t.Fatal("resume did not start the stopped sandbox")
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || stored.Status != StatusWorking {
		t.Fatalf("resume did not return a working run: %+v, %v", stored, err)
	}
}

func TestFinishSyncsThenMarksCompleted(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts := createAgentExport(t, run, t.TempDir())
	runner := readyLifecycleRunner(t, artifacts)
	app.Runner = runner
	if err := app.Finish(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || stored.Status != StatusCompleted || !stored.Synced {
		t.Fatalf("finish did not mark the run completed: %+v, %v", stored, err)
	}
	finishIndex, syncDownloadIndex := -1, -1
	for i, call := range runner.outputs {
		if call.name == "openshell" && strings.Contains(strings.Join(call.args, " "), "/workspace/finished") {
			finishIndex = i
		}
		if call.name == "openshell" && commandHasArg(call, "download") {
			syncDownloadIndex = i
		}
	}
	if syncDownloadIndex < 0 || finishIndex < syncDownloadIndex {
		t.Fatalf("finish marker did not follow sync: download=%d finish=%d", syncDownloadIndex, finishIndex)
	}
}

func TestFinishRecordsTheUserReasonEvenWhenTheAgentAlreadyExited(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	code := 0
	run.Status = StatusReady
	run.StatusDetail = "agent exited (code 0); resume available"
	run.Exited = true
	run.ExitCode = &code
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	app.Runner = readyLifecycleRunner(t, createAgentExport(t, run, t.TempDir()))
	if err := app.Finish(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || stored.Status != StatusCompleted || stored.StatusDetail != "finished by user" {
		t.Fatalf("finish reason = %+v, %v", stored, err)
	}
}

func TestInspectKeepsTheUserFinishReason(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	run.Status = StatusCompleted
	run.StatusDetail = "finished by user"
	run.Synced = true
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	app.Runner = statusRunnerFor("running", "session=0\ndead=1\nfinished=1\nexit=0\n")
	got, err := app.GetRun(context.Background(), run.ID)
	if err != nil || got.Status != StatusCompleted || got.StatusDetail != "finished by user" {
		t.Fatalf("re-inspected finish = %+v, %v", got, err)
	}
}

func TestFinishRefusesWorking(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	run.Status = StatusWorking
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := statusRunnerFor("running", workingProbe)
	app.Runner = runner
	err := app.Finish(context.Background(), run.ID)
	if err == nil || !strings.Contains(err.Error(), "Working") {
		t.Fatalf("finish refusal = %v", err)
	}
	assertNoSandboxRemoval(t, runner)
}

func TestFinishMarksStoppedRunCompletedWithoutRestartingIt(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	run.Status = StatusStopped
	run.StatusDetail = "asleep; resume available"
	run.Synced = true
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := statusRunnerFor("stopped", "")
	app.Runner = runner
	if err := app.Finish(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || stored.Status != StatusCompleted || stored.StatusDetail != "finished by user" {
		t.Fatalf("stopped finish = %+v, %v", stored, err)
	}
	for _, call := range runner.outputs {
		if call.name == "openshell" && containsCommand(call.args, "sandbox", "start") {
			t.Fatalf("finish restarted the sleeping sandbox: %#v", call)
		}
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
		Version:   1,
		Name:      "mycodex",
		Agent:     "codex",
		Image:     "localhost/openshell-codex:1.2.3-r3",
		Mode:      "gateway",
		Provider:  "provider",
		Model:     "unit-model",
		BaseURL:   "https://models.example.test/v1",
		APIKeyEnv: "UNIT_KEY",
		Policy:    policy,
		Approval:  "prompted",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell", Mode: "bogus"}); err == nil || !strings.Contains(err.Error(), `profile "mycodex" is invalid: mode must be standard or gateway`) {
		t.Fatalf("invalid profile error = %v", err)
	}
	if _, err := app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell"}); err == nil || !strings.Contains(err.Error(), "references missing legacy policy file") {
		t.Fatalf("missing policy error = %v", err)
	}
}

func TestStartApprovalPrecedence(t *testing.T) {
	ctx := context.Background()
	source := t.TempDir()
	store := Store{Base: t.TempDir()}
	policy, err := store.CreatePolicyConfiguration("default")
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		switch {
		case call.name == "git":
			return []byte(source), nil
		case call.name == "openshell" && commandHasArg(call, "gateway"):
			return []byte(`{"type":"podman"}`), nil
		case call.name == "openshell" && commandHasArg(call, "provider"):
			return []byte(`{}`), nil
		case call.name == "podman" && commandHasArg(call, "inspect"):
			return []byte(`{}`), nil
		}
		return []byte(`{}`), nil
	}}
	app := App{Store: store, Runner: runner}

	for _, tc := range []struct {
		name            string
		profileApproval string
		override        string
		want            string
	}{
		{name: "existing prompted profile", profileApproval: "prompted", want: "prompted"},
		{name: "new unattended profile", profileApproval: "unattended", want: "unattended"},
		{name: "explicit per-run override", profileApproval: "unattended", override: "prompted", want: "prompted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := testProfile("codex", "standard", tc.profileApproval)
			profile.Policy = policy.Path
			if err := store.SaveProfile(profile); err != nil {
				t.Fatal(err)
			}
			run, err := app.Start(ctx, StartOptions{
				Profile:  profile.Name,
				Gateway:  "openshell",
				Source:   source,
				Approval: tc.override,
				Dry:      true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if run.Approval != tc.want {
				t.Fatalf("run approval = %q; want %q", run.Approval, tc.want)
			}
		})
	}
}

func TestBootstrapResumesInsteadOfFinishing(t *testing.T) {
	for _, want := range []string{
		"/workspace/state/session.sh initial",
		"/workspace/state/agent-started",
		"/workspace/state/agent-entry.sh",
		"tmux kill-session -t agent",
		"exec sleep infinity",
		"export LANG=C.UTF-8",
		"export LC_ALL=C.UTF-8",
	} {
		if !strings.Contains(bootstrapScript, want) {
			t.Fatalf("bootstrap script missing %q:\n%s", want, bootstrapScript)
		}
	}
	if strings.Contains(bootstrapScript, "touch /workspace/finished") {
		t.Fatalf("bootstrap must not finish a run when the agent pane dies:\n%s", bootstrapScript)
	}
	// remain-on-exit can only be set once the tmux server exists; the first
	// new-session starts it, so set-option must come after it or every fresh
	// sandbox dies with "error connecting" under set -eu.
	optionIndex := strings.Index(bootstrapScript, "tmux set-option -g remain-on-exit on")
	sessionIndex := strings.Index(bootstrapScript, "tmux new-session")
	if optionIndex < 0 || sessionIndex < 0 || optionIndex < sessionIndex {
		t.Fatalf("remain-on-exit must be set after the first new-session:\n%s", bootstrapScript)
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

func TestVerifyEffectivePolicyAcceptsGatewayWithExplicitPort(t *testing.T) {
	effectivePolicy := []byte(`{
		"active_version": 2,
		"policy_source": "sandbox",
		"status": "effective",
		"policy": {
			"version": 1,
			"landlock": {"compatibility": "hard_requirement"},
			"network_policies": {
				"_provider_unit-provider": {
					"name": "_provider_unit-provider",
					"endpoints": [{"host": "models.example.test", "port": 8443, "protocol": "tcp"}],
					"binaries": [{"path": "/usr/local/bin/codex"}]
				}
			}
		}
	}`)
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && commandHasArg(call, "policy") {
			return effectivePolicy, nil
		}
		return nil, nil
	}}
	profile := testProfile("codex", "gateway", "prompted")
	profile.BaseURL = "https://models.example.test:8443/v1"
	app := App{Runner: runner}
	run := Run{Sandbox: "sandbox-unit123", Gateway: "gateway-one", Mode: "gateway", Approval: "prompted"}

	if err := app.VerifyEffectivePolicy(context.Background(), run, profile); err != nil {
		t.Fatalf("valid explicit-port gateway policy rejected: %v", err)
	}

	effectivePolicy = []byte(strings.Replace(string(effectivePolicy), `"port": 8443`, `"port": 443`, 1))
	err := app.VerifyEffectivePolicy(context.Background(), run, profile)
	if err == nil || !strings.Contains(err.Error(), "models.example.test:8443") {
		t.Fatalf("wrong-port gateway policy error = %v", err)
	}
}

func TestVerifyEffectivePolicyRequiresComposedProviderEndpointInEveryMode(t *testing.T) {
	const providerRule = `"network_policies": {
		"_provider_openai": {
			"name": "_provider_openai",
			"endpoints": [{"host": "api.openai.com", "port": 443, "path": "/v1/**", "protocol": "rest"}],
			"binaries": [{"path": "/usr/local/bin/codex"}]
		}
	}`
	valid := map[string][]byte{
		"OpenShell envelope": []byte(`{
			"active_version": 2,
			"policy_source": "sandbox",
			"status": "effective",
			"policy": {"version": 1, "landlock": {"compatibility": "hard_requirement"}, ` + providerRule + `}
		}`),
		"unwrapped legacy shape": []byte(`{
			"version": 1,
			"landlock": {"compatibility": "hard_requirement"},
			` + providerRule + `
		}`),
	}
	profile := testProfile("codex", "standard", "prompted")
	run := Run{Sandbox: "sandbox-unit123", Gateway: "gateway-one", Mode: "standard", Approval: "prompted"}
	for name, payload := range valid {
		runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
			if call.name == "openshell" && commandHasArg(call, "policy") {
				return payload, nil
			}
			return nil, nil
		}}
		app := App{Runner: runner}
		if err := app.VerifyEffectivePolicy(context.Background(), run, profile); err != nil {
			t.Fatalf("%s: valid composed standard policy rejected: %v", name, err)
		}
	}

	missing := []byte(`{
		"active_version": 2,
		"policy_source": "sandbox",
		"status": "effective",
		"policy": {"version": 1, "landlock": {"compatibility": "hard_requirement"}, "network_policies": {}}
	}`)
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && commandHasArg(call, "policy") {
			return missing, nil
		}
		return nil, nil
	}}
	err := (&App{Runner: runner}).VerifyEffectivePolicy(context.Background(), run, profile)
	if err == nil || !strings.Contains(err.Error(), "api.openai.com:443") || !strings.Contains(err.Error(), `provider "openai" is not ready`) {
		t.Fatalf("missing composed endpoint error = %v", err)
	}
}

func TestSandboxCreateArgsUsesExactlyOneProvider(t *testing.T) {
	run := Run{ID: "run-unit", Sandbox: "sandbox-unit", Policy: "/tmp/policy.yaml", Image: "localhost/openshell:repo-example-codex-abcdef12"}
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
	for i, arg := range args {
		if arg != "--from" {
			continue
		}
		if i+1 >= len(args) || args[i+1] != run.Image {
			t.Fatalf("--from value = %v; want the resolved run image %q", args, run.Image)
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
		return nil, nil
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
		return []byte("ended"), nil
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

func TestAttachSessionFailsFastForTerminalRun(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	run := Run{ID: "run-unit123", Sandbox: "sandbox-unit123", Status: StatusCompleted}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	app := App{Store: store, Runner: runner}
	err := app.AttachSessionStream(context.Background(), run.ID, "agent", strings.NewReader(""), io.Discard, io.Discard, DefaultTerminalSize, nil)
	if err == nil || !strings.Contains(err.Error(), "Completed") {
		t.Fatalf("attach error = %v; want terminal-run error", err)
	}
	if len(runner.outputs) != 0 || len(runner.streams) != 0 {
		t.Fatalf("commands issued for terminal run: outputs=%#v streams=%#v", runner.outputs, runner.streams)
	}
}

func TestAttachSessionPreservesProbeError(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	run := Run{ID: "run-unit123", Sandbox: "sandbox-unit123", Status: StatusWorking}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	probeError := errors.New("gateway unavailable: connection refused")
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if commandHasArg(call, "has-session") {
			return nil, probeError
		}
		return nil, errors.New("marker probe unexpectedly issued")
	}}
	app := App{Store: store, Runner: runner}
	err := app.AttachSessionStream(context.Background(), run.ID, "agent", strings.NewReader(""), io.Discard, io.Discard, DefaultTerminalSize, nil)
	if !errors.Is(err, probeError) {
		t.Fatalf("attach error = %v; want original probe error", err)
	}
	if len(runner.streams) != 0 {
		t.Fatalf("attach stream issued after probe failure: %#v", runner.streams)
	}
}

func TestRunRecordsProfileName(t *testing.T) {
	ctx := context.Background()
	store := Store{Base: t.TempDir()}
	policy, err := store.CreatePolicyConfiguration("unit-policy")
	if err != nil {
		t.Fatal(err)
	}
	profile := Profile{Version: 1, Name: "mycodex", Agent: "codex", Image: "localhost/openshell:codex-1.2.3-r3", Mode: "standard", Provider: "openai", Policy: policy.Path, Approval: "prompted"}
	if err := store.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		switch {
		case call.name == "openshell" && len(call.args) > 0 && call.args[0] == "gateway":
			return []byte(`{"name":"openshell","type":"podman"}`), nil
		case call.name == "openshell" && len(call.args) > 0 && call.args[0] == "provider":
			return []byte(`{}`), nil
		case call.name == "podman":
			return []byte(`[]`), nil
		}
		return nil, nil
	}}
	app := App{Store: store, Runner: runner}
	run, err := app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell", Dry: true})
	if err != nil {
		t.Fatal(err)
	}
	if run.ProfileName != "mycodex" {
		t.Fatalf("run profile = %q; want mycodex", run.ProfileName)
	}
	run.ID = "run-profile123"
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadRun(run.ID)
	if err != nil || stored.ProfileName != "mycodex" {
		t.Fatalf("stored run profile = %q, %v", stored.ProfileName, err)
	}
}

func TestFinishSleepingRunStaysCompletedAfterInspect(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	run.Status = StatusStopped
	run.StatusDetail = "asleep; resume available"
	run.Synced = true
	run.Asleep = true
	run.SyncedSandboxFingerprint = "fingerprint-after-sleep"
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	app.Runner = statusRunnerFor("stopped", "")
	ctx := context.Background()
	if err := app.Finish(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	// A stopped sandbox can only report its phase, so the recorded decision has
	// to survive the inspect that follows every lifecycle command.
	got, err := app.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusCompleted || got.StatusDetail != "finished by user" {
		t.Fatalf("inspect reverted the finish: %s %q", got.Status, got.StatusDetail)
	}
}

func TestFinishedSleepingRunReportsSyncedStateInsteadOfUnknown(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts := createAgentExport(t, run, t.TempDir())
	runner := readyLifecycleRunner(t, artifacts)
	stopped := false
	base := runner.output
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" {
			if containsCommand(call.args, "sandbox", "stop") {
				stopped = true
			}
			if containsCommand(call.args, "sandbox", "get") && stopped {
				return []byte(`{"phase":"stopped"}`), nil
			}
		}
		return base(call)
	}
	app.Runner = runner
	ctx := context.Background()
	if err := app.Sleep(ctx, run.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := app.Finish(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	got, err := app.GetRunWithSyncState(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusCompleted {
		t.Fatalf("status = %s; want completed", got.Status)
	}
	if got.SyncState != SyncStateUpToDate {
		t.Fatalf("sync state = %s; a synced sleeping run must not report %s", got.SyncState, SyncStateUnknown)
	}
}

func TestResumeClearsAsleep(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	run.Status = StatusStopped
	run.StatusDetail = "asleep; resume available"
	run.Synced = true
	run.Asleep = true
	run.SyncedSandboxFingerprint = "fingerprint-after-sleep"
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := newSyncRunner(t, nil)
	base := runner.output
	started := false
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name != "openshell" {
			return base(call)
		}
		if containsCommand(call.args, "sandbox", "start") {
			started = true
			return nil, nil
		}
		if containsCommand(call.args, "sandbox", "get") {
			if started {
				return []byte(`{"phase":"running"}`), nil
			}
			return []byte(`{"phase":"stopped"}`), nil
		}
		return base(call)
	}
	app.Runner = runner
	if err := app.Resume(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Asleep {
		t.Fatal("resume left the run asleep, so its sync state would never be probed again")
	}
}

func TestInspectClearsAsleepWhenSandboxRuns(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	run.Status = StatusStopped
	run.Asleep = true
	run.Synced = true
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	app.Runner = statusRunnerFor("running", workingProbe)
	got, err := app.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Asleep {
		t.Fatal("a running sandbox must clear the asleep flag")
	}
}
