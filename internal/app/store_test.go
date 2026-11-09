package app

import (
	"path/filepath"
	"testing"
)

func TestDefaultStoreUsesAgentBoxWorktreeRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTBOX_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	store, err := DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".agentbox", "worktrees")
	if got := store.WorktreesDir(); got != want {
		t.Fatalf("worktree root = %q; want %q", got, want)
	}
}
