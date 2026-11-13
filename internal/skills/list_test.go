package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stateOf finds the installation row of one skill and target.
func stateOf(t *testing.T, statuses []Status, skill, target string) Installation {
	t.Helper()
	for _, status := range statuses {
		if status.Name != skill {
			continue
		}
		for _, installation := range status.Installations {
			if installation.Target == target {
				return installation
			}
		}
	}
	t.Fatalf("no %s installation for %s", target, skill)
	return Installation{}
}

// TestListStatesCurrentStaleModifiedMissing covers the hash-derived states of
// the canonical copy. A hand-edited file that keeps its version line must
// report modified, not current.
func TestListStatesCurrentStaleModifiedMissing(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	statuses, err := List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if stateOf(t, statuses, "agentbox-cli", "agents").State != "current" {
		t.Fatalf("fresh copy state = %+v", stateOf(t, statuses, "agentbox-cli", "agents"))
	}

	// Same manifest hash, newer binary: stale.
	statuses, err = List(ListOptions{Version: "2.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if stateOf(t, statuses, "agentbox-cli", "agents").State != "stale" {
		t.Fatalf("stale copy state = %+v", stateOf(t, statuses, "agentbox-cli", "agents"))
	}

	// Byte-changed copy that keeps the version line: modified.
	path := filepath.Join(home, ".agents", "skills", "agentbox-cli", "SKILL.md")
	body := mustRead(t, path)
	edited := append(append([]byte{}, body...), []byte("extra line\n")...)
	if err := os.WriteFile(path, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	statuses, err = List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	inst := stateOf(t, statuses, "agentbox-cli", "agents")
	if inst.State != "modified" || inst.Version != "1.0.0" {
		t.Fatalf("modified copy state = %+v", inst)
	}

	// Removed copy: missing.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	statuses, err = List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if inst := stateOf(t, statuses, "agentbox-cli", "agents"); inst.State != "missing" || inst.Present {
		t.Fatalf("missing copy state = %+v", inst)
	}
}

// TestListReportsBrokenLink covers a dangling symlink: the link is present
// but resolves nowhere.
func TestListReportsBrokenLink(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(home, ".agents", "skills", "agentbox-cli", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	statuses, err := List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	inst := stateOf(t, statuses, "agentbox-cli", "claude")
	if inst.State != "broken-link" || !inst.Present {
		t.Fatalf("broken link state = %+v", inst)
	}
}

// TestListReportsSymlinkedCanonicalSkillDirectory covers a symlink where the
// canonical skill directory belongs. A dangling link and a link to a foreign
// directory with different content are both blocked, not missing or
// modified, so the doctor recommends install --force.
func TestListReportsSymlinkedCanonicalSkillDirectory(t *testing.T) {
	for _, kind := range []string{"dangling", "foreign directory"} {
		t.Run(kind, func(t *testing.T) {
			home := prepareHome(t)
			if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, ".agents", "skills", "agentbox-cli")
			if err := os.RemoveAll(dir); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(home, "agentbox-cli-target")
			if kind == "foreign directory" {
				if err := os.MkdirAll(target, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("foreign copy\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(target, dir); err != nil {
				t.Fatal(err)
			}

			statuses, err := List(ListOptions{Version: "1.0.0", Home: home})
			if err != nil {
				t.Fatal(err)
			}
			inst := stateOf(t, statuses, "agentbox-cli", "agents")
			if !inst.Present || inst.State != "unreadable" {
				t.Fatalf("symlinked canonical directory state = %+v", inst)
			}
			if !strings.Contains(inst.Detail, "install --force") {
				t.Fatalf("symlinked canonical directory lacks repair advice: %+v", inst)
			}
			if info, err := os.Lstat(dir); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("list changed the symlinked canonical path: %v %v", info, err)
			}
		})
	}
}

// TestListReportsInvalidFrontMatter covers a copy whose front matter strict
// consumers reject, such as the unquoted descriptions shipped by PR #29.
func TestListReportsInvalidFrontMatter(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".agents", "skills", "agentbox-cli", "SKILL.md")
	broken := "---\nname: agentbox-cli\ndescription: broken: colon\nversion: \"1.0.0\"\n---\n"
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	statuses, err := List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	inst := stateOf(t, statuses, "agentbox-cli", "agents")
	if inst.State != "invalid" {
		t.Fatalf("invalid front matter state = %+v", inst)
	}
	if inst.Detail == "" {
		t.Fatal("invalid front matter lacks a remediation hint")
	}
}

// TestListReportsForeignLink covers a real directory sitting at a link path.
func TestListReportsForeignLink(t *testing.T) {
	home := prepareHome(t)
	foreign := filepath.Join(home, ".claude", "skills", "agentbox-cli")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	statuses, err := List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if inst := stateOf(t, statuses, "agentbox-cli", "claude"); inst.State != "foreign" {
		t.Fatalf("foreign link state = %+v", inst)
	}
}

// TestListReportsLegacyCodexCopy covers the report-only Codex row: clean
// machines report missing, leftover PR #29 copies report legacy.
func TestListReportsLegacyCodexCopy(t *testing.T) {
	home := prepareHome(t)
	statuses, err := List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if inst := stateOf(t, statuses, "agentbox-cli", "codex"); inst.State != "missing" {
		t.Fatalf("clean codex state = %+v", inst)
	}
	legacy := filepath.Join(home, ".codex", "skills", "agentbox-cli")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "SKILL.md"), []byte("old copy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	statuses, err = List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	inst := stateOf(t, statuses, "agentbox-cli", "codex")
	if inst.State != "legacy" || !inst.Present {
		t.Fatalf("legacy codex state = %+v", inst)
	}
}

// TestListReadsLocalRoot checks the --local scope of list.
func TestListReadsLocalRoot(t *testing.T) {
	project := t.TempDir()
	statuses, err := List(ListOptions{Version: "1.0.0", Local: true, ProjectDir: project})
	if err != nil {
		t.Fatal(err)
	}
	inst := stateOf(t, statuses, "agentbox-cli", "agents")
	if inst.State != "missing" || inst.Path != filepath.Join(project, ".agents", "skills", "agentbox-cli", "SKILL.md") {
		t.Fatalf("local installation = %+v", inst)
	}
	opencode := stateOf(t, statuses, "agentbox-cli", "opencode")
	if want := filepath.Join(project, ".opencode", "skills", "agentbox-cli"); opencode.Path != want {
		t.Fatalf("local opencode path = %s, want %s", opencode.Path, want)
	}
}

// templateStatusesFor finds the template rows of one skill.
func templateStatusesFor(t *testing.T, statuses []Status, skill string) []FileStatus {
	t.Helper()
	for _, status := range statuses {
		if status.Name == skill {
			return status.Files
		}
	}
	t.Fatalf("no status for %s", skill)
	return nil
}

// fileStateOf finds one template row of one skill.
func fileStateOf(t *testing.T, statuses []Status, skill, file string) FileStatus {
	t.Helper()
	for _, status := range templateStatusesFor(t, statuses, skill) {
		if status.Name == file {
			return status
		}
	}
	t.Fatalf("no %s template status for %s", file, skill)
	return FileStatus{}
}

// TestListReportsTemplateStates covers current, stale, modified, and missing
// for shipped templates while the SKILL.md rows keep their existing shape.
func TestListReportsTemplateStates(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	statuses, err := List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if got := fileStateOf(t, statuses, "agentbox-orchestrator", "issue.md").State; got != "current" {
		t.Fatalf("fresh template state = %q", got)
	}
	if stateOf(t, statuses, "agentbox-orchestrator", "agents").State != "current" {
		t.Fatal("SKILL.md row changed shape")
	}

	statuses, err = List(ListOptions{Version: "2.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if got := fileStateOf(t, statuses, "agentbox-orchestrator", "issue.md").State; got != "stale" {
		t.Fatalf("stale template state = %q", got)
	}

	path := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator", "verdict.md")
	if err := os.WriteFile(path, []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(home, ".agents", "skills", "agentbox-orchestrator", "pr.md")); err != nil {
		t.Fatal(err)
	}
	statuses, err = List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if got := fileStateOf(t, statuses, "agentbox-orchestrator", "verdict.md").State; got != "modified" {
		t.Fatalf("modified template state = %q", got)
	}
	if got := fileStateOf(t, statuses, "agentbox-orchestrator", "pr.md").State; got != "missing" {
		t.Fatalf("missing template state = %q", got)
	}
	// Skills without templates keep the previous JSON shape.
	for _, status := range statuses {
		if status.Name == "agentbox-cli" && len(status.Files) != 0 {
			t.Fatalf("agentbox-cli gained template rows: %+v", status.Files)
		}
	}
}
