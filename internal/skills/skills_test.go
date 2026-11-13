package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedSkillsAreSelfContained(t *testing.T) {
	for _, name := range Names() {
		content, err := Embedded(name, "9.9.9")
		if err != nil {
			t.Fatal(err)
		}
		body := string(content)
		if strings.Contains(body, VersionPlaceholder) {
			t.Fatalf("%s still contains the version placeholder", name)
		}
		if got := InstalledVersion(content); got != "9.9.9" {
			t.Fatalf("%s version = %q", name, got)
		}
		for _, want := range []string{"---", "name: " + name, "description: "} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s is missing %q", name, want)
			}
		}
		for _, forbidden := range []string{"/Users/", "/home/", `C:\Users`, "sk-", "ghp_", "AKIA"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s contains %q", name, forbidden)
			}
		}
	}
}

func TestShowRejectsUnknownSkill(t *testing.T) {
	if _, err := Embedded("../agentbox-cli", "1.0.0"); err == nil {
		t.Fatal("accepted a path-like skill name")
	}
}

func TestInstallTargetPaths(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{".agents", ".codex", ".claude", filepath.Join(".config", "opencode")} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	roots := map[string]string{
		"agents":   filepath.Join(home, ".agents", "skills"),
		"codex":    filepath.Join(home, ".codex", "skills"),
		"claude":   filepath.Join(home, ".claude", "skills"),
		"opencode": filepath.Join(home, ".config", "opencode", "skills"),
	}
	for _, target := range Targets() {
		results, err := Install(Options{Target: target, Version: "1.0.0", Home: home})
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if len(results) != len(Names()) {
			t.Fatalf("%s wrote %d results", target, len(results))
		}
		for _, result := range results {
			if result.Action != "installed" {
				t.Fatalf("%s action = %s", result.Skill, result.Action)
			}
			want := filepath.Join(roots[target], result.Skill, "SKILL.md")
			if result.Path != want {
				t.Fatalf("%s path = %s, want %s", target, result.Path, want)
			}
			if _, err := os.Stat(result.Path); err != nil {
				t.Fatal(err)
			}
		}
	}
	results, err := Install(Options{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(Names())*len(Targets()) {
		t.Fatalf("default install wrote %d results", len(results))
	}
}

func TestInstallDirOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom", "skills")
	results, err := Install(Options{Dir: dir, Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Path != filepath.Join(dir, result.Skill, "SKILL.md") {
			t.Fatalf("path = %s", result.Path)
		}
		if _, err := os.Stat(result.Path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Install(Options{Dir: dir, Target: "all"}); err == nil {
		t.Fatal("accepted --dir with target all")
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(Options{Dir: dir, Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	results, err := Install(Options{Dir: dir, Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Action != "unchanged" {
			t.Fatalf("%s action = %s", result.Skill, result.Action)
		}
	}
}

func TestInstallRefusesDifferingFileWithoutForce(t *testing.T) {
	dir := t.TempDir()
	results, err := Install(Options{Dir: dir, Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	path := results[0].Path
	if err := os.WriteFile(path, []byte("locally edited\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(Options{Dir: dir, Version: "1.0.0"}); err == nil {
		t.Fatal("overwrote a differing file without --force")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "locally edited\n" {
		t.Fatalf("file was modified: %q", body)
	}
	results, err = Install(Options{Dir: dir, Version: "1.0.0", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Action != "installed" || results[0].Detail == "" {
		t.Fatalf("force result = %+v", results[0])
	}
	if got := InstalledVersion(mustRead(t, path)); got != "1.0.0" {
		t.Fatalf("version after force = %q", got)
	}
}

func TestInstallDryRunWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	results, err := Install(Options{Dir: dir, Version: "1.0.0", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Action != "would-install" {
			t.Fatalf("%s action = %s", result.Skill, result.Action)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("dry run created %s: %v", dir, err)
	}
}

func TestInstallSkipsMissingBuiltInParent(t *testing.T) {
	home := t.TempDir()
	results, err := Install(Options{Target: "claude", Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Action != "skipped" || !strings.Contains(result.Detail, "does not exist") {
			t.Fatalf("result = %+v", result)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("created a missing target parent: %v", err)
	}
}

func TestListAndShowWithoutWritableHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "missing", "home")
	statuses, err := List(ListOptions{Version: "1.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != len(Names()) {
		t.Fatalf("statuses = %d", len(statuses))
	}
	for _, status := range statuses {
		if len(status.Installations) != len(Targets()) {
			t.Fatalf("%s installations = %d", status.Name, len(status.Installations))
		}
		for _, installation := range status.Installations {
			if installation.Present || installation.State != "missing" {
				t.Fatalf("installation = %+v", installation)
			}
		}
	}
	if _, err := Embedded("agentbox-cli", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("list created the home: %v", err)
	}
}

func TestListReportsStaleInstalledCopy(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".agents"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(Options{Target: "agents", Version: "1.0.0", Home: home}); err != nil {
		t.Fatal(err)
	}
	statuses, err := List(ListOptions{Version: "2.0.0", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	var stale int
	for _, status := range statuses {
		for _, installation := range status.Installations {
			if installation.Target == "agents" && installation.State == "stale" {
				stale++
			}
		}
	}
	if stale != len(Names()) {
		t.Fatalf("stale copies = %d", stale)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
