package skills

import (
	"bytes"
	"embed"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Roles are a second embedded artifact set: one harness-native agent
// definition per non-orchestrator stage. They are installed globally because a
// repository-scoped agent definition is a project convention AgentBox does not
// own.
//
//go:embed roles/*.yaml
var embeddedRoles embed.FS

// Role is one canonical agent role. The YAML definition is harness-neutral;
// renderers turn it into the native format of each harness.
type Role struct {
	Name         string               `yaml:"name"`
	Description  string               `yaml:"description"`
	Instructions string               `yaml:"instructions"`
	Write        bool                 `yaml:"write"`
	Shell        bool                 `yaml:"shell"`
	Steps        int                  `yaml:"steps,omitempty"`
	Models       map[string]RoleModel `yaml:"models,omitempty"`
}

// RoleModel is the optional per-harness model selection of a role. An absent
// harness entry means the role inherits the harness default, and the renderer
// emits no model field.
type RoleModel struct {
	Model  string `yaml:"model"`
	Effort string `yaml:"effort,omitempty"`
}

// roleOrder is the stable order roles are installed and reported in.
var roleOrder = []string{
	"agentbox-triage",
	"agentbox-planner",
	"agentbox-implementer",
	"agentbox-fixer",
	"agentbox-verifier",
	"agentbox-reviewer",
	"agentbox-retro",
}

// RoleNames returns the embedded role names in a stable order.
func RoleNames() []string {
	out := make([]string, len(roleOrder))
	copy(out, roleOrder)
	return out
}

// ValidRoleName reports whether name identifies an embedded role.
func ValidRoleName(name string) bool {
	return containsString(roleOrder, name)
}

// RoleByName returns the canonical definition of one embedded role.
func RoleByName(name string) (Role, error) {
	if !ValidRoleName(name) {
		return Role{}, fmt.Errorf("unknown role %q (available: %s)", name, strings.Join(roleOrder, ", "))
	}
	raw, err := embeddedRoles.ReadFile("roles/" + name + ".yaml")
	if err != nil {
		return Role{}, err
	}
	return ParseRole(raw)
}

// RoleYAML returns the canonical YAML definition of one embedded role.
func RoleYAML(name string) ([]byte, error) {
	if !ValidRoleName(name) {
		return nil, fmt.Errorf("unknown role %q (available: %s)", name, strings.Join(roleOrder, ", "))
	}
	return embeddedRoles.ReadFile("roles/" + name + ".yaml")
}

// ParseRole parses one canonical role definition strictly. Unknown keys are
// rejected so a typo cannot silently drop a rule.
func ParseRole(raw []byte) (Role, error) {
	var role Role
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&role); err != nil {
		return Role{}, fmt.Errorf("role definition is not valid YAML: %w", err)
	}
	if strings.TrimSpace(role.Name) == "" || strings.TrimSpace(role.Description) == "" || strings.TrimSpace(role.Instructions) == "" {
		return Role{}, fmt.Errorf("role definition needs non-empty name, description, and instructions")
	}
	for harness, model := range role.Models {
		if !validHarness(harness) {
			return Role{}, fmt.Errorf("role %q models key %q must be claude, codex, or opencode", role.Name, harness)
		}
		if model.Model == "" && model.Effort != "" {
			return Role{}, fmt.Errorf("role %q model for %s sets an effort without a model", role.Name, harness)
		}
	}
	return role, nil
}

// ValidateRoles checks the embedded role set: every name is listed once, the
// definition names itself, and every shipped role parses.
func ValidateRoles() error {
	seen := make(map[string]bool, len(roleOrder))
	for _, name := range roleOrder {
		if seen[name] {
			return fmt.Errorf("role %q is listed twice", name)
		}
		seen[name] = true
		role, err := RoleByName(name)
		if err != nil {
			return err
		}
		if role.Name != name {
			return fmt.Errorf("role file %q defines name %q", name, role.Name)
		}
	}
	return nil
}

// RoleFile is the installed file name of a role for one harness.
func RoleFile(harness string) (string, bool) {
	switch harness {
	case "codex":
		return ".toml", true
	case "claude", "opencode":
		return ".md", true
	}
	return "", false
}

// RenderRole renders one role in the native agent format of harness. model and
// effort override the role's optional per-harness selection; an empty model
// emits no model field. The version is written as a trailing comment so a
// binary upgrade changes the rendering and staleness stays sha256-derived.
func RenderRole(role Role, harness, model, effort, version string) ([]byte, error) {
	if !validHarness(harness) {
		return nil, fmt.Errorf("harness %q must be claude, codex, or opencode", harness)
	}
	if configured, ok := role.Models[harness]; ok {
		if model == "" {
			model = configured.Model
		}
		if effort == "" {
			effort = configured.Effort
		}
	}
	switch harness {
	case "codex":
		return renderCodexRole(role, model, effort, version), nil
	case "claude":
		return renderClaudeRole(role, model, version), nil
	default:
		return renderOpenCodeRole(role, model, version), nil
	}
}

