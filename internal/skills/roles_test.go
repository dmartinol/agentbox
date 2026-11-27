package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

func roleStatusOf(t *testing.T, statuses []Status, role, harness string) Installation {
	t.Helper()
	for _, status := range statuses {
		if status.Name != role {
			continue
		}
		for _, installation := range status.Installations {
			if installation.Target == harness {
				return installation
			}
		}
	}
	t.Fatalf("no %s installation for role %s", harness, role)
	return Installation{}
}

// frontMatterOf splits a rendered Markdown agent file and parses its
// frontmatter as YAML.
func frontMatterOf(t *testing.T, content string) map[string]any {
	t.Helper()
	if !strings.HasPrefix(content, "---\n") {
		t.Fatalf("agent file does not start with frontmatter: %q", content[:40])
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		t.Fatalf("agent frontmatter is not closed: %q", content[:80])
	}
	var out map[string]any
	if err := yaml.Unmarshal([]byte(content[4:4+end]), &out); err != nil {
		t.Fatalf("frontmatter is not valid YAML: %v", err)
	}
	return out
}

// deniesAction reports whether an OpenCode permission rule list denies the
// given action outright.
func deniesAction(fm map[string]any, action string) bool {
	rules, _ := fm["permissions"].([]any)
	for _, raw := range rules {
		rule, _ := raw.(map[string]any)
		if rule["action"] == action && rule["effect"] == "deny" && rule["resource"] == "*" {
			return true
		}
	}
	return false
}

func TestEmbeddedRolesAreComplete(t *testing.T) {
	if err := ValidateRoles(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"agentbox-orchestrator",
		"agentbox-triage",
		"agentbox-planner",
		"agentbox-implementer",
		"agentbox-fixer",
		"agentbox-verifier",
		"agentbox-reviewer",
		"agentbox-retro",
	}
	if got := RoleNames(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("role names = %v", got)
	}
	for _, name := range want {
		role, err := RoleByName(name)
		if err != nil {
			t.Fatal(err)
		}
		if role.Name != name || role.Description == "" || role.Instructions == "" {
			t.Fatalf("role %s = %+v", name, role)
		}
	}
}

