package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// MCPTools is a named selection of global MCP servers for one agent. It
// stores server names only; definitions stay in the agent's own
// user-scope configuration on the host.
type MCPTools struct {
	Version int      `yaml:"version" json:"version"`
	Name    string   `yaml:"name" json:"name"`
	Agent   string   `yaml:"agent" json:"agent"`
	Servers []string `yaml:"servers" json:"servers"`
}

var mcpServerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func (m MCPTools) Validate() error {
	if m.Version != 1 || !safeName.MatchString(m.Name) {
		return fmt.Errorf("MCP tools entry needs version 1 and a lower-case name")
	}
	if m.Agent != "claude" && m.Agent != "codex" && m.Agent != "opencode" {
		return fmt.Errorf("agent must be claude, codex, or opencode")
	}
	if len(m.Servers) == 0 {
		return fmt.Errorf("MCP tools entry needs at least one server name")
	}
	seen := make(map[string]struct{}, len(m.Servers))
	for _, server := range m.Servers {
		if !mcpServerName.MatchString(server) {
			return fmt.Errorf("server name %q must use letters, digits, hyphens, or underscores", server)
		}
		if _, ok := seen[server]; ok {
			return fmt.Errorf("server name %q is listed twice", server)
		}
		seen[server] = struct{}{}
	}
	return nil
}

// MCPSelection is the resolved outcome of an MCP tools entry for one run.
type MCPSelection struct {
	Entry   MCPTools
	Servers map[string]any // agent-native server definitions, copied verbatim
	Missing []string       // selected names absent from the host source
}

func (s MCPSelection) Warnings() []string {
	if len(s.Missing) == 0 {
		return nil
	}
	missing := append([]string(nil), s.Missing...)
	sort.Strings(missing)
	warnings := make([]string, 0, len(missing))
	for _, name := range missing {
		warnings = append(warnings, fmt.Sprintf("MCP server %q from entry %q is not configured for %s on this host; starting without it", name, s.Entry.Name, s.Entry.Agent))
	}
	return warnings
}

func (s Store) MCPsDir() string            { return filepath.Join(s.Base, "mcps") }
func (s Store) MCPPath(name string) string { return filepath.Join(s.MCPsDir(), name+".yaml") }

func (s Store) SaveMCP(m MCPTools) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return atomicWrite(s.MCPPath(m.Name), b)
}

func (s Store) LoadMCP(name string) (MCPTools, error) {
	if !safeName.MatchString(name) {
		return MCPTools{}, fmt.Errorf("invalid MCP tools name")
	}
	b, err := os.ReadFile(s.MCPPath(name))
	if err != nil {
		return MCPTools{}, err
	}
	var m MCPTools
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return m, err
	}
	if m.Name != name {
		return m, fmt.Errorf("MCP tools filename and name disagree")
	}
	return m, m.Validate()
}

func (s Store) ListMCPs() ([]MCPTools, error) {
	entries, err := os.ReadDir(s.MCPsDir())
	if errors.Is(err, os.ErrNotExist) {
		return []MCPTools{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]MCPTools, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		m, err := s.LoadMCP(strings.TrimSuffix(e.Name(), ".yaml"))
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s Store) DeleteMCP(name string) error {
	if !safeName.MatchString(name) {
		return fmt.Errorf("invalid MCP tools name")
	}
	if err := os.Remove(s.MCPPath(name)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("MCP tools entry %q not found", name)
		}
		return err
	}
	return nil
}

