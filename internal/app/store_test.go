package app

import (
	"os"
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
