package app

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
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
	return createSandboxExportArtifacts(t, run, remote, root), remote
}

func createSandboxExportArtifacts(t *testing.T, run Run, sandbox, root string) map[string]string {
	t.Helper()
	bundle := filepath.Join(root, "export.bundle")
	cmd(t, sandbox, "git", "bundle", "create", bundle, run.Branch)
	patch := filepath.Join(root, "export.patch")
	cmd(t, sandbox, "git", "diff", "HEAD", "--binary", "--full-index", "--output", patch)
	archive := filepath.Join(root, "export.tar")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	list := cmd(t, sandbox, "git", "ls-files", "--others", "--exclude-standard", "-z")
	for _, name := range splitNUL([]byte(list)) {
		info, err := os.Lstat(filepath.Join(sandbox, name))
		if err != nil {
			t.Fatal(err)
		}
		header := &tar.Header{Name: name, Mode: int64(info.Mode().Perm())}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filepath.Join(sandbox, name))
			if err != nil {
				t.Fatal(err)
			}
			header.Typeflag = tar.TypeSymlink
			header.Linkname = target
			if err := writer.WriteHeader(header); err != nil {
				t.Fatal(err)
			}
			continue
		}
		source, err := os.Open(filepath.Join(sandbox, name))
		if err != nil {
			t.Fatal(err)
		}
		header.Typeflag = tar.TypeReg
		header.Size = info.Size()
		if err := writer.WriteHeader(header); err != nil {
			source.Close()
			t.Fatal(err)
		}
		if _, err := io.Copy(writer, source); err != nil {
			source.Close()
			t.Fatal(err)
		}
		source.Close()
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
	}
}

func newSyncRunner(t *testing.T, artifacts map[string]string) *fakeRunner {
	t.Helper()
	defaultHead := strings.Repeat("0", 40)
	return newSyncRunnerWithCheckout(t, artifacts, sandboxCheckout{
		Head:          defaultHead,
		CurrentBranch: "run-branch",
		BranchTip:     defaultHead,
	})
}

