package app

import (
	"path/filepath"
	"testing"
)

func TestDefaultStoreUsesAgentBoxHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTBOX_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	store, err := DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(home, ".agentbox")
	if store.Base != base {
		t.Fatalf("state directory = %q; want %q", store.Base, base)
	}
	want := filepath.Join(base, "worktrees")
	if got := store.WorktreesDir(); got != want {
		t.Fatalf("worktree root = %q; want %q", got, want)
	}
}
