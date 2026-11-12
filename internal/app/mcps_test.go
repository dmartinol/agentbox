package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestMCPToolsValidation(t *testing.T) {
	valid := MCPTools{Version: 1, Name: "unit-mcp", Agent: "claude", Servers: []string{"github", "slack"}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid entry rejected: %v", err)
	}
	for _, tc := range []struct {
		name  string
		entry MCPTools
		want  string
	}{
		{"bad name", MCPTools{Version: 1, Name: "Upper", Agent: "claude", Servers: []string{"github"}}, "lower-case name"},
		{"bad version", MCPTools{Version: 2, Name: "unit", Agent: "claude", Servers: []string{"github"}}, "version 1"},
		{"bad agent", MCPTools{Version: 1, Name: "unit", Agent: "gemin", Servers: []string{"github"}}, "agent must be"},
		{"no servers", MCPTools{Version: 1, Name: "unit", Agent: "claude"}, "at least one server"},
		{"duplicate", MCPTools{Version: 1, Name: "unit", Agent: "claude", Servers: []string{"github", "github"}}, "listed twice"},
		{"bad server name", MCPTools{Version: 1, Name: "unit", Agent: "claude", Servers: []string{"bad name!"}}, "letters, digits"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.entry.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
		})
	}
}

func TestMCPStoreRoundTrip(t *testing.T) {
	store := Store{Base: t.TempDir()}
	entry := MCPTools{Version: 1, Name: "unit-mcp", Agent: "codex", Servers: []string{"alpha", "beta"}}
	if err := store.SaveMCP(entry); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadMCP("unit-mcp")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, entry) {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}
	listed, err := store.ListMCPs()
	if err != nil || len(listed) != 1 || listed[0].Name != "unit-mcp" {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	if _, err := store.LoadMCP("missing"); !os.IsNotExist(err) {
		t.Fatalf("missing entry error = %v", err)
	}
	if err := store.DeleteMCP("unit-mcp"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteMCP("unit-mcp"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("double delete error = %v", err)
	}
	empty, err := store.ListMCPs()
	if err != nil || len(empty) != 0 {
		t.Fatalf("list after delete = %+v, %v", empty, err)
	}
}

func writeHostFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestGlobalMCPServersPerAgent(t *testing.T) {
	t.Run("claude", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", dir)
		writeHostFile(t, filepath.Join(dir, ".claude.json"), `{"mcpServers":{"alpha":{"command":"/usr/bin/true"},"beta":{"url":"https://beta.example.test/mcp","type":"http"}},"projects":{"/ignored":{"mcpServers":{"local-only":{"command":"no"}}}}}`)
		names, err := (&App{}).GlobalMCPServers("claude")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(names, []string{"alpha", "beta"}) {
			t.Fatalf("names = %v", names)
		}
	})
	t.Run("claude default home", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("CLAUDE_CONFIG_DIR", "")
		writeHostFile(t, filepath.Join(home, ".claude", ".claude.json"), `{"mcpServers":{"alpha":{"command":"/usr/bin/true"}}}`)
		names, err := (&App{}).GlobalMCPServers("claude")
		if err != nil || !reflect.DeepEqual(names, []string{"alpha"}) {
			t.Fatalf("names = %v, %v", names, err)
		}
	})
	t.Run("codex", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CODEX_HOME", dir)
		writeHostFile(t, filepath.Join(dir, "config.toml"), "model = \"gpt-5\"\n\n[mcp_servers.alpha]\ncommand = \"/usr/bin/true\"\nargs = [\"--flag\"]\n\n[mcp_servers.alpha.env]\nTOKEN = \"${UNIT_TOKEN}\"\n")
		names, err := (&App{}).GlobalMCPServers("codex")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(names, []string{"alpha"}) {
			t.Fatalf("names = %v", names)
		}
	})
	t.Run("opencode json", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		writeHostFile(t, filepath.Join(dir, "opencode", "opencode.json"), `{"model":"x","mcp":{"servers":{"alpha":{"type":"local","command":["/usr/bin/true"]}}}}`)
		names, err := (&App{}).GlobalMCPServers("opencode")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(names, []string{"alpha"}) {
			t.Fatalf("names = %v", names)
		}
	})
	t.Run("opencode jsonc fallback", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		writeHostFile(t, filepath.Join(dir, "opencode", "opencode.jsonc"), "{\n  // team servers\n  \"mcp\": {\"servers\": {\"alpha\": {\"type\": \"local\", \"command\": [\"/usr/bin/true\"]},},},\n}\n")
		names, err := (&App{}).GlobalMCPServers("opencode")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(names, []string{"alpha"}) {
			t.Fatalf("names = %v, %v", names, err)
		}
	})
	t.Run("absent source", func(t *testing.T) {
		t.Setenv("CODEX_HOME", t.TempDir())
		names, err := (&App{}).GlobalMCPServers("codex")
		if err != nil || len(names) != 0 {
			t.Fatalf("names = %v, %v", names, err)
		}
	})
	t.Run("invalid agent", func(t *testing.T) {
		if _, err := (&App{}).GlobalMCPServers("other"); err == nil || !strings.Contains(err.Error(), "agent must be") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestResolveMCPSelection(t *testing.T) {
	store := Store{Base: t.TempDir()}
	app := App{Store: store}

	t.Run("missing entry", func(t *testing.T) {
		_, err := app.ResolveMCPSelection("missing", "claude")
		if err == nil || !strings.Contains(err.Error(), `entry "missing" not found`) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("agent mismatch", func(t *testing.T) {
		if err := store.SaveMCP(MCPTools{Version: 1, Name: "claude-set", Agent: "claude", Servers: []string{"alpha"}}); err != nil {
			t.Fatal(err)
		}
		_, err := app.ResolveMCPSelection("claude-set", "codex")
		if err == nil || !strings.Contains(err.Error(), `selects agent "claude"`) || !strings.Contains(err.Error(), `"codex"`) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("drift warns and delivers the rest verbatim", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", dir)
		writeHostFile(t, filepath.Join(dir, ".claude.json"), `{"mcpServers":{"alpha":{"command":"/usr/bin/true","env":{"TOKEN":"${UNIT_TOKEN}"}}}}`)
		if err := store.SaveMCP(MCPTools{Version: 1, Name: "claude-work", Agent: "claude", Servers: []string{"alpha", "gone"}}); err != nil {
			t.Fatal(err)
		}
		selection, err := app.ResolveMCPSelection("claude-work", "claude")
		if err != nil {
			t.Fatal(err)
		}
		if len(selection.Servers) != 1 {
			t.Fatalf("servers = %+v", selection.Servers)
		}
		alpha, ok := selection.Servers["alpha"].(map[string]any)
		if !ok || alpha["command"] != "/usr/bin/true" {
			t.Fatalf("alpha not verbatim: %#v", selection.Servers["alpha"])
		}
		warnings := selection.Warnings()
		if len(warnings) != 1 || !strings.Contains(warnings[0], `"gone"`) || !strings.Contains(warnings[0], "starting without it") {
			t.Fatalf("warnings = %v", warnings)
		}
	})
	t.Run("inline secret rejected", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", dir)
		writeHostFile(t, filepath.Join(dir, ".claude.json"), `{"mcpServers":{"alpha":{"command":"/usr/bin/true","env":{"API_TOKEN":"ghp_16C7e42F292c6912E7710c838347Ae178B4a"}}}}`)
		if err := store.SaveMCP(MCPTools{Version: 1, Name: "secret-set", Agent: "claude", Servers: []string{"alpha"}}); err != nil {
			t.Fatal(err)
		}
		_, err := app.ResolveMCPSelection("secret-set", "claude")
		if err == nil || !strings.Contains(err.Error(), `"alpha"`) || !strings.Contains(err.Error(), "env.API_TOKEN") || !strings.Contains(err.Error(), "${NAME}") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("environment references accepted", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", dir)
		writeHostFile(t, filepath.Join(dir, ".claude.json"), `{"mcpServers":{"alpha":{"command":"/usr/bin/true","env":{"API_TOKEN":"${UNIT_TOKEN}","HEADER":"Bearer ${UNIT_TOKEN}","BARE":"UNIT_TOKEN","URL":"https://api.example.test/v1"}}}}`)
		if err := store.SaveMCP(MCPTools{Version: 1, Name: "ref-set", Agent: "claude", Servers: []string{"alpha"}}); err != nil {
			t.Fatal(err)
		}
		if _, err := app.ResolveMCPSelection("ref-set", "claude"); err != nil {
			t.Fatalf("references rejected: %v", err)
		}
	})
}

func TestMCPSecretValueHeuristic(t *testing.T) {
	for _, tc := range []struct {
		value  string
		secret bool
	}{
		{"${GITHUB_TOKEN}", false},
		{"UNIT_TOKEN", false},
		{"Bearer ${TOKEN}", false},
		{"{env:UNIT_TOKEN}", false},
		{"https://api.github.com/v1", false},
		{"/usr/local/bin/node", false},
		{"--flag-with-a-long-name-123", false},
		{"ghp_16C7e42F292c6912E7710c838347Ae178B4a", true},
		{"sk-ant-api03-AAAABBBBCCCCDDDDEEEEFFFF", true},
		{"Authorization: token abcdef1234567890abcdef", true},
	} {
		if got := mcpSecretValue(tc.value); got != tc.secret {
			t.Errorf("mcpSecretValue(%q) = %v; want %v", tc.value, got, tc.secret)
		}
	}
}

func TestJSONCStrip(t *testing.T) {
	body := `{
  // line comment
  "mcp": {"servers": {"alpha": {"command": ["echo", "//not-a-comment"]},},},
  /* block
     comment */
}`
	stripped := jsoncStrip([]byte(body))
	var doc map[string]any
	if err := json.Unmarshal(stripped, &doc); err != nil {
		t.Fatalf("stripped output is not valid JSON: %v\n%s", err, stripped)
	}
	mcp := doc["mcp"].(map[string]any)
	servers := mcp["servers"].(map[string]any)
	command := servers["alpha"].(map[string]any)["command"].([]any)
	if command[1] != "//not-a-comment" {
		t.Fatalf("string literal corrupted: %v", command)
	}
}

func mcpTestSelection(t *testing.T, agent string) *MCPSelection {
	t.Helper()
	return &MCPSelection{
		Entry:   MCPTools{Version: 1, Name: "unit-mcp", Agent: agent, Servers: []string{"alpha"}},
		Servers: map[string]any{"alpha": map[string]any{"command": "/usr/bin/true", "args": []any{"--flag"}, "env": map[string]any{"TOKEN": "${UNIT_TOKEN}"}}},
	}
}

func TestPrepareAgentDeliversMCPPerAgent(t *testing.T) {
	for _, agent := range []string{"claude", "codex", "opencode"} {
		for _, mode := range []string{"standard", "gateway"} {
			t.Run(agent+"/"+mode, func(t *testing.T) {
				store := Store{Base: t.TempDir()}
				app := App{Store: store}
				run := Run{ID: "run-unit123"}
				profile := testProfile(agent, mode, "prompted")
				dir, err := app.PrepareAgent(context.Background(), run, profile, "", mcpTestSelection(t, agent), nil)
				if err != nil {
					t.Fatal(err)
				}
				switch agent {
				case "claude":
					var settings map[string]any
					body, err := os.ReadFile(filepath.Join(dir, "home", ".claude", "settings.json"))
					if err != nil || json.Unmarshal(body, &settings) != nil {
						t.Fatalf("settings.json unreadable: %v %s", err, body)
					}
					if settings["enableAllProjectMcpServers"] != true {
						t.Fatalf("enableAllProjectMcpServers missing: %s", body)
					}
					if mode == "gateway" && settings["env"] == nil {
						t.Fatalf("gateway env settings lost: %s", body)
					}
					var user map[string]any
					body, err = os.ReadFile(filepath.Join(dir, "home", ".claude", ".claude.json"))
					if err != nil || json.Unmarshal(body, &user) != nil {
						t.Fatalf(".claude.json unreadable: %v %s", err, body)
					}
					servers := user["mcpServers"].(map[string]any)
					alpha, ok := servers["alpha"].(map[string]any)
					if !ok || alpha["command"] != "/usr/bin/true" {
						t.Fatalf("alpha not delivered verbatim: %s", body)
					}
				case "codex":
					body, err := os.ReadFile(filepath.Join(dir, "home", ".codex", "config.toml"))
					if err != nil {
						t.Fatal(err)
					}
					var doc map[string]any
					if err := toml.Unmarshal(body, &doc); err != nil {
						t.Fatalf("config.toml invalid after append: %v\n%s", err, body)
					}
					servers, ok := doc["mcp_servers"].(map[string]any)
					if !ok {
						t.Fatalf("mcp_servers missing: %s", body)
					}
					alpha, ok := servers["alpha"].(map[string]any)
					if !ok || alpha["command"] != "/usr/bin/true" {
						t.Fatalf("alpha not delivered: %s", body)
					}
					if mode == "gateway" && doc["model_providers"] == nil {
						t.Fatalf("gateway provider config lost: %s", body)
					}
				case "opencode":
					body, err := os.ReadFile(filepath.Join(dir, "config", "opencode", "opencode.json"))
					if err != nil {
						t.Fatal(err)
					}
					var doc map[string]any
					if err := json.Unmarshal(body, &doc); err != nil {
						t.Fatalf("opencode.json invalid: %v %s", err, body)
					}
					mcp := doc["mcp"].(map[string]any)
					servers := mcp["servers"].(map[string]any)
					alpha, ok := servers["alpha"].(map[string]any)
					if !ok || alpha["command"] != "/usr/bin/true" {
						t.Fatalf("alpha not delivered: %s", body)
					}
					if mode == "gateway" && doc["providers"] == nil {
						t.Fatalf("gateway provider config lost: %s", body)
					}
				}
			})
		}
	}
}

func TestPrepareAgentClaudeLocalApprovalWithoutSelection(t *testing.T) {
	store := Store{Base: t.TempDir()}
	app := App{Store: store}
	run := Run{ID: "run-unit123"}
	profile := testProfile("claude", "standard", "prompted")
	dir, err := app.PrepareAgent(context.Background(), run, profile, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	body, err := os.ReadFile(filepath.Join(dir, "home", ".claude", "settings.json"))
	if err != nil || json.Unmarshal(body, &settings) != nil {
		t.Fatalf("settings.json unreadable: %v %s", err, body)
	}
	if settings["enableAllProjectMcpServers"] != true {
		t.Fatalf("enableAllProjectMcpServers missing without selection: %s", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "home", ".claude", ".claude.json")); !os.IsNotExist(err) {
		t.Fatalf(".claude.json written without selection: %v", err)
	}
}

func TestPrepareAgentOpenCodeStandardWithoutSelectionKeepsNoConfig(t *testing.T) {
	store := Store{Base: t.TempDir()}
	app := App{Store: store}
	run := Run{ID: "run-unit123"}
	profile := testProfile("opencode", "standard", "prompted")
	dir, err := app.PrepareAgent(context.Background(), run, profile, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "opencode", "opencode.json")); !os.IsNotExist(err) {
		t.Fatalf("opencode.json written without selection: %v", err)
	}
}

func TestStartResolvesMCPSelection(t *testing.T) {
	ctx := context.Background()
	store := Store{Base: t.TempDir()}
	policyConfig, err := store.CreatePolicyConfiguration("unit-policy")
	if err != nil {
		t.Fatal(err)
	}
	policy := policyConfig.Path
	if err := store.SaveProfile(Profile{
		Version: 1, Name: "mycodex", Agent: "codex", Image: "localhost/openshell-codex:1.2.3-r3",
		Mode: "standard", Provider: "unit-provider", Policy: policy, Approval: "prompted",
	}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		switch {
		case call.name == "openshell" && len(call.args) > 0 && call.args[0] == "gateway":
			return []byte(`{"name":"openshell","type":"podman"}`), nil
		case call.name == "openshell" && len(call.args) > 0 && call.args[0] == "provider":
			return []byte(`{}`), nil
		case call.name == "podman":
			return []byte(`[]`), nil
		}
		return nil, nil
	}}
	app := App{Store: store, Runner: runner}

	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	writeHostFile(t, filepath.Join(codexHome, "config.toml"), "[mcp_servers.alpha]\ncommand = \"/usr/bin/true\"\n")
	if err := store.SaveMCP(MCPTools{Version: 1, Name: "unit-mcp", Agent: "codex", Servers: []string{"alpha", "gone"}}); err != nil {
		t.Fatal(err)
	}
	run, err := app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell", MCP: "unit-mcp", Dry: true})
	if err != nil {
		t.Fatal(err)
	}
	if run.MCP != "unit-mcp" {
		t.Fatalf("run.MCP = %q", run.MCP)
	}
	if len(run.MCPWarnings) != 1 || !strings.Contains(run.MCPWarnings[0], `"gone"`) {
		t.Fatalf("warnings = %v", run.MCPWarnings)
	}

	if err := store.SaveMCP(MCPTools{Version: 1, Name: "claude-mcp", Agent: "claude", Servers: []string{"alpha"}}); err != nil {
		t.Fatal(err)
	}
	_, err = app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell", MCP: "claude-mcp", Dry: true})
	if err == nil || !strings.Contains(err.Error(), `selects agent "claude"`) {
		t.Fatalf("mismatch error = %v", err)
	}

	_, err = app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell", MCP: "missing", Dry: true})
	if err == nil || !strings.Contains(err.Error(), `entry "missing" not found`) {
		t.Fatalf("missing entry error = %v", err)
	}

	run, err = app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell", Dry: true})
	if err != nil {
		t.Fatal(err)
	}
	if run.MCP != "" || len(run.MCPWarnings) != 0 {
		t.Fatalf("run without selection changed: %+v", run)
	}
}
