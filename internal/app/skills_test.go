package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSkillsValidation(t *testing.T) {
	valid := Skills{Version: 1, Name: "work-codex", Agent: "codex", Skills: []string{"orca-cli"}, Plugins: []string{"browser@openai-bundled"}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid entry rejected: %v", err)
	}
	for _, tc := range []struct {
		name  string
		entry Skills
		want  string
	}{
		{"bad name", Skills{Version: 1, Name: "Upper", Agent: "codex", Skills: []string{"alpha"}}, "lower-case name"},
		{"bad version", Skills{Version: 2, Name: "unit", Agent: "codex", Skills: []string{"alpha"}}, "version 1"},
		{"bad agent", Skills{Version: 1, Name: "unit", Agent: "gemin", Skills: []string{"alpha"}}, "agent must be"},
		{"empty", Skills{Version: 1, Name: "unit", Agent: "codex"}, "at least one skill or plugin"},
		{"duplicate", Skills{Version: 1, Name: "unit", Agent: "codex", Skills: []string{"alpha", "alpha"}}, "listed twice"},
		{"duplicate plugins", Skills{Version: 1, Name: "unit", Agent: "codex", Plugins: []string{"a@b", "a@b"}}, "listed twice"},
		{"path skill", Skills{Version: 1, Name: "unit", Agent: "codex", Skills: []string{"../evil"}}, "path-free"},
		{"hidden skill", Skills{Version: 1, Name: "unit", Agent: "codex", Skills: []string{".system"}}, "path-free"},
		{"bad plugin", Skills{Version: 1, Name: "unit", Agent: "codex", Plugins: []string{"browser"}}, "<plugin>@<marketplace>"},
		{"path plugin", Skills{Version: 1, Name: "unit", Agent: "codex", Plugins: []string{"browser@../bad"}}, "<plugin>@<marketplace>"},
		{"too many ats", Skills{Version: 1, Name: "unit", Agent: "codex", Plugins: []string{"a@b@c"}}, "<plugin>@<marketplace>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.entry.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
		})
	}
}

func TestSkillsStoreRoundTrip(t *testing.T) {
	store := Store{Base: t.TempDir()}
	entry := Skills{Version: 1, Name: "unit-skills", Agent: "codex", Skills: []string{"orca-cli"}, Plugins: []string{"browser@openai-bundled"}}
	if err := store.SaveSkills(entry); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSkills("unit-skills")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, entry) {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}
	listed, err := store.ListSkills()
	if err != nil || len(listed) != 1 || listed[0].Name != "unit-skills" {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	if _, err := store.LoadSkills("missing"); !os.IsNotExist(err) {
		t.Fatalf("missing entry error = %v", err)
	}
	if err := store.DeleteSkills("unit-skills"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSkills("unit-skills"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("double delete error = %v", err)
	}
	empty, err := store.ListSkills()
	if err != nil || len(empty) != 0 {
		t.Fatalf("list after delete = %+v, %v", empty, err)
	}
}

func TestSkillsStoreNormalizesMissingLists(t *testing.T) {
	store := Store{Base: t.TempDir()}
	for _, entry := range []Skills{
		{Version: 1, Name: "plugin-only", Agent: "codex", Plugins: []string{"browser@openai-bundled"}},
		{Version: 1, Name: "skill-only", Agent: "codex", Skills: []string{"orca-cli"}},
	} {
		if err := store.SaveSkills(entry); err != nil {
			t.Fatalf("save %s: %v", entry.Name, err)
		}
		loaded, err := store.LoadSkills(entry.Name)
		if err != nil {
			t.Fatalf("load %s: %v", entry.Name, err)
		}
		if loaded.Skills == nil || loaded.Plugins == nil {
			t.Fatalf("%s loaded nil list: %+v", entry.Name, loaded)
		}
		body, err := json.Marshal(loaded)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "null") {
			t.Fatalf("%s JSON contains null: %s", entry.Name, body)
		}
	}
	listed, err := store.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range listed {
		if entry.Skills == nil || entry.Plugins == nil {
			t.Fatalf("listed entry %s has nil list: %+v", entry.Name, entry)
		}
	}
}

