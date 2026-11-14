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
	if _, ok := frontMatterOf(t, string(openCode))["permissions"]; ok {
		t.Fatalf("writable opencode role carries edit permissions: %s", openCode)
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
	if !deniesAction(fm, "edit") || !deniesAction(fm, "shell") {
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
		"`needs-info`",
		"`out-of-scope`",
		"`blocked`",
		"`evidence`",
		"`questions`",
		"`risks`",
		"The orchestrator, not you, applies every label and state change",
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
	)

	if ValidRoleName("agentbox-orchestrator") {
		t.Fatal("a separate orchestrator subagent role is installed")
	}
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
