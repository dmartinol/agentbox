package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmartinol/agentbox/internal/skills"
)

func doctorSkillCheck(t *testing.T, a *App) Check {
	t.Helper()
	var found Check
	for _, check := range a.Doctor(context.Background()) {
		if check.Name == "skills" {
			found = check
		}
	}
	if found.Name == "" {
		t.Fatal("doctor did not report skills")
	}
	return found
}

func installedApp(t *testing.T, version string) *App {
	return &App{Store: Store{Base: t.TempDir()}, Runner: &fakeRunner{}, Version: version}
}

func makeUnreadable(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("permission bits do not restrict this user")
	}
}

func TestDoctorReportsStaleUsageSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := skills.Install(skills.Options{All: true, Version: "0.1.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
	if found.OK || !strings.Contains(found.Detail, "stale") {
		t.Fatalf("stale check = %+v", found)
	}
	if !strings.Contains(found.Detail, "skills update") {
		t.Fatalf("stale check lacks the recovery command: %s", found.Detail)
	}
}

func TestDoctorReportsCurrentUsageSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
	if !found.OK || !strings.Contains(found.Detail, "up to date") {
		t.Fatalf("current check = %+v", found)
	}
}

func TestDoctorReportsModifiedUsageSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".agents", "skills", "agentbox-cli", "SKILL.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, []byte("hand edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
	if found.OK || !strings.Contains(found.Detail, "modified") {
		t.Fatalf("modified check = %+v", found)
	}
	if !strings.Contains(found.Detail, "skills update --force") || !strings.Contains(found.Detail, "backed up") {
		t.Fatalf("modified check lacks the recovery command: %s", found.Detail)
	}
}

func TestDoctorReportsModifiedUsageSkillTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator", "pr.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, []byte("hand edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
	if found.OK || !strings.Contains(found.Detail, "modified") {
		t.Fatalf("modified template check = %+v", found)
	}
	if !strings.Contains(found.Detail, path) {
		t.Fatalf("modified template check lacks the template path: %s", found.Detail)
	}
	if !strings.Contains(found.Detail, "skills update --force") || !strings.Contains(found.Detail, "backed up") {
		t.Fatalf("modified template check lacks the recovery command: %s", found.Detail)
	}
}

