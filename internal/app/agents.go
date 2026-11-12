package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const sessionScript = `#!/bin/sh
set -u
cd /workspace/project
/bin/sh /workspace/state/agent.sh
status=$?
while :; do
  echo
  echo "Agent session ended."
  printf "Keep this run idle for reattach? [y]es/[f]inish: "
  if ! IFS= read -r answer; then
    answer=f
  fi
  case "$answer" in
    y|Y|yes|YES)
      touch /workspace/idle
      echo
      echo "Run kept idle."
      echo "Detach: Ctrl-b d. Exiting this shell keeps the run idle."
      while :; do
        /bin/sh
        echo
        echo "Idle shell ended. The run remains available for reattach."
        printf "Keep this run idle for reattach? [y]es/[f]inish: "
        if ! IFS= read -r answer; then
          answer=f
        fi
        case "$answer" in
          y|Y|yes|YES) continue ;;
          f|F|finish|FINISH|n|N|no|NO) break ;;
          *) echo "Enter y to keep the run idle or f to finish." ;;
        esac
      done
      rm -f /workspace/idle
      ;;
    f|F|finish|FINISH|n|N|no|NO)
      break
      ;;
    *)
      echo "Enter y to keep the run idle or f to finish."
      ;;
  esac
done
touch /workspace/finished
exit "$status"
`

