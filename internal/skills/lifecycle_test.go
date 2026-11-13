package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// prepareHome creates a home directory in which every harness is detectable
// and returns it.
func prepareHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, dir := range []string{".claude", ".codex", filepath.Join(".config", "opencode")} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func findResult(results []Result, action string) *Result {
	for i := range results {
		if results[i].Action == action {
			return &results[i]
		}
	}
	return nil
}

func actionsOf(results []Result, skill string) []string {
	var out []string
	for _, result := range results {
		if result.Skill == skill {
			out = append(out, result.Target+"/"+result.Action)
		}
	}
	return out
}

// TestInstallWritesCanonicalCopyAndRelativeLinks verifies the core layout
// contract: one canonical copy, relative symlinks for Claude and OpenCode,
// and nothing under ~/.codex/skills.
func TestInstallWritesCanonicalCopyAndRelativeLinks(t *testing.T) {
	home := prepareHome(t)
	results, err := Install(Options{All: true, Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range Names() {
		canonical := filepath.Join(home, ".agents", "skills", name, "SKILL.md")
		if body := mustRead(t, canonical); InstalledVersion(body) != "1.0.0" {
			t.Fatalf("%s canonical version = %s", name, InstalledVersion(body))
		}
		claudeLink := filepath.Join(home, ".claude", "skills", name)
		if got := readLink(t, claudeLink); got != "../../.agents/skills/"+name {
			t.Fatalf("claude link target = %q", got)
		}
		opencodeLink := filepath.Join(home, ".config", "opencode", "skills", name)
		if got := readLink(t, opencodeLink); got != "../../../.agents/skills/"+name {
			t.Fatalf("opencode link target = %q", got)
		}
	}
	// The manifest records the canonical copy, its hash, and both links.
	manifest := loadManifest(filepath.Join(home, ".agents", "skills"))
	if len(manifest.Skills) != len(Names()) {
		t.Fatalf("manifest skills = %d", len(manifest.Skills))
	}
	for _, name := range Names() {
		record, ok := manifest.Skills[name]
		if !ok {
			t.Fatalf("%s missing from the manifest", name)
		}
		entry, tracked := record.File("SKILL.md")
		if record.Version != "1.0.0" || !tracked || len(entry.SHA256) != 64 {
			t.Fatalf("%s manifest record = %+v", name, record)
		}
		if len(record.Files) != len(Files(name)) {
			t.Fatalf("%s manifest files = %d, want %d", name, len(record.Files), len(Files(name)))
		}
		if record.Path != filepath.Join(home, ".agents", "skills", name, "SKILL.md") {
			t.Fatalf("%s manifest path = %q", name, record.Path)
		}
		if len(record.Links) != 2 {
			t.Fatalf("%s manifest links = %v", name, record.Links)
		}
	}
	// Nothing may be written into the Codex skills directory.
	codexEntries, err := os.ReadDir(filepath.Join(home, ".codex", "skills"))
	if err == nil && len(codexEntries) != 0 {
		t.Fatalf("install wrote into ~/.codex/skills: %v", codexEntries)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	// Each shipped file produced one canonical write; each skill produced two
	// links.
	for _, name := range Names() {
		got := strings.Join(actionsOf(results, name), ",")
		var actions []string
		for range Files(name) {
			actions = append(actions, "agents/installed")
		}
		actions = append(actions, "claude/linked", "opencode/linked")
		want := strings.Join(actions, ",")
		if got != want {
			t.Fatalf("%s actions = %s, want %s", name, got, want)
		}
	}
}

func readLink(t *testing.T, path string) string {
	t.Helper()
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("read link %s: %v", path, err)
	}
	return target
}

// TestInstallIsIdempotent re-runs a completed install and expects no changes.
func TestInstallIsIdempotent(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	results, err := Install(Options{All: true, Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Action == "linked" || result.Action == "installed" {
			t.Fatalf("second install changed %s: %+v", result.Skill, result)
		}
	}
}

// TestInstallDefaultTargetsDetectedHarnesses checks that a bare install links
// only the harnesses whose directories exist.
func TestInstallDefaultTargetsDetectedHarnesses(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(Options{Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "agentbox-cli")); err != nil {
		t.Fatal("detected claude harness did not get a link")
	}
	for _, missing := range []string{
		filepath.Join(home, ".config", "opencode"),
		filepath.Join(home, ".codex"),
	} {
		if _, err := os.Stat(missing); !os.IsNotExist(err) {
			t.Fatalf("install created an undetected harness directory: %s", missing)
		}
	}
}

// TestInstallExplicitAgentCreatesHarnessDirectory verifies that an explicit
// selection creates the harness directory even when it is not detected.
func TestInstallExplicitAgentCreatesHarnessDirectory(t *testing.T) {
	home := t.TempDir()
	if _, err := Install(Options{Agents: []string{"opencode"}, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config", "opencode", "skills", "agentbox-cli")); err != nil {
		t.Fatal("explicit opencode harness did not get a link")
	}
	// Codex never receives a link even when selected explicitly.
	if _, err := Install(Options{Agents: []string{"codex"}, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".codex", "skills")); !os.IsNotExist(err) {
		t.Fatal("codex selection created ~/.codex/skills")
	}
}

// TestInstallLocalUsesProjectScopedLayout checks the --local layout: the
// canonical copy lands in ./.agents/skills and the links are relative to the
// project, not the home directory.
func TestInstallLocalUsesProjectScopedLayout(t *testing.T) {
	project := t.TempDir()
	results, err := Install(Options{Local: true, ProjectDir: project, All: true, Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(project, ".agents", "skills", "agentbox-cli", "SKILL.md")
	if _, err := os.Stat(canonical); err != nil {
		t.Fatal(err)
	}
	if got := readLink(t, filepath.Join(project, ".claude", "skills", "agentbox-cli")); got != "../../.agents/skills/agentbox-cli" {
		t.Fatalf("claude link = %q", got)
	}
	if got := readLink(t, filepath.Join(project, ".opencode", "skills", "agentbox-cli")); got != "../../.agents/skills/agentbox-cli" {
		t.Fatalf("opencode link = %q", got)
	}
	if findResult(results, "installed") == nil {
		t.Fatalf("local install results = %+v", results)
	}
}

// TestInstallDirOverrideWritesPlainCopies keeps the --dir escape hatch: plain
// copies, no manifest, and a refusal inside ~/.codex/skills.
func TestInstallDirOverrideWritesPlainCopies(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom", "skills")
	results, err := Install(Options{Dir: dir, Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Path != filepath.Join(dir, result.Skill, result.File) {
			t.Fatalf("path = %s", result.Path)
		}
		if _, err := os.Stat(result.Path); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"issue.md", "pr.md", "verdict.md", "state.md"} {
		if _, err := os.Stat(filepath.Join(dir, "agentbox-orchestrator", name)); err != nil {
			t.Fatalf("--dir did not write %s: %v", name, err)
		}
	}
	home := t.TempDir()
	if _, err := Install(Options{Dir: filepath.Join(home, ".codex", "skills"), Home: home}); err == nil {
		t.Fatal("accepted --dir inside ~/.codex/skills")
	}
	if _, err := Install(Options{Dir: dir, Agents: []string{"claude"}}); err == nil {
		t.Fatal("accepted --dir combined with --agent")
	}
}

// TestInstallDirForceBacksUpModifiedFile preserves the promise every other
// --force path makes: the replaced content lands in a timestamped backup
// before the embedded copy is written.
func TestInstallDirForceBacksUpModifiedFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom", "skills")
	if _, err := Install(Options{Dir: dir, Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agentbox-cli", "SKILL.md")
	if err := os.WriteFile(path, []byte("edited locally\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(Options{Dir: dir, Version: "2.0.0"}); err == nil {
		t.Fatal("--dir overwrote a modified file without --force")
	}
	if body := mustRead(t, path); string(body) != "edited locally\n" {
		t.Fatalf("refused --dir install changed the file: %q", body)
	}

	results, err := Install(Options{Dir: dir, Version: "2.0.0", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	var forced *Result
	for i := range results {
		if results[i].Skill == "agentbox-cli" && results[i].File == "SKILL.md" {
			forced = &results[i]
		}
	}
	if forced == nil || forced.Action != "updated" || !strings.Contains(forced.Detail, "backed up") {
		t.Fatalf("forced --dir result = %+v", results)
	}
	if got := InstalledVersion(mustRead(t, path)); got != "2.0.0" {
		t.Fatalf("version after --dir --force = %q", got)
	}
	backup := filepath.Join(dir, ".agentbox-skill-backups")
	stamps, err := os.ReadDir(backup)
	if err != nil || len(stamps) != 1 {
		t.Fatalf("--dir backup stamps = %v (%v)", stamps, err)
	}
	saved := filepath.Join(backup, stamps[0].Name(), "agentbox-cli", "SKILL.md")
	if body := mustRead(t, saved); string(body) != "edited locally\n" {
		t.Fatalf("--dir backup content = %q", body)
	}
}

// TestInstallRefusesModifiedCopyWithoutForce preserves user edits and only
// replaces them with --force after a timestamped backup.
func TestInstallRefusesModifiedCopyWithoutForce(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".agents", "skills", "agentbox-cli", "SKILL.md")
	if err := os.WriteFile(path, []byte("edited locally\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err == nil {
		t.Fatal("overwrote a modified copy without --force")
	}
	if body := mustRead(t, path); string(body) != "edited locally\n" {
		t.Fatalf("modified copy changed: %q", body)
	}
	results, err := Install(Options{All: true, Version: "1.0.0", Home: home, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if result := findResult(results, "updated"); result == nil || !strings.Contains(result.Detail, "backed up") {
		t.Fatalf("force result missing backup detail: %+v", results)
	}
	if got := InstalledVersion(mustRead(t, path)); got != "1.0.0" {
		t.Fatalf("version after force = %q", got)
	}
	backup := filepath.Join(home, ".agents", ".agentbox-skill-backups")
	stamps, err := os.ReadDir(backup)
	if err != nil || len(stamps) != 1 {
		t.Fatalf("backup stamps = %v (%v)", stamps, err)
	}
	saved := filepath.Join(backup, stamps[0].Name(), "canonical", "agentbox-cli", "SKILL.md")
	if string(mustRead(t, saved)) != "edited locally\n" {
		t.Fatalf("backup content = %q", mustRead(t, saved))
	}
}

// TestInstallRefusesForeignDestinationAtLinkPath checks that an existing real
// directory where a link belongs is reported and skipped, and replaced only
// with --force after a backup.
func TestInstallRefusesForeignDestinationAtLinkPath(t *testing.T) {
	home := prepareHome(t)
	foreign := filepath.Join(home, ".claude", "skills", "agentbox-cli")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "SKILL.md"), []byte("someone else's skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := Install(Options{All: true, Version: "1.0.0", Home: home})
	if err == nil {
		t.Fatal("linked over a foreign directory without --force")
	}
	if result := findResult(results, "skipped"); result == nil || result.Target != "claude" {
		t.Fatalf("foreign destination was not reported: %+v", results)
	}
	if body := mustRead(t, filepath.Join(foreign, "SKILL.md")); string(body) != "someone else's skill\n" {
		t.Fatalf("foreign directory was modified: %q", body)
	}
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home, Force: true}); err != nil {
		t.Fatal(err)
	}
	if got := readLink(t, foreign); got != "../../.agents/skills/agentbox-cli" {
		t.Fatalf("forced link = %q", got)
	}
	backup := filepath.Join(home, ".agents", ".agentbox-skill-backups")
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("forced replacement did not back up: %v", err)
	}
}

// TestInstallMigratesByteIdenticalLegacyCopyAtLinkPath covers the layout
// written by PR #29: a real directory holding a byte-identical SKILL.md is
// replaced by the link without --force.
func TestInstallMigratesByteIdenticalLegacyCopyAtLinkPath(t *testing.T) {
	home := prepareHome(t)
	for _, name := range Names() {
		content, err := Embedded(name, "1.0.0")
		if err != nil {
			t.Fatal(err)
		}
		legacy := filepath.Join(home, ".claude", "skills", name)
		if err := os.MkdirAll(legacy, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(legacy, "SKILL.md"), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	results, err := Install(Options{All: true, Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if result := findResult(results, "migrated"); result == nil || result.Target != "claude" {
		t.Fatalf("legacy copy was not migrated: %+v", results)
	}
	if got := readLink(t, filepath.Join(home, ".claude", "skills", "agentbox-cli")); got != "../../.agents/skills/agentbox-cli" {
		t.Fatalf("migrated link = %q", got)
	}
}

// TestInstallDryRunWritesNothing verifies the plan-only mode.
func TestInstallDryRunWritesNothing(t *testing.T) {
	home := prepareHome(t)
	results, err := Install(Options{All: true, Version: "1.0.0", Home: home, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"would-install", "would-link"} {
		if findResult(results, want) == nil {
			t.Fatalf("dry run missing %s: %+v", want, results)
		}
	}
	for _, path := range []string{
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".config", "opencode", "skills"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("dry run created %s", path)
		}
	}
}

// TestUpdateRefreshesStaleAndSkipsMissingAndModified pins the update
// lifecycle: untouched stale copies refresh, missing copies are reported, and
// modified copies are skipped without --force.
func TestUpdateRefreshesStaleAndSkipsMissingAndModified(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	// Simulate a user edit that keeps the version line.
	edited := filepath.Join(home, ".agents", "skills", "agentbox-api", "SKILL.md")
	if err := os.WriteFile(edited, []byte("---\nname: agentbox-api\ndescription: \"edited\"\nversion: \"1.0.0\"\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := Update(Options{All: true, Version: "2.0.0", Home: home})
	if err == nil {
		t.Fatal("update silently accepted a modified copy")
	}
	for _, result := range results {
		switch {
		case result.Skill == "agentbox-cli" && result.Target == "agents":
			if result.Action != "updated" {
				t.Fatalf("stale copy action = %s", result.Action)
			}
		case result.Skill == "agentbox-api" && result.Target == "agents":
			if result.Action != "skipped" {
				t.Fatalf("modified copy action = %s", result.Action)
			}
		}
	}
	if got := InstalledVersion(mustRead(t, filepath.Join(home, ".agents", "skills", "agentbox-cli", "SKILL.md"))); got != "2.0.0" {
		t.Fatalf("stale copy version after update = %q", got)
	}
	if body := mustRead(t, edited); !strings.Contains(string(body), "edited") {
		t.Fatalf("modified copy was overwritten: %q", body)
	}
	// With --force the modified copy is backed up and replaced.
	results, err = Update(Options{All: true, Version: "2.0.0", Home: home, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if result := findResult(results, "updated"); result == nil || !strings.Contains(result.Detail, "backed up") {
		t.Fatalf("forced update missing backup: %+v", results)
	}
	// Update never creates a missing canonical copy.
	if _, err := Update(Options{All: true, Version: "2.0.0", Home: t.TempDir()}); err != nil {
		t.Fatalf("update on an empty home failed: %v", err)
	}
}

// TestUpdateRepairsBrokenLink covers the broken-link remediation: a link
// whose target no longer resolves is replaced without --force.
func TestUpdateRepairsBrokenLink(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".claude", "skills", "agentbox-cli")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../.agents/skills/removed-skill", link); err != nil {
		t.Fatal(err)
	}
	results, err := Update(Options{All: true, Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if result := findResult(results, "linked"); result == nil || !strings.Contains(result.Detail, "dangling") {
		t.Fatalf("broken link was not repaired: %+v", results)
	}
	if got := readLink(t, link); got != "../../.agents/skills/agentbox-cli" {
		t.Fatalf("repaired link = %q", got)
	}
}

// TestLegacyCodexMigration verifies the conservative migration of duplicates
// written by releases before the canonical layout: byte-identical copies are
// removed, differing copies are preserved and reported.
func TestLegacyCodexMigration(t *testing.T) {
	home := prepareHome(t)
	identical, err := Embedded("agentbox-cli", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	legacyDir := filepath.Join(home, ".codex", "skills")
	if err := os.MkdirAll(filepath.Join(legacyDir, "agentbox-cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "agentbox-cli", "SKILL.md"), identical, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(legacyDir, "agentbox-api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "agentbox-api", "SKILL.md"), []byte("edited legacy copy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := Install(Options{All: true, Version: "1.0.0", Home: home})
	if err == nil {
		t.Fatal("differing legacy copy was not reported as an error")
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "agentbox-cli")); !os.IsNotExist(err) {
		t.Fatal("byte-identical legacy copy was not removed")
	}
	if body := mustRead(t, filepath.Join(legacyDir, "agentbox-api", "SKILL.md")); string(body) != "edited legacy copy\n" {
		t.Fatalf("differing legacy copy was removed: %q", body)
	}
	reported := false
	for _, result := range results {
		if result.Target == "codex" && result.Action == "skipped" && strings.Contains(result.Detail, "--force") {
			reported = true
		}
	}
	if !reported {
		t.Fatalf("differing legacy copy was not reported: %+v", results)
	}
	// --force removes the differing copy after backing it up.
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home, Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "agentbox-api")); !os.IsNotExist(err) {
		t.Fatal("differing legacy copy survived --force")
	}
	backup := filepath.Join(home, ".agents", ".agentbox-skill-backups")
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("forced migration did not back up: %v", err)
	}
}

// TestRemoveDeletesOnlyManagedPaths covers the remove lifecycle: manifest
// records go first, modified copies refuse without --force, and foreign
// content at recorded link paths is never deleted.
func TestRemoveDeletesOnlyManagedPaths(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	results, err := Remove(Options{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Action != "removed" && result.Action != "missing" {
			t.Fatalf("remove action = %+v", result)
		}
	}
	for _, name := range Names() {
		if _, err := os.Stat(filepath.Join(home, ".agents", "skills", name)); !os.IsNotExist(err) {
			t.Fatalf("%s canonical copy survived remove", name)
		}
		for _, link := range []string{
			filepath.Join(home, ".claude", "skills", name),
			filepath.Join(home, ".config", "opencode", "skills", name),
		} {
			if _, err := os.Lstat(link); !os.IsNotExist(err) {
				t.Fatalf("link survived remove: %s", link)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", ManifestFile)); !os.IsNotExist(err) {
		t.Fatal("manifest survived removing every skill")
	}
	if _, err := Remove(Options{Version: "1.0.0", Home: home}); err == nil {
		t.Fatal("remove without a manifest succeeded")
	}
}

// TestRemoveRefusesModifiedCopyWithoutForce keeps user edits on disk and
// backs them up before a forced removal.
func TestRemoveRefusesModifiedCopyWithoutForce(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".agents", "skills", "agentbox-cli", "SKILL.md")
	if err := os.WriteFile(path, []byte("edited locally\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(Options{Version: "1.0.0", Home: home}); err == nil {
		t.Fatal("removed a modified copy without --force")
	}
	if body := mustRead(t, path); string(body) != "edited locally\n" {
		t.Fatalf("modified copy was removed: %q", body)
	}
	if _, err := Remove(Options{Version: "1.0.0", Home: home, Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("forced remove left the modified copy behind")
	}
	backup := filepath.Join(home, ".agents", ".agentbox-skill-backups")
	stamps, err := os.ReadDir(backup)
	if err != nil || len(stamps) != 1 {
		t.Fatalf("backup stamps = %v (%v)", stamps, err)
	}
	saved := filepath.Join(backup, stamps[0].Name(), "canonical", "agentbox-cli", "SKILL.md")
	if string(mustRead(t, saved)) != "edited locally\n" {
		t.Fatalf("backup content = %q", mustRead(t, saved))
	}
}

// TestRemoveLeavesForeignContentAtRecordedLinkAlone replaces a recorded link
// with a real directory and verifies remove reports it without deleting it.
func TestRemoveLeavesForeignContentAtRecordedLinkAlone(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".claude", "skills", "agentbox-cli")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(link, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(link, "SKILL.md"), []byte("foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := Remove(Options{Version: "1.0.0", Home: home, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if body := mustRead(t, filepath.Join(link, "SKILL.md")); string(body) != "foreign\n" {
		t.Fatalf("foreign directory was deleted: %q", body)
	}
	for _, result := range results {
		if result.Path == link && result.Action != "skipped" {
			t.Fatalf("foreign link handling = %+v", result)
		}
	}
}

// TestRemoveAgentFilterRemovesOnlyLinksFromThoseHarnesses checks the scoped
// remove: only the recorded links of the selected harnesses disappear.
func TestRemoveAgentFilterRemovesOnlyLinksFromThoseHarnesses(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(Options{Agents: []string{"claude"}, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "agentbox-cli")); !os.IsNotExist(err) {
		t.Fatal("claude link survived a scoped remove")
	}
	for _, name := range Names() {
		if _, err := os.Stat(filepath.Join(home, ".agents", "skills", name, "SKILL.md")); err != nil {
			t.Fatalf("scoped remove deleted the canonical copy of %s", name)
		}
		if _, err := os.Lstat(filepath.Join(home, ".config", "opencode", "skills", name)); err != nil {
			t.Fatalf("scoped remove deleted the opencode link of %s", name)
		}
	}
	manifest := loadManifest(filepath.Join(home, ".agents", "skills"))
	for _, name := range Names() {
		if len(manifest.Skills[name].Links) != 1 {
			t.Fatalf("%s links after scoped remove = %v", name, manifest.Skills[name].Links)
		}
	}
}

// TestManifestRoundTrip verifies the manifest survives a save/load cycle
// with its exact fields.
func TestManifestRoundTrip(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	record := ManifestSkill{
		Version: "1.2.3",
		Path:    filepath.Join(root, "agentbox-cli", "SKILL.md"),
		Links:   []string{filepath.Join(t.TempDir(), "link")},
		Files: []ManifestEntry{{
			Name:   "SKILL.md",
			SHA256: strings.Repeat("a", 64),
			Path:   filepath.Join(root, "agentbox-cli", "SKILL.md"),
		}},
	}
	if err := saveManifest(root, &Manifest{Skills: map[string]ManifestSkill{"agentbox-cli": record}}); err != nil {
		t.Fatal(err)
	}
	loaded := loadManifest(root)
	if loaded.Version != manifestVersion {
		t.Fatalf("manifest version = %d", loaded.Version)
	}
	got, ok := loaded.Skills["agentbox-cli"]
	entry, tracked := got.File("SKILL.md")
	if !ok || got.Version != record.Version || !tracked || entry.SHA256 != record.Files[0].SHA256 || got.Path != record.Path || len(got.Links) != 1 {
		t.Fatalf("round trip = %+v (ok = %v)", got, ok)
	}
	// A corrupt manifest loads as empty instead of failing.
	if err := os.WriteFile(manifestPath(root), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if empty := loadManifest(root); len(empty.Skills) != 0 {
		t.Fatalf("corrupt manifest loaded skills = %v", empty.Skills)
	}
}

// TestDeprecatedTargetAliasMapsToAgents pins the --target compatibility map:
// the same destinations as --agent, with agents selecting no harness at all.
func TestDeprecatedTargetAliasMapsToAgents(t *testing.T) {
	for target, wantLinks := range map[string]int{
		"agents":   0,
		"codex":    0,
		"claude":   1,
		"opencode": 1,
		"all":      2,
	} {
		home := prepareHome(t)
		if _, err := Install(Options{Target: target, Version: "1.0.0", Home: home}); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		var links int
		for _, name := range Names() {
			if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", name)); err == nil {
				links++
			}
			if _, err := os.Lstat(filepath.Join(home, ".config", "opencode", "skills", name)); err == nil {
				links++
			}
		}
		if links != wantLinks*len(Names()) {
			t.Fatalf("target %s created %d links, want %d", target, links, wantLinks*len(Names()))
		}
		if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "agentbox-cli", "SKILL.md")); err != nil {
			t.Fatalf("target %s did not write the canonical copy", target)
		}
	}
	home := prepareHome(t)
	if _, err := Install(Options{Target: "bogus", Home: home}); err == nil {
		t.Fatal("accepted an unknown --target value")
	}
	if _, err := Install(Options{Target: "all", Agents: []string{"claude"}, Home: home}); err == nil {
		t.Fatal("accepted --target combined with --agent")
	}
	if _, err := Update(Options{Target: "all", Home: home}); err == nil {
		t.Fatal("update accepted --target")
	}
}

// TestInstallWritesTemplatesAsSiblings keeps the packaging contract: the
// orchestrator templates install beside SKILL.md, not in a templates/
// subdirectory.
func TestInstallWritesTemplatesAsSiblings(t *testing.T) {
	home := prepareHome(t)
	results, err := Install(Options{All: true, Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator")
	for _, name := range []string{"SKILL.md", "issue.md", "pr.md", "verdict.md", "state.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s missing beside SKILL.md: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "templates")); !os.IsNotExist(err) {
		t.Fatal("templates were installed in a templates/ subdirectory")
	}
	var files int
	for _, result := range results {
		if result.Skill == "agentbox-orchestrator" && result.Target == "agents" {
			files++
		}
	}
	if files != len(Files("agentbox-orchestrator")) {
		t.Fatalf("canonical file results = %d, want %d", files, len(Files("agentbox-orchestrator")))
	}
}

// TestUpdateRefreshesStaleAndProtectsModifiedTemplates covers per-file
// lifecycle decisions inside one skill directory.
func TestUpdateRefreshesStaleAndProtectsModifiedTemplates(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator")
	results, err := Update(Options{All: true, Version: "2.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	var refreshed bool
	for _, result := range results {
		if result.Skill == "agentbox-orchestrator" && result.File == "issue.md" {
			refreshed = result.Action == "updated" && strings.Contains(result.Detail, "refreshed from version 1.0.0")
		}
	}
	if !refreshed {
		t.Fatalf("stale template was not refreshed: %+v", actionsOf(results, "agentbox-orchestrator"))
	}
	if body := string(mustRead(t, filepath.Join(dir, "issue.md"))); !strings.Contains(body, "agentbox-orchestrator 2.0.0") {
		t.Fatalf("template footer was not refreshed: %s", body)
	}

	// A hand-edited template is refused without --force and preserved.
	path := filepath.Join(dir, "verdict.md")
	if err := os.WriteFile(path, []byte("edited verdict template\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err = Update(Options{All: true, Version: "3.0.0", Home: home})
	if err == nil {
		t.Fatal("update overwrote a modified template without --force")
	}
	var skipped bool
	for _, result := range results {
		if result.Skill == "agentbox-orchestrator" && result.File == "verdict.md" {
			skipped = result.Action == "skipped"
		}
	}
	if !skipped {
		t.Fatalf("modified template result = %+v", actionsOf(results, "agentbox-orchestrator"))
	}
	if body := mustRead(t, path); string(body) != "edited verdict template\n" {
		t.Fatalf("modified template changed: %q", body)
	}

	// --force backs up the template at its managed relative path and replaces it.
	if _, err := Update(Options{All: true, Version: "3.0.0", Home: home, Force: true}); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(home, ".agents", ".agentbox-skill-backups")
	stamps, err := os.ReadDir(backup)
	if err != nil || len(stamps) == 0 {
		t.Fatalf("backup stamps = %v (%v)", stamps, err)
	}
	saved := filepath.Join(backup, stamps[0].Name(), "canonical", "agentbox-orchestrator", "verdict.md")
	if string(mustRead(t, saved)) != "edited verdict template\n" {
		t.Fatalf("template backup content = %q", mustRead(t, saved))
	}
}

// TestUpdateAddsTemplatesToLegacyManifestV1Install migrates a version 1
// install without orphaning it: the recorded SKILL.md hash is recognised, the
// skill reports stale against a newer binary, and update adds the templates
// and rewrites the manifest as version 2.
func TestUpdateAddsTemplatesToLegacyManifestV1Install(t *testing.T) {
	home := prepareHome(t)
	root := filepath.Join(home, ".agents", "skills")
	dir := filepath.Join(root, "agentbox-orchestrator")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content, err := Embedded("agentbox-orchestrator", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := `{"version":1,"skills":{"agentbox-orchestrator":{"version":"1.0.0","sha256":"` + hashHex(content) + `","path":"` + path + `","links":[]}}}`
	if err := os.WriteFile(filepath.Join(root, ManifestFile), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	// A v1 record still proves the copy is AgentBox's, so it reports stale
	// rather than modified, and the missing templates are visible.
	statuses, err := List(ListOptions{Version: "2.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, statuses, "agentbox-orchestrator", "agents").State; got != "stale" {
		t.Fatalf("v1 canonical state = %q, want stale", got)
	}
	for _, file := range templateStatusesFor(t, statuses, "agentbox-orchestrator") {
		if file.State != "missing" {
			t.Fatalf("%s state = %+v", file.Name, file)
		}
	}

	results, err := Update(Options{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	var added int
	for _, result := range results {
		if result.Skill == "agentbox-orchestrator" && result.Target == "agents" && result.Action == "installed" {
			added++
		}
	}
	if added != 4 {
		t.Fatalf("installed template rows = %d: %s", added, actionsOf(results, "agentbox-orchestrator"))
	}
	for _, name := range []string{"issue.md", "pr.md", "verdict.md", "state.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s was not added by update: %v", name, err)
		}
	}
	body := mustRead(t, filepath.Join(root, ManifestFile))
	if !strings.Contains(string(body), `"version": 2`) {
		t.Fatalf("manifest was not rewritten as version 2: %s", body)
	}
	manifest := loadManifest(root)
	if got := len(manifest.Skills["agentbox-orchestrator"].Files); got != len(Files("agentbox-orchestrator")) {
		t.Fatalf("migrated manifest files = %d", got)
	}
}

// TestRemoveDeletesTemplatesAndKeepsForeignFiles proves remove deletes only
// managed files and leaves unrelated user content and the directory behind.
func TestRemoveDeletesTemplatesAndKeepsForeignFiles(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator")
	foreign := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(foreign, []byte("user notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(Options{Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SKILL.md", "issue.md", "pr.md", "verdict.md", "state.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s survived remove", name)
		}
	}
	if body := mustRead(t, foreign); string(body) != "user notes\n" {
		t.Fatalf("remove touched a foreign file: %q", body)
	}
}

// TestRemoveKeepsUnrecordedTemplate keeps a file at a shipped template path
// that the manifest never recorded, even when the file is byte-identical to
// the embedded template.
func TestRemoveKeepsUnrecordedTemplate(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".agents", "skills")
	dir := filepath.Join(root, "agentbox-orchestrator")
	unrecorded := filepath.Join(dir, "issue.md")
	original := mustRead(t, unrecorded)

	manifest := loadManifest(root)
	record := manifest.Skills["agentbox-orchestrator"]
	kept := record.Files[:0]
	for _, file := range record.Files {
		if file.Name != "issue.md" {
			kept = append(kept, file)
		}
	}
	record.Files = kept
	manifest.Skills["agentbox-orchestrator"] = record
	if err := saveManifest(root, manifest); err != nil {
		t.Fatal(err)
	}

	results, err := Remove(Options{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if body := mustRead(t, unrecorded); !bytes.Equal(body, original) {
		t.Fatalf("remove deleted or changed an unrecorded template: %q", body)
	}
	var reported bool
	for _, result := range results {
		if result.Skill == "agentbox-orchestrator" && result.File == "issue.md" {
			reported = result.Action == "skipped" && strings.Contains(result.Detail, "not recorded")
		}
	}
	if !reported {
		t.Fatalf("unrecorded template was not reported as left alone: %+v", actionsOf(results, "agentbox-orchestrator"))
	}
	for _, name := range []string{"SKILL.md", "pr.md", "verdict.md", "state.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s survived remove", name)
		}
	}
}

// TestRemoveRefusesModifiedTemplateWithoutForce keeps the manifest record so
// a later forced removal can still clean the modified template.
func TestRemoveRefusesModifiedTemplateWithoutForce(t *testing.T) {
	home := prepareHome(t)
	if _, err := Install(Options{All: true, Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".agents", "skills")
	dir := filepath.Join(root, "agentbox-orchestrator")
	path := filepath.Join(dir, "pr.md")
	if err := os.WriteFile(path, []byte("edited pr template\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(Options{Version: "1.0.0", Home: home}); err == nil {
		t.Fatal("removed a modified template without --force")
	}
	if body := mustRead(t, path); string(body) != "edited pr template\n" {
		t.Fatalf("modified template was removed: %q", body)
	}
	if _, ok := loadManifest(root).Skills["agentbox-orchestrator"]; !ok {
		t.Fatal("manifest record was dropped while a modified template remained")
	}
	if _, err := Remove(Options{Version: "1.0.0", Home: home, Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("forced remove left the modified template behind")
	}
}
