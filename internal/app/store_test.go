package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyRunLoadsWithoutSyncStateMetadata(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	legacy := `{
  "id": "run-legacy",
  "created": "2026-01-01T10:00:00Z",
  "source": "/repo",
  "worktree": "/trees/run-legacy",
  "branch": "agentbox/run-legacy",
  "baseline": "abc",
  "last_synced": "def",
  "fingerprint": "host-fingerprint",
  "sandbox": "sandbox-legacy",
  "gateway": "openshell",
  "agent": "codex",
  "image": "localhost/openshell:codex-1.0.0-r3",
  "mode": "standard",
  "model": "",
  "approval": "prompted",
  "status": "stopped",
  "status_detail": "stopped by user",
  "synced": true,
  "policy": "/policies/run-legacy.yaml"
}
`
	if err := os.WriteFile(store.RunPath("run-legacy"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	run, err := store.LoadRun("run-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if !run.LastSuccessfulSyncAt.IsZero() || run.SyncedSandboxFingerprint != "" {
		t.Fatalf("legacy run gained sync metadata: %+v", run)
	}
	if run.LastSynced != "def" || run.Fingerprint != "host-fingerprint" || !run.Synced {
		t.Fatalf("legacy sync fields changed: %+v", run)
	}
	if run.SyncState != "" {
		t.Fatalf("legacy run carries a persisted sync state: %q", run.SyncState)
	}
	run.SyncState = SyncStateNeedsSync
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(store.RunPath("run-legacy"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "sync_state") {
		t.Fatalf("SaveRun persisted derived sync state:\n%s", body)
	}
}

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

func TestListProfilesIgnoresGeneratedProviderProfiles(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	profile := Profile{
		Version: 1, Name: "codex-gateway", Agent: "codex", Image: "image", Mode: "gateway",
		Model: "model", Provider: "model-gateway", Policy: "/tmp/policy.yaml", Approval: "prompted",
		BaseURL: "https://models.example.test/v1", APIKeyEnv: "UNIT_MODEL_KEY",
	}
	if err := store.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	provider := "id: agentbox-gateway-bearer-models-example-test\ndisplay_name: AgentBox gateway-bearer\n"
	providerPath := filepath.Join(store.ProfilesDir(), "agentbox-gateway-bearer-models-example-test-provider.yaml")
	if err := os.WriteFile(providerPath, []byte(provider), 0600); err != nil {
		t.Fatal(err)
	}
	profiles, err := store.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Name != profile.Name {
		t.Fatalf("profiles = %#v", profiles)
	}
}
