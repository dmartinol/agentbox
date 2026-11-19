package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type RunPolicyProvenance struct {
	Source      string `json:"source"`
	Gateway     string `json:"gateway"`
	Provider    string `json:"provider"`
	Mode        string `json:"mode"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Rule        string `json:"rule"`
	AgentBinary string `json:"agent_binary"`
	LegacyPath  string `json:"legacy_path,omitempty"`
}

type providerEndpoint struct {
	provider string
	host     string
	port     int
}

func isGeneratedDefaultPolicy(store Store, profile Profile) bool {
	return filepath.Clean(profile.Policy) == filepath.Clean(filepath.Join(store.PolicyDir(), profile.Name+".yaml"))
}

func deriveProviderEndpoint(profile Profile) (providerEndpoint, error) {
	if profile.Mode == "standard" {
		definition, ok := BuiltinProviderByName(profile.Provider)
		if !ok {
			return providerEndpoint{}, fmt.Errorf("profile %q cannot derive its provider host: provider %q is not a built-in openai or anthropic provider; use standard mode with a built-in provider or gateway mode with an absolute base_url", profile.Name, profile.Provider)
		}
		return providerEndpoint{provider: profile.Provider, host: definition.Host, port: 443}, nil
	}

	endpoint, err := url.Parse(strings.TrimSpace(profile.BaseURL))
	if err != nil {
		if port := invalidBaseURLPort(profile.BaseURL); port != "" {
			return providerEndpoint{}, fmt.Errorf("profile %q cannot derive its provider host: base_url %q has invalid port %q; use a port from 1 to 65535", profile.Name, profile.BaseURL, port)
		}
		return providerEndpoint{}, invalidGatewayBaseURLError(profile.Name, profile.BaseURL)
	}
	if !endpoint.IsAbs() || endpoint.Hostname() == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return providerEndpoint{}, invalidGatewayBaseURLError(profile.Name, profile.BaseURL)
	}
	port := 443
	if endpoint.Scheme == "http" {
		port = 80
	}
	if endpoint.Port() != "" {
		parsed, err := strconv.Atoi(endpoint.Port())
		if err != nil || parsed < 1 || parsed > 65535 {
			return providerEndpoint{}, fmt.Errorf("profile %q cannot derive its provider host: base_url %q has invalid port %q; use a port from 1 to 65535", profile.Name, profile.BaseURL, endpoint.Port())
		}
		port = parsed
	}
	return providerEndpoint{provider: profile.Provider, host: strings.ToLower(endpoint.Hostname()), port: port}, nil
}

func invalidGatewayBaseURLError(profileName, baseURL string) error {
	return fmt.Errorf("profile %q is invalid: gateway mode requires an absolute base_url with a host and http(s) scheme (for example https://models.example.com/v1)", profileName)
}

func invalidBaseURLPort(rawURL string) string {
	withoutScheme := rawURL
	if index := strings.Index(rawURL, "://"); index >= 0 {
		withoutScheme = rawURL[index+3:]
	}
	if index := strings.IndexAny(withoutScheme, "/?#"); index >= 0 {
		withoutScheme = withoutScheme[:index]
	}
	if index := strings.LastIndex(withoutScheme, ":"); index >= 0 {
		return withoutScheme[index+1:]
	}
	return ""
}

func providerRuleName(host string, port int) string {
	slug := strings.Trim(strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, strings.ToLower(host)), "-")
	name := fmt.Sprintf("provider-%s-%d", slug, port)
	if len(name) <= 63 {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	return name[:56] + "-" + hex.EncodeToString(sum[:3])
}

func (a *App) legacyPolicyNetwork(profile Profile) (map[string]NetworkRule, string, error) {
	if isGeneratedDefaultPolicy(a.Store, profile) {
		return nil, "", nil
	}
	legacy, err := os.ReadFile(profile.Policy)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("profile %q references missing legacy policy file %s; restore it, select another policy, or use the generated default", profile.Name, profile.Policy)
		}
		return nil, "", err
	}
	network, err := legacyNetworkForAgent(legacy, profile.Agent)
	if err != nil {
		return nil, "", fmt.Errorf("profile %q legacy policy %s is invalid: %v; fix it or select the generated default", profile.Name, profile.Policy, err)
	}
	return network, profile.Policy, nil
}

func legacyNetworkForAgent(policy []byte, agent string) (map[string]NetworkRule, error) {
	if _, err := ParsePolicy(policy); err != nil {
		return nil, err
	}
	converted, err := PolicyForAgent(policy, agent)
	if err != nil {
		return nil, err
	}
	parsed, err := ParsePolicy(converted)
	if err != nil {
		return nil, err
	}
	return parsed.Network, nil
}

func deriveRunPolicy(profile Profile, gateway string, legacy map[string]NetworkRule, legacyPath string, runImage RunImage) ([]byte, *RunPolicyProvenance, error) {
	endpoint, err := deriveProviderEndpoint(profile)
	if err != nil {
		return nil, nil, err
	}
	agentBinary, err := AgentBinary(profile.Agent)
	if err != nil {
		return nil, nil, fmt.Errorf("profile %q cannot derive its agent binary: %v", profile.Name, err)
	}
	var policy Policy
	if err := yaml.Unmarshal(DefaultPolicy(), &policy); err != nil {
		return nil, nil, err
	}
	policy.Network = map[string]NetworkRule{}
	ruleName := providerRuleName(endpoint.host, endpoint.port)
	providerRule := NetworkRule{
		Name:      ruleName,
		Endpoints: []Endpoint{{Host: endpoint.host, Port: endpoint.port, Protocol: "tcp"}},
		Binaries:  []Binary{{Path: agentBinary}},
	}
	if err := mergeNetworkRule(&policy, ruleName, providerRule, "generated provider rule"); err != nil {
		return nil, nil, fmt.Errorf("profile %q cannot derive its run policy: %w", profile.Name, err)
	}
	for _, name := range sortedNetworkRuleNames(legacy) {
		rule := legacy[name]
		rule.Name = name
		if err := mergeNetworkRule(&policy, name, rule, "legacy policy "+legacyPath); err != nil {
			return nil, nil, fmt.Errorf("profile %q cannot derive its run policy: %w", profile.Name, err)
		}
	}
	if runImage.Source == ImageSourceRepo {
		var fragment Policy
		if err := yaml.Unmarshal(runImage.fragment, &fragment); err != nil {
			return nil, nil, fmt.Errorf("profile %q cannot compose repository policy fragment %s: %v", profile.Name, runImage.FragmentPath, err)
		}
		if _, err := ParsePolicy(runImage.fragment); err != nil {
			return nil, nil, fmt.Errorf("profile %q cannot compose repository policy fragment %s: %v", profile.Name, runImage.FragmentPath, err)
		}
		for _, name := range sortedNetworkRuleNames(fragment.Network) {
			rule := fragment.Network[name]
			rule.Name = name
			if err := mergeNetworkRule(&policy, name, rule, "repository fragment "+runImage.FragmentPath); err != nil {
				return nil, nil, fmt.Errorf("profile %q cannot derive its run policy: %w", profile.Name, err)
			}
		}
	}
	out, err := yaml.Marshal(policy)
	if err != nil {
		return nil, nil, fmt.Errorf("profile %q cannot derive its run policy: %w", profile.Name, err)
	}
	if _, err := ParsePolicy(out); err != nil {
		return nil, nil, fmt.Errorf("profile %q cannot derive its run policy: %w", profile.Name, err)
	}
	provenance := &RunPolicyProvenance{
		Source: "derived", Gateway: gateway, Provider: endpoint.provider,
		Mode: profile.Mode, Host: endpoint.host, Port: endpoint.port,
		Rule: ruleName, AgentBinary: agentBinary, LegacyPath: legacyPath,
	}
	if legacyPath != "" {
		provenance.Source = "derived+legacy"
	}
	return out, provenance, nil
}

func mergeNetworkRule(policy *Policy, name string, rule NetworkRule, source string) error {
	if policy.Network == nil {
		policy.Network = map[string]NetworkRule{}
	}
	if existing, ok := policy.Network[name]; ok {
		if reflect.DeepEqual(canonicalRule(existing), canonicalRule(rule)) {
			return nil
		}
		return fmt.Errorf("network rule %q is defined differently by %s and the derived policy; align them before running", name, source)
	}
	if _, ok := identicalNetworkRule(policy.Network, rule); ok {
		return nil
	}
	rule.Name = name
	policy.Network[name] = rule
	return nil
}

func sortedNetworkRuleNames(rules map[string]NetworkRule) []string {
	names := make([]string, 0, len(rules))
	for name := range rules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func identicalNetworkRule(rules map[string]NetworkRule, candidate NetworkRule) (string, bool) {
	for _, name := range sortedNetworkRuleNames(rules) {
		if reflect.DeepEqual(canonicalRule(rules[name]), canonicalRule(candidate)) {
			return name, true
		}
	}
	return "", false
}