// TestDoctorRecommendsInstallForModifiedTemplateWithoutCanonicalSkill covers
// a template modified after SKILL.md was removed. Update skips the whole
// skill in that state, so the doctor must recommend install --force.
func TestDoctorRecommendsInstallForModifiedTemplateWithoutCanonicalSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator")
	if err := os.Remove(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pr.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, []byte("hand edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
	if found.OK || !strings.Contains(found.Detail, "modified: "+path) {
		t.Fatalf("modified template check = %+v", found)
	}
	if !strings.Contains(found.Detail, "agentbox skills install --force") || !strings.Contains(found.Detail, "backed up") {
		t.Fatalf("modified template without SKILL.md lacks the install command: %s", found.Detail)
	}
	if strings.Contains(found.Detail, "skills update") {
		t.Fatalf("modified template without SKILL.md recommends update: %s", found.Detail)
	}
}

// TestDoctorRecommendsInstallForStaleTemplateWithoutCanonicalSkill covers a
// stale template whose canonical SKILL.md is gone. Install creates SKILL.md
// and refreshes the stale template; update would skip the skill entirely.
func TestDoctorRecommendsInstallForStaleTemplateWithoutCanonicalSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := skills.Install(skills.Options{All: true, Version: "0.1.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator")
	if err := os.Remove(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
	if found.OK || !strings.Contains(found.Detail, "stale: ") {
		t.Fatalf("stale template check = %+v", found)
	}
	templates := []string{filepath.Join(dir, "issue.md"), filepath.Join(dir, "pr.md"), filepath.Join(dir, "verdict.md"), filepath.Join(dir, "state.md")}
	expected := "stale: " + strings.Join(templates, ", ") + "; run `agentbox skills install`"
	if !strings.Contains(found.Detail, expected) {
		t.Fatalf("stale template without SKILL.md lacks the install command: %s", found.Detail)
	}
	forbidden := "stale: " + strings.Join(templates, ", ") + "; run `agentbox skills update`"
	if strings.Contains(found.Detail, forbidden) {
		t.Fatalf("stale template without SKILL.md recommends update: %s", found.Detail)
	}
}

// TestDoctorReportsUnreadableTemplateAndForceRepairsIt covers the unreadable
// file state: the doctor names the path and the force command, and that
// command repairs the destination.
func TestDoctorReportsUnreadableTemplateAndForceRepairsIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator", "verdict.md")
	makeUnreadable(t, path)
	found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
	if found.OK || !strings.Contains(found.Detail, "unreadable files: "+path) {
		t.Fatalf("unreadable template check = %+v", found)
	}
	if !strings.Contains(found.Detail, "skills update --force") || !strings.Contains(found.Detail, "backed up") {
		t.Fatalf("unreadable template check lacks the repair command: %s", found.Detail)
	}
	if _, err := skills.Update(skills.Options{All: true, Version: "0.2.0", Home: home, Force: true}); err != nil {
		t.Fatalf("recommended repair failed: %v", err)
	}
	if body, err := os.ReadFile(path); err != nil || len(body) == 0 {
		t.Fatalf("template was not repaired: %v", err)
	}
}

// TestDoctorRecommendsInstallForUnreadableTemplateWithoutCanonicalSkill keeps
// the advice aligned with update's skip rule when SKILL.md is absent.
func TestDoctorRecommendsInstallForUnreadableTemplateWithoutCanonicalSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator")
	if err := os.Remove(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.md")
	makeUnreadable(t, path)
	found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
	if found.OK || !strings.Contains(found.Detail, "unreadable files: "+path) {
		t.Fatalf("unreadable template check = %+v", found)
	}
	if !strings.Contains(found.Detail, "agentbox skills install --force") {
		t.Fatalf("unreadable template without SKILL.md lacks the install command: %s", found.Detail)
	}
	if strings.Contains(found.Detail, "skills update") {
		t.Fatalf("unreadable template without SKILL.md recommends update: %s", found.Detail)
	}
}

// blockCanonicalSkillPath replaces the canonical skill directory with a
// regular file, or a symlink to one, the way a stray file can block the
// shared .agents/skills layout. It returns the SKILL.md path the doctor
// reports as unreadable.
func blockCanonicalSkillPath(t *testing.T, home, skill string, symlink bool) string {
	t.Helper()
	dir := filepath.Join(home, ".agents", "skills", skill)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(home, skill+"-blocker")
	if err := os.WriteFile(blocker, []byte("not a skill directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if symlink {
		if err := os.Symlink(blocker, dir); err != nil {
			t.Fatal(err)
		}
	} else if err := os.WriteFile(dir, []byte("not a skill directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "SKILL.md")
}

func canonicalInstallationOf(t *testing.T, statuses []skills.Status, skill string) skills.Installation {
	t.Helper()
	for _, status := range statuses {
		if status.Name != skill {
			continue
		}
		for _, installation := range status.Installations {
			if installation.Target == "agents" {
				return installation
			}
		}
	}
	t.Fatalf("no canonical installation for %s", skill)
	return skills.Installation{}
}

// TestDoctorReportsUnreadableCanonicalSkillPath covers a regular file and a
// symlink to one at ~/.agents/skills/agentbox-orchestrator. Update --force
// backs up the blocker and then skips the skill because SKILL.md is gone, so
// the doctor must recommend install --force, which repairs the path.
func TestDoctorReportsUnreadableCanonicalSkillPath(t *testing.T) {
	for _, tc := range []struct {
		name    string
		symlink bool
	}{
		{name: "regular file"},
		{name: "symlink to file", symlink: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home}); err != nil {
				t.Fatal(err)
			}
			path := blockCanonicalSkillPath(t, home, "agentbox-orchestrator", tc.symlink)

			statuses, err := skills.List(skills.ListOptions{Version: "0.2.0", Home: home})
			if err != nil {
				t.Fatal(err)
			}
			if inst := canonicalInstallationOf(t, statuses, "agentbox-orchestrator"); !inst.Present || inst.State != "unreadable" {
				t.Fatalf("canonical installation = %+v", inst)
			}

			found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
			if found.OK || !strings.Contains(found.Detail, "unreadable: "+path) {
				t.Fatalf("unreadable canonical check = %+v", found)
			}
			if !strings.Contains(found.Detail, "agentbox skills install --force") {
				t.Fatalf("unreadable canonical check lacks the install command: %s", found.Detail)
			}
			if strings.Contains(found.Detail, "skills update") {
				t.Fatalf("unreadable canonical check recommends update: %s", found.Detail)
			}

			if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home, Force: true}); err != nil {
				t.Fatalf("recommended repair failed: %v", err)
			}
			if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
				t.Fatalf("canonical path was not repaired: %v", err)
			}
			if body, err := os.ReadFile(path); err != nil || len(body) == 0 {
				t.Fatalf("SKILL.md was not restored: %v", err)
			}
			if repaired := doctorSkillCheck(t, installedApp(t, "0.2.0")); !repaired.OK {
				t.Fatalf("skills check still failing after repair: %+v", repaired)
			}
		})
	}
}

