package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestParsePolicyDenialsReadsRealSandboxSamples(t *testing.T) {
	output := []byte(`[1795092202.461] [sandbox] [OCSF ] [ocsf] NET:REFUSE [MED] DENIED example.com [reason:policy_dns_ineligible]
[1795092202.467] [sandbox] [OCSF ] [ocsf] NET:OPEN [MED] DENIED /usr/bin/curl(0) -> example.com:443 [reason:transparent_tcp_policy_denied]
[1795092241.660] [sandbox] [OCSF ] [ocsf] NET:OPEN [MED] DENIED /usr/local/lib/node_modules/@openai/codex/node_modules/@openai/codex-linux-arm64/vendor/aarch64-unknown-linux-musl/bin/codex(0) -> ab.chatgpt.com:443 [reason:transparent_tcp_policy_denied]
2026-04-01T04:04:13.074Z OCSF NET:OPEN [INFO] ALLOWED /usr/bin/curl(58) -> api.github.com:443 [policy:github_api engine:opa]
2026-04-01T04:04:13.074Z OCSF HTTP:POST [MED] DENIED POST https://api.github.com/user/repos [policy:github_api engine:opa]
`)
	denials := ParsePolicyDenials(output)
	if len(denials) != 2 {
		t.Fatalf("denials = %#v; want two actionable network denials", denials)
	}
	if denials[0].Host != "example.com" || denials[0].Port != 443 || denials[0].Binary != "/usr/bin/curl" {
		t.Fatalf("first denial = %#v", denials[0])
	}
	if denials[0].Reason != "transparent_tcp_policy_denied" || !denials[0].Valid {
		t.Fatalf("first reasoning = %#v", denials[0])
	}
	if denials[1].Host != "ab.chatgpt.com" || denials[1].Binary == "/usr/local/bin/codex" {
		t.Fatalf("second denial = %#v; the sandbox binary must be preserved exactly", denials[1])
	}
}

func TestParsePolicyDenialsForRunCanonicalizesAVendoredAgentBinary(t *testing.T) {
	output := []byte(`[1795092241.660] [sandbox] [OCSF ] [ocsf] NET:OPEN [MED] DENIED /usr/local/lib/node_modules/@openai/codex/vendor/abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN012345/bin/codex(0) -> ab.chatgpt.com:443 [reason:transparent_tcp_policy_denied]
`)
	denials := ParsePolicyDenialsForRun(output, "/usr/local/bin/codex")
	if len(denials) != 1 {
		t.Fatalf("denials = %#v", denials)
	}
	denial := denials[0]
	if !denial.Valid || denial.Binary != "/usr/local/bin/codex" {
		t.Fatalf("denial = %#v, want the canonical agent binary", denial)
	}
	if denial.ObservedBinary != "/usr/local/lib/node_modules/@openai/codex/vendor/abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN012345/bin/codex" {
		t.Fatalf("observed binary = %q", denial.ObservedBinary)
	}
}

func TestParsePolicyDenialsSupportsLegacyFieldsAndDeduplicates(t *testing.T) {
	output := []byte(`action=deny dst_host=api.github.com dst_port=443 binary=/usr/bin/curl deny_reason="no matching network policy"
action=deny dst_host=api.github.com dst_port=443 binary=/usr/bin/curl deny_reason="no matching network policy"
OCSF NET:OPEN [MED] DENIED /usr/bin/curl(64) -> httpbin.org:443 [policy:- engine:opa] [reason:no matching policy]
`)
	denials := ParsePolicyDenials(output)
	if len(denials) != 2 {
		t.Fatalf("denials = %#v; want two unique entries", denials)
	}
	if denials[0].Host != "api.github.com" || denials[0].Reason != "no matching network policy" {
		t.Fatalf("legacy denial = %#v", denials[0])
	}
	if denials[1].Host != "httpbin.org" || denials[1].Port != 443 {
		t.Fatalf("ocsf denial = %#v", denials[1])
	}
}

