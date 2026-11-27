package app

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// RepoConfig is the parsed .agentbox/config.yaml manifest declaring a
// repository's development and CI needs: image packages, pinned toolchains,
// image verification commands, and the exact runtime egress for repository
// work. The file is committed at the repository root and must stay
// credential-free; ParseRepoConfig rejects secret-like content anywhere in it.
type RepoConfig struct {
	Schema int              `yaml:"schema"`
	Image  RepoImageSection `yaml:"image"`
	Verify []string         `yaml:"verify"`
	Hosts  []RepoHostRule   `yaml:"hosts"`
}

// RepoImageSection declares what a repository worker image adds on top of the
// profile base image.
type RepoImageSection struct {
	// Packages are plain apt package names installed at image build time.
	Packages []string `yaml:"packages"`
	// Toolchains are checksum-pinned downloads installed at image build time.
	Toolchains []RepoToolchain `yaml:"toolchains"`
}

// RepoToolchain is one pinned toolchain archive installed into the image.
type RepoToolchain struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	SHA256 string `yaml:"sha256"`
	Bin    string `yaml:"bin"`
}

// RepoHostRule is one exact runtime egress allowance: a host, a port, and the
// binaries that may reach it.
type RepoHostRule struct {
	Host     string   `yaml:"host" json:"host"`
	Port     int      `yaml:"port" json:"port"`
	Binaries []string `yaml:"binaries" json:"binaries"`
}

// RepoConfigPath returns the manifest path under a repository root.
func RepoConfigPath(root string) string {
	return filepath.Join(root, ".agentbox", "config.yaml")
}

// RepoConfigPresent reports whether root declares a repository manifest
// file. Presence says nothing about validity; setup reports that.
func RepoConfigPresent(root string) bool {
	_, err := os.Stat(RepoConfigPath(root))
	return err == nil
}