// TestDoctorReportsSymlinkedCanonicalSkillDirectoryAndForceRepairs covers a
// dangling link and a link to a foreign directory where the canonical skill
// directory belongs. Both are unreadable rather than missing or modified, so
// the doctor recommends install --force. The repair moves the link entry
// itself into the backup and leaves the link target untouched.
func TestDoctorReportsSymlinkedCanonicalSkillDirectoryAndForceRepairs(t *testing.T) {
	const skill = "agentbox-orchestrator"
	for _, kind := range []string{"dangling", "foreign directory"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home}); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, ".agents", "skills", skill)
			if err := os.RemoveAll(dir); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(home, skill+"-target")
			foreign := kind == "foreign directory"
			if foreign {
				if err := os.MkdirAll(target, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("foreign copy\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, "foreign-marker"), []byte("keep\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(target, dir); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "SKILL.md")

			statuses, err := skills.List(skills.ListOptions{Version: "0.2.0", Home: home})
			if err != nil {
				t.Fatal(err)
			}
			if inst := canonicalInstallationOf(t, statuses, skill); !inst.Present || inst.State != "unreadable" {
				t.Fatalf("canonical installation = %+v", inst)
			}

			found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
			if found.OK || !strings.Contains(found.Detail, "unreadable: "+path) {
				t.Fatalf("symlinked canonical check = %+v", found)
			}
			if !strings.Contains(found.Detail, "agentbox skills install --force") {
				t.Fatalf("symlinked canonical check lacks the install command: %s", found.Detail)
			}
			if strings.Contains(found.Detail, "skills update") {
				t.Fatalf("symlinked canonical check recommends update: %s", found.Detail)
			}

			if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home, Force: true}); err != nil {
				t.Fatalf("recommended repair failed: %v", err)
			}
			info, err := os.Lstat(dir)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				t.Fatalf("canonical path is not a real directory after repair: %v %v", info, err)
			}
			want, err := skills.Embedded(skill, "0.2.0")
			if err != nil {
				t.Fatal(err)
			}
			if body, err := os.ReadFile(path); err != nil || !bytes.Equal(body, want) {
				t.Fatalf("SKILL.md was not restored to the embedded copy: %v", err)
			}

			matches, err := filepath.Glob(filepath.Join(home, ".agents", ".agentbox-skill-backups", "*", "canonical", skill))
			if err != nil || len(matches) != 1 {
				t.Fatalf("expected one backed-up symlink entry, got %v (%v)", matches, err)
			}
			backup, err := os.Lstat(matches[0])
			if err != nil || backup.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("backup did not preserve the symlink entry: %v %v", backup, err)
			}
			if got, err := os.Readlink(matches[0]); err != nil || got != target {
				t.Fatalf("backed-up symlink target = %q (%v), want %q", got, err, target)
			}

			if foreign {
				if body, err := os.ReadFile(filepath.Join(target, "SKILL.md")); err != nil || string(body) != "foreign copy\n" {
					t.Fatalf("repair changed the symlink target: %q (%v)", body, err)
				}
				if body, err := os.ReadFile(filepath.Join(target, "foreign-marker")); err != nil || string(body) != "keep\n" {
					t.Fatalf("repair removed target content: %q (%v)", body, err)
				}
			} else if _, err := os.Lstat(target); err == nil {
				t.Fatalf("dangling link target %s unexpectedly exists", target)
			}
			if repaired := doctorSkillCheck(t, installedApp(t, "0.2.0")); !repaired.OK {
				t.Fatalf("skills check still failing after repair: %+v", repaired)
			}
		})
	}
}

