package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeOrchestrationRegistry(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "orchestration.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadOrchestrationRegistry(t *testing.T, body string) *OrchestrationRegistry {
	t.Helper()
	registry, err := LoadOrchestrationRegistry(writeOrchestrationRegistry(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if registry == nil {
		t.Fatal("registry is nil")
	}
	return registry
}

func resolveRegistry(t *testing.T, registry *OrchestrationRegistry, ref OrchestrationProjectRef) ResolvedOrchestration {
	t.Helper()
	resolved, err := ResolveOrchestration(registry, "test.yaml", ref)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func wantSelection(t *testing.T, name string, selection ResolvedOrchestrationSelection, runtime, agent, profile, model, effort, source string) {
	t.Helper()
	if selection.Runtime != runtime || selection.Agent != agent || selection.Profile != profile || selection.Model != model || selection.Effort != effort || selection.Source != source {
		t.Fatalf("%s = %+v; want runtime=%q agent=%q profile=%q model=%q effort=%q source=%q", name, selection, runtime, agent, profile, model, effort, source)
	}
}

func TestOrchestrationRegistryExampleResolves(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "examples", "orchestration.yaml")
	registry, err := LoadOrchestrationRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if registry == nil {
		t.Fatal("example registry is missing")
	}
	resolved, err := ResolveOrchestration(registry, path, OrchestrationProjectRef{
		Root:   "/Users/example/projects/agentbox",
		Remote: "git@github.com:dmartinol/agentbox.git",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.Present || resolved.Default != "standard" || resolved.Scope != "project" || resolved.Project != "agentbox" {
		t.Fatalf("resolved header = %+v", resolved)
	}
	if got, want := strings.Join(resolved.Sources, ","), "default:standard,global.config:standard,global,project:agentbox.config:heavy,project:agentbox"; got != want {
		t.Fatalf("sources = %q; want %q", got, want)
	}
	wantSelection(t, "orchestrator", resolved.Orchestrator, "orca", "codex", "", "<orchestrator-model-id>", "high", "global.config:standard")
	wantSelection(t, "triage", resolved.Stages["triage"], "orca", "codex", "", "<worker-model-id>", "medium", "global.config:standard.default")
	wantSelection(t, "plan", resolved.Stages["plan"], "orca", "codex", "enmaas", "<heavy-coding-model-id>", "high", "project:agentbox.config:heavy.group:coding")
	wantSelection(t, "implement", resolved.Stages["implement"], "orca", "codex", "enmaas", "<heavy-coding-model-id>", "high", "project:agentbox.stage:implement")
	wantSelection(t, "verify", resolved.Stages["verify"], "orca", "codex", "", "<worker-model-id>", "medium", "global.config:standard.default")
	wantSelection(t, "review", resolved.Stages["review"], "orca", "claude", "", "<reviewer-model-id>", "high", "global.config:standard.stage:review")
}

func TestResolveOrchestrationGlobalOnly(t *testing.T) {
	registry := loadOrchestrationRegistry(t, `
version: 1
default: base
configs:
  base:
    workers:
      default: {runtime: orca, agent: codex, model: base-model, effort: medium}
      groups:
        coding: {effort: high}
      stages:
        review: {model: review-model}
global:
  workers:
    groups:
      other: {agent: claude}
`)
	resolved := resolveRegistry(t, registry, OrchestrationProjectRef{})
	if resolved.Scope != "global" || resolved.Project != "" {
		t.Fatalf("scope = %q project = %q", resolved.Scope, resolved.Project)
	}
	wantSelection(t, "plan", resolved.Stages["plan"], "orca", "codex", "", "base-model", "high", "default:base.group:coding")
	wantSelection(t, "triage", resolved.Stages["triage"], "orca", "claude", "", "base-model", "medium", "global.group:other")
	wantSelection(t, "review", resolved.Stages["review"], "orca", "claude", "", "review-model", "medium", "global.group:other")
}

func TestResolveOrchestrationExplicitDefault(t *testing.T) {
	registry := loadOrchestrationRegistry(t, `
version: 1
default: fast
configs:
  fast:
    orchestration: {runtime: harness, agent: claude}
    workers:
      default: {runtime: harness, agent: claude, model: fast-model}
  slow:
    workers:
      default: {runtime: orca, agent: codex, model: slow-model}
`)
	resolved := resolveRegistry(t, registry, OrchestrationProjectRef{})
	if resolved.Scope != "default" || resolved.Default != "fast" {
		t.Fatalf("resolved = %+v", resolved)
	}
	if got, want := strings.Join(resolved.Sources, ","), "default:fast"; got != want {
		t.Fatalf("sources = %q; want %q", got, want)
	}
	wantSelection(t, "orchestrator", resolved.Orchestrator, "harness", "claude", "", "", "", "default:fast")
	wantSelection(t, "retro", resolved.Stages["retro"], "harness", "claude", "", "fast-model", "", "default:fast.default")
}

func TestResolveOrchestrationProjectOverride(t *testing.T) {
	registry := loadOrchestrationRegistry(t, `
version: 1
default: base
configs:
  base:
    workers:
      default: {runtime: orca, agent: codex, model: base-model}
projects:
  - name: repo
    root: /repo
    workers:
      groups:
        coding: {model: repo-model}
  - name: nested
    root: /repo/sub
    workers:
      stages:
        implement: {effort: xhigh}
`)
	resolved := resolveRegistry(t, registry, OrchestrationProjectRef{Root: "/repo/sub/worktree"})
	if resolved.Scope != "project" || resolved.Project != "nested" {
		t.Fatalf("scope = %q project = %q", resolved.Scope, resolved.Project)
	}
	wantSelection(t, "plan", resolved.Stages["plan"], "orca", "codex", "", "base-model", "", "default:base.default")
	wantSelection(t, "implement", resolved.Stages["implement"], "orca", "codex", "", "base-model", "xhigh", "project:nested.stage:implement")
}

func TestResolveOrchestrationStageOverrideClearsInheritedModel(t *testing.T) {
	registry := loadOrchestrationRegistry(t, `
version: 1
default: base
configs:
  base:
    workers:
      default: {runtime: orca, agent: codex, model: base-model, effort: medium}
      groups:
        coding: {model: coding-model, effort: high}
      stages:
        implement: {model: "", effort: xhigh}
`)
	resolved := resolveRegistry(t, registry, OrchestrationProjectRef{})
	wantSelection(t, "plan", resolved.Stages["plan"], "orca", "codex", "", "coding-model", "high", "default:base.group:coding")
	wantSelection(t, "implement", resolved.Stages["implement"], "orca", "codex", "", "", "xhigh", "default:base.stage:implement")
	if got := resolved.Stages["implement"].Sources["model"]; got != "default:base.stage:implement" {
		t.Fatalf("cleared model source = %q", got)
	}
}

func TestResolveOrchestrationAmbiguousProjectRoot(t *testing.T) {
	registry := loadOrchestrationRegistry(t, `
version: 1
default: base
configs:
  base: {}
projects:
  - name: first
    root: /repo
  - name: second
    root: /repo
`)
	_, err := ResolveOrchestration(registry, "test.yaml", OrchestrationProjectRef{Root: "/repo/sub"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous orchestration project match") || !strings.Contains(err.Error(), "first, second") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveOrchestrationAmbiguousRemote(t *testing.T) {
	registry := loadOrchestrationRegistry(t, `
version: 1
default: base
configs:
  base: {}
projects:
  - name: one
    remote: https://github.com/owner/repo.git
  - name: two
    remote: git@github.com:owner/repo.git
`)
	_, err := ResolveOrchestration(registry, "test.yaml", OrchestrationProjectRef{Remote: "ssh://git@github.com/owner/repo.git"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous orchestration project match") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveOrchestrationRootAndRemoteDisagree(t *testing.T) {
	registry := loadOrchestrationRegistry(t, `
version: 1
default: base
configs:
  base: {}
projects:
  - name: by-root
    root: /repo
  - name: by-remote
    remote: github.com/owner/repo
`)
	_, err := ResolveOrchestration(registry, "test.yaml", OrchestrationProjectRef{Root: "/repo", Remote: "https://github.com/owner/repo.git"})
	if err == nil || !strings.Contains(err.Error(), `root /repo selects "by-root" but remote`) {
		t.Fatalf("error = %v", err)
	}
}

func TestOrchestrationRegistryRejectsUnknownFields(t *testing.T) {
	_, err := LoadOrchestrationRegistry(writeOrchestrationRegistry(t, `
version: 1
default: base
configs:
  base:
    api_key: value
`))
	if err == nil || !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("error = %v", err)
	}
}

func TestOrchestrationRegistryRejectsUnknownStageAndGroup(t *testing.T) {
	for name, body := range map[string]string{
		"stage": `
version: 1
default: base
configs:
  base:
    workers:
      stages:
        deploy: {}
`,
		"group": `
version: 1
default: base
configs:
  base:
    workers:
      groups:
        testing: {}
`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadOrchestrationRegistry(writeOrchestrationRegistry(t, body))
			if err == nil || !strings.Contains(err.Error(), "not a stage") && !strings.Contains(err.Error(), "not a stage group") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestOrchestrationRegistryRejectsDanglingConfig(t *testing.T) {
	for name, body := range map[string]string{
		"default": `
version: 1
default: missing
configs:
  base: {}
`,
		"global": `
version: 1
default: base
configs:
  base: {}
global:
  config: missing
`,
		"project": `
version: 1
default: base
configs:
  base: {}
projects:
  - name: repo
    root: /repo
    config: missing
`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadOrchestrationRegistry(writeOrchestrationRegistry(t, body))
			if err == nil || !strings.Contains(err.Error(), "is not defined in configs") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestOrchestrationRegistryRejectsRelativeProjectRoot(t *testing.T) {
	_, err := LoadOrchestrationRegistry(writeOrchestrationRegistry(t, `
version: 1
default: base
configs:
  base: {}
projects:
  - name: repo
    root: relative/path
`))
	if err == nil || !strings.Contains(err.Error(), "must be an absolute path") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveOrchestrationMissingRegistry(t *testing.T) {
	resolved, err := ResolveOrchestrationFile(filepath.Join(t.TempDir(), "orchestration.yaml"), OrchestrationProjectRef{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Present || resolved.Scope != "none" || len(resolved.Stages) != len(OrchestrationStageNames()) {
		t.Fatalf("resolved = %+v", resolved)
	}
}

func TestResolveOrchestrationWarnsAboutUnsatisfiableAgentBoxEffort(t *testing.T) {
	registry := loadOrchestrationRegistry(t, `
version: 1
default: base
configs:
  base:
    workers:
      default: {runtime: agentbox, agent: codex, effort: high}
`)
	resolved := resolveRegistry(t, registry, OrchestrationProjectRef{})
	if len(resolved.Warnings) < 2 {
		t.Fatalf("warnings = %v", resolved.Warnings)
	}
	joined := strings.Join(resolved.Warnings, "\n")
	if !strings.Contains(joined, "selects no profile") || !strings.Contains(joined, "cannot apply an effort") {
		t.Fatalf("warnings = %v", resolved.Warnings)
	}
}

func TestNormalizeOrchestrationRemote(t *testing.T) {
	for input, want := range map[string]string{
		"git@github.com:owner/repo.git":     "github.com/owner/repo",
		"https://github.com/owner/repo.git": "github.com/owner/repo",
		"ssh://git@github.com/owner/repo":   "github.com/owner/repo",
		"http://example.com/team/repo/":     "example.com/team/repo",
		" GIT@GitHub.com:Owner/Repo.git ":   "github.com/owner/repo",
	} {
		if got := normalizeOrchestrationRemote(input); got != want {
			t.Fatalf("normalizeOrchestrationRemote(%q) = %q; want %q", input, got, want)
		}
	}
}

func TestOrchestrationRegistryRejectsRemoteCredentials(t *testing.T) {
	_, err := LoadOrchestrationRegistry(writeOrchestrationRegistry(t, `
version: 1
default: base
configs:
  base: {}
projects:
  - name: repo
    remote: https://user:token@github.com/owner/repo.git
`))
	if err == nil || !strings.Contains(err.Error(), "must not contain credentials") {
		t.Fatalf("error = %v", err)
	}
}

func TestOrchestrationRegistryRejectsControlCharacters(t *testing.T) {
	_, err := LoadOrchestrationRegistry(writeOrchestrationRegistry(t, `
version: 1
default: base
configs:
  base:
    workers:
      default:
        model: "bad\u0001model"
`))
	if err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("error = %v", err)
	}
}

func TestOrchestrationRegistryRejectsMultipleDocuments(t *testing.T) {
	_, err := LoadOrchestrationRegistry(writeOrchestrationRegistry(t, `
version: 1
default: base
configs:
  base: {}
---
version: 1
default: base
configs:
  base: {}
`))
	if err == nil || !strings.Contains(err.Error(), "exactly one YAML document") {
		t.Fatalf("error = %v", err)
	}
}

func TestOrchestrationExampleHasNoSecretFields(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "docs", "examples", "orchestration.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, forbidden := range []string{"api_key:", "api-key:", "password:", "secret:", "token:", "sk-", "ghp_", "AKIA"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("example registry contains %q", forbidden)
		}
	}
}