// LoadRepoConfig reads and validates the repository manifest under root. A
// repository without a manifest returns an error wrapping os.ErrNotExist,
// which callers must distinguish from an invalid manifest.
func LoadRepoConfig(root string) (RepoConfig, error) {
	path := RepoConfigPath(root)
	b, err := os.ReadFile(path)
	if err != nil {
		return RepoConfig{}, err
	}
	cfg, err := ParseRepoConfig(b)
	if err != nil {
		return RepoConfig{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// ParseRepoConfig parses and validates manifest bytes. Every violation names
// the offending field (for example hosts[0].port) so the error is actionable.
func ParseRepoConfig(b []byte) (RepoConfig, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return RepoConfig{}, fmt.Errorf("invalid YAML: %w", err)
	}
	if len(doc.Content) == 0 {
		return RepoConfig{}, fmt.Errorf("empty manifest: schema: 1 is required")
	}
	if err := scanSecrets(doc.Content[0], nil); err != nil {
		return RepoConfig{}, err
	}
	var cfg RepoConfig
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return RepoConfig{}, err
	}
	if cfg.Schema != 1 {
		return RepoConfig{}, fmt.Errorf("schema: must be 1 (found %d)", cfg.Schema)
	}
	if err := validateRepoPackages(cfg.Image.Packages); err != nil {
		return RepoConfig{}, err
	}
	if err := validateRepoToolchains(&cfg.Image); err != nil {
		return RepoConfig{}, err
	}
	if err := validateRepoVerify(cfg.Verify); err != nil {
		return RepoConfig{}, err
	}
	if err := validateRepoHosts(cfg.Hosts); err != nil {
		return RepoConfig{}, err
	}
	return cfg, nil
}

var (
	// Debian policy package names: lowercase letters, digits, '+', '-', '.';
	// at least two characters, starting alphanumeric. This rejects URLs,
	// version pins (name=version), and shell metacharacters.
	repoPackageName = regexp.MustCompile(`^[a-z0-9][a-z0-9+.\-]{1,}$`)
	// One DNS label: 1-63 characters, alphanumeric ends, hyphens inside.
	repoHostLabel = `[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?`
	repoExactHost = regexp.MustCompile(`^` + repoHostLabel + `(?:\.` + repoHostLabel + `)*$`)
	repoSHA256    = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	// A long contiguous run of token-safe characters; fields whose values are
	// structurally validated are exempt (see repoOpaqueExemptFields), every
	// other value this long is treated as a pasted credential. The class
	// covers the base64 and base64url alphabets, so it also matches ordinary
	// "/" and "-" separators in absolute paths.
	repoOpaqueRun = regexp.MustCompile(`[A-Za-z0-9+/=_-]{40,}`)
)

// repoOpaqueExemptFields names manifest fields whose string values are
// validated by shape rather than accepted as free form, so the opaque-run
// heuristic must not apply to them. Without the exemption an ordinary
// hyphenated binary path of 40 or more characters, such as
// /usr/local/uv-aarch64-unknown-linux-gnu/uv, is mistaken for a credential
// because "/" and "-" are inside the heuristic's character class.
var repoOpaqueExemptFields = map[string]struct{}{
	"sha256":   {}, // exactly 64 hexadecimal characters
	"host":     {}, // exact hostname, see repoExactHost
	"bin":      {}, // toolchain binary path, see validateRepoBinaryPath
	"binaries": {}, // host-rule binary paths, see validateRepoBinaryPath
}

// pathContainsOpaqueExemptField reports whether any element of a manifest
// field path names a structurally validated field. Host-rule binaries are
// indexed (hosts.0.binaries.0), so ancestors count, not just the leaf.
func pathContainsOpaqueExemptField(path []string) bool {
	for _, part := range path {
		if _, ok := repoOpaqueExemptFields[part]; ok {
			return true
		}
	}
	return false
}

const maxRepoHostNameLen = 253

// Secret-like key fragments, matched case-insensitively against normalized
// (lowercased, hyphens mapped to underscores) field names anywhere in the file.
var repoSecretKeyFragments = []string{
	"token", "secret", "password", "passwd", "credential",
	"api_key", "apikey", "private_key", "privatekey", "authorization",
}

// Secret-like value fragments, matched case-insensitively against every
// string value in the file.
var repoSecretValueHints = []string{
	"token=", "secret=", "password=", "passwd=", "api_key=", "apikey=",
	"x-api-key", "authorization:", "bearer ", "ghp_", "gho_", "github_pat_",
	"xoxb", "xoxp",
}

func validateRepoPackages(packages []string) error {
	for i, name := range packages {
		if !repoPackageName.MatchString(name) {
			return fmt.Errorf("image.packages[%d]: %q is not a plain apt package name; use lowercase letters, digits, '+', '-', and '.', without URLs, version pins (name=version), or shell characters", i, name)
		}
	}
	return nil
}

func validateRepoToolchains(image *RepoImageSection) error {
	names := make(map[string]bool)
	bins := make(map[string]bool)
	for i := range image.Toolchains {
		field := fmt.Sprintf("image.toolchains[%d]", i)
		tc := &image.Toolchains[i]
		if !safeName.MatchString(tc.Name) {
			return fmt.Errorf("%s.name: %q must be a short lowercase identifier (letters, digits, hyphens), like %q", field, tc.Name, "go")
		}
		if names[tc.Name] {
			return fmt.Errorf("%s.name: duplicate toolchain name %q", field, tc.Name)
		}
		names[tc.Name] = true
		u, err := url.Parse(tc.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("%s.url: %q must be an HTTPS download URL", field, tc.URL)
		}
		if strings.ContainsFunc(tc.URL, unicode.IsSpace) {
			return fmt.Errorf("%s.url: must not contain whitespace; percent-encode spaces in the URL", field)
		}
		if !repoSHA256.MatchString(tc.SHA256) {
			return fmt.Errorf("%s.sha256: must be exactly 64 hexadecimal characters", field)
		}
		tc.SHA256 = strings.ToLower(tc.SHA256)
		if err := validateRepoBinaryPath(field+".bin", tc.Bin); err != nil {
			return err
		}
		if bins[tc.Bin] {
			return fmt.Errorf("%s.bin: duplicate toolchain binary path %q", field, tc.Bin)
		}
		bins[tc.Bin] = true
	}
	return nil
}

func validateRepoVerify(commands []string) error {
	for i, cmd := range commands {
		field := fmt.Sprintf("verify[%d]", i)
		switch {
		case strings.TrimSpace(cmd) == "":
			return fmt.Errorf("%s: must be a non-empty command", field)
		case strings.ContainsFunc(cmd, unicode.IsControl):
			return fmt.Errorf("%s: must be a single command on one line", field)
		case cmd != strings.TrimSpace(cmd):
			return fmt.Errorf("%s: must not have leading or trailing whitespace", field)
		case strings.HasSuffix(cmd, `\`):
			return fmt.Errorf("%s: must not end with a line continuation", field)
		}
	}
	return nil
}

func validateRepoHosts(hosts []RepoHostRule) error {
	seen := make(map[string]int)
	for i, rule := range hosts {
		field := fmt.Sprintf("hosts[%d]", i)
		switch {
		case rule.Host == "":
			return fmt.Errorf("%s.host: is required", field)
		case len(rule.Host) > maxRepoHostNameLen:
			return fmt.Errorf("%s.host: hostname exceeds %d characters", field, maxRepoHostNameLen)
		case !repoExactHost.MatchString(rule.Host):
			return fmt.Errorf("%s.host: %q must be an exact hostname; wildcards, CIDR blocks, schemes, ports, and paths are not allowed", field, rule.Host)
		case rule.Port < 1 || rule.Port > 65535:
			return fmt.Errorf("%s.port: must be between 1 and 65535", field)
		case len(rule.Binaries) == 0:
			return fmt.Errorf("%s.binaries: at least one binary path is required", field)
		}
		key := fmt.Sprintf("%s:%d", strings.ToLower(rule.Host), rule.Port)
		if first, dup := seen[key]; dup {
			return fmt.Errorf("%s: duplicate host %s (already declared at hosts[%d])", field, key, first)
		}
		seen[key] = i
		for j, bin := range rule.Binaries {
			if err := validateRepoBinaryPath(fmt.Sprintf("%s.binaries[%d]", field, j), bin); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateHostRule applies the manifest's exactness rules to one runtime host
// rule. Denials must not widen egress: hosts are exact, ports are valid, and a
// binary is one exact path rather than a glob. Secret-like values are refused
// here for every caller, including the live run-policy path.
func ValidateHostRule(rule RepoHostRule) error {
	if err := validateRepoHosts([]RepoHostRule{rule}); err != nil {
		return err
	}
	if err := checkSecretValue(rule.Host, []string{"hosts", "host"}); err != nil {
		return err
	}
	for i, binary := range rule.Binaries {
		if strings.ContainsAny(binary, "*?[]") {
			return fmt.Errorf("hosts[0].binaries[%d]: %q must be one exact binary path; wildcards are not allowed", i, binary)
		}
		if strings.HasSuffix(binary, "/") {
			return fmt.Errorf("hosts[0].binaries[%d]: %q must name an executable, not a directory", i, binary)
		}
		if err := checkSecretValue(binary, []string{"hosts", "binaries", strconv.Itoa(i)}); err != nil {
			return err
		}
	}
	return nil
}

// AppendRepoHostRule adds one exact host rule to a repository manifest. It
// preserves the existing YAML document (including comments) and is idempotent:
// when the host and port already exist, only a missing binary is appended. The
// result is re-parsed with the full manifest validator before it is returned.
func AppendRepoHostRule(b []byte, rule RepoHostRule) ([]byte, error) {
	if err := ValidateHostRule(rule); err != nil {
		return nil, err
	}
	if _, err := ParseRepoConfig(b); err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("manifest must be a YAML mapping")
	}
	root := doc.Content[0]
	hosts := mappingValue(root, "hosts")
	if hosts == nil {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "hosts"},
			&yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"},
		)
		hosts = root.Content[len(root.Content)-1]
	}
	if hosts.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("hosts: must be a list of host rules")
	}
	binary := rule.Binaries[0]
	for _, entry := range hosts.Content {
		host := mappingValue(entry, "host")
		port := mappingValue(entry, "port")
		if host == nil || port == nil {
			continue
		}
		entryPort, err := strconv.Atoi(port.Value)
		if err != nil || !strings.EqualFold(host.Value, rule.Host) || entryPort != rule.Port {
			continue
		}
		binaries := mappingValue(entry, "binaries")
		if binaries == nil || binaries.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("hosts: existing %s:%d rule has no binaries list", rule.Host, rule.Port)
		}
		for _, existing := range binaries.Content {
			if existing.Value == binary {
				return b, nil
			}
		}
		binaries.Content = append(binaries.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: binary})
		return marshalRepoConfig(&doc)
	}
	hosts.Content = append(hosts.Content, &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "host"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: rule.Host},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "port"},
			{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(rule.Port)},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "binaries"},
			{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Tag: "!!str", Value: binary},
			}},
		},
	})
	return marshalRepoConfig(&doc)
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func marshalRepoConfig(doc *yaml.Node) ([]byte, error) {
	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	if _, err := ParseRepoConfig(out); err != nil {
		return nil, err
	}
	return out, nil
}

// validateRepoBinaryPath checks one sandbox binary path. Absolute paths are
// required; OpenShell-style trailing "**" globs are allowed.
func validateRepoBinaryPath(field, path string) error {
	switch {
	case path == "":
		return fmt.Errorf("%s: binary path is required", field)
	case !strings.HasPrefix(path, "/"):
		return fmt.Errorf("%s: %q must be an absolute path", field, path)
	case strings.ContainsFunc(path, unicode.IsControl) || strings.ContainsAny(path, " \t"):
		return fmt.Errorf("%s: must not contain whitespace", field)
	case strings.Contains(path, ".."):
		return fmt.Errorf("%s: must not contain '..'", field)
	}
	return nil
}

// scanSecrets walks the raw YAML tree and rejects secret-like field names or
// values anywhere in the manifest, including behind anchors and merge keys.
// Values are never echoed back in errors, so a rejected secret is not
// republished through logs or dialogs.
func scanSecrets(node *yaml.Node, path []string) error {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := scanSecrets(child, path); err != nil {
				return err
			}
		}
	case yaml.AliasNode:
		return scanSecrets(node.Alias, path)
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if err := checkSecretKey(key.Value, path); err != nil {
				return err
			}
			if err := scanSecrets(value, append(path, key.Value)); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for i, item := range node.Content {
			if err := scanSecrets(item, append(path, strconv.Itoa(i))); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if node.Tag == "!!binary" {
			return fmt.Errorf("%s: binary-encoded values are not allowed; the manifest must stay credential-free", yamlFieldPath(path))
		}
		if node.Tag == "!!str" {
			return checkSecretValue(node.Value, path)
		}
	}
	return nil
}

func checkSecretKey(key string, path []string) error {
	normalized := strings.ReplaceAll(strings.ToLower(key), "-", "_")
	secretLike := normalized == "key"
	for _, fragment := range repoSecretKeyFragments {
		if strings.Contains(normalized, fragment) {
			secretLike = true
			break
		}
	}
	if !secretLike {
		return nil
	}
	return fmt.Errorf("%s: secret-like field name %q is not allowed; the manifest must stay credential-free", yamlFieldPath(append(path, key)), key)
}

func checkSecretValue(value string, path []string) error {
	lower := strings.ToLower(value)
	for _, hint := range repoSecretValueHints {
		if strings.Contains(lower, hint) {
			return fmt.Errorf("%s: secret-like value is not allowed (contains %q); the manifest must stay credential-free", yamlFieldPath(path), hint)
		}
	}
	// Digests, hostnames, and binary paths are opaque by nature but validated
	// by shape, so the opaque-run heuristic does not apply. Value hints above
	// still reject credentials pasted into any of them.
	if pathContainsOpaqueExemptField(path) {
		return nil
	}
	if repoOpaqueRun.MatchString(value) {
		return fmt.Errorf("%s: secret-like value is not allowed (opaque run of 40+ characters); the manifest must stay credential-free", yamlFieldPath(path))
	}
	return nil
}

// yamlFieldPath renders a dotted field path with bracketed sequence indexes,
// for example image.toolchains[0].url.
func yamlFieldPath(parts []string) string {
	var b strings.Builder
	for i, part := range parts {
		if i > 0 && isDigits(part) {
			b.WriteByte('[')
			b.WriteString(part)
			b.WriteByte(']')
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(part)
	}
	return b.String()
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
