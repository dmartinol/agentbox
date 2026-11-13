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

// collapsed returns the rendered orchestrator skill with whitespace collapsed
// so assertions do not depend on line wrapping.
func collapsed(t *testing.T) string {
	t.Helper()
	content, err := Embedded("agentbox-orchestrator", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(string(content)), " ")
}

// wantAll fails when body is missing one of the expected fragments.
func wantAll(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q", want)
		}
	}
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

// TestOrchestratorSkillRoleBoundary pins the MUST boundary: orchestration and
// optional capability-driven forge management, never inline stage work.
func TestOrchestratorSkillRoleBoundary(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Role boundary (MUST)",
		"Task orchestration (mandatory):** triage, select and dispatch every stage's worker",
		"Git and forge management (optional, capability-driven):**",
		"MUST NOT plan, implement, verify, or review inline",
		"If it starts doing stage work, it stops and dispatches a worker",
		"A resumed run redispatches the stage; it never continues a previous worker's session",
	)
}

// TestOrchestratorSkillSelection covers asking once, recording every runtime
// and model selection, confirming inherited defaults, and refusing silent
// substitution.
func TestOrchestratorSkillSelection(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Runtime and model selection",
		"Ask once during triage",
		"Confirm an inherited default explicitly before recording it",
		"| Orchestrator | runtime (`orca`, `agentbox`, or `harness`), agent, model, effort |",
		"| Plan worker | runtime, agent, model, effort |",
		"| Implement worker | runtime, agent, model, effort |",
		"| Verify worker | runtime, agent, model, effort |",
		"| Review worker | runtime, agent, model, effort |",
		"The orchestrator cannot switch its own model from inside the turn",
		"sets `blocked` with the exact reason",
		"Never fall back to a default, a cheaper model, or a different agent",
		"Reviewer independence",
	)
}

// TestOrchestratorSkillStageDispatch covers per-stage workers, isolated
// worktrees, and fresh verify and review worktrees at the PR head.
func TestOrchestratorSkillStageDispatch(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Stage ownership and isolation",
		"| triage | orchestrator |",
		"| plan | worker of the selected type | its own new worktree |",
		"| implement | worker of the selected type, branch `orchestrator/<issue>-<slug>` |",
		"| verify | fresh worker | fresh worktree checked out at the pull request head |",
		"| review | fresh worker | fresh worktree checked out at the pull request head |",
		"| merge / close | orchestrator |",
		"Every stage except triage and merge/close runs in its own selected worker/worktree",
		"Plan, implement, verify, and review are never performed by the orchestrator turn",
		"must not share the implementer's worktree and must not see uncommitted state",
		"A fix cycle dispatches a new implementer and then a new reviewer at the new head",
		"A rebase or force-push invalidates prior verification and review evidence",
	)
}

// TestOrchestratorSkillForgeMatrix pins the single stage-aware capability
// matrix and its consequences for sandbox and host workers.
func TestOrchestratorSkillForgeMatrix(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Forge capability matrix",
		"| Worker type | Can commit | Can push | Can open the PR | Can post comments or verdicts | Can merge or close |",
		"| AgentBox sandbox worker | yes, local only | no | no | no | no |",
		"| Orca child worktree (host) | yes | yes | yes | yes, except verdicts and state | no |",
		"| Harness subagent (host) | yes | yes | yes | yes, except verdicts and state | no |",
		"| Orchestrator | only when a worker cannot | only for a sandbox worker | only for a sandbox worker | yes, including verdicts and state | yes |",
		"the orchestrator syncs the run into the host worktree, verifies the branch head, pushes it, and opens the pull request",
		"it may commit, push, and open the pull request itself",
		"a push somewhere other than `orchestrator/<issue>-<slug>` is a blocked outcome",
		"merge, close, the state comment, and review verdicts remain orchestrator-authored",
		"A reviewer returns its verdict artifact; the orchestrator posts it",
	)
}

// TestOrchestratorSkillProvenance covers the stable workflow id, the
// machine-readable marker, the visible footer, and duplicate-verdict refusal.
func TestOrchestratorSkillProvenance(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Comment provenance",
		"Create a stable `workflow_id` at triage",
		"Every workflow comment, pull request body, and verdict carries exactly one machine-readable marker",
		"<!-- orchestrator-workflow: {",
		"\"workflow\":\"orchestrator-owner-name-35-a1b2c3\"",
		"\"stage\":\"review\"",
		"\"worker\":\"orca\"",
		"\"worker_id\":\"worker-7\"",
		"\"model\":\"model-id\"",
		"\"commit\":\"0123456789abcdef0123456789abcdef01234567\"",
		"\"pr\":42",
		"and exactly one visible footer",
		"Orchestrated by agentbox-orchestrator 9.9.9 · workflow",
		"· stage review · worker orca/codex · model model-id · commit 0123456789abcdef0123456789abcdef01234567",
		"Include the stage, worker, model, and commit in every marker and footer",
		"Refuse a second verdict for the same `(workflow, stage, commit)`",
		"Provenance never implies a human author",
		"A bot account or GitHub App that gives platform-level identity is the documented deferred alternative",
	)
}

// TestOrchestratorSkillTemplates covers the four shipped templates and the
// explicit override precedence.
func TestOrchestratorSkillTemplates(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"## Templates",
		"The skill ships four templates and installs them beside `SKILL.md`",
		"| `issue.md` |",
		"| `pr.md` |",
		"| `verdict.md` |",
		"| `state.md` |",
		"Resolve a template from the first source that defines it",
		"1. an explicit user instruction for this issue;",
		"2. the repository's `.github/orchestrator/<kind>.md`;",
		"3. the template shipped with this skill.",
		"Never edit a repository's own templates",
	)
}

