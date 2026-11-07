package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const enmaasHost = "api.enmaas.devshift.net"
const enmaasURL = "https://api.enmaas.devshift.net"

func (a *App) PrepareAgent(ctx context.Context, r Run, p Profile, prompt string) (string, error) {
	dir := filepath.Join(a.Store.RunsDir(), r.ID+"-config")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0700); err != nil {
		return "", err
	}
	switch p.Agent {
	case "codex":
		config := filepath.Join(home, ".codex")
		if err := os.MkdirAll(config, 0700); err != nil {
			return "", err
		}
		if p.Mode == "enmaas" {
			body := fmt.Sprintf("[model_providers.enmaas]\nname = \"EnMaaS\"\nbase_url = \"%s/v1\"\nenv_key = \"ENMAAS_KEY\"\nwire_api = \"responses\"\nrequires_openai_auth = false\n", enmaasURL)
			if err := os.WriteFile(filepath.Join(config, "config.toml"), []byte(body), 0600); err != nil {
				return "", err
			}
			profile := fmt.Sprintf("model = %q\nmodel_provider = \"enmaas\"\nweb_search = \"disabled\"\n", p.Model)
			if err := os.WriteFile(filepath.Join(config, "enmaas.config.toml"), []byte(profile), 0600); err != nil {
				return "", err
			}
		}
	case "opencode":
		xdg := filepath.Join(dir, "config", "opencode")
		if err := os.MkdirAll(xdg, 0700); err != nil {
			return "", err
		}
		if p.Mode == "enmaas" {
			model := p.Model
			settings := map[string]any{"$schema": "https://opencode.ai/config.json", "model": "enmaas/" + model, "update": "disable", "providers": map[string]any{"enmaas": map[string]any{"name": "EnMaaS", "env": []string{"ENMAAS_KEY"}, "package": "@opencode/ai/providers/openai-compatible", "settings": map[string]any{"baseURL": enmaasURL + "/v1"}, "models": map[string]any{model: map[string]any{"name": model}}}}}
			b, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(filepath.Join(xdg, "opencode.json"), append(b, '\n'), 0600); err != nil {
				return "", err
			}
		}
	case "claude":
		if p.Mode == "enmaas" {
			claude := filepath.Join(home, ".claude")
			if err := os.MkdirAll(claude, 0700); err != nil {
				return "", err
			}
			settings := map[string]any{"env": map[string]string{"ANTHROPIC_BASE_URL": enmaasURL, "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY": "1"}}
			b, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(filepath.Join(claude, "settings.json"), append(b, '\n'), 0600); err != nil {
				return "", err
			}
		}
	}
	script, err := agentScript(p, prompt)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.sh"), []byte(script), 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func agentScript(p Profile, prompt string) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	args := []string{agentCommand(p.Agent)}
	if p.Mode == "enmaas" {
		switch p.Agent {
		case "codex":
			args = append(args, "--profile", "enmaas")
		case "claude":
			args = append(args, "--model", p.Model)
		case "opencode":
			args = append(args, "--standalone", "--model", "enmaas/"+p.Model)
		}
	} else if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if p.Approval == "unattended" {
		switch p.Agent {
		case "codex":
			args = append(args, "--dangerously-bypass-approvals-and-sandbox")
		case "claude":
			args = append(args, "--dangerously-skip-permissions")
		case "opencode":
			args = append(args, "--auto")
		}
	}
	for _, arg := range p.Args {
		for _, bad := range []string{"--yolo", "--dangerously-skip-permissions", "--dangerously-bypass-approvals-and-sandbox", "--auto"} {
			if p.Approval == "prompted" && arg == bad {
				return "", fmt.Errorf("approval-bypass flag %s conflicts with prompted profile", bad)
			}
		}
		args = append(args, arg)
	}
	if prompt != "" {
		if p.Approval == "unattended" {
			switch p.Agent {
			case "codex":
				args = append([]string{"codex", "exec"}, args[1:]...)
				args = append(args, prompt)
			case "claude":
				args = append(args, "-p", prompt)
			case "opencode":
				args = append([]string{"opencode", "run"}, args[1:]...)
				args = append(args, prompt)
			}
		} else {
			switch p.Agent {
			case "codex", "claude":
				args = append(args, prompt)
			case "opencode":
				args = append([]string{"opencode", "mini"}, args[1:]...)
				args = append(args, "--prompt", prompt)
			}
		}
	}
	var b strings.Builder
	b.WriteString("#!/bin/sh\nset -eu\ncd /workspace/project\nexport HOME=/workspace/state/home\nexport XDG_CONFIG_HOME=/workspace/state/config\nexport XDG_DATA_HOME=/workspace/state/data\nexport CODEX_HOME=/workspace/state/home/.codex\nexport CLAUDE_CONFIG_DIR=/workspace/state/home/.claude\n")
	if p.Mode == "enmaas" {
		b.WriteString("unset CLAUDE_CODE_USE_VERTEX ANTHROPIC_VERTEX_PROJECT_ID ANTHROPIC_VERTEX_REGION CLOUD_ML_REGION\n")
		if p.Agent == "claude" {
			b.WriteString("export ANTHROPIC_BASE_URL=" + enmaasURL + "\n")
		}
	}
	b.WriteString("exec")
	for _, arg := range args {
		b.WriteByte(' ')
		b.WriteString(shellQuote(arg))
	}
	b.WriteByte('\n')
	return b.String(), nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func (a *App) ProviderSetup(ctx context.Context, kind, name, gateway string, dry, update bool) error {
	if kind != "enmaas-header" && kind != "enmaas-bearer" && kind != "openai" && kind != "anthropic" {
		return fmt.Errorf("provider type must be enmaas-header, enmaas-bearer, openai, or anthropic")
	}
	if !safeName.MatchString(name) {
		return fmt.Errorf("invalid provider name")
	}
	credential := "ENMAAS_KEY"
	if kind == "openai" {
		credential = "OPENAI_API_KEY"
	}
	if kind == "anthropic" {
		credential = "ANTHROPIC_API_KEY"
	}
	if os.Getenv(credential) == "" {
		return fmt.Errorf("%s environment reference is missing", credential)
	}
	if err := a.Store.Ensure(); err != nil {
		return err
	}
	var authStyle, header, envName, host string
	switch kind {
	case "enmaas-header":
		authStyle, header, envName, host = "header", "x-api-key", "ENMAAS_KEY, ANTHROPIC_API_KEY", enmaasHost
	case "enmaas-bearer":
		authStyle, header, envName, host = "bearer", "authorization", "ENMAAS_KEY", enmaasHost
	case "openai":
		authStyle, header, envName, host = "bearer", "authorization", "OPENAI_API_KEY", "api.openai.com"
	case "anthropic":
		authStyle, header, envName, host = "header", "x-api-key", "ANTHROPIC_API_KEY", "api.anthropic.com"
	}
	profileID := "agentbox-" + kind
	profile := fmt.Sprintf("id: %s\ndisplay_name: AgentBox %s\ncategory: inference\ninference_capable: true\ncredentials:\n  - name: api_key\n    env_vars: [%s]\n    required: true\n    auth_style: %s\n    header_name: %s\nendpoints:\n  - host: %s\n    port: 443\n    path: /v1/**\n    protocol: rest\n    access: full\n    enforcement: enforce\nbinaries:\n  - /usr/local/bin/claude\n  - /usr/local/bin/codex\n  - /usr/local/bin/opencode\n  - /usr/local/bin/node\n  - /usr/local/lib/node_modules/@openai/codex/**/bin/codex\n  - /usr/local/lib/node_modules/@opencode/**/bin/opencode\n  - /usr/local/lib/node_modules/@anthropic-ai/claude-code/**\n", profileID, kind, envName, authStyle, header, host)
	path := filepath.Join(a.Store.ProfilesDir(), profileID+"-provider.yaml")
	if dry {
		fmt.Print(profile)
		return nil
	}
	gatewayArgs := []string{}
	if gateway != "" {
		gatewayArgs = []string{"--gateway", gateway}
	}
	if update {
		exportArgs := append([]string{"profile", "export", profileID, "-o", "yaml"}, gatewayArgs...)
		current, err := a.Runner.Output(ctx, "openshell", exportArgs...)
		if err != nil {
			return err
		}
		var oldProfile, newProfile map[string]any
		if err := yaml.Unmarshal(current, &oldProfile); err != nil {
			return err
		}
		if err := yaml.Unmarshal([]byte(profile), &newProfile); err != nil {
			return err
		}
		version, ok := oldProfile["resource_version"]
		if !ok {
			return fmt.Errorf("existing provider profile has no resource version")
		}
		newProfile["resource_version"] = version
		b, err := yaml.Marshal(newProfile)
		if err != nil {
			return err
		}
		profile = string(b)
	}
	if err := atomicWrite(path, []byte(profile)); err != nil {
		return err
	}
	if update {
		if _, err := a.Runner.Output(ctx, "openshell", append([]string{"profile", "update", profileID, "--file", path}, gatewayArgs...)...); err != nil {
			return err
		}
		if _, err := a.Runner.Output(ctx, "openshell", append([]string{"provider", "get", name}, gatewayArgs...)...); err == nil {
			return nil
		}
	} else {
		if _, err := a.Runner.Output(ctx, "openshell", append([]string{"profile", "lint", "--file", path}, gatewayArgs...)...); err != nil {
			return err
		}
		if _, err := a.Runner.Output(ctx, "openshell", append([]string{"profile", "import", "--file", path}, gatewayArgs...)...); err != nil {
			return err
		}
	}
	// OpenShell reads the credential from its environment; the value never appears in argv.
	args := []string{"provider", "create", "--name", name, "--type", profileID, "--credential", credential}
	args = append(args, gatewayArgs...)
	_, err := a.Runner.Output(ctx, "openshell", args...)
	return err
}
