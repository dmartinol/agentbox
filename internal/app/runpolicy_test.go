package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveProviderEndpointStandardAndGateway(t *testing.T) {
	standard, err := deriveProviderEndpoint(Profile{Name: "standard", Mode: "standard", Provider: "anthropic"})
	if err != nil {
		t.Fatal(err)
	}
	if standard.host != "api.anthropic.com" || standard.port != 443 {
		t.Fatalf("standard endpoint = %+v", standard)
	}

	tests := []struct {
		baseURL string
		host    string
		port    int
	}{
		{baseURL: "https://Models.Example.com/v1", host: "Models.Example.com", port: 443},
		{baseURL: "http://models.example.com/v1", host: "models.example.com", port: 80},
		{baseURL: "https://models.example.com:8443/v1", host: "models.example.com", port: 8443},
	}
	for _, test := range tests {
		gateway, err := deriveProviderEndpoint(Profile{Name: "gateway", Mode: "gateway", Provider: "enmaas", BaseURL: test.baseURL})
		if err != nil {
			t.Fatal(err)
		}
		if gateway.host != test.host || gateway.port != test.port {
			t.Fatalf("gateway endpoint for %s = %+v", test.baseURL, gateway)
		}
	}
}

func TestDeriveProviderEndpointErrors(t *testing.T) {
	tests := []struct {
		profile Profile
		want    string
	}{
		{profile: Profile{Name: "custom", Mode: "standard", Provider: "enmaas"}, want: "is not a built-in openai or anthropic provider"},
		{profile: Profile{Name: "relative", Mode: "gateway", Provider: "enmaas", BaseURL: "models.example.com/v1"}, want: "gateway mode requires an absolute base_url"},
		{profile: Profile{Name: "scheme", Mode: "gateway", Provider: "enmaas", BaseURL: "ftp://models.example.com"}, want: "gateway mode requires an absolute base_url"},
		{profile: Profile{Name: "port", Mode: "gateway", Provider: "enmaas", BaseURL: "https://models.example.com:70000"}, want: `invalid port "70000"`},
	}
	for _, test := range tests {
		_, err := deriveProviderEndpoint(test.profile)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("error for %+v = %v, want %q", test.profile, err, test.want)
		}
	}
}

func TestProviderRuleShapeAndLongName(t *testing.T) {
	profile := Profile{Name: "gateway", Agent: "codex", Mode: "gateway", Provider: "enmaas", BaseURL: "https://models.example.com:8443"}
	policy, provenance, err := deriveRunPolicy(profile, "gateway", nil, "", RunImage{})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	rule := parsed.Network["provider-models-example-com-8443"]
	if len(rule.Endpoints) != 1 || rule.Endpoints[0] != (Endpoint{Host: "models.example.com", Port: 8443, Protocol: "tcp"}) {
		t.Fatalf("provider endpoints = %+v", rule)
	}
	if len(rule.Binaries) != 1 || rule.Binaries[0].Path != "/usr/local/bin/codex" {
		t.Fatalf("provider binaries = %+v", rule)
	}
	if provenance.Rule != "provider-models-example-com-8443" || provenance.AgentBinary != "/usr/local/bin/codex" {
		t.Fatalf("provenance = %+v", provenance)
	}

	longHost := strings.Repeat("a", 64) + ".example.com"
	profile.BaseURL = "https://" + longHost + ":443"
	policy, provenance, err = deriveRunPolicy(profile, "gateway", nil, "", RunImage{})
	if err != nil {
		t.Fatal(err)
	}
	if len(provenance.Rule) != 63 || !strings.HasPrefix(provenance.Rule, "provider-") {
		t.Fatalf("long rule = %q (%d)", provenance.Rule, len(provenance.Rule))
	}
	if provenance.Rule != "provider-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-0763ba" {
		t.Fatalf("long rule hash = %q", provenance.Rule)
	}
	if _, err := ParsePolicy(policy); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedDefaultPolicyIsIgnoredWhenMissing(t *testing.T) {
	store := Store{Base: t.TempDir()}
	profile := Profile{Name: "generated", Agent: "codex", Mode: "standard", Provider: "openai", Policy: filepath.Join(store.PolicyDir(), "generated.yaml")}
	app := App{Store: store}
	legacy, legacyPath, err := app.legacyPolicyNetwork(profile)
	if err != nil {
		t.Fatal(err)
	}
	if legacy != nil || legacyPath != "" {
		t.Fatalf("generated default was treated as legacy: %#v %q", legacy, legacyPath)
	}
	policy, provenance, err := deriveRunPolicy(profile, "openshell", nil, "", RunImage{})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Network) != 1 || parsed.Process.User != "sandbox" || parsed.Landlock.Compatibility != "hard_requirement" {
		t.Fatalf("derived policy = %+v", parsed)
	}
	if provenance.Source != "derived" || provenance.LegacyPath != "" {
		t.Fatalf("provenance = %+v", provenance)
	}
}