// TestOrchestratorSkillStateSchema covers the expanded record and the
// sensitive-data rules that still apply when model and effort are recorded.
func TestOrchestratorSkillStateSchema(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"<!-- orchestrator-state -->",
		"schema: 2",
		"workflow_id:",
		"orchestrator: {runtime:",
		"stages:",
		"stage: implement",
		"stage: verify",
		"stage: review",
		"worktree:",
		"review_cycles:",
		"blocked_reason:",
		"updated:",
		"The record may contain model and effort identifiers",
		"must never contain credentials, environment-variable values, account identifiers, absolute host paths, private URLs, logs, or secret values",
	)
}

// TestOrchestratorTemplateFiles verifies the embedded inventory, the sibling
// install names, and that every template renders the version and carries
// provenance.
func TestOrchestratorTemplateFiles(t *testing.T) {
	files := Files("agentbox-orchestrator")
	wantNames := []string{"SKILL.md", "issue.md", "pr.md", "verdict.md", "state.md"}
	if len(files) != len(wantNames) {
		t.Fatalf("files = %v", files)
	}
	for i, want := range wantNames {
		if files[i].Name != want {
			t.Fatalf("file %d = %q, want %q", i, files[i].Name, want)
		}
		if files[i].Template != (want != "SKILL.md") {
			t.Fatalf("%s template flag = %v", want, files[i].Template)
		}
		if _, err := embedded.ReadFile(files[i].Source); err != nil {
			t.Fatalf("%s source %q: %v", want, files[i].Source, err)
		}
	}
	for _, name := range []string{"agentbox-cli", "agentbox-api"} {
		if got := Files(name); len(got) != 1 || got[0].Name != "SKILL.md" {
			t.Fatalf("%s inventory = %v; other skills stay single-file", name, got)
		}
	}

	headings := map[string][]string{
		"issue.md":   {"## Summary", "## Current behavior", "## Proposed design", "## Acceptance criteria", "## Test plan", "## Risks", "## Out of scope", "## Related"},
		"pr.md":      {"## Summary", "## Changes", "## Evidence", "Closes #<issue>", "## Risks", "## Provenance"},
		"verdict.md": {"## Verdict", "## Findings", "## Evidence at commit", "## Required changes", "## Provenance"},
		"state.md":   {"<!-- orchestrator-state -->", "schema: 2", "workflow_id:"},
	}
	for file, wants := range headings {
		content, err := EmbeddedFile("agentbox-orchestrator", file, "9.9.9")
		if err != nil {
			t.Fatal(err)
		}
		body := string(content)
		if strings.Contains(body, VersionPlaceholder) {
			t.Fatalf("%s still contains the version placeholder", file)
		}
		wantAll(t, body, wants...)
		wantAll(t, body,
			"<!-- orchestrator-workflow: {",
			"Orchestrated by agentbox-orchestrator 9.9.9 · workflow",
		)
	}
}

// TestOrchestratorSkillStateRecordExampleIsClean enforces the sensitive-data
// rules on the documented examples and shipped templates.
func TestOrchestratorSkillStateRecordExampleIsClean(t *testing.T) {
	docs := []string{rawSkill(t, "agentbox-orchestrator")}
	for _, file := range Files("agentbox-orchestrator") {
		raw, err := embedded.ReadFile(file.Source)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, string(raw))
	}
	for _, body := range docs {
		for _, forbidden := range []string{
			"/Users/", "/home/", `C:\Users`, "sk-", "ghp_", "AKIA",
			"api.openai.com", "api.anthropic.com", "gpt-", "claude-", "gemini",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("documentation contains %q", forbidden)
			}
		}
	}
}

// TestOrchestratorSkillInstallsAndLists covers install, list, and show for
// the third skill, including its sibling templates.
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
		if result.Target == "agents" && result.File == "" {
			t.Fatalf("orchestrator canonical result lacks a file: %+v", result)
		}
	}
	if !seen {
		t.Fatal("install ignored the orchestrator skill")
	}
	dir := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator")
	canonical := filepath.Join(dir, "SKILL.md")
	if body := mustRead(t, canonical); InstalledVersion(body) != "1.0.0" {
		t.Fatalf("orchestrator version = %s", InstalledVersion(body))
	}
	for _, template := range []string{"issue.md", "pr.md", "verdict.md", "state.md"} {
		if _, err := os.Stat(filepath.Join(dir, template)); err != nil {
			t.Fatalf("%s is not installed beside SKILL.md: %v", template, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "agentbox-orchestrator", "issue.md")); err != nil {
		t.Fatalf("templates are not reachable through the claude link: %v", err)
	}
	manifest := loadManifest(filepath.Join(home, ".agents", "skills"))
	record, ok := manifest.Skills["agentbox-orchestrator"]
	if !ok || len(record.Files) != len(Files("agentbox-orchestrator")) {
		t.Fatalf("orchestrator manifest record = %+v", record)
	}

	statuses, err := List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	var listed bool
	for _, status := range statuses {
		if status.Name != "agentbox-orchestrator" {
			continue
		}
		listed = true
		if stateOf(t, statuses, "agentbox-orchestrator", "agents").State != "current" {
			t.Fatal("orchestrator not current after install")
		}
		if len(status.Files) != 4 {
			t.Fatalf("template statuses = %+v", status.Files)
		}
		for _, file := range status.Files {
			if file.State != "current" {
				t.Fatalf("%s state = %+v", file.Name, file)
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
