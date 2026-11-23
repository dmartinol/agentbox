package app

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func createSyncFixture(t *testing.T) (App, Run, string) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "init", "-q")
	cmd(t, source, "git", "config", "user.name", "Unit Tester")
	cmd(t, source, "git", "config", "user.email", "unit@example.invalid")
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "add", ".")
	cmd(t, source, "git", "commit", "-qm", "source base")
	store := Store{Base: filepath.Join(root, "state")}
	app := App{Store: store, Runner: OSRunner{}}
	run, err := app.CreateWorktree(ctx, source, "run-unit123")
	if err != nil {
		t.Fatal(err)
	}
	run.Status = "running"
	run.Gateway = "gateway-one"
	run.Sandbox = "sandbox-unit123"
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	return app, run, source
}

func createAgentExport(t *testing.T, run Run, root string) map[string]string {
	t.Helper()
	artifacts, _ := createAgentExportWithRepo(t, run, root)
	return artifacts
}

func createAgentExportWithRepo(t *testing.T, run Run, root string) (map[string]string, string) {
	t.Helper()
	remote := filepath.Join(root, "sandbox-project")
	cmd(t, root, "git", "clone", run.Worktree, remote)
	cmd(t, remote, "git", "config", "user.name", "Sandbox Agent")
	cmd(t, remote, "git", "config", "user.email", "agent@example.invalid")
	if err := os.WriteFile(filepath.Join(remote, "tracked.txt"), []byte("committed agent edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, remote, "git", "add", "tracked.txt")
	cmd(t, remote, "git", "commit", "-qm", "agent commit")
	if err := os.WriteFile(filepath.Join(remote, "tracked.txt"), []byte("uncommitted agent edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remote, "untracked.txt"), []byte("new agent file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "export.bundle")
	cmd(t, remote, "git", "bundle", "create", bundle, run.Branch)
	patch := filepath.Join(root, "export.patch")
	cmd(t, remote, "git", "diff", "HEAD", "--binary", "--full-index", "--output", patch)
	archive := filepath.Join(root, "export.tar")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	body := []byte("new agent file\n")
	if err := writer.WriteHeader(&tar.Header{Name: "untracked.txt", Mode: 0600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"/workspace/export.bundle": bundle,
		"/workspace/export.patch":  patch,
		"/workspace/export.tar":    archive,
	}, remote
}

func newSyncRunner(t *testing.T, artifacts map[string]string) *fakeRunner {
	t.Helper()
	runner := &fakeRunner{}
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "git" {
			return (OSRunner{}).Output(context.Background(), "git", call.args...)
		}
		if call.name != "openshell" {
			return nil, nil
		}
		joined := strings.Join(call.args, " ")
		if containsCommand(call.args, "sandbox", "get") {
			return []byte(`{"phase":"running"}`), nil
		}
		if strings.Contains(joined, "tmux has-session -t agent") {
			return []byte("session=1\ndead=0\ncommand=codex\nactivity={\"state\":\"working\"}\n"), nil
		}
		for index, arg := range call.args {
			if arg != "download" || index+3 >= len(call.args) {
				continue
			}
			remote, local := call.args[index+2], call.args[index+3]
			source, ok := artifacts[remote]
			if !ok {
				return nil, os.ErrNotExist
			}
			body, err := os.ReadFile(source)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(local, body, 0600); err != nil {
				return nil, err
			}
			return nil, nil
		}
		return nil, nil
	}
	return runner
}

func newSyncRunnerWithProbe(t *testing.T, artifacts map[string]string, probe string) *fakeRunner {
	t.Helper()
	runner := newSyncRunner(t, artifacts)
	base := runner.output
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && containsCommand(call.args, "node", "-e") {
			return []byte(probe), nil
		}
		return base(call)
	}
	return runner
}

func cloneSandboxRepo(t *testing.T, run Run) string {
	t.Helper()
	root := t.TempDir()
	sandbox := filepath.Join(root, "sandbox")
	cmd(t, root, "git", "clone", "-q", run.Worktree, sandbox)
	cmd(t, sandbox, "git", "config", "user.name", "Sandbox Agent")
	cmd(t, sandbox, "git", "config", "user.email", "agent@example.invalid")
	return sandbox
}

func sandboxProbeJSON(t *testing.T, dir string) string {
	t.Helper()
	snapshot, err := collectSnapshotDir(context.Background(), OSRunner{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func sandboxSnapshotFingerprint(t *testing.T, dir string) string {
	t.Helper()
	snapshot, err := collectSnapshotDir(context.Background(), OSRunner{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.Fingerprint()
}

func newSandboxStateRunner(t *testing.T, sandbox string) *fakeRunner {
	t.Helper()
	runner := &fakeRunner{}
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "git" {
			return (OSRunner{}).Output(context.Background(), "git", call.args...)
		}
		if call.name != "openshell" {
			return nil, nil
		}
		if containsCommand(call.args, "sandbox", "get") {
			return []byte(`{"phase":"running"}`), nil
		}
		joined := strings.Join(call.args, " ")
		if strings.Contains(joined, "tmux has-session -t agent") {
			return []byte("session=1\ndead=0\ncommand=sh\n"), nil
		}
		if containsCommand(call.args, "node", "-e") {
			return []byte(sandboxProbeJSON(t, sandbox)), nil
		}
		return nil, nil
	}
	return runner
}

func newSyncStateFixture(t *testing.T) (*App, Run, string, *fakeRunner) {
	t.Helper()
	app, run, _ := createSyncFixture(t)
	sandbox := cloneSandboxRepo(t, run)
	runner := newSandboxStateRunner(t, sandbox)
	app.Runner = runner
	run.Status = StatusWorking
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	return &app, run, sandbox, runner
}

func getRunWithState(t *testing.T, app *App, id string) Run {
	t.Helper()
	run, err := app.GetRun(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestSyncTransfersCommitAndUncommittedFilesToRunWorktree(t *testing.T) {
	app, run, source := createSyncFixture(t)
	artifacts := createAgentExport(t, run, t.TempDir())
	runner := newSyncRunner(t, artifacts)
	app.Runner = runner
	if err := app.Sync(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"tracked.txt": "uncommitted agent edit\n", "untracked.txt": "new agent file\n"} {
		got, err := os.ReadFile(filepath.Join(run.Worktree, name))
		if err != nil || string(got) != want {
			t.Fatalf("synced %s = %q, %v; want %q", name, got, err, want)
		}
	}
	log := cmd(t, run.Worktree, "git", "log", "--format=%s", "-2")
	if !strings.Contains(log, "agent commit") {
		t.Fatalf("agent commit missing from review branch: %s", log)
	}
	original, err := os.ReadFile(filepath.Join(source, "tracked.txt"))
	if err != nil || string(original) != "base\n" {
		t.Fatalf("sync changed original checkout: %q, %v", original, err)
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || !stored.Synced || stored.LastSynced == run.Baseline {
		t.Fatalf("run sync state not updated: %+v, %v", stored, err)
	}
}

func TestSyncRetainsMirrorArtifactsForReapply(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts, sandbox := createAgentExportWithRepo(t, run, t.TempDir())
	app.Runner = newSyncRunnerWithProbe(t, artifacts, sandboxProbeJSON(t, sandbox))
	if err := app.Sync(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	patchPath := filepath.Join(app.Store.RunsDir(), run.ID+"-mirror.patch")
	tarPath := filepath.Join(app.Store.RunsDir(), run.ID+"-mirror.tar")
	for _, path := range []string{patchPath, tarPath} {
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			t.Fatalf("mirror artifact missing or empty: %s (%v)", path, err)
		}
	}
	if err := app.Sync(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{patchPath, tarPath} {
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			t.Fatalf("mirror artifact not retained for re-apply: %s (%v)", path, err)
		}
	}
}

func TestSyncRefusesLocallyChangedRunWorktreeBeforeRemoteTransfer(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	if err := os.WriteFile(filepath.Join(run.Worktree, "tracked.txt"), []byte("local conflict\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := newSyncRunner(t, nil)
	app.Runner = runner
	err := app.Sync(context.Background(), run.ID)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected conflict refusal, got %v", err)
	}
	for _, call := range runner.outputs {
		if call.name == "openshell" {
			t.Fatalf("remote transfer started despite local conflict: %#v", call)
		}
	}
}

func TestSyncRollsBackGitChangesWhenUntrackedArchiveConflicts(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts := createAgentExport(t, run, t.TempDir())
	archive := filepath.Join(t.TempDir(), "conflict.tar")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	body := []byte("archive must not overwrite tracked file\n")
	if err := writer.WriteHeader(&tar.Header{Name: "tracked.txt", Mode: 0600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	artifacts["/workspace/export.tar"] = archive
	app.Runner = newSyncRunner(t, artifacts)
	err = app.Sync(context.Background(), run.ID)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected archive collision to abort sync, got %v", err)
	}
	head := strings.TrimSpace(cmd(t, run.Worktree, "git", "rev-parse", "HEAD"))
	if head != run.LastSynced {
		t.Fatalf("sync failure left merged commits behind: got %s want %s", head, run.LastSynced)
	}
	if status := cmd(t, run.Worktree, "git", "status", "--short"); status != "" {
		t.Fatalf("sync failure left partial file edits: %s", status)
	}
}

func TestValidateTarRejectsEscapingSymlinks(t *testing.T) {
	for _, target := range []string{"../../outside", "/tmp/outside"} {
		t.Run(target, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "unsafe.tar")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			writer := tar.NewWriter(file)
			if err := writer.WriteHeader(&tar.Header{Name: "nested/link", Linkname: target, Typeflag: tar.TypeSymlink}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if err := validateTar(path); err == nil {
				t.Fatal("escaping symlink accepted")
			}
		})
	}
	if !safeArchiveSymlink("nested/link", "../sibling") || safeArchiveSymlink("link", "../../outside") {
		t.Fatal("relative symlink boundary classification is incorrect")
	}
}

func TestSyncRecordsSuccessfulSnapshotMetadata(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts, sandbox := createAgentExportWithRepo(t, run, t.TempDir())
	app.Runner = newSyncRunnerWithProbe(t, artifacts, sandboxProbeJSON(t, sandbox))
	if err := app.Sync(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastSuccessfulSyncAt.IsZero() || stored.LastSuccessfulSyncAt.Location() != time.UTC {
		t.Fatalf("last successful sync time = %v; want a UTC timestamp", stored.LastSuccessfulSyncAt)
	}
	if stored.SyncedSandboxFingerprint == "" {
		t.Fatal("successful sync did not record a sandbox fingerprint")
	}
	if !stored.Synced || stored.LastSynced == run.Baseline {
		t.Fatalf("existing sync fields were not updated: %+v", stored)
	}
	if stored.Fingerprint == "" {
		t.Fatal("host safety fingerprint was not recorded")
	}
	snapshot, err := collectSnapshotDir(context.Background(), OSRunner{}, sandbox)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SyncedSandboxFingerprint != snapshot.Fingerprint() {
		t.Fatalf("recorded fingerprint %q does not match the exported snapshot %q", stored.SyncedSandboxFingerprint, snapshot.Fingerprint())
	}
	updated := getRunWithState(t, &app, run.ID)
	if updated.SyncState != SyncStateUpToDate {
		t.Fatalf("sync state after successful sync = %q; want %q", updated.SyncState, SyncStateUpToDate)
	}
}

func TestFailedSyncPreservesSuccessfulSnapshotMetadata(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts, sandbox := createAgentExportWithRepo(t, run, t.TempDir())
	app.Runner = newSyncRunnerWithProbe(t, artifacts, sandboxProbeJSON(t, sandbox))
	if err := app.Sync(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	before, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	app.Runner = newSyncRunner(t, nil)
	if err := app.Sync(context.Background(), run.ID); err == nil {
		t.Fatal("expected the second sync to fail")
	}
	after, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.LastSuccessfulSyncAt.Equal(before.LastSuccessfulSyncAt) || after.SyncedSandboxFingerprint != before.SyncedSandboxFingerprint {
		t.Fatalf("failed sync changed successful metadata: before=%+v after=%+v", before, after)
	}
}

func TestSyncStateCoversCleanDirtyConflictAndUnavailable(t *testing.T) {
	app, run, sandbox, runner := newSyncStateFixture(t)

	clean := getRunWithState(t, app, run.ID)
	if clean.SyncState != SyncStateNoChanges {
		t.Fatalf("clean new run state = %q; want %q", clean.SyncState, SyncStateNoChanges)
	}
	if clean.SyncedSandboxFingerprint != "" || !clean.LastSuccessfulSyncAt.IsZero() {
		t.Fatalf("clean new run unexpectedly has sync metadata: %+v", clean)
	}

	before := sandboxSnapshotFingerprint(t, sandbox)
	if err := os.WriteFile(filepath.Join(sandbox, "tracked.txt"), []byte("first edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first := sandboxSnapshotFingerprint(t, sandbox)
	if first == before {
		t.Fatal("tracked edit did not change the snapshot fingerprint")
	}
	if state := getRunWithState(t, app, run.ID).SyncState; state != SyncStateNeedsSync {
		t.Fatalf("tracked edit state = %q; want %q", state, SyncStateNeedsSync)
	}

	if err := os.WriteFile(filepath.Join(sandbox, "tracked.txt"), []byte("second edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second := sandboxSnapshotFingerprint(t, sandbox)
	if second == first {
		t.Fatal("second edit to an already-dirty file did not change the fingerprint")
	}
	if state := getRunWithState(t, app, run.ID).SyncState; state != SyncStateNeedsSync {
		t.Fatalf("second tracked edit state = %q; want %q", state, SyncStateNeedsSync)
	}

	if err := os.WriteFile(filepath.Join(sandbox, "untracked.txt"), []byte("new file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	untracked := sandboxSnapshotFingerprint(t, sandbox)
	if untracked == second {
		t.Fatal("untracked add did not change the fingerprint")
	}
	if err := os.WriteFile(filepath.Join(sandbox, "untracked.txt"), []byte("edited file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	editedUntracked := sandboxSnapshotFingerprint(t, sandbox)
	if editedUntracked == untracked {
		t.Fatal("untracked edit did not change the fingerprint")
	}
	if err := os.Remove(filepath.Join(sandbox, "untracked.txt")); err != nil {
		t.Fatal(err)
	}
	removedUntracked := sandboxSnapshotFingerprint(t, sandbox)
	if removedUntracked == editedUntracked {
		t.Fatal("untracked remove did not change the fingerprint")
	}

	cmd(t, sandbox, "git", "add", "tracked.txt")
	cmd(t, sandbox, "git", "commit", "-qm", "agent commit")
	committed := sandboxSnapshotFingerprint(t, sandbox)
	if committed == removedUntracked {
		t.Fatal("new commit did not change the fingerprint")
	}
	if state := getRunWithState(t, app, run.ID).SyncState; state != SyncStateNeedsSync {
		t.Fatalf("new commit state = %q; want %q", state, SyncStateNeedsSync)
	}

	if err := os.WriteFile(filepath.Join(run.Worktree, "tracked.txt"), []byte("host conflict\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if state := getRunWithState(t, app, run.ID).SyncState; state != SyncStateBlocked {
		t.Fatalf("host conflict state = %q; want %q", state, SyncStateBlocked)
	}

	base := runner.output
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && containsCommand(call.args, "node", "-e") {
			return nil, fmt.Errorf("sandbox probe unavailable")
		}
		return base(call)
	}
	if state := getRunWithState(t, app, run.ID).SyncState; state != SyncStateUnknown {
		t.Fatalf("failed probe state = %q; want %q", state, SyncStateUnknown)
	}
}

func TestListAndReconcileBoundsSyncProbes(t *testing.T) {
	app, run, _, runner := newSyncStateFixture(t)
	run.Synced = true
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := app.ListAndReconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	probes := 0
	for _, call := range runner.outputs {
		joined := strings.Join(call.args, " ")
		if call.name == "openshell" && containsCommand(call.args, "node", "-e") {
			probes++
		}
		if strings.Contains(joined, "bundle create") || strings.Contains(joined, "/workspace/export") || strings.Contains(joined, "download") {
			t.Fatalf("fleet overview performed an export or transfer: %s", joined)
		}
	}
	if probes != 1 {
		t.Fatalf("overview sync probes = %d; want one cached probe", probes)
	}
}

func TestRunJSONIncludesDerivedSyncState(t *testing.T) {
	app, run, sandbox, _ := newSyncStateFixture(t)
	if err := os.WriteFile(filepath.Join(sandbox, "tracked.txt"), []byte("pending\n"), 0600); err != nil {
		t.Fatal(err)
	}
	updated := getRunWithState(t, app, run.ID)
	body, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["sync_state"] != string(SyncStateNeedsSync) {
		t.Fatalf("sync_state = %v; want %q", doc["sync_state"], SyncStateNeedsSync)
	}
	if _, ok := doc["last_successful_sync_at"]; !ok {
		t.Fatal("run JSON is missing last_successful_sync_at")
	}
	if _, ok := doc["synced_sandbox_fingerprint"]; !ok {
		t.Fatal("run JSON is missing synced_sandbox_fingerprint")
	}
}
