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
	Host     string   `yaml:"host"`
	Port     int      `yaml:"port"`
	Binaries []string `yaml:"binaries"`
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
	// A long contiguous run of token-safe characters; the sha256 field and
	// hostnames are exempt, every other value this long is treated as a
	// pasted credential.
	repoOpaqueRun = regexp.MustCompile(`[A-Za-z0-9+/=_-]{40,}`)
)

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
	if len(path) > 0 {
		// sha256 fields are legitimately opaque, and hostnames are
		// structurally validated as exact hostnames elsewhere; both are
		// exempt from the opaque-run heuristic.
		if last := path[len(path)-1]; last == "sha256" || last == "host" {
			return nil
		}
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
