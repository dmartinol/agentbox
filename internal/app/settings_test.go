package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsDefaultsAndRoundTrip(t *testing.T) {
	store := Store{Base: t.TempDir()}
	settings, err := store.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Worktrees.Root != filepath.Join(store.Base, "worktrees") || settings.Worktrees.BranchPrefix != "agentbox" {
		t.Fatalf("unexpected defaults: %+v", settings)
	}
	root := filepath.Join(t.TempDir(), "worktrees")
	settings.Worktrees.Root = root
	settings.Worktrees.BranchPrefix = "runs"
	if err := store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Worktrees.Root != root || loaded.Worktrees.BranchPrefix != "runs" {
		t.Fatalf("settings did not round trip: %+v", loaded)
	}
}

func TestWorktreeLocationUsesProjectAndRunName(t *testing.T) {
	store := Store{Base: t.TempDir(), WorktreeRoot: filepath.Join(t.TempDir(), "trees")}
	app := App{Store: store, Runner: &fakeRunner{}}
	root, branch, err := app.WorktreeLocation("agentbox", "auth-refactor", "run-123")
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join(store.WorktreesDir(), "agentbox", "auth-refactor")
	if root != wantRoot || branch != "agentbox/agentbox/auth-refactor" {
		t.Fatalf("worktree location = %q %q; want %q and %q", root, branch, wantRoot, "agentbox/agentbox/auth-refactor")
	}
}

func TestRunWorktreeUsesDocumentedDestination(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name         string
		store        Store
		wantWorktree string
	}{
		{
			name:         "project run",
			store:        Store{Base: filepath.Join(root, "default-state")},
			wantWorktree: filepath.Join(root, "default-state", "worktrees", "demo", "fix"),
		},
		{
			name:         "custom worktree root",
			store:        Store{Base: filepath.Join(root, "custom-state"), WorktreeRoot: filepath.Join(root, "custom-worktrees")},
			wantWorktree: filepath.Join(root, "custom-worktrees", "demo", "fix"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "source")
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
			cmd(t, source, "git", "commit", "-qm", "base")

			application := App{Store: test.store, Runner: OSRunner{}}
			worktree, branch, err := application.WorktreeLocation("demo", "fix", "run-test123")
			if err != nil {
				t.Fatal(err)
			}
			if worktree != test.wantWorktree {
				t.Fatalf("worktree location = %q; want %q", worktree, test.wantWorktree)
			}
			run, err := application.CreateWorktreeAt(context.Background(), source, "run-test123", worktree, branch)
			if err != nil {
				t.Fatal(err)
			}
			if run.Worktree != worktree {
				t.Fatalf("run worktree = %q; want %q", run.Worktree, worktree)
			}
		})
	}
}

func TestValidateRunNameRejectsDuplicateProjectName(t *testing.T) {
	store := Store{Base: t.TempDir()}
	app := App{Store: store, Runner: &fakeRunner{}}
	if err := store.SaveRun(Run{ID: "run-one", Name: "auth-refactor", Project: "agentbox", Status: "stopped"}); err != nil {
		t.Fatal(err)
	}
	if err := app.validateRunName("agentbox", "auth-refactor"); err == nil {
		t.Fatal("duplicate run name was accepted")
	}
	if err := app.validateRunName("other", "auth-refactor"); err != nil {
		t.Fatalf("same name in another project should be allowed: %v", err)
	}
}