// GlobalMCPServers lists the global MCP server names configured for an
// agent on the host. It reads names only and never returns definitions.
func (a *App) GlobalMCPServers(agent string) ([]string, error) {
	servers, err := readGlobalMCPServers(agent)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// ResolveMCPSelection loads an MCP tools entry, enforces agent
// compatibility with the run profile, and extracts the selected server
// definitions from the agent's host configuration. Names missing from the
// host source produce warnings, not errors.
func (a *App) ResolveMCPSelection(name, agent string) (MCPSelection, error) {
	entry, err := a.Store.LoadMCP(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return MCPSelection{}, fmt.Errorf("MCP tools entry %q not found; create it from the MCP Tools page or with `agentbox mcp create`", name)
		}
		return MCPSelection{}, err
	}
	if entry.Agent != agent {
		return MCPSelection{}, fmt.Errorf("MCP tools entry %q selects agent %q but the run profile uses %q", name, entry.Agent, agent)
	}
	servers, err := readGlobalMCPServers(agent)
	if err != nil {
		return MCPSelection{}, err
	}
	selection := MCPSelection{Entry: entry, Servers: map[string]any{}}
	for _, want := range entry.Servers {
		definition, ok := servers[want]
		if !ok {
			selection.Missing = append(selection.Missing, want)
			continue
		}
		if err := scanMCPServer(want, definition); err != nil {
			return MCPSelection{}, err
		}
		selection.Servers[want] = definition
	}
	return selection, nil
}

// mcpHostSources returns the candidate host configuration paths for an
// agent's user-scope MCP configuration, in priority order.
func mcpHostSources(agent string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	switch agent {
	case "claude":
		dir := os.Getenv("CLAUDE_CONFIG_DIR")
		if dir == "" {
			dir = filepath.Join(home, ".claude")
		}
		return []string{filepath.Join(dir, ".claude.json")}, nil
	case "codex":
		dir := os.Getenv("CODEX_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".codex")
		}
		return []string{filepath.Join(dir, "config.toml")}, nil
	case "opencode":
		dir := os.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".config")
		}
		base := filepath.Join(dir, "opencode")
		return []string{filepath.Join(base, "opencode.json"), filepath.Join(base, "opencode.jsonc")}, nil
	}
	return nil, fmt.Errorf("agent must be claude, codex, or opencode")
}

// readGlobalMCPServers reads the MCP portion of the agent's host
// user-scope configuration. An absent source is not an error: it yields no
// servers, which surfaces as drift warnings at run start.
func readGlobalMCPServers(agent string) (map[string]any, error) {
	paths, err := mcpHostSources(agent)
	if err != nil {
		return nil, err
	}
	var body []byte
	path := ""
	for _, candidate := range paths {
		b, err := os.ReadFile(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		body, path = b, candidate
		break
	}
	if body == nil {
		return map[string]any{}, nil
	}
	switch agent {
	case "claude":
		return parseClaudeMCP(body, path)
	case "codex":
		return parseCodexMCP(body, path)
	case "opencode":
		if strings.HasSuffix(path, ".jsonc") {
			body = jsoncStrip(body)
		}
		return parseOpenCodeMCP(body, path)
	}
	return nil, fmt.Errorf("agent must be claude, codex, or opencode")
}

func parseClaudeMCP(body []byte, path string) (map[string]any, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	raw, ok := doc["mcpServers"]
	if !ok {
		return map[string]any{}, nil
	}
	var servers map[string]any
	if err := json.Unmarshal(raw, &servers); err != nil || servers == nil {
		return nil, fmt.Errorf("%s: mcpServers must be an object of MCP servers", path)
	}
	return servers, nil
}

func parseCodexMCP(body []byte, path string) (map[string]any, error) {
	var doc map[string]any
	if err := toml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("%s is not valid TOML: %w", path, err)
	}
	servers, ok := doc["mcp_servers"].(map[string]any)
	if !ok || servers == nil {
		return map[string]any{}, nil
	}
	return servers, nil
}

func parseOpenCodeMCP(body []byte, path string) (map[string]any, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	raw, ok := doc["mcp"]
	if !ok {
		return map[string]any{}, nil
	}
	var mcp map[string]json.RawMessage
	if err := json.Unmarshal(raw, &mcp); err != nil {
		return nil, fmt.Errorf("%s: mcp must be an object: %w", path, err)
	}
	raw, ok = mcp["servers"]
	if !ok {
		return map[string]any{}, nil
	}
	var servers map[string]any
	if err := json.Unmarshal(raw, &servers); err != nil || servers == nil {
		return nil, fmt.Errorf("%s: mcp.servers must be an object of MCP servers", path)
	}
	return servers, nil
}