func TestEmbeddedHandoffRoleContracts(t *testing.T) {
	type handoffRoleContract struct {
		name                  string
		artifactPath          string
		completionNote        string
		write                 bool
		shell                 bool
		materializedInputPath string
		legacyPhrases         []string
		roleSpecificRules     []string
	}

	cases := []handoffRoleContract{
		{
			name:                  "agentbox-planner",
			artifactPath:          ".agentbox-stage/plan-<iteration>.md",
			completionNote:        "Handoff complete: .agentbox-stage/plan-<iteration>.md.",
			write:                 false,
			shell:                 true,
			materializedInputPath: "",
			legacyPhrases:         []string{"then return:", "return the plan"},
		},
		{
			name:                  "agentbox-implementer",
			artifactPath:          ".agentbox-stage/implement-<iteration>.md",
			completionNote:        "Handoff complete: .agentbox-stage/implement-<iteration>.md.",
			write:                 true,
			shell:                 true,
			materializedInputPath: "",
			legacyPhrases:         []string{"return the result in one final message"},
			roleSpecificRules: []string{
				"Write `.agentbox-stage/implement-<iteration>.md` with exactly these top-level result fields: `base_sha`, `commit`, `branch`, `changed_files`, `gates`, and `remaining_risk`.",
				"Record the exact commit the sandbox started from as `base_sha` before making changes; do not substitute the final head, a tree-equal snapshot, or a remembered base.",
			},
		},
		{
			name:                  "agentbox-fixer",
			artifactPath:          ".agentbox-stage/fix-<iteration>.md",
			completionNote:        "Handoff complete: .agentbox-stage/fix-<iteration>.md.",
			write:                 true,
			shell:                 true,
			materializedInputPath: "",
			legacyPhrases:         []string{"and return:"},
		},
		{
			name:                  "agentbox-verifier",
			artifactPath:          ".agentbox-stage/verify-<iteration>.md",
			completionNote:        "Handoff complete: .agentbox-stage/verify-<iteration>.md.",
			write:                 false,
			shell:                 true,
			materializedInputPath: "",
			legacyPhrases:         []string{"Return the `verify-task.md` artifact", "Report only:"},
		},
		{
			name:                  "agentbox-reviewer",
			artifactPath:          ".agentbox-stage/review-<iteration>.md",
			completionNote:        "Handoff complete: .agentbox-stage/review-<iteration>.md.",
			write:                 false,
			shell:                 true,
			materializedInputPath: ".agentbox-stage/context/pr-body.md",
			legacyPhrases:         []string{"Return the verdict artifact"},
		},
	}

	orchestrator, err := Embedded("agentbox-orchestrator", "test")
	if err != nil {
		t.Fatal(err)
	}
	orchestratorInstructions := string(orchestrator)
	for _, contract := range []string{
		".agentbox-stage/<stage>-<iteration>.md",
		"return only a completion note naming that path",
		"refuse a worker result that returns an artifact body",
		"Triage and retro are exempt from that body-return refusal",
	} {
		if !strings.Contains(orchestratorInstructions, contract) {
			t.Fatalf("embedded orchestrator skill is missing durable-artifact contract %q", contract)
		}
	}

	docBytes, err := os.ReadFile(filepath.Join("..", "..", "docs", "_cli", "skills.md"))
	if err != nil {
		t.Fatal(err)
	}
	documentation := string(docBytes)
	docRows := map[string]string{
		"agentbox-planner":     "Task-spec inputs and a full plan in `.agentbox-stage/plan-<iteration>.md`; returns only `Handoff complete: .agentbox-stage/plan-<iteration>.md.`.",
		"agentbox-implementer": "Scoped implementation, tests, gates, commit, and risk evidence in `.agentbox-stage/implement-<iteration>.md`; returns only `Handoff complete: .agentbox-stage/implement-<iteration>.md.`.",
		"agentbox-fixer":       "Accepted merge-blocking findings, commit, gates, unresolved items, and blockers in `.agentbox-stage/fix-<iteration>.md`; returns only `Handoff complete: .agentbox-stage/fix-<iteration>.md.`.",
		"agentbox-verifier":    "Gate rerun at the pull-request head, recorded in `.agentbox-stage/verify-<iteration>.md`; returns only `Handoff complete: .agentbox-stage/verify-<iteration>.md.`.",
		"agentbox-reviewer":    "Delta-scoped review from task-spec inputs, including `.agentbox-stage/context/pr-body.md` with its `body_sha256`, written to `.agentbox-stage/review-<iteration>.md`; returns only `Handoff complete: .agentbox-stage/review-<iteration>.md.`.",
	}
	rendererParagraph := "The five handoff roles take materialized input paths from their task spec, write only their untracked `.agentbox-stage/<stage>-<iteration>.md` handoff, and return only `Handoff complete: <path>.`. Planner, verifier, and reviewer are read-only: Claude omits `Edit` and `Write`, OpenCode denies `edit`, and both retain shell so the role writes its handoff through the shell. Codex cannot enforce read-only mode in an agent file, so its instruction rule for those handoff roles allows only that explicitly named untracked handoff and still forbids tracked repository edits; read-only roles that define no handoff, such as triage, retro, and the orchestrator, render a read-only rule with no file-write exception. Implementer and fixer remain writable. Triage and retro return complete bodies because they are shell-less and tracker-blind."
	if !strings.Contains(documentation, rendererParagraph) {
		t.Fatal("skills documentation is missing the handoff renderer paragraph")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role, err := RoleByName(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			rawRole, err := RoleYAML(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			canonicalRole, err := ParseRole(rawRole)
			if err != nil {
				t.Fatal(err)
			}
			if canonicalRole.Name != tc.name || canonicalRole.Write != tc.write || canonicalRole.Shell != tc.shell || canonicalRole.Instructions != role.Instructions {
				t.Fatalf("canonical role contract = %+v", canonicalRole)
			}
			if role.Write != tc.write || role.Shell != tc.shell {
				t.Fatalf("role capabilities for %s = write %t, shell %t", tc.name, role.Write, role.Shell)
			}
			if !strings.Contains(role.Description, tc.artifactPath) || !strings.Contains(role.Description, tc.completionNote) {
				t.Fatalf("role description for %s is missing its artifact and completion contract: %q", tc.name, role.Description)
			}
			for _, phrase := range []string{
				"return the commit and evidence",
				"report the addressed finding ids",
				"returning gates and observations",
				"return a verdict artifact",
				"return a body",
			} {
				if strings.Contains(role.Description, phrase) {
					t.Fatalf("role description for %s contains legacy return phrase %q", tc.name, phrase)
				}
			}

			instructions := strings.Join(strings.Fields(role.Instructions), " ")
			requiredInstructions := []string{
				"Take every materialized input path only from the task spec; never construct an `$AGENTBOX_HOME`, network, or forge path.",
				"The handoff must be untracked and non-empty; never stage it.",
				tc.artifactPath,
				tc.completionNote,
			}
			if !tc.write {
				requiredInstructions = append(requiredInstructions, "Write the handoff only through your retained shell after confirming its path is untracked; do not use a native edit tool.")
			}
			requiredInstructions = append(requiredInstructions, tc.roleSpecificRules...)
			for _, phrase := range requiredInstructions {
				if !strings.Contains(instructions, phrase) {
					t.Fatalf("role %s is missing instruction %q", tc.name, phrase)
				}
			}
			for _, phrase := range tc.legacyPhrases {
				if strings.Contains(instructions, phrase) {
					t.Fatalf("role %s still contains legacy return phrase %q", tc.name, phrase)
				}
			}
			if tc.materializedInputPath != "" && !strings.Contains(instructions, tc.materializedInputPath) {
				t.Fatalf("role %s is missing materialized input path %q", tc.name, tc.materializedInputPath)
			}
			if tc.name == "agentbox-reviewer" {
				for _, phrase := range []string{
					"`body_sha256`",
					"## Evidence at commit",
					"PR body SHA-256: <body_sha256>",
					"You have no forge access; do not call a tracker or forge API, fetch the pull request, or construct a network or forge path.",
				} {
					if !strings.Contains(instructions, phrase) {
						t.Fatalf("reviewer is missing contract %q", phrase)
					}
				}
			}

			expectedDocRow := "| `" + tc.name + "` |"
			if !strings.Contains(documentation, expectedDocRow) || !strings.Contains(documentation, docRows[tc.name]) {
				t.Fatalf("documentation row for %s is missing its artifact and completion contract", tc.name)
			}

			codexContent, err := RenderRole(role, "codex", "", "", "test")
			if err != nil {
				t.Fatal(err)
			}
			var codexParsed struct {
				DeveloperInstructions string `toml:"developer_instructions"`
			}
			if _, err := toml.Decode(string(codexContent), &codexParsed); err != nil {
				t.Fatalf("codex role is not valid TOML: %v\n%s", err, codexContent)
			}
			codexInstructions := codexParsed.DeveloperInstructions
			if !strings.Contains(codexInstructions, tc.artifactPath) || !strings.Contains(codexInstructions, tc.completionNote) {
				t.Fatalf("codex role %s is missing its handoff contract", tc.name)
			}
			if !tc.write {
				if !strings.Contains(codexInstructions, "except for the untracked `.agentbox-stage/<stage>-<iteration>.md` handoff explicitly named by your role instructions") {
					t.Fatalf("read-only codex role %s is missing the scoped handoff exception", tc.name)
				}
				if !strings.Contains(codexInstructions, "never create, edit, move, or delete any tracked repository file or any other file") {
					t.Fatalf("read-only codex role %s is missing the tracked-file prohibition", tc.name)
				}
			} else if strings.Contains(codexInstructions, "This role is read-only") {
				t.Fatalf("writable codex role %s contains the read-only rule", tc.name)
			}

			claudeContent, err := RenderRole(role, "claude", "", "", "test")
			if err != nil {
				t.Fatal(err)
			}
			claudeFrontMatter := frontMatterOf(t, string(claudeContent))
			claudeTools, _ := claudeFrontMatter["tools"].(string)
			claudeToolSet := make(map[string]bool)
			for _, tool := range strings.Split(claudeTools, ", ") {
				claudeToolSet[tool] = true
			}
			if claudeToolSet["Agent"] {
				t.Fatalf("stage role %s can dispatch agents", tc.name)
			}
			if !claudeToolSet["Bash"] {
				t.Fatalf("role %s lost shell access", tc.name)
			}
			if tc.write {
				if !claudeToolSet["Edit"] || !claudeToolSet["Write"] {
					t.Fatalf("writable claude role %s lost edit tools: %q", tc.name, claudeTools)
				}
			} else if claudeToolSet["Edit"] || claudeToolSet["Write"] {
				t.Fatalf("read-only claude role %s has edit tools: %q", tc.name, claudeTools)
			}

			openCodeContent, err := RenderRole(role, "opencode", "", "", "test")
			if err != nil {
				t.Fatal(err)
			}
			openCodeFrontMatter := frontMatterOf(t, string(openCodeContent))
			if !deniesAction(openCodeFrontMatter, "subagent") {
				t.Fatalf("stage role %s can start a subagent", tc.name)
			}
			if tc.write {
				if deniesAction(openCodeFrontMatter, "edit") || deniesAction(openCodeFrontMatter, "shell") {
					t.Fatalf("writable opencode role %s lost a capability", tc.name)
				}
			} else if !deniesAction(openCodeFrontMatter, "edit") || deniesAction(openCodeFrontMatter, "shell") {
				t.Fatalf("read-only opencode role %s has the wrong handoff capability", tc.name)
			}
		})
	}

	for _, name := range []string{"agentbox-triage", "agentbox-retro", "agentbox-orchestrator"} {
		t.Run(name+"/codex-read-only-without-handoff", func(t *testing.T) {
			role, err := RoleByName(name)
			if err != nil {
				t.Fatal(err)
			}
			if role.DefinesHandoff() {
				t.Fatalf("role %s unexpectedly defines a stage handoff", name)
			}
			codexContent, err := RenderRole(role, "codex", "", "", "test")
			if err != nil {
				t.Fatal(err)
			}
			var codexParsed struct {
				DeveloperInstructions string `toml:"developer_instructions"`
			}
			if _, err := toml.Decode(string(codexContent), &codexParsed); err != nil {
				t.Fatalf("codex role is not valid TOML: %v\n%s", err, codexContent)
			}
			codexInstructions := codexParsed.DeveloperInstructions
			if !strings.Contains(codexInstructions, "This role is read-only") || !strings.Contains(codexInstructions, "do not create, edit, move, or delete any file, including tracked repository files") {
				t.Fatalf("codex role %s is missing the strict read-only rule", name)
			}
			if strings.Contains(codexInstructions, "except for the untracked") {
				t.Fatalf("codex role %s contains an unscoped handoff exception", name)
			}
		})
	}

	verdictBytes, err := os.ReadFile(filepath.Join("agentbox-orchestrator", "templates", "verdict.md"))
	if err != nil {
		t.Fatal(err)
	}
	if verdict := string(verdictBytes); !strings.Contains(verdict, "git rev-parse HEAD") || !strings.Contains(verdict, "Reviewed commit (actual HEAD)") {
		t.Fatal("verdict template is missing the exact-head evidence contract")
	}

	implementer, err := RoleByName("agentbox-implementer")
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{
		"git diff --name-only <base>..HEAD",
		"git status --short",
		"must equal the exact committed path list",
	} {
		if !strings.Contains(implementer.Instructions, phrase) {
			t.Fatalf("implementer is missing changed-files contract %q", phrase)
		}
	}

	skillBytes, err := os.ReadFile(filepath.Join("agentbox-orchestrator", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{"openshell sandbox upload", "Path-list parity", "git diff --name-only <base>..HEAD"} {
		if !strings.Contains(string(skillBytes), phrase) {
			t.Fatalf("orchestrator skill is missing contract %q", phrase)
		}
	}
}

// TestRenderCodexRoleParsesAndCarriesModel pins the Codex TOML shape: the
// three required keys always exist, a configured model adds both model fields,
// and write: false is rendered as instruction text.
func TestRenderCodexRoleParsesAndCarriesModel(t *testing.T) {
	role := Role{
		Name:         "agentbox-verifier",
		Description:  "Verify one pull request.",
		Instructions: "Run the gates.",
		Write:        false,
		Shell:        true,
		Models:       map[string]RoleModel{"codex": {Model: "gpt-5.1-codex", Effort: "high"}},
	}
	content, err := RenderRole(role, "codex", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Name                  string `toml:"name"`
		Description           string `toml:"description"`
		DeveloperInstructions string `toml:"developer_instructions"`
		Model                 string `toml:"model"`
		ModelReasoningEffort  string `toml:"model_reasoning_effort"`
	}
	if _, err := toml.Decode(string(content), &parsed); err != nil {
		t.Fatalf("codex role is not valid TOML: %v\n%s", err, content)
	}
	if parsed.Name != role.Name || parsed.Description != role.Description || parsed.DeveloperInstructions == "" {
		t.Fatalf("codex role = %+v", parsed)
	}
	if parsed.Model != "gpt-5.1-codex" || parsed.ModelReasoningEffort != "high" {
		t.Fatalf("codex model fields = %+v", parsed)
	}
	if !strings.Contains(parsed.DeveloperInstructions, "read-only") {
		t.Fatalf("write: false is not rendered as instruction text: %s", parsed.DeveloperInstructions)
	}
	if RoleWriteLimitation("codex", role) == "" {
		t.Fatal("codex write limitation is not reported")
	}

	// Without a model the renderer emits no model field at all.
	plain := Role{Name: "agentbox-planner", Description: "Plan.", Instructions: "Plan it.", Write: false, Shell: true}
	content, err = RenderRole(plain, "codex", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "model") {
		t.Fatalf("role without a model rendered a model field: %s", content)
	}

	// An explicit render override wins over the role configuration.
	content, err = RenderRole(role, "codex", "override-model", "low", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `model = "override-model"`) || !strings.Contains(string(content), `model_reasoning_effort = "low"`) {
		t.Fatalf("render override missing: %s", content)
	}
}

// TestRenderClaudeAndOpenCodeRoles pins the write capability shapes: Claude
// omits Edit and Write when write is false, and OpenCode denies the edit
// action. A writable role gets both.
func TestRenderClaudeAndOpenCodeRoles(t *testing.T) {
	readOnly := Role{Name: "agentbox-reviewer", Description: "Review.", Instructions: "Review it.", Write: false, Shell: true}
	claude, err := RenderRole(readOnly, "claude", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	fm := frontMatterOf(t, string(claude))
	if fm["name"] != "agentbox-reviewer" || fm["description"] != "Review." {
		t.Fatalf("claude frontmatter = %+v", fm)
	}
	tools, _ := fm["tools"].(string)
	if strings.Contains(tools, "Edit") || strings.Contains(tools, "Write") {
		t.Fatalf("read-only claude role can write: %q", tools)
	}
	if !strings.Contains(tools, "Bash") {
		t.Fatalf("read-only claude role cannot run gates: %q", tools)
	}

	openCode, err := RenderRole(readOnly, "opencode", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	fm = frontMatterOf(t, string(openCode))
	if fm["mode"] != "subagent" {
		t.Fatalf("opencode frontmatter = %+v", fm)
	}
	if !deniesAction(fm, "edit") {
		t.Fatalf("read-only opencode role does not deny edit: %+v", fm)
	}
	if deniesAction(fm, "shell") {
		t.Fatalf("read-only opencode role denies its gate shell: %+v", fm)
	}
	if !deniesAction(fm, "subagent") {
		t.Fatalf("stage opencode role can start another coordination layer: %+v", fm)
	}

	writable := Role{Name: "agentbox-implementer", Description: "Implement.", Instructions: "Implement it.", Write: true, Shell: true}
	claude, err = RenderRole(writable, "claude", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	tools, _ = frontMatterOf(t, string(claude))["tools"].(string)
	if !strings.Contains(tools, "Edit") || !strings.Contains(tools, "Write") {
		t.Fatalf("writable claude role cannot write: %q", tools)
	}
	openCode, err = RenderRole(writable, "opencode", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	fm = frontMatterOf(t, string(openCode))
	if deniesAction(fm, "edit") || deniesAction(fm, "shell") {
		t.Fatalf("writable opencode role lost a capability: %+v", fm["permissions"])
	}
	if !deniesAction(fm, "subagent") {
		t.Fatalf("stage opencode role can start another coordination layer: %+v", fm["permissions"])
	}
}

// TestRenderShellLessRoleRemovesShellAccess pins the stronger limit used by
// triage and retro: Claude omits Bash, OpenCode denies the shell and edit
// Codex carries the rule as instruction text with a reported limitation.
func TestRenderShellLessRoleRemovesShellAccess(t *testing.T) {
	role := Role{Name: "agentbox-triage", Description: "Triage.", Instructions: "Triage it.", Write: false, Shell: false}

	claude, err := RenderRole(role, "claude", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	tools, _ := frontMatterOf(t, string(claude))["tools"].(string)
	if strings.Contains(tools, "Bash") || strings.Contains(tools, "Edit") || strings.Contains(tools, "Write") {
		t.Fatalf("shell-less claude role can act on the host: %q", tools)
	}
	if !strings.Contains(tools, "Read") {
		t.Fatalf("shell-less claude role lost read access: %q", tools)
	}

	openCode, err := RenderRole(role, "opencode", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	fm := frontMatterOf(t, string(openCode))
	if !deniesAction(fm, "edit") || !deniesAction(fm, "shell") || !deniesAction(fm, "subagent") {
		t.Fatalf("shell-less opencode role permissions = %+v", fm["permissions"])
	}

	codex, err := RenderRole(role, "codex", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(codex), "no shell access") {
		t.Fatalf("codex shell-less role is missing the no-shell rule: %s", codex)
	}
	if RoleShellLimitation("codex", role) == "" {
		t.Fatal("codex shell limitation is not reported")
	}
	if RoleShellLimitation("claude", role) != "" || RoleShellLimitation("codex", Role{Shell: true}) != "" {
		t.Fatal("shell limitation is reported where the harness enforces it")
	}
}

// TestRenderControllerRole pins the orchestrator controller capabilities:
// no source edits, a shell for dispatch and forge commands, child-agent
// dispatch, and explicit top-level selection in OpenCode.
func TestRenderControllerRole(t *testing.T) {
	role, err := RoleByName("agentbox-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	if role.Write || !role.Shell || !role.Dispatch || !role.TopLevel {
		t.Fatalf("orchestrator capabilities = %+v", role)
	}

	claude, err := RenderRole(role, "claude", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	tools, _ := frontMatterOf(t, string(claude))["tools"].(string)
	for _, want := range []string{"Read", "Bash", "Agent"} {
		if !strings.Contains(tools, want) {
			t.Fatalf("orchestrator claude tools lack %s: %q", want, tools)
		}
	}
	if strings.Contains(tools, "Edit") || strings.Contains(tools, "Write") {
		t.Fatalf("orchestrator claude role can edit source: %q", tools)
	}

	openCode, err := RenderRole(role, "opencode", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	fm := frontMatterOf(t, string(openCode))
	if fm["mode"] != "primary" {
		t.Fatalf("orchestrator opencode mode = %v", fm["mode"])
	}
	if !deniesAction(fm, "edit") || deniesAction(fm, "shell") || deniesAction(fm, "subagent") {
		t.Fatalf("orchestrator opencode permissions = %+v", fm["permissions"])
	}

	codex, err := RenderRole(role, "codex", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(codex), "never launch another copy of this role") {
		t.Fatalf("orchestrator codex role lacks the single-controller rule: %s", codex)
	}
	if RoleDispatchLimitation("codex", role) == "" {
		t.Fatal("codex dispatch limitation is not reported")
	}
}

// TestEmbeddedRoleContracts pins the input/output contract of the roles added
// for triage, fix cycles, and retro. These are instruction contracts: the
// tracker and forge rules must be explicit because neither harness format can
// grant or revoke a tracker capability separately from the shell.
func TestEmbeddedRoleContracts(t *testing.T) {
	flat := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	triage, err := RoleByName("agentbox-triage")
	if err != nil {
		t.Fatal(err)
	}
	if triage.Write || triage.Shell {
		t.Fatalf("triage can act on the host: %+v", triage)
	}
	wantAll(t, flat(triage.Instructions),
		"tracker-blind",
		"Never call a tracker or forge API",
		"`ready`",
		"`ready-simple`",
		"`needs-info`",
		"`out-of-scope`",
		"`blocked`",
		"`acceptance_criteria`",
		"`gates`",
		"`evidence`",
		"`questions`",
		"`risks`",
		"The orchestrator, not you, applies every label and state change",
		"without adding another workflow label",
		"single action that unblocks it",
	)

	implementer, err := RoleByName("agentbox-implementer")
	if err != nil {
		t.Fatal(err)
	}
	if !implementer.Write || !implementer.Shell {
		t.Fatalf("implementer cannot work in its worktree: %+v", implementer)
	}
	wantAll(t, flat(implementer.Instructions),
		"do not build, vendor, or commit a replacement shim",
		"use the native edit tool or `git apply`",
		"records the stage as `Completed`",
		"resumed by the orchestrator, not by you",
	)

	fixer, err := RoleByName("agentbox-fixer")
	if err != nil {
		t.Fatal(err)
	}
	if !fixer.Write || !fixer.Shell {
		t.Fatalf("fixer cannot work in its code worktree: %+v", fixer)
	}
	wantAll(t, flat(fixer.Instructions),
		"`blocker` and `major`",
		"not fixer work",
		"existing pull request branch",
		"call a tracker",
		"open or merge a pull request",
		"`findings`",
		"`commit`",
		"`gates`",
		"`unresolved`",
		"`blockers`",
		"never launch another agent",
	)

	retro, err := RoleByName("agentbox-retro")
	if err != nil {
		t.Fatal(err)
	}
	if retro.Write || retro.Shell {
		t.Fatalf("retro can act on the host: %+v", retro)
	}
	wantAll(t, flat(retro.Instructions),
		"Never call a tracker or forge API",
		"Return no proposals",
		"empty list",
		"cross-run pattern",
		"`observation`",
		"`evidence`",
		"`impact`",
		"`context`",
		"`acceptance_criteria`",
		"`related`",
		"skips you when triage rejected the issue or a preflight failure blocked the workflow before any work started",
	)

	orchestrator, err := RoleByName("agentbox-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	if orchestrator.Write || !orchestrator.Shell || !orchestrator.Dispatch || !orchestrator.TopLevel {
		t.Fatalf("orchestrator capabilities = %+v", orchestrator)
	}
	wantAll(t, flat(orchestrator.Instructions),
		"`agentbox-orchestrator` skill first",
		"only source of truth",
		"Never triage, plan, implement, fix, verify, review, or run retro inline",
		"Configuration authoring is a separate, non-workflow mode from the skill",
		"interview for every setting, write only the user-owned registry under `$AGENTBOX_HOME`, and verify the saved scope",
		"Do not start a workflow or dispatch a stage for that request",
		"The no-repository-write rule still applies",
		"Never launch another `agentbox-orchestrator`",
		"explicit user assignment",
		"or hand the workflow to another coordination layer",
		"stop and report the exact missing capability",
		"does not start a second orchestrator",
		"Start each stage worker as the stage agent in a visible Orca worker or terminal",
		"Never nest one agent inside another to run the same stage",
		"never bypass a runtime's sandbox or approval controls",
		"Retain worker sessions and worktrees until the stage evidence is captured and the final report is delivered",
	)
}

// TestInstallWritesRolesForEveryHarness covers the main lifecycle contract:
// one native file per role and harness, recorded in the manifest and reported
// current.
func TestInstallWritesRolesForEveryHarness(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(home, ".claude", "agents", "agentbox-planner.md")
	codexPath := filepath.Join(home, ".codex", "agents", "agentbox-planner.toml")
	openCodePath := filepath.Join(home, ".config", "opencode", "agents", "agentbox-planner.md")
	for _, path := range []string{claudePath, codexPath, openCodePath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("role was not installed at %s: %v", path, err)
		}
	}
	manifest := loadManifest(filepath.Join(home, ".agents", "skills"))
	if len(manifest.Roles) != len(RoleNames()) {
		t.Fatalf("manifest roles = %d", len(manifest.Roles))
	}
	record := manifest.Roles["agentbox-planner"]
	if record.Version != "1.0.0" || len(record.Harnesses) != 3 {
		t.Fatalf("role manifest record = %+v", record)
	}
	if file, ok := record.File("codex"); !ok || file.SHA256 != hashHex(mustRead(t, codexPath)) {
		t.Fatalf("codex role manifest entry = %+v", file)
	}

	statuses, err := ListRoles(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range RoleNames() {
		for _, harness := range harnesses {
			if inst := roleStatusOf(t, statuses, name, harness); inst.State != "current" {
				t.Fatalf("%s on %s = %+v", name, harness, inst)
			}
		}
	}
}

// TestUpdateRefreshesStaleRole covers the version-matched refresh path.
func TestUpdateRefreshesStaleRole(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	statuses, err := ListRoles(ListOptions{Version: "2.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if inst := roleStatusOf(t, statuses, "agentbox-reviewer", "codex"); inst.State != "stale" {
		t.Fatalf("role state = %+v", inst)
	}
	results, err := Update(Options{All: true, Version: "2.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, result := range results {
		if result.Skill == "agentbox-reviewer" && result.Target == "codex" {
			found = result.Action == "updated"
		}
	}
	if !found {
		t.Fatalf("codex role was not refreshed: %+v", results)
	}
	if record := loadManifest(filepath.Join(home, ".agents", "skills")).Roles["agentbox-reviewer"]; record.Version != "2.0.0" {
		t.Fatalf("role version = %+v", record)
	}
}

// TestRemoveBacksUpModifiedRole covers the --force ownership rule for roles.
func TestRemoveBacksUpModifiedRole(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "agents", "agentbox-verifier.md")
	if err := os.WriteFile(path, []byte("hand edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(Options{All: true, Home: home}); err == nil {
		t.Fatal("remove deleted a modified role without --force")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("modified role was removed: %v", err)
	}
	if _, err := Remove(Options{All: true, Home: home, Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("forced remove left the modified role: %v", err)
	}
	backups, err := filepath.Glob(filepath.Join(home, ".agents", ".agentbox-skill-backups", "*", "roles", "claude", "agentbox-verifier"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("modified role backup = %v (%v)", backups, err)
	}
	if body := string(mustRead(t, backups[0])); body != "hand edited\n" {
		t.Fatalf("backup content = %q", body)
	}
}

// TestRoleInstallRefusesForeignFileUnlessForced protects a user-defined agent
// that happens to share a role name.
func TestRoleInstallRefusesForeignFileUnlessForced(t *testing.T) {
	home := prepareHome(t)
	path := filepath.Join(home, ".codex", "agents", "agentbox-planner.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err == nil {
		t.Fatal("install overwrote a foreign agent file")
	}
	if body := string(mustRead(t, path)); body != "foreign\n" {
		t.Fatalf("foreign agent file changed: %q", body)
	}
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home, Force: true}); err != nil {
		t.Fatal(err)
	}
	if body := string(mustRead(t, path)); !strings.Contains(body, "developer_instructions") {
		t.Fatalf("forced install did not write the role: %q", body)
	}
}

// TestRemoveDeletesOnlyManagedRoleFiles covers --skip-roles and the ownership
// rule: an unrecorded file at a role path is left alone.
func TestRemoveDeletesOnlyManagedRoleFiles(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(home, ".claude", "agents", "custom-agent.md")
	if err := os.WriteFile(foreign, []byte("custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(Options{All: true, Home: home, SkipRoles: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "agents", "agentbox-planner.toml")); err != nil {
		t.Fatalf("--skip-roles removed a role: %v", err)
	}
	if _, err := Remove(Options{All: true, Home: home}); err != nil {
		t.Fatal(err)
	}
	for _, name := range RoleNames() {
		if _, err := os.Stat(filepath.Join(home, ".codex", "agents", name+".toml")); !os.IsNotExist(err) {
			t.Fatalf("role %s survived remove", name)
		}
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("remove deleted a foreign agent file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", ManifestFile)); !os.IsNotExist(err) {
		t.Fatalf("manifest survived a full remove: %v", err)
	}
}

// TestRoleScopesAreSkipped keeps --local and --dir from writing global agent
// definitions.
func TestRoleScopesAreSkipped(t *testing.T) {
	project := t.TempDir()
	results, err := Install(Options{Local: true, ProjectDir: project, Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	skipped := 0
	for _, result := range results {
		if result.Target == "roles" {
			if result.Action != "skipped" || !strings.Contains(result.Detail, "global-only") {
				t.Fatalf("local role result = %+v", result)
			}
			skipped++
		}
	}
	if skipped != len(RoleNames()) {
		t.Fatalf("local role rows = %d", skipped)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "agents")); !os.IsNotExist(err) {
		t.Fatalf("local install wrote agent files: %v", err)
	}

	dir := filepath.Join(t.TempDir(), "skills")
	results, err = Install(Options{Dir: dir, Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if skipped := countRoleRows(results); skipped != len(RoleNames()) {
		t.Fatalf("--dir role rows = %d", skipped)
	}
	if _, err := os.Stat(filepath.Join(dir, "roles")); !os.IsNotExist(err) {
		t.Fatalf("--dir wrote role files: %v", err)
	}
}

func countRoleRows(results []Result) int {
	count := 0
	for _, result := range results {
		if result.Target == "roles" {
			count++
		}
	}
	return count
}
