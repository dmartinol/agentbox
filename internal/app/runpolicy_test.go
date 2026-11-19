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
		{baseURL: "https://Models.Example.com/v1", host: "models.example.com", port: 443},
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

func TestDerivedPolicyDefersProviderEndpointToOpenShell(t *testing.T) {
	profile := Profile{Name: "gateway", Agent: "codex", Mode: "gateway", Provider: "enmaas", BaseURL: "https://models.example.com:8443"}
	policy, provenance, err := deriveRunPolicy(profile, "gateway", nil, "", RunImage{})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Network) != 0 {
		t.Fatalf("derived policy wrote provider rules OpenShell already composes: %+v", parsed.Network)
	}
	if provenance.Rule != "_provider_enmaas" {
		t.Fatalf("provider rule = %q, want the composed OpenShell rule", provenance.Rule)
	}
	if provenance.AgentBinary != "/usr/local/bin/codex" {
		t.Fatalf("agent binary = %q", provenance.AgentBinary)
	}
	if provenance.Host != "models.example.com" || provenance.Port != 8443 {
		t.Fatalf("provenance endpoint = %s:%d", provenance.Host, provenance.Port)
	}
}

func TestDerivedPolicyLowercasesGatewayHost(t *testing.T) {
	profile := Profile{Name: "gateway", Agent: "codex", Mode: "gateway", Provider: "enmaas", BaseURL: "https://Models.Example.com/v1"}
	policy, provenance, err := deriveRunPolicy(profile, "gateway", nil, "", RunImage{})
	if err != nil {
		t.Fatal(err)
	}
	if provenance.Host != "models.example.com" {
		t.Fatalf("provenance host = %q, want lower-case host", provenance.Host)
	}
	parsed, err := ParsePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Network) != 0 {
		t.Fatalf("derived policy = %+v, want no network rules", parsed.Network)
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
	if len(parsed.Network) != 0 || parsed.Process.User != "sandbox" || parsed.Landlock.Compatibility != "hard_requirement" {
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
	if len(parsed.Network) != 1 || parsed.Network["legacy-host"].Binaries[0].Path != "/usr/local/bin/codex" {
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
	shared := NetworkRule{
		Endpoints: []Endpoint{{Host: "proxy.example.com", Port: 443, Protocol: "tcp"}},
		Binaries:  []Binary{{Path: "/usr/local/bin/codex"}},
	}
	legacy := map[string]NetworkRule{"legacy-proxy": shared}
	fragmentRule := shared
	fragmentRule.Name = "host-proxy-example-com-443"
	fragment := testPolicyBytes(t, map[string]NetworkRule{"host-proxy-example-com-443": fragmentRule})
	runImage := RunImage{Source: ImageSourceRepo, FragmentPath: "repo.yaml", fragment: fragment}
	profile := Profile{Name: "standard", Agent: "codex", Mode: "standard", Provider: "openai"}
	policy, _, err := deriveRunPolicy(profile, "openshell", legacy, "/tmp/legacy.yaml", runImage)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Network) != 1 || parsed.Network["legacy-proxy"].Name != "legacy-proxy" {
		t.Fatalf("deduplicated policy = %+v", parsed.Network)
	}

	different := fragmentRule
	different.Binaries = []Binary{{Path: "/usr/local/go/bin/go"}}
	fragment = testPolicyBytes(t, map[string]NetworkRule{"legacy-proxy": different})
	runImage.fragment = fragment
	_, _, err = deriveRunPolicy(profile, "openshell", legacy, "/tmp/legacy.yaml", runImage)
	if err == nil || !strings.Contains(err.Error(), `profile "standard"`) || !strings.Contains(err.Error(), `network rule "legacy-proxy"`) {
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
	legacy := map[string]NetworkRule{
		"legacy-proxy": {
			Endpoints: []Endpoint{{Host: "proxy.example.com", Port: 443, Protocol: "tcp"}},
			Binaries:  []Binary{{Path: "/usr/local/bin/codex"}},
		},
	}
	fragmentRule := NetworkRule{
		Name:      "host-proxy-example-com-443",
		Endpoints: []Endpoint{{Host: "proxy.example.com", Port: 443, Protocol: "tcp"}},
		Binaries:  []Binary{{Path: "/usr/local/go/bin/go"}},
	}
	fragment := testPolicyBytes(t, map[string]NetworkRule{"host-proxy-example-com-443": fragmentRule})
	runImage := RunImage{Source: ImageSourceRepo, FragmentPath: "repo.yaml", fragment: fragment}
	policy, _, err := deriveRunPolicy(Profile{Name: "standard", Agent: "codex", Mode: "standard", Provider: "openai"}, "openshell", legacy, "/tmp/legacy.yaml", runImage)
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

func TestDerivedPolicyRejectsDuplicatedProviderEndpoint(t *testing.T) {
	profile := Profile{Name: "standard", Agent: "codex", Mode: "standard", Provider: "openai"}
	duplicate := map[string]NetworkRule{
		"legacy-openai": {
			Endpoints: []Endpoint{{Host: "API.OpenAI.com", Port: 443, Protocol: "tcp"}},
			Binaries:  []Binary{{Path: "/usr/local/bin/codex"}},
		},
	}
	_, _, err := deriveRunPolicy(profile, "openshell", duplicate, "/tmp/legacy.yaml", RunImage{})
	if err == nil || !strings.Contains(err.Error(), `provider "openai"`) || !strings.Contains(err.Error(), "api.openai.com:443") {
		t.Fatalf("legacy duplicate error = %v", err)
	}

	fragment := testPolicyBytes(t, duplicate)
	runImage := RunImage{Source: ImageSourceRepo, FragmentPath: "repo.yaml", fragment: fragment}
	_, _, err = deriveRunPolicy(profile, "openshell", nil, "", runImage)
	if err == nil || !strings.Contains(err.Error(), "repository fragment repo.yaml") {
		t.Fatalf("fragment duplicate error = %v", err)
	}
}