var (
	mcpEnvReference = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*(?:-[^}]*)?\}|\{env:[A-Za-z_][A-Za-z0-9_]*\}`)
	mcpDenseValue   = regexp.MustCompile(`^[A-Za-z0-9_.=-]+$`)
)

// scanMCPServer rejects inline secret-like values in a server definition.
// Environment references ("${VAR}", "{env:VAR}", or a bare variable name)
// pass; literal values that look like credentials fail with an actionable
// error. The scan is a best-effort heuristic, like the profile argument
// check.
func scanMCPServer(server string, definition any) error {
	var walk func(path string, value any) error
	walk = func(path string, value any) error {
		switch typed := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if err := walk(path+"."+key, typed[key]); err != nil {
					return err
				}
			}
		case []any:
			for i, item := range typed {
				if err := walk(fmt.Sprintf("%s[%d]", path, i), item); err != nil {
					return err
				}
			}
		case string:
			if mcpSecretValue(typed) {
				return fmt.Errorf("MCP server %q has an inline secret-like value at %s; reference an environment variable such as ${NAME} instead", server, path)
			}
		}
		return nil
	}
	return walk(server, definition)
}

func mcpSecretValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	// Bare environment-variable names are references.
	if envName.MatchString(trimmed) {
		return false
	}
	// References embedded in a value (for example "Bearer ${TOKEN}") are not
	// secrets; test what remains after removing them.
	remainder := strings.TrimSpace(mcpEnvReference.ReplaceAllString(trimmed, ""))
	if remainder == "" {
		return false
	}
	if secretName.MatchString(remainder) {
		return true
	}
	// Credential-shaped literals: long, dense character runs containing
	// both letters and digits. Command flags are exempt.
	if strings.HasPrefix(remainder, "-") {
		return false
	}
	if len(remainder) >= 20 && mcpDenseValue.MatchString(remainder) && strings.ContainsAny(remainder, "0123456789") {
		return true
	}
	return false
}

// jsoncStrip removes // and /* */ comments and trailing commas that JSONC
// allows. It never inspects content inside string literals.
func jsoncStrip(body []byte) []byte {
	var out []byte
	inString, escaped := false, false
	for i := 0; i < len(body); i++ {
		c := body[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(body) && body[i+1] == '/':
			for i < len(body) && body[i] != '\n' {
				i++
			}
			out = append(out, '\n')
		case c == '/' && i+1 < len(body) && body[i+1] == '*':
			end := bytes.Index(body[i+2:], []byte("*/"))
			if end < 0 {
				return finishJSONC(out)
			}
			i = i + 2 + end + 1
		default:
			out = append(out, c)
		}
	}
	return finishJSONC(out)
}

// LocalMCPFiles returns the agent-native MCP files committed at a project
// root, detected by filename only. Detection never reads file contents.
func LocalMCPFiles(root string) []string {
	candidates := []string{
		".mcp.json",                       // claude
		"opencode.json", "opencode.jsonc", // opencode
		filepath.Join(".opencode", "opencode.json"), filepath.Join(".opencode", "opencode.jsonc"),
		filepath.Join(".codex", "config.toml"), // codex (trusted projects only)
	}
	var found []string
	for _, candidate := range candidates {
		if info, err := os.Stat(filepath.Join(root, candidate)); err == nil && !info.IsDir() {
			found = append(found, candidate)
		}
	}
	return found
}

// writeCodexMCP appends the selected MCP server tables to the generated
// CODEX_HOME/config.toml. The file is AgentBox-generated, so appending
// tables is safe; definitions are copied verbatim from the host selection.
func writeCodexMCP(configDir string, mcp *MCPSelection) error {
	if mcp == nil || len(mcp.Servers) == 0 {
		return nil
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(map[string]any{"mcp_servers": mcp.Servers}); err != nil {
		return err
	}
	path := filepath.Join(configDir, "config.toml")
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	body := append(existing, '\n')
	body = append(body, buf.Bytes()...)
	return os.WriteFile(path, body, 0600)
}

// writeJSONFile writes a compact, readable JSON document with a trailing
// newline.
func writeJSONFile(path string, doc map[string]any) error {
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}

var jsoncTrailingComma = regexp.MustCompile(`,(\s*[}\]])`)

func finishJSONC(body []byte) []byte {
	return jsoncTrailingComma.ReplaceAll(body, []byte("$1"))
}
