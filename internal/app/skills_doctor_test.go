package app

import (
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