// TestDoctorReportsUnreadableCanonicalInstallsForEverySkill covers skills
// without templates, whose unreadable canonical state was previously omitted
// from doctor output entirely.
func TestDoctorReportsUnreadableCanonicalInstallsForEverySkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{
		"agentbox-cli":          blockCanonicalSkillPath(t, home, "agentbox-cli", false),
		"agentbox-api":          blockCanonicalSkillPath(t, home, "agentbox-api", true),
		"agentbox-orchestrator": blockCanonicalSkillPath(t, home, "agentbox-orchestrator", false),
	}
	found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
	if found.OK {
		t.Fatalf("unreadable canonical check = %+v", found)
	}
	const prefix = "unreadable: "
	const suffix = "; run `agentbox skills install --force` (the previous path is backed up)"
	start := strings.Index(found.Detail, prefix)
	if start < 0 {
		t.Fatalf("doctor reported no unreadable canonical path: %s", found.Detail)
	}
	group, _, ok := strings.Cut(found.Detail[start+len(prefix):], suffix)
	if !ok {
		t.Fatalf("unreadable canonical group lacks the install advice: %s", found.Detail)
	}
	for skill, path := range paths {
		if !strings.Contains(group, path) {
			t.Fatalf("doctor omitted %s at %s: %s", skill, path, found.Detail)
		}
	}
	if !strings.Contains(found.Detail, "agentbox skills install --force") {
		t.Fatalf("unreadable canonical check lacks the install command: %s", found.Detail)
	}

	if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home, Force: true}); err != nil {
		t.Fatalf("recommended repair failed: %v", err)
	}
	for skill, path := range paths {
		if body, err := os.ReadFile(path); err != nil || len(body) == 0 {
			t.Fatalf("%s was not repaired: %v", skill, err)
		}
	}
	if repaired := doctorSkillCheck(t, installedApp(t, "0.2.0")); !repaired.OK {
		t.Fatalf("skills check still failing after repair: %+v", repaired)
	}
}

func TestDoctorReportsInvalidFrontMatter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	// Reproduce the PR #29 defect: an unquoted description with a colon.
	path := filepath.Join(home, ".agents", "skills", "agentbox-cli", "SKILL.md")
	broken := "---\nname: agentbox-cli\ndescription: broken: colon\nversion: \"0.2.0\"\n---\nbody\n"
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
	if found.OK || !strings.Contains(found.Detail, "invalid front matter") {
		t.Fatalf("invalid check = %+v", found)
	}
	if !strings.Contains(found.Detail, "skills install --force") {
		t.Fatalf("invalid check lacks the recovery command: %s", found.Detail)
	}
}

func TestDoctorReportsBrokenLinksAndLegacyCodexCopies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := skills.Install(skills.Options{All: true, Version: "0.2.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".claude", "skills", "agentbox-cli")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../.agents/skills/gone", link); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(home, ".codex", "skills", "agentbox-api")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "SKILL.md"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
	if found.OK {
		t.Fatalf("check = %+v", found)
	}
	for _, want := range []string{"broken links", "legacy Codex copies", "skills install", "skills update"} {
		if !strings.Contains(found.Detail, want) {
			t.Fatalf("detail missing %q: %s", want, found.Detail)
		}
	}
}

func TestDoctorReportsNoInstalledUsageSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	found := doctorSkillCheck(t, installedApp(t, "0.2.0"))
	if !found.OK || !strings.Contains(found.Detail, "no installed usage skills") {
		t.Fatalf("empty check = %+v", found)
	}
}
