package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testProfile(agent, mode, approval string) Profile {
	return Profile{
		Version:   1,
		Name:      "unit-profile",
		Agent:     agent,
		Image:     "localhost/openshell-" + agent + ":1.2.3-r3",
		Mode:      mode,
		Model:     "unit-model",
		Provider:  "unit-provider",
		Policy:    "/tmp/unit-policy.yaml",
		Approval:  approval,
		BaseURL:   "https://models.example.test/v1",
		APIKeyEnv: "UNIT_MODEL_KEY",
	}
}

func TestPrepareAgentGeneratesIsolatedConfigForEachAgentMode(t *testing.T) {
	for _, agent := range []string{"claude", "codex", "opencode"} {
		for _, mode := range []string{"standard", "gateway"} {
			t.Run(agent+"/"+mode, func(t *testing.T) {
				store := Store{Base: t.TempDir()}
				app := App{Store: store}
				run := Run{ID: "run-unit123"}
				profile := testProfile(agent, mode, "prompted")
				dir, err := app.PrepareAgent(context.Background(), run, profile, "print 'hello'", nil)
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
				if mode == "gateway" {
					switch agent {
					case "codex":
						config, err := os.ReadFile(filepath.Join(dir, "home", ".codex", "config.toml"))
						if err != nil || !strings.Contains(string(config), "UNIT_MODEL_KEY") || !strings.Contains(string(config), "https://models.example.test/v1") {
							t.Fatalf("Codex gateway config lacks endpoint/reference: %v %s", err, config)
						}
					case "opencode":
						config, err := os.ReadFile(filepath.Join(dir, "config", "opencode", "opencode.json"))
						if err != nil || !json.Valid(config) || !strings.Contains(string(config), "UNIT_MODEL_KEY") || !strings.Contains(string(config), "https://models.example.test/v1") {
							t.Fatalf("OpenCode gateway config lacks endpoint/reference: %v %s", err, config)
						}
					case "claude":
						settings, err := os.ReadFile(filepath.Join(dir, "home", ".claude", "settings.json"))
						if err != nil || !json.Valid(settings) || !strings.Contains(string(settings), "https://models.example.test") {
							t.Fatalf("Claude gateway endpoint missing: %v %s", err, settings)
						}
						if !strings.Contains(string(script), `export ANTHROPIC_API_KEY="${UNIT_MODEL_KEY}"`) {
							t.Fatalf("Claude gateway script does not reference the gateway key environment variable:\n%s", script)
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

func TestCodexStandardScriptUsesOpenAIProviderKey(t *testing.T) {
	profile := testProfile("codex", "standard", "prompted")
	profile.APIKeyEnv = "OPENAI_API_KEY"
	script, err := agentScript(profile, "do work")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`printf '%s' "$OPENAI_API_KEY" | codex login --with-api-key`,
		"--no-daemon",
		"--sandbox",
		"danger-full-access",
		"--model",
		"unit-model",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("Codex standard script lacks %q:\n%s", want, script)
		}
	}
}

func TestAgentScriptUsesUTF8Locale(t *testing.T) {
	script, err := agentScript(testProfile("codex", "standard", "prompted"), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"export LANG=C.UTF-8", "export LC_ALL=C.UTF-8"} {
		if !strings.Contains(script, want) {
			t.Fatalf("agent script lacks %q:\n%s", want, script)
		}
	}
}

func TestCodexStandardScriptWorksWithoutModel(t *testing.T) {
	profile := testProfile("codex", "standard", "prompted")
	profile.Model = ""
	profile.APIKeyEnv = "OPENAI_API_KEY"
	script, err := agentScript(profile, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, "--model") {
		t.Fatalf("empty standard model should use Codex default:\n%s", script)
	}
	for _, want := range []string{"--no-daemon", "--sandbox", "danger-full-access"} {
		if !strings.Contains(script, want) {
			t.Fatalf("Codex standard script lacks %q:\n%s", want, script)
		}
	}
}

func TestOpenCodeGatewayScriptUsesConfigModel(t *testing.T) {
	script, err := agentScript(testProfile("opencode", "gateway", "unattended"), "do work")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, "--model") {
		t.Fatalf("OpenCode gateway script must use its generated config instead of --model:\n%s", script)
	}
	for _, want := range []string{"opencode", "run", "--standalone", "--auto", "do work"} {
		if !strings.Contains(script, want) {
			t.Fatalf("OpenCode gateway script lacks %q:\n%s", want, script)
		}
	}
}

func TestProviderSetupKeepsCredentialValueOutOfGeneratedData(t *testing.T) {
	const sentinel = "unit-secret-must-not-be-written"
	t.Setenv("UNIT_MODEL_KEY", sentinel)
	runner := &fakeRunner{}
	store := Store{Base: t.TempDir()}
	app := App{Store: store, Runner: runner}
	if err := app.ProviderSetup(context.Background(), "gateway-bearer", "unit-provider", "models.example.test", "UNIT_MODEL_KEY", "gateway-one", false, false); err != nil {
		t.Fatal(err)
	}
	provider, err := os.ReadFile(filepath.Join(store.ProfilesDir(), "agentbox-gateway-bearer-models-example-test-provider.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(provider), sentinel) || !strings.Contains(string(provider), "UNIT_MODEL_KEY") {
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

func TestProviderSetupAllowsCurrentAgentBinaryLayouts(t *testing.T) {
	t.Setenv("UNIT_MODEL_KEY", "unit-secret")
	runner := &fakeRunner{}
	store := Store{Base: t.TempDir()}
	app := App{Store: store, Runner: runner}
	if err := app.ProviderSetup(context.Background(), "gateway-bearer", "unit-provider", "models.example.test", "UNIT_MODEL_KEY", "gateway-one", false, false); err != nil {
		t.Fatal(err)
	}
	provider, err := os.ReadFile(filepath.Join(store.ProfilesDir(), "agentbox-gateway-bearer-models-example-test-provider.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// The agent images install npm packages whose bin entries moved over time:
	// @opencode/cli ships a self-contained bun executable at
	// @opencode/cli/bin/opencode.exe and @openai/codex ships a node shim at
	// @openai/codex/bin/codex.js. The middleware only lets allowlisted
	// binaries connect to enforced endpoints, so the generated profile must
	// cover the whole package trees rather than stale bin paths.
	for _, want := range []string{
		"/usr/local/lib/node_modules/@openai/codex/**\n",
		"/usr/local/lib/node_modules/@opencode/**\n",
		"/usr/local/lib/node_modules/@anthropic-ai/claude-code/**\n",
	} {
		if !strings.Contains(string(provider), want) {
			t.Fatalf("provider profile missing binary allowlist entry %q:\n%s", want, provider)
		}
	}
	for _, stale := range []string{
		"@openai/codex/**/bin/codex",
		"@opencode/**/bin/opencode",
	} {
		if strings.Contains(string(provider), stale) {
			t.Fatalf("provider profile still pins stale binary path %q:\n%s", stale, provider)
		}
	}
}

func TestSessionScriptOffersIdleReattach(t *testing.T) {
	for _, want := range []string{
		"Keep this run idle for reattach?",
		"touch /workspace/idle",
		"Exiting this shell keeps the run idle",
		"The run remains available for reattach",
		"Detach: Ctrl-b d",
		"touch /workspace/finished",
	} {
		if !strings.Contains(sessionScript, want) {
			t.Fatalf("session script missing %q:\n%s", want, sessionScript)
		}
	}
}

func TestProviderSetupUpdatesExistingProfileAfterLintConflict(t *testing.T) {
	t.Setenv("UNIT_MODEL_KEY", "unit-secret")
	store := Store{Base: t.TempDir()}
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		switch {
		case call.name == "openshell" && len(call.args) > 0 && call.args[0] == "profile" && call.args[1] == "lint":
			return nil, errors.New("Provider profile diagnostics: custom provider profile already exists")
		case call.name == "openshell" && len(call.args) > 0 && call.args[0] == "profile" && call.args[1] == "export":
			return []byte("id: agentbox-gateway-bearer-models-example-test\nresource_version: 42\n"), nil
		default:
			return nil, nil
		}
	}}
	app := App{Store: store, Runner: runner}
	if err := app.ProviderSetup(context.Background(), "gateway-bearer", "unit-provider", "models.example.test", "UNIT_MODEL_KEY", "gateway-one", false, false); err != nil {
		t.Fatal(err)
	}
	provider, err := os.ReadFile(filepath.Join(store.ProfilesDir(), "agentbox-gateway-bearer-models-example-test-provider.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(provider), "resource_version: 42") {
		t.Fatalf("existing provider resource version was not preserved: %s", provider)
	}
	var sawExport, sawUpdate bool
	for _, call := range runner.outputs {
		if commandHasArg(call, "export") {
			sawExport = true
		}
		if commandHasArg(call, "update") {
			sawUpdate = true
		}
	}
	if !sawExport || !sawUpdate {
		t.Fatalf("provider conflict did not use update flow: %#v", runner.outputs)
	}
}

func TestProviderSetupAcceptsModelBaseURLAsGatewayHost(t *testing.T) {
	t.Setenv("UNIT_MODEL_KEY", "unit-secret")
	store := Store{Base: t.TempDir()}
	app := App{Store: store, Runner: &fakeRunner{}}
	if err := app.ProviderSetup(context.Background(), "gateway-header", "unit-provider", "https://api.enmaas.devshift.net/v1/", "UNIT_MODEL_KEY", "gateway-one", false, false); err != nil {
		t.Fatal(err)
	}
	provider, err := os.ReadFile(filepath.Join(store.ProfilesDir(), "agentbox-gateway-header-api-enmaas-devshift-net-provider.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(provider), "host: api.enmaas.devshift.net") || strings.Contains(string(provider), "https://api.enmaas.devshift.net") {
		t.Fatalf("gateway host was not normalized: %s", provider)
	}
}
