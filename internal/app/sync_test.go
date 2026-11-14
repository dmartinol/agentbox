package app

import (
	"archive/tar"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	}
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
