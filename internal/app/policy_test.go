package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
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

func TestApplyPolicyConfigurationKeepsRepositoryFragment(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(store.PolicyDir(), "standard.yaml")
	if err := os.WriteFile(configPath, DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	fragmentPath := filepath.Join(store.PolicyDir(), "repo-example.yaml")
	fragment := strings.Replace(string(DefaultPolicy()), "network_policies: {}", `network_policies:
  host-proxy-golang-org-443:
    name: host-proxy-golang-org-443
    endpoints:
      - host: proxy.golang.org
        port: 443
        protocol: tcp
    binaries:
      - path: /usr/local/go/bin/go
`, 1)
	if err := os.WriteFile(fragmentPath, []byte(fragment), 0600); err != nil {
		t.Fatal(err)
	}
	composed, err := composeRepoPolicy(DefaultPolicy(), configPath, []byte(fragment), fragmentPath)
	if err != nil {
		t.Fatal(err)
	}
	runPolicy := filepath.Join(store.PolicyDir(), "run-unit123.yaml")
	if err := os.WriteFile(runPolicy, composed, 0600); err != nil {
		t.Fatal(err)
	}
	run := Run{ID: "run-unit123", Policy: runPolicy, PolicyFragment: fragmentPath, Sandbox: "sandbox-unit123", Gateway: "gateway-one", Agent: "codex"}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	app := App{Store: store, Runner: &fakeRunner{}}
	if err := app.ApplyPolicyConfiguration(context.Background(), "standard", run.ID); err != nil {
		t.Fatal(err)
	}
	applied, err := os.ReadFile(runPolicy)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := ParsePolicy(applied)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := policy.Network["host-proxy-golang-org-443"]; !ok {
		t.Fatalf("applied policy lost the repository fragment: %v", policy.Network)
	}
}

func TestApplyPolicyConfigurationRequiresRepositoryFragment(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.PolicyDir(), "standard.yaml"), DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	runPolicy := filepath.Join(store.PolicyDir(), "run-unit123.yaml")
	if err := os.WriteFile(runPolicy, DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	run := Run{ID: "run-unit123", Policy: runPolicy, PolicyFragment: filepath.Join(store.PolicyDir(), "missing.yaml"), Sandbox: "sandbox-unit123", Gateway: "gateway-one", Agent: "codex"}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	app := App{Store: store, Runner: &fakeRunner{}}
	err := app.ApplyPolicyConfiguration(context.Background(), "standard", run.ID)
	if err == nil || !strings.Contains(err.Error(), "rebuild it with `agentbox repo setup`") {
		t.Fatalf("error = %v, want a fragment remediation hint", err)
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
	result, err := app.savePolicyEdit(context.Background(), run, old, next)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != PolicyEditStaticPending {
		t.Fatalf("outcome = %q; want %q", result.Outcome, PolicyEditStaticPending)
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
	if result.PolicyPath != run.PendingPolicy {
		t.Fatalf("result policy path = %q; want %q", result.PolicyPath, run.PendingPolicy)
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
			result, err := app.savePolicyEdit(context.Background(), run, old, next)
			if fail {
				if err == nil || !strings.Contains(err.Error(), "the local policy was restored") {
					t.Fatalf("expected apply failure with rollback, got %v", err)
				}
				if result.Outcome != PolicyEditOpenShellRejected || !result.RolledBack {
					t.Fatalf("rejection result = %+v", result)
				}
				got, readErr := os.ReadFile(path)
				if readErr != nil || string(got) != string(old) {
					t.Fatalf("rollback did not restore original policy: %v", readErr)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if result.Outcome != PolicyEditLiveApplied || result.RolledBack {
					t.Fatalf("live result = %+v", result)
				}
				if result.PolicyPath != path {
					t.Fatalf("result policy path = %q; want %q", result.PolicyPath, path)
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

func TestDeletePolicyLifecycleGuards(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	defaultPolicy, err := store.CreatePolicyConfiguration("profile-name")
	if err != nil {
		t.Fatal(err)
	}
	otherPolicy, err := store.CreatePolicyConfiguration("other")
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := store.CreatePolicyConfiguration("orphan")
	if err != nil {
		t.Fatal(err)
	}
	profile := Profile{Version: 1, Name: "profile-name", Agent: "codex", Mode: "standard", Provider: "provider", Policy: otherPolicy.Path, Approval: "prompted"}
	if err := store.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	app := App{Store: store, Runner: &fakeRunner{}}

	err = app.DeletePolicyConfiguration("profile-name")
	var conflict *ConflictError
	if err == nil || !errors.As(err, &conflict) || !slices.Equal(conflict.Dependents, []string{"profile-name"}) {
		t.Fatalf("generated-default guard = %v", err)
	}

	err = app.DeletePolicyConfiguration("other")
	if err == nil || !errors.As(err, &conflict) || !slices.Equal(conflict.Dependents, []string{"profile-name"}) {
		t.Fatalf("reference guard = %v", err)
	}

	if err := app.DeletePolicyConfiguration("orphan"); err != nil {
		t.Fatalf("orphan delete = %v", err)
	}
	if _, err := os.Stat(orphan.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan retained: %v", err)
	}
	if _, err := os.Stat(defaultPolicy.Path); err != nil {
		t.Fatalf("generated default removed: %v", err)
	}

	profile.Policy = otherPolicy.Path
	if err := store.DeleteProject("does-not-exist"); err == nil {
		t.Fatal("unexpected project delete")
	}
	if err := os.Remove(store.ProfilePath("profile-name")); err != nil {
		t.Fatal(err)
	}
	if err := app.DeletePolicyConfiguration("profile-name"); err != nil {
		t.Fatalf("orphan default after profile removal = %v", err)
	}
}

func TestDeletePolicyConfigurationRunGuards(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	live, err := store.CreatePolicyConfiguration("run-live123")
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.CreatePolicyConfiguration("run-pending123-pending")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(Run{ID: "run-live123", Name: "live-run", Policy: live.Path}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(Run{ID: "run-pending123", Name: "pending-run", PendingPolicy: pending.Path}); err != nil {
		t.Fatal(err)
	}
	app := App{Store: store, Runner: &fakeRunner{}}

	cases := map[string][]string{
		"run-live123":            {"live-run"},
		"run-pending123-pending": {"pending-run"},
	}
	for name, expected := range cases {
		err := app.DeletePolicyConfiguration(name)
		var conflict *ConflictError
		if err == nil || !errors.As(err, &conflict) || conflict.Reason != "runs reference it" || !slices.Equal(conflict.Dependents, expected) {
			t.Fatalf("%s run guard = %v", name, err)
		}
	}
	if _, err := os.Stat(live.Path); err != nil {
		t.Fatalf("live run policy removed: %v", err)
	}
	if _, err := os.Stat(pending.Path); err != nil {
		t.Fatalf("pending run policy removed: %v", err)
	}
}

func policyEditRunner(probe string, applyErr error) *fakeRunner {
	return &fakeRunner{output: func(cmd recordedCommand) ([]byte, error) {
		switch {
		case cmd.name == "openshell" && slices.Contains(cmd.args, "sandbox") && slices.Contains(cmd.args, "get"):
			return []byte(`{"phase":"running"}`), nil
		case cmd.name == "openshell" && slices.Contains(cmd.args, "sandbox") && slices.Contains(cmd.args, "exec"):
			return []byte(probe), nil
		case cmd.name == "openshell" && slices.Contains(cmd.args, "policy") && slices.Contains(cmd.args, "set"):
			return nil, applyErr
		default:
			return nil, nil
		}
	}}
}

func newPolicyEditTestRun(t *testing.T) (App, Run, string) {
	t.Helper()
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.PolicyDir(), "run-unit123.yaml")
	if err := os.WriteFile(path, DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	run := Run{ID: "run-unit123", Policy: path, Sandbox: "sandbox-unit123", Gateway: "gateway-one", Agent: "codex", Status: StatusWorking}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	return App{Store: store}, run, path
}

func TestAgentBinaryDerivesSupportedLauncherOnly(t *testing.T) {
	for agent, want := range map[string]string{
		"claude":   "/usr/local/bin/claude",
		"codex":    "/usr/local/bin/codex",
		"opencode": "/usr/local/bin/opencode",
	} {
		got, err := AgentBinary(agent)
		if err != nil || got != want {
			t.Fatalf("AgentBinary(%q) = %q, %v; want %q", agent, got, err, want)
		}
	}
	if _, err := AgentBinary("unknown"); err == nil {
		t.Fatal("unsupported agent was accepted")
	}
	paths, err := agentEgressBinaries("codex")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/usr/local/bin/codex", "/usr/local/bin/node", "/usr/bin/git", "/usr/lib/git-core/**", "/usr/bin/curl", "/usr/bin/git-lfs"}
	if !slices.Equal(paths, want) {
		t.Fatalf("egress binaries = %#v; want %#v", paths, want)
	}
	if _, err := agentEgressBinaries("unknown"); err == nil {
		t.Fatal("egress binaries accepted an unsupported agent")
	}
}

func TestPolicyEditDryRunPreviewsWithoutMutating(t *testing.T) {
	app, run, path := newPolicyEditTestRun(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	app.Runner = runner

	result, err := app.PolicyEdit(context.Background(), run.ID, "api.example.test", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != PolicyEditNetworkPreview {
		t.Fatalf("outcome = %q; want %q", result.Outcome, PolicyEditNetworkPreview)
	}
	if !strings.Contains(result.Diff, "host-api-example-test") || !strings.Contains(result.Diff, "+") {
		t.Fatalf("diff = %q", result.Diff)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("dry run mutated policy: %v", err)
	}
	if len(runner.outputs) != 0 {
		t.Fatalf("dry run issued commands: %#v", runner.outputs)
	}
}

func TestPolicyEditDryRunReportsStaticPreview(t *testing.T) {
	app, run, path := newPolicyEditTestRun(t)
	app.Runner = &fakeRunner{}
	editor := filepath.Join(t.TempDir(), "editor.sh")
	script := "#!/bin/sh\nperl -0pi -e 's|    - /tmp\\n|    - /tmp\\n    - /srv\\n|' \"$1\"\n"
	if err := os.WriteFile(editor, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editor)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.PolicyEdit(context.Background(), run.ID, "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != PolicyEditStaticPreview {
		t.Fatalf("outcome = %q; want %q", result.Outcome, PolicyEditStaticPreview)
	}
	if !strings.Contains(result.Diff, "+    - /srv") {
		t.Fatalf("diff = %q", result.Diff)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("dry run mutated policy: %v", err)
	}
}

func TestRunPolicyFileReturnsTheRunCopy(t *testing.T) {
	app, run, path := newPolicyEditTestRun(t)
	gotPath, content, err := app.RunPolicyFile(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != path || content != string(DefaultPolicy()) {
		t.Fatalf("RunPolicyFile = %q, %q; want %q with the default policy", gotPath, content, path)
	}
	if _, _, err := app.RunPolicyFile("missing-run"); err == nil {
		t.Fatal("expected an error for an unknown run")
	}
}

func TestPolicyEditAppliesNetworkChangeLive(t *testing.T) {
	app, run, path := newPolicyEditTestRun(t)
	runner := policyEditRunner("session=1\ndead=0\ncommand=codex\nplan=0\nidle=0\nfinished=0\nexit=\nactivity=\nmtime=\n", nil)
	app.Runner = runner

	result, err := app.PolicyEdit(context.Background(), run.ID, "api.example.test", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != PolicyEditLiveApplied || result.RolledBack {
		t.Fatalf("result = %+v", result)
	}
	if result.PolicyPath != path {
		t.Fatalf("result policy path = %q; want %q", result.PolicyPath, path)
	}
	if !strings.Contains(result.Diff, "host-api-example-test") {
		t.Fatalf("diff = %q", result.Diff)
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), "api.example.test") {
		t.Fatalf("live policy not saved: %v\n%s", err, content)
	}
	var set *recordedCommand
	for i := range runner.outputs {
		if slices.Contains(runner.outputs[i].args, "policy") && slices.Contains(runner.outputs[i].args, "set") {
			set = &runner.outputs[i]
		}
	}
	if set == nil {
		t.Fatalf("OpenShell policy set not invoked: %#v", runner.outputs)
	}
	for _, want := range []string{run.Sandbox, run.Policy, "--wait", run.Gateway} {
		if !commandHasArg(*set, want) {
			t.Fatalf("policy set missing %q: %#v", want, set.args)
		}
	}
}

func TestPolicyEditRejectsRunWithoutLiveSandbox(t *testing.T) {
	app, run, path := newPolicyEditTestRun(t)
	runner := policyEditRunner("session=0\ndead=1\ncommand=\nplan=0\nidle=0\nfinished=1\nexit=0\nactivity=\nmtime=\n", nil)
	app.Runner = runner
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.PolicyEdit(context.Background(), run.ID, "api.example.test", "", false)
	var noSandbox *NoLiveSandboxError
	if err == nil || !errors.As(err, &noSandbox) {
		t.Fatalf("error = %v", err)
	}
	if result.Outcome != PolicyEditNoLiveSandbox || result.Diff != "" {
		t.Fatalf("result = %+v", result)
	}
	if err.Error() != "Allow host is available only for Ready, Working, and Waiting agents." {
		t.Fatalf("error text = %q", err.Error())
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(before, after) {
		t.Fatalf("policy mutated for non-attachable run: %v", readErr)
	}
	for _, call := range runner.outputs {
		if slices.Contains(call.args, "policy") && slices.Contains(call.args, "set") {
			t.Fatalf("policy set invoked for non-attachable run: %#v", call.args)
		}
	}
}

func TestPolicyEditReportsOpenShellRejectionAndRollback(t *testing.T) {
	app, run, path := newPolicyEditTestRun(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	app.Runner = policyEditRunner("session=1\ndead=0\ncommand=codex\nplan=0\nidle=0\nfinished=0\nexit=\nactivity=\nmtime=\n", errors.New("policy rejected"))

	result, err := app.PolicyEdit(context.Background(), run.ID, "api.example.test", "", false)
	if err == nil || !strings.Contains(err.Error(), "the local policy was restored: policy rejected") {
		t.Fatalf("error = %v", err)
	}
	if result.Outcome != PolicyEditOpenShellRejected || !result.RolledBack || result.Diff == "" {
		t.Fatalf("result = %+v", result)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(before, after) {
		t.Fatalf("rollback did not restore policy: %v", readErr)
	}
}

func TestPolicyEditReportsRollbackFailure(t *testing.T) {
	app, run, path := newPolicyEditTestRun(t)
	applyErr := errors.New("policy rejected")
	runner := policyEditRunner("session=1\ndead=0\ncommand=codex\nplan=0\nidle=0\nfinished=0\nexit=\nactivity=\nmtime=\n", nil)
	runner.output = func(cmd recordedCommand) ([]byte, error) {
		switch {
		case cmd.name == "openshell" && slices.Contains(cmd.args, "sandbox") && slices.Contains(cmd.args, "get"):
			return []byte(`{"phase":"running"}`), nil
		case cmd.name == "openshell" && slices.Contains(cmd.args, "sandbox") && slices.Contains(cmd.args, "exec"):
			return []byte("session=1\ndead=0\ncommand=codex\nplan=0\nidle=0\nfinished=0\nexit=\nactivity=\nmtime=\n"), nil
		case cmd.name == "openshell" && slices.Contains(cmd.args, "policy") && slices.Contains(cmd.args, "set"):
			if err := os.Remove(path); err != nil {
				return nil, err
			}
			if err := os.Mkdir(path, 0700); err != nil {
				return nil, err
			}
			return nil, applyErr
		default:
			return nil, nil
		}
	}
	app.Runner = runner

	result, err := app.PolicyEdit(context.Background(), run.ID, "api.example.test", "", false)
	if err == nil || !strings.Contains(err.Error(), "local policy rollback failed") || !strings.Contains(err.Error(), applyErr.Error()) {
		t.Fatalf("error = %v", err)
	}
	if result.Outcome != PolicyEditOpenShellRejected || result.RolledBack || result.Diff == "" {
		t.Fatalf("result = %+v", result)
	}
}