func writeSkill(t *testing.T, root, name, description string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReadSkillDescriptionParsesYAMLFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	body := "---\nname: folded\ndescription: >-\n  A folded description\n  across two lines.\n---\n\nBody is ignored.\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if got := readSkillDescription(path); got != "A folded description across two lines." {
		t.Fatalf("description = %q", got)
	}
	if got := readSkillDescription(filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("missing description = %q", got)
	}
}

func TestDiscoverSkillsPerAgent(t *testing.T) {
	t.Run("claude standalone and plugin bundles", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", dir)
		writeSkill(t, filepath.Join(dir, "skills"), "orca-cli", "Orca command line")
		writeSkill(t, filepath.Join(dir, "plugins", "marketplaces", "team", "browser", "skills"), "browse", "Browse pages")
		writeSkill(t, filepath.Join(dir, "plugins", "cache", "team", "browser", "1.0.0", "skills"), "browse", "Browse pages")
		writeSkill(t, filepath.Join(dir, "plugins", "cache", "team", "browser", "1.0.0", "skills"), "capture", "Capture pages")
		available, err := (&App{}).DiscoverSkills("claude")
		if err != nil {
			t.Fatal(err)
		}
		if len(available.Skills) != 1 || available.Skills[0].Name != "orca-cli" || available.Skills[0].Description != "Orca command line" {
			t.Fatalf("skills = %+v", available.Skills)
		}
		if len(available.Plugins) != 1 || available.Plugins[0].ID != "browser@team" || available.Plugins[0].Skills != 2 {
			t.Fatalf("plugins = %+v", available.Plugins)
		}
	})

	t.Run("codex skips hidden dirs and reads cache plugins", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CODEX_HOME", dir)
		writeSkill(t, filepath.Join(dir, "skills"), "alpha", "Alpha skill")
		writeSkill(t, filepath.Join(dir, "skills"), ".system", "Hidden")
		writeSkill(t, filepath.Join(dir, "plugins", "cache", "openai-bundled", "browser", "0.1.26", "skills"), "browse", "Browse pages")
		available, err := (&App{}).DiscoverSkills("codex")
		if err != nil {
			t.Fatal(err)
		}
		if len(available.Skills) != 1 || available.Skills[0].Name != "alpha" {
			t.Fatalf("skills = %+v", available.Skills)
		}
		if len(available.Plugins) != 1 || available.Plugins[0].ID != "browser@openai-bundled" || available.Plugins[0].Skills != 1 {
			t.Fatalf("plugins = %+v", available.Plugins)
		}
	})

	t.Run("opencode skills only", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		writeSkill(t, filepath.Join(dir, "opencode", "skills"), "alpha", "Alpha skill")
		available, err := (&App{}).DiscoverSkills("opencode")
		if err != nil {
			t.Fatal(err)
		}
		if len(available.Skills) != 1 || len(available.Plugins) != 0 {
			t.Fatalf("available = %+v", available)
		}
	})

	t.Run("missing sources are empty", func(t *testing.T) {
		t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
		available, err := (&App{}).DiscoverSkills("claude")
		if err != nil || len(available.Skills) != 0 || len(available.Plugins) != 0 {
			t.Fatalf("available = %+v, %v", available, err)
		}
	})

	t.Run("symlinked skill follows target", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CODEX_HOME", dir)
		target := writeSkill(t, t.TempDir(), "real-skill", "Real skill")
		if err := os.MkdirAll(filepath.Join(dir, "skills"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, "skills", "linked")); err != nil {
			t.Fatal(err)
		}
		available, err := (&App{}).DiscoverSkills("codex")
		if err != nil || len(available.Skills) != 1 || available.Skills[0].Name != "linked" || available.Skills[0].Description != "Real skill" {
			t.Fatalf("available = %+v, %v", available, err)
		}
	})

	t.Run("invalid agent", func(t *testing.T) {
		if _, err := (&App{}).DiscoverSkills("other"); err == nil || !strings.Contains(err.Error(), "agent must be") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestResolveSkillsSelection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	store := Store{Base: t.TempDir()}
	app := App{Store: store}
	skillDir := writeSkill(t, filepath.Join(dir, "skills"), "orca-cli", "Orca command line")
	writeSkill(t, filepath.Join(dir, "skills"), "shared", "Shared standalone")
	writeSkill(t, filepath.Join(dir, "plugins", "cache", "openai-bundled", "browser", "0.1.26", "skills"), "browse", "Browse pages")
	writeSkill(t, filepath.Join(dir, "plugins", "cache", "openai-bundled", "browser", "0.1.26", "skills"), "shared", "Shared plugin")

	t.Run("missing entry", func(t *testing.T) {
		_, err := app.ResolveSkillsSelection("missing", "codex")
		if err == nil || !strings.Contains(err.Error(), `entry "missing" not found`) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("agent mismatch", func(t *testing.T) {
		if err := store.SaveSkills(Skills{Version: 1, Name: "claude-set", Agent: "claude", Skills: []string{"orca-cli"}}); err != nil {
			t.Fatal(err)
		}
		_, err := app.ResolveSkillsSelection("claude-set", "codex")
		if err == nil || !strings.Contains(err.Error(), `selects agent "claude"`) || !strings.Contains(err.Error(), `"codex"`) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("drift warns and keeps order", func(t *testing.T) {
		if err := store.SaveSkills(Skills{Version: 1, Name: "codex-work", Agent: "codex", Skills: []string{"orca-cli", "gone"}, Plugins: []string{"browser@openai-bundled", "missing@market"}}); err != nil {
			t.Fatal(err)
		}
		selection, err := app.ResolveSkillsSelection("codex-work", "codex")
		if err != nil {
			t.Fatal(err)
		}
		if len(selection.Sources) != 3 {
			t.Fatalf("sources = %+v", selection.Sources)
		}
		want := []string{"orca-cli", "browse", "shared"}
		for i, source := range selection.Sources {
			if source.Name != want[i] {
				t.Fatalf("source order = %+v; want %v", selection.Sources, want)
			}
		}
		if selection.Sources[0].Path != skillDir {
			t.Fatalf("standalone path = %s", selection.Sources[0].Path)
		}
		warnings := strings.Join(selection.Warnings(), "\n")
		for _, want := range []string{`skill "gone"`, `plugin "missing@market"`, "starting without it"} {
			if !strings.Contains(warnings, want) {
				t.Fatalf("warnings lack %q:\n%s", want, warnings)
			}
		}
	})
	t.Run("duplicate delivered skill is first-wins", func(t *testing.T) {
		if err := store.SaveSkills(Skills{Version: 1, Name: "dedupe", Agent: "codex", Skills: []string{"shared"}, Plugins: []string{"browser@openai-bundled"}}); err != nil {
			t.Fatal(err)
		}
		selection, err := app.ResolveSkillsSelection("dedupe", "codex")
		if err != nil {
			t.Fatal(err)
		}
		if len(selection.Sources) != 2 {
			t.Fatalf("sources = %+v", selection.Sources)
		}
		if selection.Sources[0].Path != filepath.Join(dir, "skills", "shared") {
			t.Fatalf("standalone copy did not win: %+v", selection.Sources)
		}
		if len(selection.Duplicates) != 1 || selection.Duplicates[0] != "shared" {
			t.Fatalf("duplicates = %v", selection.Duplicates)
		}
	})
}

func TestPrepareAgentDeliversSkillsPerAgent(t *testing.T) {
	for _, agent := range []string{"claude", "codex", "opencode"} {
		t.Run(agent, func(t *testing.T) {
			source := writeSkill(t, t.TempDir(), "orca-cli", "Orca command line")
			executable := filepath.Join(source, "scripts", "run.sh")
			if err := os.MkdirAll(filepath.Dir(executable), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(executable, []byte("#!/bin/sh\necho ok\n"), 0755); err != nil {
				t.Fatal(err)
			}
			selection := &SkillsSelection{
				Entry:   Skills{Version: 1, Name: "unit-skills", Agent: agent, Skills: []string{"orca-cli"}},
				Sources: []skillSource{{Name: "orca-cli", Path: source}},
			}
			store := Store{Base: t.TempDir()}
			app := App{Store: store}
			run := Run{ID: "run-unit123"}
			dir, err := app.PrepareAgent(context.Background(), run, testProfile(agent, "standard", "prompted"), "", nil, selection)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(skillDeliveryDir(agent, dir), "orca-cli")
			body, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
			if err != nil || !strings.Contains(string(body), "Orca command line") {
				t.Fatalf("delivered skill unreadable: %v %s", err, body)
			}
			info, err := os.Stat(filepath.Join(target, "scripts", "run.sh"))
			if err != nil || info.Mode().Perm()&0111 == 0 {
				t.Fatalf("executable bit lost: %v %v", err, info.Mode())
			}
		})
	}
}

func TestPrepareAgentResolvesSymlinkedSkillDirectory(t *testing.T) {
	target := writeSkill(t, t.TempDir(), "real-skill", "Real skill")
	linkRoot := t.TempDir()
	link := filepath.Join(linkRoot, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	selection := &SkillsSelection{
		Entry:   Skills{Version: 1, Name: "unit-skills", Agent: "codex", Skills: []string{"linked"}},
		Sources: []skillSource{{Name: "linked", Path: link}},
	}
	app := App{Store: Store{Base: t.TempDir()}}
	dir, err := app.PrepareAgent(context.Background(), Run{ID: "run-unit123"}, testProfile("codex", "standard", "prompted"), "", nil, selection)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "home", ".codex", "skills", "linked", "SKILL.md"))
	if err != nil || !strings.Contains(string(body), "Real skill") {
		t.Fatalf("symlinked skill not copied: %v %s", err, body)
	}
}

func TestPrepareAgentWithoutSkillsLeavesSkillsUnchanged(t *testing.T) {
	app := App{Store: Store{Base: t.TempDir()}}
	dir, err := app.PrepareAgent(context.Background(), Run{ID: "run-unit123"}, testProfile("codex", "standard", "prompted"), "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "home", ".codex", "skills")); !os.IsNotExist(err) {
		t.Fatalf("skills directory written without selection: %v", err)
	}
}

func TestStartResolvesSkillsSelection(t *testing.T) {
	ctx := context.Background()
	store := Store{Base: t.TempDir()}
	policyConfig, err := store.CreatePolicyConfiguration("unit-policy")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(Profile{
		Version: 1, Name: "mycodex", Agent: "codex", Image: "localhost/openshell-codex:1.2.3-r3",
		Mode: "standard", Provider: "unit-provider", Policy: policyConfig.Path, Approval: "prompted",
	}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		switch {
		case call.name == "openshell" && len(call.args) > 0 && call.args[0] == "gateway":
			return []byte(`{"name":"openshell","type":"podman"}`), nil
		case call.name == "openshell" && len(call.args) > 0 && call.args[0] == "provider":
			return []byte(`{}`), nil
		case call.name == "podman":
			return []byte(`[]`), nil
		}
		return nil, nil
	}}
	app := App{Store: store, Runner: runner}
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	writeSkill(t, filepath.Join(codexHome, "skills"), "orca-cli", "Orca command line")
	if err := store.SaveSkills(Skills{Version: 1, Name: "unit-skills", Agent: "codex", Skills: []string{"orca-cli", "gone"}}); err != nil {
		t.Fatal(err)
	}
	run, err := app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell", Skills: "unit-skills", Dry: true})
	if err != nil {
		t.Fatal(err)
	}
	if run.Skills != "unit-skills" || len(run.SkillsWarnings) != 1 || !strings.Contains(run.SkillsWarnings[0], `"gone"`) {
		t.Fatalf("run skills selection = %+v", run)
	}
	if err := store.SaveSkills(Skills{Version: 1, Name: "claude-skills", Agent: "claude", Skills: []string{"orca-cli"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell", Skills: "claude-skills", Dry: true}); err == nil || !strings.Contains(err.Error(), `selects agent "claude"`) {
		t.Fatalf("mismatch error = %v", err)
	}
	run, err = app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell", Dry: true})
	if err != nil {
		t.Fatal(err)
	}
	if run.Skills != "" || len(run.SkillsWarnings) != 0 {
		t.Fatalf("run without selection changed: %+v", run)
	}
}
