package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validRepoConfigYAML = `schema: 1

image:
  packages:
    - make
  toolchains:
    - name: go
      url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz
      sha256: 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840
      bin: /usr/local/go/bin/go

verify:
  - go version
  - make --version

hosts:
  - host: proxy.golang.org
    port: 443
    binaries: [/usr/local/go/bin/go]
  - host: sum.golang.org
    port: 443
    binaries: [/usr/local/go/bin/go]
  - host: registry.npmjs.org
    port: 443
    binaries: [/usr/local/bin/npm, /usr/local/bin/node]
  - host: github.com
    port: 22
    binaries: [/usr/bin/git]
`

func TestParseRepoConfigValid(t *testing.T) {
	cases := map[string]struct {
		yaml  string
		check func(t *testing.T, cfg RepoConfig)
	}{
		"full manifest": {
			yaml: validRepoConfigYAML,
			check: func(t *testing.T, cfg RepoConfig) {
				if cfg.Schema != 1 {
					t.Fatalf("schema = %d", cfg.Schema)
				}
				if got := cfg.Image.Packages; len(got) != 1 || got[0] != "make" {
					t.Fatalf("packages = %v", got)
				}
				tc := cfg.Image.Toolchains
				if len(tc) != 1 || tc[0].Name != "go" || tc[0].Bin != "/usr/local/go/bin/go" || tc[0].URL != "https://go.dev/dl/go1.27.4.linux-amd64.tar.gz" {
					t.Fatalf("toolchains = %+v", tc)
				}
				if len(cfg.Verify) != 2 || cfg.Verify[0] != "go version" || cfg.Verify[1] != "make --version" {
					t.Fatalf("verify = %v", cfg.Verify)
				}
				if len(cfg.Hosts) != 4 {
					t.Fatalf("hosts = %+v", cfg.Hosts)
				}
				h := cfg.Hosts[3]
				if h.Host != "github.com" || h.Port != 22 || len(h.Binaries) != 1 || h.Binaries[0] != "/usr/bin/git" {
					t.Fatalf("github.com rule = %+v", h)
				}
			},
		},
		"schema only": {
			yaml: "schema: 1\n",
			check: func(t *testing.T, cfg RepoConfig) {
				if len(cfg.Image.Packages) != 0 || len(cfg.Image.Toolchains) != 0 || len(cfg.Verify) != 0 || len(cfg.Hosts) != 0 {
					t.Fatalf("expected empty sections: %+v", cfg)
				}
			},
		},
		"empty hosts": {
			yaml: "schema: 1\nhosts: []\n",
			check: func(t *testing.T, cfg RepoConfig) {
				if len(cfg.Hosts) != 0 {
					t.Fatalf("hosts = %+v", cfg.Hosts)
				}
			},
		},
		"single-label host": {
			yaml: "schema: 1\nhosts:\n  - host: localhost\n    port: 443\n    binaries: [/usr/bin/curl]\n",
			check: func(t *testing.T, cfg RepoConfig) {
				if cfg.Hosts[0].Host != "localhost" {
					t.Fatalf("host = %q", cfg.Hosts[0].Host)
				}
			},
		},
		"glob binary path": {
			yaml: "schema: 1\nhosts:\n  - host: github.com\n    port: 443\n    binaries: [/usr/lib/git-core/**]\n",
			check: func(t *testing.T, cfg RepoConfig) {
				if got := cfg.Hosts[0].Binaries[0]; got != "/usr/lib/git-core/**" {
					t.Fatalf("binary = %q", got)
				}
			},
		},
		"uppercase sha256 normalized": {
			yaml: "schema: 1\nimage:\n  toolchains:\n    - name: go\n      url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz\n      sha256: 7226736C2339719CDA28FBF3A54D71AA63BCEFE376E98CEF00034268DAB24840\n      bin: /usr/local/go/bin/go\n",
			check: func(t *testing.T, cfg RepoConfig) {
				want := strings.ToLower("7226736C2339719CDA28FBF3A54D71AA63BCEFE376E98CEF00034268DAB24840")
				if got := cfg.Image.Toolchains[0].SHA256; got != want {
					t.Fatalf("sha256 = %q; want %q", got, want)
				}
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := ParseRepoConfig([]byte(tc.yaml))
			if err != nil {
				t.Fatalf("valid manifest rejected: %v", err)
			}
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

func TestParseRepoConfigRejections(t *testing.T) {
	const toolchain = "schema: 1\nimage:\n  toolchains:\n    - name: go\n      url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz\n      sha256: 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840\n      bin: /usr/local/go/bin/go\n"
	const host = "schema: 1\nhosts:\n  - host: proxy.golang.org\n    port: 443\n    binaries: [/usr/local/go/bin/go]\n"
	cases := map[string]struct {
		yaml string
		want []string
	}{
		"schema version":               {"schema: 2\n", []string{"schema"}},
		"schema missing":               {"image:\n  packages: [make]\n", []string{"schema"}},
		"empty file":                   {"", []string{"schema"}},
		"unknown field":                {"schema: 1\nbogus: true\n", []string{"bogus"}},
		"package is a URL":             {"schema: 1\nimage:\n  packages: [https://example.com/make.deb]\n", []string{"image.packages[0]"}},
		"package version pin":          {"schema: 1\nimage:\n  packages: [make=4.3-4.1]\n", []string{"image.packages[0]"}},
		"package shell metacharacters": {"schema: 1\nimage:\n  packages: ['make; rm -rf /']\n", []string{"image.packages[0]"}},
		"package uppercase":            {"schema: 1\nimage:\n  packages: [Make]\n", []string{"image.packages[0]"}},
		"package empty":                {"schema: 1\nimage:\n  packages: ['']\n", []string{"image.packages[0]"}},
		"toolchain name missing": {
			"schema: 1\nimage:\n  toolchains:\n    - url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz\n      sha256: 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840\n      bin: /usr/local/go/bin/go\n",
			[]string{"image.toolchains[0].name"},
		},
		"toolchain name invalid": {
			"schema: 1\nimage:\n  toolchains:\n    - name: Go_1\n      url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz\n      sha256: 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840\n      bin: /usr/local/go/bin/go\n",
			[]string{"image.toolchains[0].name"},
		},
		"toolchain url not https": {
			"schema: 1\nimage:\n  toolchains:\n    - name: go\n      url: http://go.dev/dl/go1.27.4.linux-amd64.tar.gz\n      sha256: 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840\n      bin: /usr/local/go/bin/go\n",
			[]string{"image.toolchains[0].url"},
		},
		"toolchain url not a URL": {
			"schema: 1\nimage:\n  toolchains:\n    - name: go\n      url: go.dev/dl/go1.27.4.linux-amd64.tar.gz\n      sha256: 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840\n      bin: /usr/local/go/bin/go\n",
			[]string{"image.toolchains[0].url"},
		},
		"toolchain sha256 short": {
			"schema: 1\nimage:\n  toolchains:\n    - name: go\n      url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz\n      sha256: abc123\n      bin: /usr/local/go/bin/go\n",
			[]string{"image.toolchains[0].sha256"},
		},
		"toolchain sha256 not hex": {
			"schema: 1\nimage:\n  toolchains:\n    - name: go\n      url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz\n      sha256: zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz\n      bin: /usr/local/go/bin/go\n",
			[]string{"image.toolchains[0].sha256"},
		},
		"toolchain bin relative": {
			"schema: 1\nimage:\n  toolchains:\n    - name: go\n      url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz\n      sha256: 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840\n      bin: go\n",
			[]string{"image.toolchains[0].bin"},
		},
		"duplicate toolchain names": {
			toolchain + "    - name: go\n      url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz\n      sha256: 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840\n      bin: /usr/local/go/bin/go2\n",
			[]string{"image.toolchains[1].name"},
		},
		"duplicate toolchain bins": {
			toolchain + "    - name: go2\n      url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz\n      sha256: 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840\n      bin: /usr/local/go/bin/go\n",
			[]string{"image.toolchains[1].bin"},
		},
		"verify empty":                 {"schema: 1\nverify: ['']\n", []string{"verify[0]"}},
		"verify newline":               {"schema: 1\nverify: [\"go version\\nmake --version\"]\n", []string{"verify[0]"}},
		"verify leading whitespace":    {"schema: 1\nverify: [' go version']\n", []string{"verify[0]"}},
		"verify trailing continuation": {"schema: 1\nverify: ['go version \\']\n", []string{"verify[0]"}},
		"host wildcard":                {"schema: 1\nhosts:\n  - host: '*.example.com'\n    port: 443\n    binaries: [/usr/bin/curl]\n", []string{"hosts[0].host"}},
		"host with scheme":             {"schema: 1\nhosts:\n  - host: https://example.com\n    port: 443\n    binaries: [/usr/bin/curl]\n", []string{"hosts[0].host"}},
		"host with path":               {"schema: 1\nhosts:\n  - host: example.com/api\n    port: 443\n    binaries: [/usr/bin/curl]\n", []string{"hosts[0].host"}},
		"host with port suffix":        {"schema: 1\nhosts:\n  - host: example.com:443\n    port: 443\n    binaries: [/usr/bin/curl]\n", []string{"hosts[0].host"}},
		"host CIDR":                    {"schema: 1\nhosts:\n  - host: 10.0.0.0/8\n    port: 443\n    binaries: [/usr/bin/curl]\n", []string{"hosts[0].host"}},
		"host underscore":              {"schema: 1\nhosts:\n  - host: sum_golang.org\n    port: 443\n    binaries: [/usr/bin/curl]\n", []string{"hosts[0].host"}},
		"host label too long":          {"schema: 1\nhosts:\n  - host: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.example.com\n    port: 443\n    binaries: [/usr/bin/curl]\n", []string{"hosts[0].host"}},
		"host empty":                   {"schema: 1\nhosts:\n  - host: ''\n    port: 443\n    binaries: [/usr/bin/curl]\n", []string{"hosts[0].host"}},
		"port missing":                 {"schema: 1\nhosts:\n  - host: example.com\n    binaries: [/usr/bin/curl]\n", []string{"hosts[0].port"}},
		"port too high":                {"schema: 1\nhosts:\n  - host: example.com\n    port: 70000\n    binaries: [/usr/bin/curl]\n", []string{"hosts[0].port"}},
		"binaries empty":               {"schema: 1\nhosts:\n  - host: example.com\n    port: 443\n    binaries: []\n", []string{"hosts[0].binaries"}},
		"binary relative":              {"schema: 1\nhosts:\n  - host: example.com\n    port: 443\n    binaries: [git]\n", []string{"hosts[0].binaries[0]"}},
		"binary traversal":             {"schema: 1\nhosts:\n  - host: example.com\n    port: 443\n    binaries: [/usr/bin/../bin/git]\n", []string{"hosts[0].binaries[0]"}},
		"duplicate host and port":      {host + "  - host: proxy.golang.org\n    port: 443\n    binaries: [/usr/bin/curl]\n", []string{"hosts[1]"}},
		"duplicate host ignoring case": {host + "  - host: PROXY.GOLANG.ORG\n    port: 443\n    binaries: [/usr/bin/curl]\n", []string{"hosts[1]"}},
		"secret-like field name":       {"schema: 1\ntoken: abc\n", []string{"token", "credential-free"}},
		"secret-like nested field":     {"schema: 1\nimage:\n  password: hunter2\n", []string{"password", "credential-free"}},
		"secret in toolchain url": {
			"schema: 1\nimage:\n  toolchains:\n    - name: go\n      url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz?token=abc123\n      sha256: 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840\n      bin: /usr/local/go/bin/go\n",
			[]string{"image.toolchains[0].url", "credential-free"},
		},
		"high entropy value":   {"schema: 1\nimage:\n  packages: [ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefgh]\n", []string{"image.packages[0]", "credential-free"}},
		"alias-hidden secret":  {"schema: 1\nverify:\n  - &cmd ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefgh\n", []string{"verify[0]", "credential-free"}},
		"binary-encoded value": {"schema: 1\nverify:\n  - !!binary Z2hwX3NlY3JldA==\n", []string{"verify[0]", "credential-free"}},
		"invalid yaml":         {"schema: [1\n", []string{"invalid YAML"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseRepoConfig([]byte(tc.yaml))
			if err == nil {
				t.Fatal("invalid manifest accepted")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not mention %q", err.Error(), want)
				}
			}
		})
	}
}

func writeRepoConfig(t *testing.T, root, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".agentbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(RepoConfigPath(root), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRepoConfigMissingDistinctFromInvalid(t *testing.T) {
	root := t.TempDir()
	_, err := LoadRepoConfig(root)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing manifest error = %v; want os.ErrNotExist", err)
	}

	writeRepoConfig(t, root, "schema: 2\n")
	_, err = LoadRepoConfig(root)
	if err == nil {
		t.Fatal("invalid manifest accepted")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid manifest reported as missing")
	}
	if !strings.Contains(err.Error(), "schema") || !strings.Contains(err.Error(), RepoConfigPath(root)) {
		t.Fatalf("error %q does not name the field or the file", err)
	}
}

func TestLoadRepoConfigValid(t *testing.T) {
	root := t.TempDir()
	writeRepoConfig(t, root, validRepoConfigYAML)
	cfg, err := LoadRepoConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Hosts[0].Host; got != "proxy.golang.org" {
		t.Fatalf("first host = %q", got)
	}
	if got := RepoConfigPath(root); got != filepath.Join(root, ".agentbox", "config.yaml") {
		t.Fatalf("RepoConfigPath = %q", got)
	}
}

// TestCommittedRepoConfig keeps the repository's own manifest parseable so a
// typo fails in CI instead of silently disabling the declarations at run time.
func TestCommittedRepoConfig(t *testing.T) {
	cfg, err := LoadRepoConfig(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("committed manifest is missing or invalid: %v", err)
	}
	if len(cfg.Image.Toolchains) == 0 {
		t.Fatal("committed manifest declares no toolchains; Go would be unavailable in the sandbox")
	}
	if len(cfg.Hosts) == 0 {
		t.Fatal("committed manifest declares no runtime hosts; module downloads would be blocked")
	}
}
