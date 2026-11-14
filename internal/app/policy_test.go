package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePolicyRejectsUnsafeOrMalformedRules(t *testing.T) {
	if _, err := ParsePolicy(DefaultPolicy()); err != nil {
		t.Fatalf("default policy rejected: %v", err)
	}
	cases := map[string]string{
		"soft landlock":   strings.Replace(string(DefaultPolicy()), "hard_requirement", "best_effort", 1),
		"missing workdir": strings.Replace(string(DefaultPolicy()), "include_workdir: true", "include_workdir: false", 1),
		"invalid port":    strings.Replace(string(DefaultPolicy()), "network_policies: {}", "network_policies:\n  test-rule:\n    name: test-rule\n    endpoints:\n      - host: example.test\n        port: 70000\n    binaries:\n      - path: /usr/bin/curl\n", 1),
		"relative binary": strings.Replace(string(DefaultPolicy()), "network_policies: {}", "network_policies:\n  test-rule:\n    name: test-rule\n    endpoints:\n      - host: example.test\n        port: 443\n    binaries:\n      - path: curl\n", 1),
	}
	for name, policy := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePolicy([]byte(policy)); err == nil {
				t.Fatal("invalid policy was accepted")
			}
		})
	}
}

func TestDefaultPolicyAllowsAgentTemporaryFiles(t *testing.T) {
	policy, err := ParsePolicy(DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/tmp", "/var/tmp"} {
		found := false
		for _, path := range policy.Filesystem.ReadWrite {
			if path == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("default policy does not allow temporary files under %s: %v", want, policy.Filesystem.ReadWrite)
		}
	}
}

func TestAddNetworkRuleProducesValidRuleAndRejectsBadInputs(t *testing.T) {
	got, err := addNetworkRule(DefaultPolicy(), "codex", "api.example.test", "")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := ParsePolicy(got)
	if err != nil {
		t.Fatal(err)
	}
	rule, ok := policy.Network["host-api-example-test"]
	wantBinaries := []string{"/usr/local/bin/codex", "/usr/local/bin/node", "/usr/bin/git", "/usr/lib/git-core/**", "/usr/bin/curl", "/usr/bin/git-lfs"}
	if !ok || rule.Endpoints[0].Host != "api.example.test" || len(rule.Binaries) != len(wantBinaries) {
		t.Fatalf("unexpected generated rule: %+v", rule)
	}
	for i, want := range wantBinaries {
		if rule.Binaries[i].Path != want {
			t.Fatalf("binary %d = %q; want %q", i, rule.Binaries[i].Path, want)
		}
	}
	override, err := addNetworkRule(DefaultPolicy(), "codex", "api.example.test", "/usr/bin/curl")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(override)
	if err != nil {
		t.Fatal(err)
	}
	if binaries := parsed.Network["host-api-example-test"].Binaries; len(binaries) != 1 || binaries[0].Path != "/usr/bin/curl" {
		t.Fatalf("explicit binary override = %+v", binaries)
	}
	if _, err := addNetworkRule(DefaultPolicy(), "codex", "bad host", "relative/curl"); err == nil {
		t.Fatal("invalid rule input was accepted")
	}
}

