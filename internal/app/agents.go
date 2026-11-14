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

const activityScript = `#!/bin/sh
set -u
state=${1:-}
case "$state" in
  working|waiting|done|idle) ;;
  *) exit 0 ;;
esac
detail=${2:-}
event=${3:-}
detail=$(printf '%s' "$detail" | tr -d '"\\\r\n')
marker=/workspace/state/interrupted-turn

# Harness hooks pipe their JSON payload in. The session wrapper keeps a
# terminal on stdin, where reading would block.
payload=
if [ ! -t 0 ]; then
  payload=$(cat 2>/dev/null || true)
fi
turn=$(printf '%s' "$payload" | sed -n 's/.*"turn_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)

# A question is a tool call that blocks until the user answers, so the turn is
# waiting for input instead of working. Look at the payload before tool_input
# so text inside the call cannot be mistaken for the tool name.
if [ "$event" = PreToolUse ] && [ "$state" = working ]; then
  tool=$(printf '%s' "${payload%%\"tool_input\"*}" | sed -n 's/.*"tool_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
  case "$tool" in
    request_user_input) state=waiting; detail="asking a question" ;;
  esac
fi

# ESC ends a turn, but Codex still delivers results for tools that were in
# flight, so a late progress hook for the interrupted turn must not restore
# Working or Waiting. Only a new prompt, or a new session, moves past it.
if [ -n "$turn" ]; then
  case "$event" in
    PreToolUse|PostToolUse|PermissionRequest|Notification|Elicitation|Stop)
      if [ "$(cat "$marker" 2>/dev/null || true)" = "$turn" ]; then
        exit 0
      fi
      ;;
  esac
fi

mkdir -p /workspace/state 2>/dev/null || true
case "$event" in
  Interrupt) [ -n "$turn" ] && printf '%s' "$turn" > "$marker" 2>/dev/null || true ;;
  UserPromptSubmit|SessionStart|SessionEnd) rm -f "$marker" 2>/dev/null || true ;;
esac
printf '{"state":"%s","detail":"%s"}\n' "$state" "$detail" > /workspace/state/activity.json.tmp
mv /workspace/state/activity.json.tmp /workspace/state/activity.json
`

const sessionScript = `#!/bin/sh
set -u
cd /workspace/project
activity() {
  /bin/sh /workspace/state/activity.sh "$1" "${2:-}" 2>/dev/null || true
}
/bin/sh /workspace/state/agent.sh
status=$?
while :; do
  echo
  echo "Agent session ended."
  activity waiting "agent exited; choose idle or finish"
  printf "Keep this run idle for reattach? [y]es/[f]inish: "
  if ! IFS= read -r answer; then
    answer=f
  fi
  case "$answer" in
    y|Y|yes|YES)
      touch /workspace/idle
      activity idle "kept idle for reattach"
      echo
      echo "Run kept idle."
      echo "Detach: Ctrl-b d. Exiting this shell keeps the run idle."
      while :; do
        /bin/sh
        echo
        echo "Idle shell ended. The run remains available for reattach."
        activity waiting "idle shell ended; choose idle or finish"
        printf "Keep this run idle for reattach? [y]es/[f]inish: "
        if ! IFS= read -r answer; then
          answer=f
        fi
        case "$answer" in
          y|Y|yes|YES) continue ;;
          f|F|finish|FINISH|n|N|no|NO) break ;;
          *) echo "Enter y to keep this run idle or f to finish." ;;
        esac
      done
      rm -f /workspace/idle
      ;;
    f|F|finish|FINISH|n|N|no|NO)
      break
      ;;
    *)
      echo "Enter y to keep this run idle or f to finish."
      ;;
  esac
done
activity done "session finished"
touch /workspace/finished
exit "$status"
`

// activityHook maps a harness event to an explicit activity state.
type activityHook struct {
	Event  string
	State  string
	Detail string
}

var codexActivityHooks = []activityHook{
	{"SessionStart", "idle", "session started"},
	{"UserPromptSubmit", "working", "prompt submitted"},
	{"PreToolUse", "working", "running a tool"},
	{"PostToolUse", "working", "processing tool result"},
	{"PermissionRequest", "waiting", "permission requested"},
	{"Stop", "done", "turn finished"},
	{"SessionEnd", "done", "session ended"},
	{"Interrupt", "idle", "interrupted"},
}