func TestParsePolicyDenialsMarksUnsafeScopesInvalid(t *testing.T) {
	output := []byte(`OCSF NET:OPEN [MED] DENIED /usr/bin/curl(64) -> *.example.com:443 [reason:no matching policy]
`)
	denials := ParsePolicyDenials(output)
	if len(denials) != 1 || denials[0].Valid || denials[0].InvalidReason == "" {
		t.Fatalf("denials = %#v; wildcard host must stay visible but invalid", denials)
	}
}

func TestAppendRepoHostRuleIsMechanicalAndIdempotent(t *testing.T) {
	manifest := []byte(`schema: 1 # keep this comment
image:
  packages: [make]
verify:
  - make --version
hosts:
  - host: api.example.test
    port: 443
    binaries: [/usr/bin/curl]
`)
	rule := RepoHostRule{Host: "example.com", Port: 443, Binaries: []string{"/usr/bin/node"}}
	next, err := AppendRepoHostRule(manifest, rule)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(next), "# keep this comment") {
		t.Fatalf("comment was lost:\n%s", next)
	}
	cfg, err := ParseRepoConfig(next)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Hosts) != 2 || cfg.Hosts[1].Host != "example.com" || cfg.Hosts[1].Binaries[0] != "/usr/bin/node" {
		t.Fatalf("hosts = %+v", cfg.Hosts)
	}
	again, err := AppendRepoHostRule(next, rule)
	if err != nil || !bytes.Equal(again, next) {
		t.Fatalf("append was not idempotent: %v\n%s\n---\n%s", err, next, again)
	}
	merged, err := AppendRepoHostRule(next, RepoHostRule{Host: "api.example.test", Port: 443, Binaries: []string{"/usr/bin/node"}})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = ParseRepoConfig(merged)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Hosts) != 2 || len(cfg.Hosts[0].Binaries) != 2 {
		t.Fatalf("same host/port was not merged: %+v", cfg.Hosts)
	}
}

func TestAppendRepoHostRuleAllowsLongHyphenatedBinaryPath(t *testing.T) {
	manifest := []byte("schema: 1\nimage:\n  packages: []\nverify: []\nhosts: []\n")
	const uv = "/usr/local/uv-aarch64-unknown-linux-gnu/uv"
	rule := RepoHostRule{Host: "files.pythonhosted.org", Port: 443, Binaries: []string{uv}}
	if err := ValidateHostRule(rule); err != nil {
		t.Fatalf("long hyphenated binary path rejected: %v", err)
	}
	next, err := AppendRepoHostRule(manifest, rule)
	if err != nil {
		t.Fatalf("append rejected long hyphenated binary path: %v", err)
	}
	cfg, err := ParseRepoConfig(next)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Hosts) != 1 || cfg.Hosts[0].Host != "files.pythonhosted.org" || cfg.Hosts[0].Binaries[0] != uv {
		t.Fatalf("hosts = %+v", cfg.Hosts)
	}
}

func TestAppendRepoHostRuleRejectsUnsafeScopes(t *testing.T) {
	manifest := []byte("schema: 1\nimage:\n  packages: []\nverify: []\nhosts: []\n")
	if _, err := AppendRepoHostRule(manifest, RepoHostRule{Host: "*.example.com", Port: 443, Binaries: []string{"/usr/bin/curl"}}); err == nil {
		t.Fatal("wildcard host was accepted")
	}
	if _, err := AppendRepoHostRule(manifest, RepoHostRule{Host: "example.com", Port: 443, Binaries: []string{"/usr/bin/*"}}); err == nil {
		t.Fatal("binary wildcard was accepted")
	}
	if _, err := AppendRepoHostRule(manifest, RepoHostRule{Host: "example.com", Port: 443, Binaries: []string{"/usr/bin/curl token="}}); err == nil {
		t.Fatal("secret-like binary was accepted")
	}
}