// codexReadOnlyRule is rendered into the Codex developer instructions because
// Codex agent files cannot narrow sandbox or approval capabilities. The install
// output repeats the limitation instead of implying enforcement.
const codexReadOnlyRule = "This role is read-only: do not create, edit, move, or delete files. Codex agent files cannot narrow sandbox or approval settings, so this restriction is instruction-only."

// codexNoShellRule is rendered into the Codex developer instructions because
// Codex agent files cannot remove shell access. Triage and retro use it so they
// cannot reach the tracker or forge through `gh` or `git`.
const codexNoShellRule = "This role has no shell access: do not run commands, including `git` and `gh`. Codex agent files cannot remove shell access, so this restriction is instruction-only."

func renderCodexRole(role Role, model, effort, version string) []byte {
	var b strings.Builder
	writeTOMLString(&b, "name", role.Name)
	writeTOMLString(&b, "description", role.Description)
	if model != "" {
		writeTOMLString(&b, "model", model)
	}
	if effort != "" {
		writeTOMLString(&b, "model_reasoning_effort", effort)
	}
	instructions := role.Instructions
	if !role.Write {
		instructions = strings.TrimRight(instructions, "\n") + "\n\n" + codexReadOnlyRule + "\n"
	}
	if !role.Shell {
		instructions = strings.TrimRight(instructions, "\n") + "\n\n" + codexNoShellRule + "\n"
	}
	writeTOMLString(&b, "developer_instructions", instructions)
	b.WriteString("# agentbox-role: " + effectiveVersion(version) + "\n")
	return []byte(b.String())
}

// writeTOMLString writes one TOML basic string. Newlines are escaped, so the
// value is valid TOML regardless of the instruction text.
func writeTOMLString(b *strings.Builder, key, value string) {
	b.WriteString(key)
	b.WriteString(" = ")
	b.WriteString(strconv.Quote(value))
	b.WriteString("\n")
}

func renderClaudeRole(role Role, model, version string) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	writeYAMLString(&b, "name", role.Name)
	writeYAMLString(&b, "description", role.Description)
	tools := "Read, Grep, Glob"
	if role.Shell {
		tools += ", Bash"
	}
	if role.Write {
		tools += ", Edit, Write"
	}
	writeYAMLString(&b, "tools", tools)
	if model != "" {
		writeYAMLString(&b, "model", model)
	}
	b.WriteString("---\n\n")
	b.WriteString(role.Instructions)
	if !strings.HasSuffix(role.Instructions, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n<!-- agentbox-role: " + effectiveVersion(version) + " -->\n")
	return []byte(b.String())
}

func renderOpenCodeRole(role Role, model, version string) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	writeYAMLString(&b, "description", role.Description)
	writeYAMLString(&b, "mode", "subagent")
	if model != "" {
		writeYAMLString(&b, "model", model)
	}
	if role.Steps > 0 {
		fmt.Fprintf(&b, "steps: %d\n", role.Steps)
	}
	// OpenCode V2 agent permissions are an ordered rule list. Denying `edit`
	// covers the write tools, and denying `shell` removes command execution.
	if !role.Write || !role.Shell {
		b.WriteString("permissions:\n")
		if !role.Write {
			b.WriteString("  - action: edit\n    resource: \"*\"\n    effect: deny\n")
		}
		if !role.Shell {
			b.WriteString("  - action: shell\n    resource: \"*\"\n    effect: deny\n")
		}
	}
	b.WriteString("---\n\n")
	b.WriteString(role.Instructions)
	if !strings.HasSuffix(role.Instructions, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n<!-- agentbox-role: " + effectiveVersion(version) + " -->\n")
	return []byte(b.String())
}

// writeYAMLString writes one double-quoted YAML scalar. strconv.Quote escapes
// newlines and quotes, so multi-line descriptions stay valid YAML.
func writeYAMLString(b *strings.Builder, key, value string) {
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(strconv.Quote(value))
	b.WriteString("\n")
}

// RoleWriteLimitation describes how a harness represents write: false. Codex
// cannot enforce the restriction; the others omit or deny the write actions.
func RoleWriteLimitation(harness string, role Role) string {
	if role.Write || harness != "codex" {
		return ""
	}
	return "Codex agent files cannot enforce write: false; the read-only rule is instruction text"
}

// RoleShellLimitation describes how a harness represents shell: false. Codex
// cannot enforce the restriction; the others remove or deny the shell tool.
func RoleShellLimitation(harness string, role Role) string {
	if role.Shell || harness != "codex" {
		return ""
	}
	return "Codex agent files cannot enforce shell: false; the no-shell rule is instruction text"
}