func (a *App) PrepareAgent(ctx context.Context, r Run, p Profile, prompt string, mcp *MCPSelection) (string, error) {
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
		if p.Mode == "gateway" {
			body := fmt.Sprintf("[model_providers.gateway]\nname = \"Model gateway\"\nbase_url = %q\nenv_key = %q\nwire_api = \"responses\"\nrequires_openai_auth = false\n", p.BaseURL, p.APIKeyEnv)
			if err := os.WriteFile(filepath.Join(config, "config.toml"), []byte(body), 0600); err != nil {
				return "", err
			}
			profile := fmt.Sprintf("model = %q\nmodel_provider = \"gateway\"\nweb_search = \"disabled\"\n", p.Model)
			if err := os.WriteFile(filepath.Join(config, "gateway.config.toml"), []byte(profile), 0600); err != nil {
				return "", err
			}
		}
		if err := writeCodexMCP(config, mcp); err != nil {
			return "", err
		}
	case "opencode":
		xdg := filepath.Join(dir, "config", "opencode")
		if err := os.MkdirAll(xdg, 0700); err != nil {
			return "", err
		}
		settings := map[string]any{}
		if p.Mode == "gateway" {
			model := p.Model
			settings = map[string]any{"$schema": "https://opencode.ai/config.json", "model": "gateway/" + model, "update": "disable", "providers": map[string]any{"gateway": map[string]any{"name": "Model gateway", "env": []string{p.APIKeyEnv}, "package": "@opencode/ai/providers/openai-compatible", "settings": map[string]any{"baseURL": p.BaseURL}, "models": map[string]any{model: map[string]any{"name": model}}}}}
		}
		if mcp != nil && len(mcp.Servers) > 0 {
			settings["mcp"] = map[string]any{"servers": mcp.Servers}
		}
		if len(settings) > 0 {
			b, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(filepath.Join(xdg, "opencode.json"), append(b, '\n'), 0600); err != nil {
				return "", err
			}
		}
	case "claude":
		claude := filepath.Join(home, ".claude")
		if err := os.MkdirAll(claude, 0700); err != nil {
			return "", err
		}
		// enableAllProjectMcpServers makes committed .mcp.json servers load
		// without an interactive approval prompt: a cloned repository cannot
		// approve its own servers, and only user-scope settings apply in
		// untrusted folders.
		settings := map[string]any{"enableAllProjectMcpServers": true}
		if p.Mode == "gateway" {
			settings["env"] = map[string]string{"ANTHROPIC_BASE_URL": gatewaySiteURL(p.BaseURL), "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY": "1"}
		}
		if mcp != nil && len(mcp.Servers) > 0 {
			// User-scope servers: project-local definitions keep precedence
			// on name conflicts ("local wins").
			if err := writeJSONFile(filepath.Join(claude, ".claude.json"), map[string]any{"mcpServers": mcp.Servers}); err != nil {
				return "", err
			}
		}
		if err := writeJSONFile(filepath.Join(claude, "settings.json"), settings); err != nil {
			return "", err
		}
	}
	script, err := agentScript(p, prompt)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.sh"), []byte(script), 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "session.sh"), []byte(sessionScript), 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func agentScript(p Profile, prompt string) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	args := []string{agentCommand(p.Agent)}
	if p.Agent == "codex" {
		// The AgentBox image intentionally omits `ps`; run Codex without its
		// background app-server daemon in the ephemeral OpenShell sandbox.
		args = append(args, "--no-daemon")
	}
	if p.Mode == "gateway" {
		switch p.Agent {
		case "codex":
			args = append(args, "--profile", "gateway")
		case "claude":
			args = append(args, "--model", p.Model)
		case "opencode":
			// OpenCode reads the gateway model from its generated config.
			args = append(args, "--standalone")
		}
	} else {
		if p.Agent == "codex" {
			// AgentBox already applies the OpenShell sandbox. Disable Codex's
			// nested sandbox so its network calls can use the provider policy.
			args = append(args, "--sandbox", "danger-full-access")
		}
		if p.Model != "" {
			args = append(args, "--model", p.Model)
		}
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
				args = append([]string{"codex", "--no-daemon", "exec"}, args[2:]...)
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
	b.WriteString("export LANG=" + sandboxLocale + "\nexport LC_ALL=" + sandboxLocale + "\n")
	if p.Mode == "standard" && p.Agent == "codex" && p.APIKeyEnv == "OPENAI_API_KEY" {
		b.WriteString(`if [ -n "\${OPENAI_API_KEY:-}" ]; then printf '%s' "$OPENAI_API_KEY" | codex login --with-api-key >/dev/null; fi
`)
	}
	if p.Mode == "gateway" {
		b.WriteString("unset CLAUDE_CODE_USE_VERTEX ANTHROPIC_VERTEX_PROJECT_ID ANTHROPIC_VERTEX_REGION CLOUD_ML_REGION\n")
		if p.Agent == "claude" {
			b.WriteString("export ANTHROPIC_BASE_URL=" + shellQuote(gatewaySiteURL(p.BaseURL)) + "\n")
			if p.APIKeyEnv != "ANTHROPIC_API_KEY" {
				b.WriteString("export ANTHROPIC_API_KEY=\"${" + p.APIKeyEnv + "}\"\n")
			}
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

func gatewaySiteURL(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	return strings.TrimSuffix(trimmed, "/v1")
}

func gatewayProfileSlug(host string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(host) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func normalizeGatewayHost(value string) (string, error) {
	host := strings.TrimSpace(value)
	if host == "" {
		return "", fmt.Errorf("gateway provider type requires a host")
	}
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	parsed, err := url.Parse(host)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("invalid gateway host")
	}
	return parsed.Hostname(), nil
}

func (a *App) ProviderSetup(ctx context.Context, kind, name, host, credential, gateway string, dry, update bool) error {
	var builtin BuiltinProvider
	var isBuiltin bool
	switch kind {
	case "gateway-bearer", "gateway-header":
	case "openai", "anthropic":
		builtin, isBuiltin = BuiltinProviderByKind(kind)
	default:
		return fmt.Errorf("provider type must be gateway-bearer, gateway-header, openai, or anthropic")
	}
	if !safeName.MatchString(name) {
		return fmt.Errorf("invalid provider name")
	}
	if isBuiltin {
		if name != builtin.Name {
			return fmt.Errorf("provider type %s is built in as %q; custom provider names are not supported", kind, builtin.Name)
		}
		host, credential = builtin.Host, builtin.Credential
	} else {
		normalizedHost, err := normalizeGatewayHost(host)
		if err != nil {
			return err
		}
		host = normalizedHost
		if !envName.MatchString(credential) {
			return fmt.Errorf("gateway provider type requires an API key environment variable name")
		}
	}
	if os.Getenv(credential) == "" {
		return fmt.Errorf("%s environment reference is missing", credential)
	}
	if err := a.Store.Ensure(); err != nil {
		return err
	}
	var authStyle, header, envNameValue string
	switch kind {
	case "gateway-bearer":
		authStyle, header, envNameValue = "bearer", "authorization", credential
	case "gateway-header":
		authStyle, header, envNameValue = "header", "x-api-key", credential
	case "openai":
		authStyle, header, envNameValue = "bearer", "authorization", "OPENAI_API_KEY"
	case "anthropic":
		authStyle, header, envNameValue = "header", "x-api-key", "ANTHROPIC_API_KEY"
	}
	profileID := "agentbox-" + kind
	if kind == "gateway-bearer" || kind == "gateway-header" {
		profileID += "-" + gatewayProfileSlug(host)
	}
	displayName := "AgentBox " + kind
	if strings.HasPrefix(kind, "gateway-") {
		displayName += " (" + host + ")"
	}
	profile := fmt.Sprintf("id: %s\ndisplay_name: %s\ncategory: inference\ninference_capable: true\ncredentials:\n  - name: api_key\n    env_vars: [%s]\n    required: true\n    auth_style: %s\n    header_name: %s\nendpoints:\n  - host: %s\n    port: 443\n    path: /v1/**\n    protocol: rest\n    access: full\n    enforcement: enforce\nbinaries:\n  - /usr/local/bin/claude\n  - /usr/local/bin/codex\n  - /usr/local/bin/opencode\n  - /usr/local/bin/node\n  - /usr/local/lib/node_modules/@openai/codex/**\n  - /usr/local/lib/node_modules/@opencode/**\n  - /usr/local/lib/node_modules/@anthropic-ai/claude-code/**\n", profileID, displayName, envNameValue, authStyle, header, host)
	path := filepath.Join(a.Store.ProfilesDir(), profileID+"-provider.yaml")
	if dry {
		fmt.Print(profile)
		return nil
	}
	gatewayArgs := []string{}
	if gateway != "" {
		gatewayArgs = []string{"--gateway", gateway}
	}

	updateExistingProfile := func() error {
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
		return atomicWrite(path, []byte(profile))
	}

	if update {
		if err := updateExistingProfile(); err != nil {
			return err
		}
		if _, err := a.Runner.Output(ctx, "openshell", append([]string{"profile", "update", profileID, "--file", path}, gatewayArgs...)...); err != nil {
			return err
		}
		if _, err := a.Runner.Output(ctx, "openshell", append([]string{"provider", "get", name}, gatewayArgs...)...); err == nil {
			return nil
		}
	} else {
		if err := atomicWrite(path, []byte(profile)); err != nil {
			return err
		}
		lintArgs := append([]string{"profile", "lint", "--file", path}, gatewayArgs...)
		if _, err := a.Runner.Output(ctx, "openshell", lintArgs...); err != nil {
			// Retrying provider setup is common. If OpenShell reports that the
			// generated profile already exists, preserve its resource_version
			// and update it instead of failing the whole setup.
			if !strings.Contains(strings.ToLower(err.Error()), "already exists") {
				return err
			}
			if err := updateExistingProfile(); err != nil {
				return err
			}
			updateArgs := append([]string{"profile", "update", profileID, "--file", path}, gatewayArgs...)
			if _, err := a.Runner.Output(ctx, "openshell", updateArgs...); err != nil {
				return err
			}
			if _, err := a.Runner.Output(ctx, "openshell", append([]string{"provider", "get", name}, gatewayArgs...)...); err == nil {
				return nil
			}
		} else {
			importArgs := append([]string{"profile", "import", "--file", path}, gatewayArgs...)
			if _, err := a.Runner.Output(ctx, "openshell", importArgs...); err != nil {
				return err
			}
		}
	}

	// OpenShell reads the credential from its environment; the value never appears in argv.
	args := []string{"provider", "create", "--name", name, "--type", profileID, "--credential", credential}
	args = append(args, gatewayArgs...)
	if _, err := a.Runner.Output(ctx, "openshell", args...); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			if _, getErr := a.Runner.Output(ctx, "openshell", append([]string{"provider", "get", name}, gatewayArgs...)...); getErr == nil {
				return nil
			}
		}
		return err
	}
	return nil
}

func (a *App) ProviderUpdateCredential(ctx context.Context, name, credential, gateway string) error {
	if definition, ok := BuiltinProviderByName(name); ok {
		return fmt.Errorf("provider %q is built in; its credential environment is fixed to %s", name, definition.Credential)
	}
	if !safeName.MatchString(name) {
		return fmt.Errorf("invalid provider name")
	}
	if !envName.MatchString(credential) {
		return fmt.Errorf("provider requires an API key environment variable name")
	}
	args := []string{"provider", "update", name, "--credential", credential}
	if gateway != "" {
		args = append(args, "--gateway", gateway)
	}
	_, err := a.Runner.Output(ctx, "openshell", args...)
	return err
}

func (a *App) ProviderDelete(ctx context.Context, name, gateway string) error {
	if _, ok := BuiltinProviderByName(name); ok {
		return fmt.Errorf("provider %q is built in and cannot be deleted", name)
	}
	if !safeName.MatchString(name) {
		return fmt.Errorf("invalid provider name")
	}
	profiles, err := a.Store.ListProfiles()
	if err != nil {
		return err
	}
	usedBy := make([]string, 0)
	for _, profile := range profiles {
		if profile.Provider == name {
			usedBy = append(usedBy, profile.Name)
		}
	}
	if len(usedBy) > 0 {
		sort.Strings(usedBy)
		return fmt.Errorf("provider %q is used by profiles: %s", name, strings.Join(usedBy, ", "))
	}
	args := []string{"provider", "delete", name}
	if gateway != "" {
		args = append(args, "--gateway", gateway)
	}
	_, err = a.Runner.Output(ctx, "openshell", args...)
	return err
}
