package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testProfile(agent, mode, approval string) Profile {
	return Profile{
		Version:  1,
		Name:     "unit-profile",
		Agent:    agent,
		Image:    "localhost/openshell-" + agent + ":1.2.3-r3",
		Mode:     mode,
		Model:    "rits/zai-org/glm-5-3",
		Provider: "unit-provider",
		Policy:   "/tmp/unit-policy.yaml",
		Approval: approval,
	}
}

func TestPrepareAgentGeneratesIsolatedConfigForEachAgentMode(t *testing.T) {
	for _, agent := range []string{"claude", "codex", "opencode"} {
		for _, mode := range []string{"standard", "enmaas"} {
			t.Run(agent+"/"+mode, func(t *testing.T) {
				store := Store{Base: t.TempDir()}
				app := App{Store: store}
				run := Run{ID: "run-unit123"}
				profile := testProfile(agent, mode, "prompted")
				dir, err := app.PrepareAgent(context.Background(), run, profile, "print 'hello'")
				if err != nil {
					t.Fatal(err)
				}
				script, err := os.ReadFile(filepath.Join(dir, "agent.sh"))
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{"HOME=/workspace/state/home", "XDG_CONFIG_HOME=/workspace/state/config", "XDG_DATA_HOME=/workspace/state/data", "print", "hello"} {
					if !strings.Contains(string(script), want) {
						t.Errorf("script lacks %q:\n%s", want, script)
					}
				}
				if strings.Contains(string(script), "dangerously") || strings.Contains(string(script), "--auto") {
					t.Fatalf("prompted config bypasses approval: %s", script)
				}
				if mode == "enmaas" {
					switch agent {
					case "codex":
						config, err := os.ReadFile(filepath.Join(dir, "home", ".codex", "config.toml"))
						if err != nil || !strings.Contains(string(config), "ENMAAS_KEY") || !strings.Contains(string(config), enmaasURL+"/v1") {
							t.Fatalf("Codex EnMaaS config lacks endpoint/reference: %v %s", err, config)
						}
					case "opencode":
						config, err := os.ReadFile(filepath.Join(dir, "config", "opencode", "opencode.json"))
						if err != nil || !json.Valid(config) || !strings.Contains(string(config), "ENMAAS_KEY") || !strings.Contains(string(config), enmaasURL+"/v1") {
							t.Fatalf("OpenCode EnMaaS config lacks endpoint/reference: %v %s", err, config)
						}
					case "claude":
						settings, err := os.ReadFile(filepath.Join(dir, "home", ".claude", "settings.json"))
						if err != nil || !json.Valid(settings) || !strings.Contains(string(settings), enmaasURL) {
							t.Fatalf("Claude EnMaaS endpoint missing: %v %s", err, settings)
						}
					}
				}
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) == 0 {
					t.Fatalf("configuration directory missing: %v", err)
				}
			})
		}
	}
}

func TestAgentScriptApprovalModesAndArgumentGuards(t *testing.T) {
	for _, agent := range []string{"claude", "codex", "opencode"} {
		t.Run(agent, func(t *testing.T) {
			prompted := testProfile(agent, "standard", "prompted")
			script, err := agentScript(prompted, "do work")
			if err != nil {
				t.Fatal(err)
			}
			for _, bypass := range []string{"--dangerously-skip-permissions", "--dangerously-bypass-approvals-and-sandbox", "--auto"} {
				if strings.Contains(script, bypass) {
					t.Fatalf("prompted %s script includes %s", agent, bypass)
				}
			}
			prompted.Args = []string{"--yolo"}
			if _, err := agentScript(prompted, ""); err == nil {
				t.Fatal("prompted mode accepted a bypass argument")
			}
			unattended := testProfile(agent, "standard", "unattended")
			script, err = agentScript(unattended, "do work")
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"claude": "--dangerously-skip-permissions", "codex": "--dangerously-bypass-approvals-and-sandbox", "opencode": "--auto"}[agent]
			if !strings.Contains(script, want) {
				t.Fatalf("unattended %s script lacks %s:\n%s", agent, want, script)
			}
		})
	}
}

func TestProviderSetupKeepsCredentialValueOutOfGeneratedData(t *testing.T) {
	const sentinel = "unit-secret-must-not-be-written"
	t.Setenv("ENMAAS_KEY", sentinel)
	runner := &fakeRunner{}
	store := Store{Base: t.TempDir()}
	app := App{Store: store, Runner: runner}
	if err := app.ProviderSetup(context.Background(), "enmaas-bearer", "unit-provider", "gateway-one", false, false); err != nil {
		t.Fatal(err)
	}
	provider, err := os.ReadFile(filepath.Join(store.ProfilesDir(), "agentbox-enmaas-bearer-provider.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(provider), sentinel) || !strings.Contains(string(provider), "ENMAAS_KEY") {
		t.Fatalf("provider profile leaked or omitted credential reference: %s", provider)
	}
	for _, call := range runner.outputs {
		for _, arg := range call.args {
			if strings.Contains(arg, sentinel) {
				t.Fatalf("secret appeared in command argument: %#v", call)
			}
		}
	}
}

func TestSessionScriptOffersIdleReattach(t *testing.T) {
	for _, want := range []string{
		"Keep this run idle for reattach?",
		"Restart: /bin/sh /workspace/state/agent.sh",
		"Detach: Ctrl-b d",
		"touch /workspace/finished",
	} {
		if !strings.Contains(sessionScript, want) {
			t.Fatalf("session script missing %q:\n%s", want, sessionScript)
		}
	}
}
