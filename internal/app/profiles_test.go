package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func profileFixture(name, agent, policy string) Profile {
	return Profile{Version: 1, Name: name, Agent: agent, Image: "image", Mode: "standard", Provider: "provider", Policy: policy, Approval: "prompted"}
}

func TestDeleteProfileRemovesRegistration(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	policy := filepath.Join(store.PolicyDir(), "policy.yaml")
	if err := os.WriteFile(policy, DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if err := store.SaveProfile(profileFixture(name, "codex", policy)); err != nil {
			t.Fatal(err)
		}
	}
	app := App{Store: store, Runner: &fakeRunner{}}
	if err := app.DeleteProfile("one"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.ProfilePath("one")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("profile file still exists: %v", err)
	}
	if _, err := os.Stat(store.ProfilePath("two")); err != nil {
		t.Fatalf("other profile removed: %v", err)
	}
}

func TestDeleteProfileRefusedForActiveRun(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	policy := filepath.Join(store.PolicyDir(), "policy.yaml")
	if err := os.WriteFile(policy, DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if err := store.SaveProfile(profileFixture(name, "codex", policy)); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveRun(Run{ID: "run-live123", Name: "live-run", ProfileName: "one", Status: StatusWorking}); err != nil {
		t.Fatal(err)
	}
	app := App{Store: store, Runner: &fakeRunner{}}
	err := app.DeleteProfile("one")
	var conflict *ConflictError
	if err == nil || !errors.As(err, &conflict) {
		t.Fatalf("error = %v; want conflict", err)
	}
	if !slices.Equal(conflict.Dependents, []string{"live-run"}) {
		t.Fatalf("dependents = %#v", conflict.Dependents)
	}
}

func TestDeleteProfileAllowedAfterRunTerminal(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	policy := filepath.Join(store.PolicyDir(), "policy.yaml")
	if err := os.WriteFile(policy, DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if err := store.SaveProfile(profileFixture(name, "codex", policy)); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveRun(Run{ID: "run-done123", ProfileName: "one", Status: StatusCompleted}); err != nil {
		t.Fatal(err)
	}
	app := App{Store: store, Runner: &fakeRunner{}}
	if err := app.DeleteProfile("one"); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteProfileLastAgentGuard(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	policy := filepath.Join(store.PolicyDir(), "policy.yaml")
	if err := os.WriteFile(policy, DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(profileFixture("only-codex", "codex", policy)); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(profileFixture("claude", "claude", policy)); err != nil {
		t.Fatal(err)
	}
	app := App{Store: store, Runner: &fakeRunner{}}
	err := app.DeleteProfile("only-codex")
	var conflict *ConflictError
	if err == nil || !errors.As(err, &conflict) || conflict.Reason == "" {
		t.Fatalf("error = %v; want last-agent conflict", err)
	}
	if err := app.DeleteProfile("claude"); err == nil {
		t.Fatal("last claude profile was accepted")
	}
	if err := store.SaveProfile(profileFixture("second-codex", "codex", policy)); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteProfile("only-codex"); err != nil {
		t.Fatalf("delete with same-agent profile remaining: %v", err)
	}
}