func newSyncRunnerWithCheckout(t *testing.T, artifacts map[string]string, checkout sandboxCheckout) *fakeRunner {
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
		if containsCommand(call.args, "node", "-e") {
			probe, marshalErr := json.Marshal(syncSnapshot{
				Branch:     []byte(checkout.CurrentBranch),
				Head:       checkout.Head,
				BranchTip:  checkout.BranchTip,
				DiffSHA256: hashBytes(nil),
			})
			if marshalErr != nil {
				return nil, marshalErr
			}
			return probe, nil
		}
		if containsCommand(call.args, "/bin/sh", "-c") && strings.Contains(joined, "/workspace/export.bundle") {
			return []byte(checkout.Head + "\n" + checkout.CurrentBranch + "\n" + checkout.BranchTip + "\n"), nil
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

func newSandboxSyncRunner(t *testing.T, artifacts map[string]string, sandbox, branch string) *fakeRunner {
	t.Helper()
	head := strings.TrimSpace(cmd(t, sandbox, "git", "rev-parse", "refs/heads/"+branch))
	runner := newSyncRunnerWithCheckout(t, artifacts, sandboxCheckout{
		Head:          head,
		CurrentBranch: branch,
		BranchTip:     head,
	})
	base := runner.output
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && containsCommand(call.args, "node", "-e") {
			return []byte(sandboxProbeJSON(t, sandbox, branch)), nil
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

func sandboxProbeJSON(t *testing.T, dir, branch string) string {
	t.Helper()
	snapshot, err := collectSnapshotDir(context.Background(), OSRunner{}, dir, branch)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func sandboxSnapshotFingerprint(t *testing.T, dir, branch string) string {
	t.Helper()
	snapshot, err := collectSnapshotDir(context.Background(), OSRunner{}, dir, branch)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.Fingerprint()
}

func newSandboxStateRunner(t *testing.T, sandbox, branch string) *fakeRunner {
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
			return []byte(sandboxProbeJSON(t, sandbox, branch)), nil
		}
		return nil, nil
	}
	return runner
}

func newSyncStateFixture(t *testing.T) (*App, Run, string, *fakeRunner) {
	t.Helper()
	app, run, _ := createSyncFixture(t)
	sandbox := cloneSandboxRepo(t, run)
	runner := newSandboxStateRunner(t, sandbox, run.Branch)
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

func TestSyncRecordsTransferredCommitAndTree(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts, sandbox := createAgentExportWithRepo(t, run, t.TempDir())
	sandboxCommit := strings.TrimSpace(cmd(t, sandbox, "git", "rev-parse", "refs/heads/"+run.Branch))
	sandboxTree := strings.TrimSpace(cmd(t, sandbox, "git", "rev-parse", sandboxCommit+"^{tree}"))
	runner := newSyncRunnerWithCheckout(t, artifacts, sandboxCheckout{
		Head:          sandboxCommit,
		CurrentBranch: run.Branch,
		BranchTip:     sandboxCommit,
	})
	probe := sandboxProbeJSON(t, sandbox, run.Branch)
	base := runner.output
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && containsCommand(call.args, "node", "-e") {
			return []byte(probe), nil
		}
		return base(call)
	}
	app.Runner = runner
	if err := app.Sync(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	hostCommit := strings.TrimSpace(cmd(t, run.Worktree, "git", "rev-parse", "HEAD"))
	hostTree := strings.TrimSpace(cmd(t, run.Worktree, "git", "rev-parse", "HEAD^{tree}"))
	if stored.LastSynced != sandboxCommit || hostCommit != sandboxCommit {
		t.Fatalf("transferred commit mismatch: last_synced=%s host=%s sandbox=%s", stored.LastSynced, hostCommit, sandboxCommit)
	}
	if hostTree != sandboxTree {
		t.Fatalf("transferred tree mismatch: host=%s sandbox=%s", hostTree, sandboxTree)
	}
	if updated := getRunWithState(t, &app, run.ID); updated.SyncState != SyncStateUpToDate {
		t.Fatalf("sync state after successful transfer = %q; want %q", updated.SyncState, SyncStateUpToDate)
	}
}

func assertSyncRefusalPreservedState(t *testing.T, app App, run Run, runner *fakeRunner, before Run, beforeHead, beforeStatus string) {
	t.Helper()
	after, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("refused sync changed run record: before=%+v after=%+v", before, after)
	}
	hostHead := strings.TrimSpace(cmd(t, run.Worktree, "git", "rev-parse", "HEAD"))
	hostStatus := cmd(t, run.Worktree, "git", "status", "--short")
	if hostHead != beforeHead || hostStatus != beforeStatus {
		t.Fatalf("refused sync changed host worktree: head=%s status=%q; want head=%s status=%q", hostHead, hostStatus, beforeHead, beforeStatus)
	}
	for _, path := range []string{
		runArtifactPath(app.Store, run.ID, runArtifactMirrorPatch),
		runArtifactPath(app.Store, run.ID, runArtifactMirrorTar),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("refused sync changed mirror artifact %s: %v", path, err)
		}
	}
	for _, call := range runner.outputs {
		if containsCommand(call.args, "sandbox", "download") {
			t.Fatalf("refused sync downloaded an export: %#v", call)
		}
		if call.name != "git" {
			continue
		}
		for _, mutation := range []string{"fetch", "merge", "apply"} {
			for _, arg := range call.args {
				if arg == mutation {
					t.Fatalf("refused sync mutated the host with git %s: %#v", mutation, call)
				}
			}
		}
	}
}

func TestSyncRefusesSideBranchCheckoutBeforeHostTransfer(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	_, sandbox := createAgentExportWithRepo(t, run, t.TempDir())
	branchTip := strings.TrimSpace(cmd(t, sandbox, "git", "rev-parse", "refs/heads/"+run.Branch))
	sideBranch := "agent/side-work"
	cmd(t, sandbox, "git", "switch", "-q", "-c", sideBranch)
	if err := os.WriteFile(filepath.Join(sandbox, "side.txt"), []byte("side branch work\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, sandbox, "git", "add", "side.txt")
	cmd(t, sandbox, "git", "commit", "-qm", "side branch work")
	sideHead := strings.TrimSpace(cmd(t, sandbox, "git", "rev-parse", "HEAD"))
	run.Synced = false
	run.SyncedSandboxFingerprint = ""
	run.LastSuccessfulSyncAt = time.Time{}
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	before, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeHead := strings.TrimSpace(cmd(t, run.Worktree, "git", "rev-parse", "HEAD"))
	beforeStatus := cmd(t, run.Worktree, "git", "status", "--short")
	runner := newSyncRunnerWithCheckout(t, nil, sandboxCheckout{
		Head:          sideHead,
		CurrentBranch: sideBranch,
		BranchTip:     branchTip,
	})
	app.Runner = runner
	err = app.Sync(context.Background(), run.ID)
	want := fmt.Sprintf("refusing sync: sandbox checkout %q is at %s, but run branch %q is at %s; move the intended commit onto %q in the sandbox (for example: git switch %q && git cherry-pick %s), then retry agentbox agent sync %s", sideBranch, sideHead, run.Branch, branchTip, run.Branch, run.Branch, sideHead, run.ID)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("side-branch refusal mismatch: got %v; want %q", err, want)
	}
	assertSyncRefusalPreservedState(t, app, run, runner, before, beforeHead, beforeStatus)
}

func TestSyncRefusesDetachedHeadBeforeHostTransfer(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	_, sandbox := createAgentExportWithRepo(t, run, t.TempDir())
	branchTip := strings.TrimSpace(cmd(t, sandbox, "git", "rev-parse", "refs/heads/"+run.Branch))
	cmd(t, sandbox, "git", "switch", "-q", "--detach", "HEAD")
	if err := os.WriteFile(filepath.Join(sandbox, "detached.txt"), []byte("detached work\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, sandbox, "git", "add", "detached.txt")
	cmd(t, sandbox, "git", "commit", "-qm", "detached work")
	detachedHead := strings.TrimSpace(cmd(t, sandbox, "git", "rev-parse", "HEAD"))
	run.Synced = false
	run.SyncedSandboxFingerprint = ""
	run.LastSuccessfulSyncAt = time.Time{}
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	before, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeHead := strings.TrimSpace(cmd(t, run.Worktree, "git", "rev-parse", "HEAD"))
	beforeStatus := cmd(t, run.Worktree, "git", "status", "--short")
	runner := newSyncRunnerWithCheckout(t, nil, sandboxCheckout{
		Head:      detachedHead,
		BranchTip: branchTip,
	})
	app.Runner = runner
	err = app.Sync(context.Background(), run.ID)
	want := fmt.Sprintf("refusing sync: sandbox checkout %q is at %s, but run branch %q is at %s; move the intended commit onto %q in the sandbox (for example: git switch %q && git cherry-pick %s), then retry agentbox agent sync %s", "detached HEAD", detachedHead, run.Branch, branchTip, run.Branch, run.Branch, detachedHead, run.ID)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("detached-head refusal mismatch: got %v; want %q", err, want)
	}
	assertSyncRefusalPreservedState(t, app, run, runner, before, beforeHead, beforeStatus)
}

func TestSyncAllowsDetachedHeadAtRunBranchTip(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts, sandbox := createAgentExportWithRepo(t, run, t.TempDir())
	sandboxCommit := strings.TrimSpace(cmd(t, sandbox, "git", "rev-parse", "refs/heads/"+run.Branch))
	sandboxTree := strings.TrimSpace(cmd(t, sandbox, "git", "rev-parse", sandboxCommit+"^{tree}"))
	cmd(t, sandbox, "git", "switch", "-q", "--detach", "HEAD")
	runner := newSyncRunnerWithCheckout(t, artifacts, sandboxCheckout{
		Head:      sandboxCommit,
		BranchTip: sandboxCommit,
	})
	probe := sandboxProbeJSON(t, sandbox, run.Branch)
	base := runner.output
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && containsCommand(call.args, "node", "-e") {
			return []byte(probe), nil
		}
		return base(call)
	}
	app.Runner = runner
	if err := app.Sync(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	hostCommit := strings.TrimSpace(cmd(t, run.Worktree, "git", "rev-parse", "HEAD"))
	hostTree := strings.TrimSpace(cmd(t, run.Worktree, "git", "rev-parse", "HEAD^{tree}"))
	if stored.LastSynced != sandboxCommit || hostCommit != sandboxCommit {
		t.Fatalf("detached-at-tip commit mismatch: last_synced=%s host=%s sandbox=%s", stored.LastSynced, hostCommit, sandboxCommit)
	}
	if hostTree != sandboxTree {
		t.Fatalf("detached-at-tip tree mismatch: host=%s sandbox=%s", hostTree, sandboxTree)
	}
	updated, err := app.GetRunWithSyncState(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SyncState != SyncStateUpToDate || updated.SyncState.Label() != "Up to date" {
		t.Fatalf("detached-at-tip sync state = %s (%s); want %s (%s)", updated.SyncState, updated.SyncState.Label(), SyncStateUpToDate, SyncStateUpToDate.Label())
	}
}

func TestSyncAllowsSideBranchAtRunBranchTip(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts, sandbox := createAgentExportWithRepo(t, run, t.TempDir())
	branchTip := strings.TrimSpace(cmd(t, sandbox, "git", "rev-parse", "refs/heads/"+run.Branch))
	sideBranch := "agent/side-work"
	cmd(t, sandbox, "git", "switch", "-q", "-c", sideBranch)
	runner := newSyncRunnerWithCheckout(t, artifacts, sandboxCheckout{
		Head:          branchTip,
		CurrentBranch: sideBranch,
		BranchTip:     branchTip,
	})
	var snapshot syncSnapshot
	if err := json.Unmarshal([]byte(sandboxProbeJSON(t, sandbox, run.Branch)), &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Branch = []byte(sideBranch)
	probe, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	base := runner.output
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && containsCommand(call.args, "node", "-e") {
			return probe, nil
		}
		return base(call)
	}
	app.Runner = runner
	if err := app.Sync(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := app.GetRunWithSyncState(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SyncState != SyncStateUpToDate || updated.SyncState.Label() != "Up to date" {
		t.Fatalf("side-branch-at-tip sync state = %s (%s); want %s (%s)", updated.SyncState, updated.SyncState.Label(), SyncStateUpToDate, SyncStateUpToDate.Label())
	}
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
	app.Runner = newSyncRunnerWithProbe(t, artifacts, sandboxProbeJSON(t, sandbox, run.Branch))
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
	app.Runner = newSyncRunnerWithProbe(t, artifacts, sandboxProbeJSON(t, sandbox, run.Branch))
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
	snapshot, err := collectSnapshotDir(context.Background(), OSRunner{}, sandbox, run.Branch)
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
	app.Runner = newSyncRunnerWithProbe(t, artifacts, sandboxProbeJSON(t, sandbox, run.Branch))
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

func TestSyncDoesNotSkipChangedSandboxFingerprint(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	sandbox := cloneSandboxRepo(t, run)
	if err := os.WriteFile(filepath.Join(sandbox, "tracked.txt"), []byte("first agent edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, sandbox, "git", "commit", "-qam", "first agent edit")
	artifacts := createSandboxExportArtifacts(t, run, sandbox, t.TempDir())
	app.Runner = newSandboxSyncRunner(t, artifacts, sandbox, run.Branch)
	if err := app.Sync(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	before, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(sandbox, "tracked.txt"), []byte("second agent edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, sandbox, "git", "commit", "-qam", "second agent edit")
	artifacts = createSandboxExportArtifacts(t, run, sandbox, t.TempDir())
	runner := newSandboxSyncRunner(t, artifacts, sandbox, run.Branch)
	app.Runner = runner
	if err := app.Sync(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	sandboxCommit := strings.TrimSpace(cmd(t, sandbox, "git", "rev-parse", "refs/heads/"+run.Branch))
	hostCommit := strings.TrimSpace(cmd(t, run.Worktree, "git", "rev-parse", "HEAD"))
	after, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hostCommit != sandboxCommit || after.LastSynced != sandboxCommit || hostCommit == before.LastSynced {
		t.Fatalf("changed sandbox snapshot was not transferred: before=%s host=%s sandbox=%s", before.LastSynced, hostCommit, sandboxCommit)
	}
	if !after.LastSuccessfulSyncAt.After(before.LastSuccessfulSyncAt) || after.SyncedSandboxFingerprint == before.SyncedSandboxFingerprint {
		t.Fatalf("changed sandbox sync did not update successful metadata: before=%+v after=%+v", before, after)
	}
	foundDownload, foundMerge := false, false
	for _, call := range runner.outputs {
		if call.name == "openshell" && containsCommand(call.args, "sandbox", "download") {
			foundDownload = true
		}
		if call.name == "git" && containsCommand(call.args, "merge", "--ff-only") {
			foundMerge = true
		}
	}
	if !foundDownload || !foundMerge {
		t.Fatalf("changed sandbox snapshot did not run the transfer path: download=%v merge=%v", foundDownload, foundMerge)
	}
}

func TestSyncFallsBackToTransferWhenProbeFails(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	sandbox := cloneSandboxRepo(t, run)
	if err := os.WriteFile(filepath.Join(sandbox, "tracked.txt"), []byte("first agent edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, sandbox, "git", "commit", "-qam", "first agent edit")
	artifacts := createSandboxExportArtifacts(t, run, sandbox, t.TempDir())
	app.Runner = newSandboxSyncRunner(t, artifacts, sandbox, run.Branch)
	if err := app.Sync(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	before, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(sandbox, "tracked.txt"), []byte("second agent edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, sandbox, "git", "commit", "-qam", "second agent edit")
	artifacts = createSandboxExportArtifacts(t, run, sandbox, t.TempDir())
	runner := newSandboxSyncRunner(t, artifacts, sandbox, run.Branch)
	base := runner.output
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && containsCommand(call.args, "node", "-e") {
			return nil, fmt.Errorf("sandbox probe unavailable")
		}
		return base(call)
	}
	app.Runner = runner
	if err := app.Sync(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	foundDownload := false
	for _, call := range runner.outputs {
		if call.name == "openshell" && containsCommand(call.args, "sandbox", "download") {
			foundDownload = true
		}
	}
	sandboxCommit := strings.TrimSpace(cmd(t, sandbox, "git", "rev-parse", "refs/heads/"+run.Branch))
	hostCommit := strings.TrimSpace(cmd(t, run.Worktree, "git", "rev-parse", "HEAD"))
	after, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !foundDownload || hostCommit != sandboxCommit || !after.LastSuccessfulSyncAt.After(before.LastSuccessfulSyncAt) {
		t.Fatalf("probe failure did not fall back to transfer: download=%v host=%s sandbox=%s before=%s after=%s", foundDownload, hostCommit, sandboxCommit, before.LastSuccessfulSyncAt, after.LastSuccessfulSyncAt)
	}
}

func TestSyncRequiresSuccessfulSnapshotMetadataToSkip(t *testing.T) {
	for name, mutate := range map[string]func(*Run){
		"zero last successful sync":   func(r *Run) { r.LastSuccessfulSyncAt = time.Time{} },
		"empty synced sandbox digest": func(r *Run) { r.SyncedSandboxFingerprint = "" },
		"empty host fingerprint":      func(r *Run) { r.Fingerprint = "" },
	} {
		t.Run(name, func(t *testing.T) {
			app, run, _ := createSyncFixture(t)
			sandbox := cloneSandboxRepo(t, run)
			if err := os.WriteFile(filepath.Join(sandbox, "tracked.txt"), []byte("agent edit\n"), 0600); err != nil {
				t.Fatal(err)
			}
			cmd(t, sandbox, "git", "commit", "-qam", "agent edit")
			artifacts := createSandboxExportArtifacts(t, run, sandbox, t.TempDir())
			app.Runner = newSandboxSyncRunner(t, artifacts, sandbox, run.Branch)
			if err := app.Sync(context.Background(), run.ID); err != nil {
				t.Fatal(err)
			}
			stored, err := app.Store.LoadRun(run.ID)
			if err != nil {
				t.Fatal(err)
			}
			mutate(&stored)
			if err := app.Store.SaveRun(stored); err != nil {
				t.Fatal(err)
			}
			runner := newSandboxSyncRunner(t, artifacts, sandbox, run.Branch)
			app.Runner = runner
			err = app.Sync(context.Background(), run.ID)
			if stored.Fingerprint == "" {
				if err == nil || !strings.Contains(err.Error(), "local run worktree changed") {
					t.Fatalf("empty host fingerprint unexpectedly bypassed the existing host-drift refusal: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			foundTransfer := false
			for _, call := range runner.outputs {
				if call.name == "openshell" && containsCommand(call.args, "sandbox", "download") {
					foundTransfer = true
				}
			}
			if !foundTransfer {
				t.Fatal("missing successful-sync metadata skipped the transfer path")
			}
		})
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

	before := sandboxSnapshotFingerprint(t, sandbox, run.Branch)
	if err := os.WriteFile(filepath.Join(sandbox, "tracked.txt"), []byte("first edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first := sandboxSnapshotFingerprint(t, sandbox, run.Branch)
	if first == before {
		t.Fatal("tracked edit did not change the snapshot fingerprint")
	}
	if state := getRunWithState(t, app, run.ID).SyncState; state != SyncStateNeedsSync {
		t.Fatalf("tracked edit state = %q; want %q", state, SyncStateNeedsSync)
	}

	if err := os.WriteFile(filepath.Join(sandbox, "tracked.txt"), []byte("second edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second := sandboxSnapshotFingerprint(t, sandbox, run.Branch)
	if second == first {
		t.Fatal("second edit to an already-dirty file did not change the fingerprint")
	}
	if state := getRunWithState(t, app, run.ID).SyncState; state != SyncStateNeedsSync {
		t.Fatalf("second tracked edit state = %q; want %q", state, SyncStateNeedsSync)
	}

	if err := os.WriteFile(filepath.Join(sandbox, "untracked.txt"), []byte("new file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	untracked := sandboxSnapshotFingerprint(t, sandbox, run.Branch)
	if untracked == second {
		t.Fatal("untracked add did not change the fingerprint")
	}
	if err := os.WriteFile(filepath.Join(sandbox, "untracked.txt"), []byte("edited file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	editedUntracked := sandboxSnapshotFingerprint(t, sandbox, run.Branch)
	if editedUntracked == untracked {
		t.Fatal("untracked edit did not change the fingerprint")
	}
	if err := os.Remove(filepath.Join(sandbox, "untracked.txt")); err != nil {
		t.Fatal(err)
	}
	removedUntracked := sandboxSnapshotFingerprint(t, sandbox, run.Branch)
	if removedUntracked == editedUntracked {
		t.Fatal("untracked remove did not change the fingerprint")
	}

	cmd(t, sandbox, "git", "add", "tracked.txt")
	cmd(t, sandbox, "git", "commit", "-qm", "agent commit")
	committed := sandboxSnapshotFingerprint(t, sandbox, run.Branch)
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

func TestSyncStateReportsWrongCheckout(t *testing.T) {
	app, run, sandbox, _ := newSyncStateFixture(t)
	run.Synced = false
	run.SyncedSandboxFingerprint = ""
	run.LastSuccessfulSyncAt = time.Time{}
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	cmd(t, sandbox, "git", "switch", "-q", "-c", "agent/wrong-checkout")
	if err := os.WriteFile(filepath.Join(sandbox, "wrong-checkout.txt"), []byte("wrong checkout\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, sandbox, "git", "add", "wrong-checkout.txt")
	cmd(t, sandbox, "git", "commit", "-qm", "wrong checkout")
	updated := getRunWithState(t, app, run.ID)
	if updated.SyncState != SyncStateWrongCheckout {
		t.Fatalf("wrong-checkout state = %q; want %q", updated.SyncState, SyncStateWrongCheckout)
	}
	if updated.SyncState.Label() != "Wrong checkout" {
		t.Fatalf("wrong-checkout label = %q", updated.SyncState.Label())
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