func TestLegacyPolicyAddsOnlyNetworkRules(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(store.PolicyDir(), "shared-legacy.yaml")
	legacy := strings.Replace(string(DefaultPolicy()), "run_as_user: sandbox", "run_as_user: attacker", 1)
	legacy = strings.Replace(legacy, "network_policies: {}", `network_policies:
  legacy-host:
    name: legacy-host
    endpoints:
      - host: legacy.example.com
        port: 443
    binaries:
      - path: /usr/local/bin/codex
`, 1)
	if err := os.WriteFile(legacyPath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	profile := Profile{Name: "legacy", Agent: "codex", Mode: "standard", Provider: "openai", Policy: legacyPath}
	app := App{Store: store}
	network, path, err := app.legacyPolicyNetwork(profile)
	if err != nil {
		t.Fatal(err)
	}
	if path != legacyPath || len(network) != 1 {
		t.Fatalf("legacy network = %#v path %q", network, path)
	}
	policy, provenance, err := deriveRunPolicy(profile, "openshell", network, path, RunImage{})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Process.User != "sandbox" || parsed.Landlock.Compatibility != "hard_requirement" {
		t.Fatalf("legacy policy replaced static sections: %+v", parsed)
	}
	if len(parsed.Network) != 2 || parsed.Network["legacy-host"].Binaries[0].Path != "/usr/local/bin/codex" {
		t.Fatalf("legacy network rules = %+v", parsed.Network)
	}
	if provenance.Source != "derived+legacy" || provenance.LegacyPath != legacyPath {
		t.Fatalf("provenance = %+v", provenance)
	}
}

func TestLegacyPolicyMissingAndInvalidErrors(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	profile := Profile{Name: "legacy", Agent: "codex", Mode: "standard", Provider: "openai", Policy: filepath.Join(store.PolicyDir(), "shared-legacy.yaml")}
	app := App{Store: store}
	_, _, err := app.legacyPolicyNetwork(profile)
	if err == nil || !strings.Contains(err.Error(), "references missing legacy policy file") {
		t.Fatalf("missing error = %v", err)
	}
	if err := os.WriteFile(profile.Policy, []byte("not: yaml"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err = app.legacyPolicyNetwork(profile)
	if err == nil || !strings.Contains(err.Error(), "legacy policy") || !strings.Contains(err.Error(), "is invalid") {
		t.Fatalf("invalid error = %v", err)
	}
}

func TestDerivedPolicyDeduplicatesAndPreservesConflicts(t *testing.T) {
	providerRule := NetworkRule{
		Endpoints: []Endpoint{{Host: "api.openai.com", Port: 443, Protocol: "tcp"}},
		Binaries:  []Binary{{Path: "/usr/local/bin/codex"}},
	}
	fragmentRule := providerRule
	fragmentRule.Name = "host-api-openai-com-443"
	fragment := testPolicyBytes(t, map[string]NetworkRule{"host-api-openai-com-443": fragmentRule})
	runImage := RunImage{Source: ImageSourceRepo, FragmentPath: "repo.yaml", fragment: fragment}
	profile := Profile{Name: "standard", Agent: "codex", Mode: "standard", Provider: "openai"}
	policy, _, err := deriveRunPolicy(profile, "openshell", nil, "", runImage)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Network) != 1 || parsed.Network["provider-api-openai-com-443"].Name != "provider-api-openai-com-443" {
		t.Fatalf("deduplicated policy = %+v", parsed.Network)
	}

	different := fragmentRule
	different.Binaries = []Binary{{Path: "/usr/local/go/bin/go"}}
	fragment = testPolicyBytes(t, map[string]NetworkRule{"provider-api-openai-com-443": different})
	runImage.fragment = fragment
	_, _, err = deriveRunPolicy(profile, "openshell", nil, "", runImage)
	if err == nil || !strings.Contains(err.Error(), `profile "standard"`) || !strings.Contains(err.Error(), `network rule "provider-api-openai-com-443"`) {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestDerivedPolicyMergesLegacyRulesInDeterministicOrder(t *testing.T) {
	endpoints := []Endpoint{{Host: "legacy.example.com", Port: 443, Protocol: "tcp"}}
	binaries := []Binary{{Path: "/usr/local/bin/codex"}}
	legacy := map[string]NetworkRule{
		"zeta-legacy":  {Endpoints: endpoints, Binaries: binaries},
		"alpha-legacy": {Endpoints: endpoints, Binaries: binaries},
	}
	profile := Profile{Name: "standard", Agent: "codex", Mode: "standard", Provider: "openai"}
	for attempt := 0; attempt < 20; attempt++ {
		policy, _, err := deriveRunPolicy(profile, "openshell", legacy, "/tmp/legacy.yaml", RunImage{})
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		parsed, err := ParsePolicy(policy)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := parsed.Network["alpha-legacy"]; !ok {
			t.Fatalf("attempt %d kept %+v, want the alphabetically first rule", attempt, parsed.Network)
		}
		if _, ok := parsed.Network["zeta-legacy"]; ok {
			t.Fatalf("attempt %d duplicated a canonical-equal rule: %+v", attempt, parsed.Network)
		}
	}
}

func TestDerivedPolicyKeepsSameEndpointDifferentBinariesSeparate(t *testing.T) {
	fragmentRule := NetworkRule{
		Name:      "host-proxy-golang-org-443",
		Endpoints: []Endpoint{{Host: "api.openai.com", Port: 443, Protocol: "tcp"}},
		Binaries:  []Binary{{Path: "/usr/local/go/bin/go"}},
	}
	fragment := testPolicyBytes(t, map[string]NetworkRule{"host-proxy-golang-org-443": fragmentRule})
	runImage := RunImage{Source: ImageSourceRepo, FragmentPath: "repo.yaml", fragment: fragment}
	policy, _, err := deriveRunPolicy(Profile{Name: "standard", Agent: "codex", Mode: "standard", Provider: "openai"}, "openshell", nil, "", runImage)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Network) != 2 {
		t.Fatalf("same endpoint with different binaries collapsed: %+v", parsed.Network)
	}
}
