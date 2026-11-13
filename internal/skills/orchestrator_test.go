package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rawSkill returns the unrendered embedded file so tests can assert on the
// literal front matter syntax.
func rawSkill(t *testing.T, name string) string {
	t.Helper()
	body, err := embedded.ReadFile(name + "/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestOrchestratorSkillFrontMatter(t *testing.T) {
	content, err := Embedded("agentbox-orchestrator", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	fm, err := ParseFrontMatter(content)
	if err != nil {
		t.Fatalf("front matter does not parse strictly: %v", err)
	}
	if fm.Name != "agentbox-orchestrator" {
		t.Fatalf("name = %q", fm.Name)
	}
	if fm.Version != "9.9.9" {
		t.Fatalf("version = %q", fm.Version)
	}
	if strings.TrimSpace(fm.Description) == "" {
		t.Fatal("description is empty")
	}
	if strings.ContainsAny(fm.Description, "\n\r") {
		t.Fatal("description is not a single line")
	}
	// The description must be a quoted scalar in the source file, unlike the
	// unquoted style that shipped before issue #30.
	for _, line := range strings.Split(rawSkill(t, "agentbox-orchestrator"), "\n") {
		if strings.HasPrefix(line, "description:") {
			if !strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(line, "description:")), `"`) {
				t.Fatalf("description is not quoted: %s", line)
			}
			return
		}
	}
	t.Fatal("no description line in the front matter")
}

func TestOrchestratorSkillWorkflowContent(t *testing.T) {
	content, err := Embedded("agentbox-orchestrator", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	// Collapse whitespace so assertions do not depend on line wrapping.
	body := strings.Join(strings.Fields(string(content)), " ")

	// Six orchestrator states, each with a label.
	for _, state := range []string{"triage", "ready", "in-progress", "in-review", "approved", "blocked"} {
		if !strings.Contains(body, "orchestrator:"+state) {
			t.Fatalf("missing the %s state label", state)
		}
	}
	// Safe label transitions: add before remove, exactly one active label.
	for _, want := range []string{"add-before-remove", "then remove the old one", "never carries"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing label-transition rule %q", want)
		}
	}
	// Explicit runtime selection without silent fallback.
	for _, want := range []string{
		"never silently fall back",
		"`orca` (recommended",
		"`harness` (native subagents",
		"`agentbox` (recommended",
		"`orca` (child worktree)",
		"`harness` (review-only or sequential",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing runtime selection %q", want)
		}
	}
	// Override precedence.
	for _, want := range []string{".github/orchestrator.yaml", "AGENTS.md", "CONTRIBUTING"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing precedence source %q", want)
		}
	}
	// Durable state record: marker, schema fields, and sensitive-data rules.
	if !strings.Contains(body, "<!-- orchestrator-state -->") {
		t.Fatal("missing the state record marker")
	}
	for _, field := range []string{
		"schema:", "repo:", "issue:", "state:", "labels:", "orchestrator:", "worker:",
		"worker_id:", "branch:", "pr:", "commit:", "gates:", "review_cycles:",
		"blocked_reason:", "updated:",
	} {
		if !strings.Contains(body, field) {
			t.Fatalf("state record is missing the %s field", field)
		}
	}
	for _, forbidden := range []string{"logs, secrets, account IDs, absolute host paths, private URLs", "model and provider names"} {
		if !strings.Contains(body, forbidden) {
			t.Fatalf("state record lacks the exclusion rule for %q", forbidden)
		}
	}
	// Gates.
	for _, gate := range []string{"**Triage.**", "**Plan.**", "**Implement.**", "**Verify.**", "**Pull request.**", "**Independent review.**", "**Merge.**", "**Close.**"} {
		if !strings.Contains(body, gate) {
			t.Fatalf("missing the %s gate", gate)
		}
	}
	for _, want := range []string{
		"exactly one writer in its own worktree",
		"orchestrator/<issue>-<slug>",
		"self-contained task spec",
		"on the synced commit",
		"Worker-reported results are advisory",
		"Conventional Commits",
		"Closes #<issue>",
		"cannot approve its own PR",
		"invalidates the evidence",
		"prefer a merge commit",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing workflow rule %q", want)
		}
	}
	// AgentBox worker loop.
	for _, want := range []string{
		"agentbox agent start",
		"--approval unattended",
		"agentbox agent show RUN_ID --json",
		"agentbox agent logs RUN_ID",
		"agentbox agent diff RUN_ID",
		"agentbox agent sync RUN_ID",
		"agentbox agent delete RUN_ID",
		"per-task deadline",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing worker-loop element %q", want)
		}
	}
	// Worker contract: pure worker, result report, and the actor rule.
	for _, want := range []string{
		"pure worker",
		"It never pushes, opens a PR, comments on the tracker, labels, or merges",
		"branch name, HEAD SHA, changed paths, gate output, and any blockers",
		"only actor that talks to the tracker and the forge",
		"pushes that branch from the host",
		"never needs a remote, a forge token",
		"may push and open the PR",
		"still verifies and owns the merge",
		"not a prerequisite for this skill",
		"AgentBox <agentbox@local.invalid>",
		"preserve authorship",
		"host run worktree",
		"pushes it from the host",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing worker-contract rule %q", want)
		}
	}
	// Seven-operation adapter contract.
	for _, op := range []string{"read issue", "list ready", "create issue", "set labels", "comment/upsert state", "link PR", "close"} {
		if !strings.Contains(body, op) {
			t.Fatalf("adapter contract is missing the %q operation", op)
		}
	}
	// GitHub adapter and Jira/Linear delegation without an implementation.
	for _, want := range []string{"gh issue view", "gh issue list", "gh issue create", "gh issue edit", "gh pr create", "gh issue close", "gh label create", "orca-jira", "orca-linear"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing adapter element %q", want)
		}
	}
}

