package app

import (
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