func TestListPolicyConfigurationsExcludesRunCopies(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(store.PolicyDir(), "standard.yaml")
	if err := os.WriteFile(policyPath, DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	runPolicy := filepath.Join(store.PolicyDir(), "run-unit123.yaml")
	if err := os.WriteFile(runPolicy, DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	pendingPolicy := filepath.Join(store.PolicyDir(), "run-unit123-pending.yaml")
	if err := os.WriteFile(pendingPolicy, DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	profile := Profile{
		Version: 1, Name: "dev", Agent: "codex", Mode: "standard",
		Provider: "provider", Policy: policyPath, Approval: "prompted",
	}
	if err := store.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(Run{ID: "run-unit123", Policy: runPolicy, PendingPolicy: pendingPolicy}); err != nil {
		t.Fatal(err)
	}
	policies, err := store.ListPolicyConfigurations()
	if err != nil {
		t.Fatal(err)
	}
	if len(policies) != 1 {
		t.Fatalf("policies = %#v; want only standard", policies)
	}
	if policies[0].Name != "standard" || policies[0].Path != policyPath {
		t.Fatalf("unexpected policy: %+v", policies[0])
	}
	if len(policies[0].UsedBy) != 1 || policies[0].UsedBy[0] != "dev" {
		t.Fatalf("policy users = %#v; want dev", policies[0].UsedBy)
	}
}

func TestPolicyForAgentRewritesOnlyAgentBinary(t *testing.T) {
	policy := strings.Replace(string(DefaultPolicy()), "network_policies: {}", `network_policies:
  host-api:
    name: host-api
    endpoints:
      - host: api.example.test
        port: 443
        protocol: tcp
    binaries:
      - path: /usr/local/bin/codex
      - path: /usr/bin/curl
`, 1)
	got, err := PolicyForAgent([]byte(policy), "claude")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(got)
	if err != nil {
		t.Fatal(err)
	}
	rule := parsed.Network["host-api"]
	if rule.Binaries[0].Path != "/usr/local/bin/claude" {
		t.Fatalf("agent binary = %q; want claude", rule.Binaries[0].Path)
	}
	if rule.Binaries[1].Path != "/usr/bin/curl" {
		t.Fatalf("tool binary changed: %q", rule.Binaries[1].Path)
	}
}

func TestApplyPolicyConfigurationAdaptsAgentAndAppliesNetwork(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(store.PolicyDir(), "standard.yaml")
	config := strings.Replace(string(DefaultPolicy()), "network_policies: {}", `network_policies:
  host-api:
    name: host-api
    endpoints:
      - host: api.example.test
        port: 443
        protocol: tcp
    binaries:
      - path: /usr/local/bin/codex
      - path: /usr/bin/curl
`, 1)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	runPolicy := filepath.Join(store.PolicyDir(), "run-unit123.yaml")
	if err := os.WriteFile(runPolicy, DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	run := Run{ID: "run-unit123", Policy: runPolicy, Sandbox: "sandbox-unit123", Gateway: "gateway-one", Agent: "claude"}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	app := App{Store: store, Runner: runner}
	if err := app.ApplyPolicyConfiguration(context.Background(), "standard", run.ID); err != nil {
		t.Fatal(err)
	}
	applied, err := os.ReadFile(runPolicy)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(applied)
	if err != nil {
		t.Fatal(err)
	}
	rule := parsed.Network["host-api"]
	if rule.Binaries[0].Path != "/usr/local/bin/claude" || rule.Binaries[1].Path != "/usr/bin/curl" {
		t.Fatalf("applied binaries were not adapted correctly: %+v", rule.Binaries)
	}
	if len(runner.outputs) != 1 || runner.outputs[0].name != "openshell" || !commandHasArg(runner.outputs[0], "--wait") {
		t.Fatalf("policy was not applied live: %#v", runner.outputs)
	}
}

func TestSavePolicyEditDefersStaticChanges(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.PolicyDir(), "run-unit123.yaml")
	old := DefaultPolicy()
	if err := os.WriteFile(path, old, 0600); err != nil {
		t.Fatal(err)
	}
	run := Run{ID: "run-unit123", Policy: path, Sandbox: "sandbox-unit123", Gateway: "gateway-one"}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	app := App{Store: store, Runner: &fakeRunner{}}
	next := []byte(strings.Replace(string(old), "    - /tmp\n", "    - /var/tmp\n", 1))
	if err := app.savePolicyEdit(context.Background(), run, old, next); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != string(old) {
		t.Fatalf("live policy changed for a static edit: %v", err)
	}
	run, err = store.LoadRun(run.ID)
	if err != nil || run.PendingPolicy == "" {
		t.Fatalf("pending policy not recorded: %+v, %v", run, err)
	}
	pending, err := os.ReadFile(run.PendingPolicy)
	if err != nil || string(pending) != string(next) {
		t.Fatalf("pending policy contents mismatch: %v", err)
	}
}

func TestSavePolicyEditAppliesNetworkChangeAndRollsBackOnFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "apply", true: "rollback"}[fail], func(t *testing.T) {
			store := Store{Base: t.TempDir()}
			if err := store.Ensure(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(store.PolicyDir(), "run-unit123.yaml")
			old := DefaultPolicy()
			if err := os.WriteFile(path, old, 0600); err != nil {
				t.Fatal(err)
			}
			run := Run{ID: "run-unit123", Policy: path, Sandbox: "sandbox-unit123", Gateway: "gateway-one"}
			if err := store.SaveRun(run); err != nil {
				t.Fatal(err)
			}
			next, err := addNetworkRule(old, "codex", "api.example.test", "/usr/bin/codex")
			if err != nil {
				t.Fatal(err)
			}
			runner := &fakeRunner{}
			if fail {
				runner.output = func(recordedCommand) ([]byte, error) { return nil, errors.New("policy rejected") }
			}
			app := App{Store: store, Runner: runner}
			err = app.savePolicyEdit(context.Background(), run, old, next)
			if fail {
				if err == nil || !strings.Contains(err.Error(), "local policy restored") {
					t.Fatalf("expected apply failure with rollback, got %v", err)
				}
				got, readErr := os.ReadFile(path)
				if readErr != nil || string(got) != string(old) {
					t.Fatalf("rollback did not restore original policy: %v", readErr)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				got, readErr := os.ReadFile(path)
				if readErr != nil || string(got) != string(next) {
					t.Fatalf("network edit not saved: %v", readErr)
				}
			}
			if len(runner.outputs) != 1 || runner.outputs[0].name != "openshell" || !commandHasArg(runner.outputs[0], "--wait") {
				t.Fatalf("policy update command was not issued with wait: %#v", runner.outputs)
			}
		})
	}
}

func TestPolicyConfigurationContentRoundTrip(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	created, err := store.CreatePolicyConfiguration("reusable")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadPolicyConfigurationContent("reusable")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded, DefaultPolicy()) {
		t.Fatalf("initial content mismatch:\n%s", loaded)
	}
	next, err := addNetworkRule(loaded, "codex", "api.example.test", "/usr/local/bin/codex")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePolicyConfigurationContent("reusable", next); err != nil {
		t.Fatal(err)
	}
	configuration, err := store.LoadPolicyConfiguration("reusable")
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Name != "reusable" || configuration.Path != created.Path {
		t.Fatalf("configuration = %#v", configuration)
	}
	if err := store.SavePolicyConfigurationContent("reusable", []byte("version: 2")); err == nil {
		t.Fatal("invalid policy was accepted")
	}
}

func TestPolicyConfigurationUsedByProfiles(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	policy, err := store.CreatePolicyConfiguration("shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(Profile{
		Version: 1, Name: "codex-shared", Agent: "codex", Image: "image", Mode: "standard",
		Provider: "provider", Policy: policy.Path, Approval: "prompted",
	}); err != nil {
		t.Fatal(err)
	}
	configuration, err := store.LoadPolicyConfiguration("shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(configuration.UsedBy) != 1 || configuration.UsedBy[0] != "codex-shared" {
		t.Fatalf("used by = %#v", configuration.UsedBy)
	}
}