// Claude Code has no Interrupt hook; Stop does not fire on user interrupt, so
// Notification is the closest explicit "needs attention" signal.
var claudeActivityHooks = []activityHook{
	{"SessionStart", "idle", "session started"},
	{"UserPromptSubmit", "working", "prompt submitted"},
	{"PreToolUse", "working", "running a tool"},
	{"PostToolUse", "working", "processing tool result"},
	{"PermissionRequest", "waiting", "permission requested"},
	{"Notification", "waiting", "agent needs attention"},
	{"Elicitation", "waiting", "agent asked for input"},
	{"Stop", "done", "turn finished"},
	{"SessionEnd", "done", "session ended"},
}

func hooksDoc(hooks []activityHook) map[string]any {
	events := map[string]any{}
	for _, hook := range hooks {
		command := "/bin/sh /workspace/state/activity.sh " + hook.State + " " + shellQuote(hook.Detail) + " " + hook.Event
		events[hook.Event] = []any{
			map[string]any{
				"hooks": []any{
					map[string]any{"type": "command", "command": command, "timeout": 10},
				},
			},
		}
	}
	return map[string]any{"hooks": events}
}

func (a *App) PrepareAgent(ctx context.Context, r Run, p Profile, prompt string, mcp *MCPSelection, skills *SkillsSelection) (string, error) {
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
		configBody := "[projects.\"/workspace/project\"]\ntrust_level = \"trusted\"\n"
		if p.Mode == "gateway" {
			configBody += fmt.Sprintf("[model_providers.gateway]\nname = \"Model gateway\"\nbase_url = %q\nenv_key = %q\nwire_api = \"responses\"\nrequires_openai_auth = false\n", p.BaseURL, p.APIKeyEnv)
			profile := fmt.Sprintf("model = %q\nmodel_provider = \"gateway\"\nweb_search = \"disabled\"\n", p.Model)
			if err := os.WriteFile(filepath.Join(config, "gateway.config.toml"), []byte(profile), 0600); err != nil {
				return "", err
			}
		}
		if err := os.WriteFile(filepath.Join(config, "config.toml"), []byte(configBody), 0600); err != nil {
			return "", err
		}
		if err := writeCodexMCP(config, mcp); err != nil {
			return "", err
		}
		if err := writeJSONFile(filepath.Join(config, "hooks.json"), hooksDoc(codexActivityHooks)); err != nil {
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
		settings := map[string]any{"enableAllProjectMcpServers": true, "hooks": hooksDoc(claudeActivityHooks)["hooks"]}
		if p.Mode == "gateway" {
			settings["env"] = map[string]string{"ANTHROPIC_BASE_URL": gatewaySiteURL(p.BaseURL), "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY": "1"}
		}
		state := map[string]any{
			"projects": map[string]any{
				"/workspace/project": map[string]any{
					"hasTrustDialogAccepted":      true,
					"hasTrustDialogHooksAccepted": true,
				},
			},
		}
		if mcp != nil && len(mcp.Servers) > 0 {
			// User-scope servers: project-local definitions keep precedence
			// on name conflicts ("local wins").
			state["mcpServers"] = mcp.Servers
		}
		if err := writeJSONFile(filepath.Join(claude, ".claude.json"), state); err != nil {
			return "", err
		}
		if err := writeJSONFile(filepath.Join(claude, "settings.json"), settings); err != nil {
			return "", err
		}
	}
	if err := deliverSkills(dir, skills); err != nil {
		return "", err
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
	if err := os.WriteFile(filepath.Join(dir, "activity.sh"), []byte(activityScript), 0700); err != nil {
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
		// AgentBox already applies the OpenShell sandbox. Disable Codex's
		// nested sandbox in every profile mode so its network calls follow the
		// run policy instead of Codex's default network-disabled sandbox.
		args = append(args, "--sandbox", "danger-full-access")
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
	b.WriteString("#!/bin/sh\nset -eu\ncd /workspace/project\nexport HOME=/workspace/state/home\nexport XDG_CONFIG_HOME=/workspace/state/config\nexport XDG_DATA_HOME=/workspace/state/data\nexport CODEX_HOME=/workspace/state/home/.codex\nexport CLAUDE_CONFIG_DIR=/workspace/state/home/.claude\nexport TMPDIR=/tmp\n")
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
	if p.Agent == "codex" {
		// Generated hooks are trusted by construction; older pinned images do
		// not know the flag, so only pass it when the binary advertises it.
		b.WriteString("hookflag=\nif codex --help 2>/dev/null | grep -q -- --dangerously-bypass-hook-trust; then hookflag=--dangerously-bypass-hook-trust; fi\n")
	}
	b.WriteString("exec")
	for i, arg := range args {
		b.WriteByte(' ')
		if i == 1 && p.Agent == "codex" {
			b.WriteString("$hookflag ")
		}
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
