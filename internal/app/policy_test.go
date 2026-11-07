package app

import (
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
	if !ok || rule.Endpoints[0].Host != "api.example.test" || rule.Binaries[0].Path != "/usr/local/bin/codex" {
		t.Fatalf("unexpected generated rule: %+v", rule)
	}
	if _, err := addNetworkRule(DefaultPolicy(), "codex", "bad host", "relative/curl"); err == nil {
		t.Fatal("invalid rule input was accepted")
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