// TestOrchestratorSkillStateRecordExampleIsClean enforces the sensitive-data
// rules on the documented example itself.
func TestOrchestratorSkillStateRecordExampleIsClean(t *testing.T) {
	body := rawSkill(t, "agentbox-orchestrator")
	for _, forbidden := range []string{
		"/Users/", "/home/", `C:\Users`, "sk-", "ghp_", "AKIA",
		"api.openai.com", "api.anthropic.com", "gpt-", "claude-", "gemini",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("state record example contains %q", forbidden)
		}
	}
}

// TestOrchestratorSkillInstallsAndLists covers install, list, and show for
// the third skill through the issue #30 lifecycle.
func TestOrchestratorSkillInstallsAndLists(t *testing.T) {
	home := prepareHome(t)
	results, err := Install(Options{All: true, Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, result := range results {
		if result.Skill != "agentbox-orchestrator" {
			continue
		}
		seen = true
		if result.Target == "agents" && result.Action != "installed" {
			t.Fatalf("orchestrator canonical action = %s", result.Action)
		}
		if result.Target == "claude" && result.Action != "linked" {
			t.Fatalf("orchestrator claude action = %s", result.Action)
		}
	}
	if !seen {
		t.Fatal("install ignored the orchestrator skill")
	}
	canonical := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator", "SKILL.md")
	if body := mustRead(t, canonical); InstalledVersion(body) != "1.0.0" {
		t.Fatalf("orchestrator version = %s", InstalledVersion(body))
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "agentbox-orchestrator")); err != nil {
		t.Fatal("orchestrator claude link missing")
	}
	manifest := loadManifest(filepath.Join(home, ".agents", "skills"))
	if _, ok := manifest.Skills["agentbox-orchestrator"]; !ok {
		t.Fatal("orchestrator missing from the manifest")
	}

	statuses, err := List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	var listed bool
	for _, status := range statuses {
		if status.Name == "agentbox-orchestrator" {
			listed = true
			if stateOf(t, statuses, "agentbox-orchestrator", "agents").State != "current" {
				t.Fatal("orchestrator not current after install")
			}
		}
	}
	if !listed {
		t.Fatal("list ignored the orchestrator skill")
	}

	content, err := Embedded("agentbox-orchestrator", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "# Issue-to-PR orchestrator flow") {
		t.Fatal("show would not render the orchestrator skill body")
	}
	if _, err := Embedded("agentbox-orchestrator-extra", "1.0.0"); err == nil {
		t.Fatal("accepted a lookalike skill name")
	}
}
