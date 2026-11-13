package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmartinol/agentbox/internal/app"
	"github.com/dmartinol/agentbox/internal/skills"
)

func captureStdout(t *testing.T, action func() error) (string, error) {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = write
	callErr := action()
	_ = write.Close()
	os.Stdout = previous
	defer read.Close()
	body, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes.TrimSpace(body)), callErr
}

func TestDoctorJSONIsParseable(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error { return dispatch(context.Background(), a, []string{"doctor", "--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var checks []app.Check
	if err := json.Unmarshal([]byte(output), &checks); err != nil || len(checks) < 5 {
		t.Fatalf("doctor output is not a checks JSON array: %s (%v)", output, err)
	}
}

func TestImageBuildPreviewReturnsRecipeAndExactVersionAsJSON(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"image", "build", "--dry-run", "--version", "0.167.0", "codex"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var record app.ImageRecord
	if err := json.Unmarshal([]byte(output), &record); err != nil {
		t.Fatalf("image preview output is invalid JSON: %s (%v)", output, err)
	}
	if record.Agent != "codex" || record.Version != "0.167.0" || record.Tag != "localhost/openshell:codex-0.167.0-r3" {
		t.Fatalf("unexpected image preview: %+v", record)
	}
}

type appfakeRunner struct{}

func (*appfakeRunner) Output(context.Context, string, ...string) ([]byte, error) {
	return []byte(`{}`), nil
}
func (*appfakeRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func TestVersionOutputIsParseable(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error { return dispatch(context.Background(), a, []string{"--version"}) })
	if err != nil {
		t.Fatal(err)
	}
	if output == "" {
		t.Fatal("version output is empty")
	}
}

func TestSkillsInstallListAndShow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{".claude", ".codex", filepath.Join(".config", "opencode")} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	if err := dispatch(context.Background(), a, []string{"skills", "install"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".agents", "skills", "agentbox-cli", "SKILL.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `version: "`+binaryVersion()+`"`) {
		t.Fatalf("installed skill is not version matched: %s", body[:200])
	}
	// A bare install links exactly Claude and OpenCode and never writes into
	// ~/.codex/skills.
	if target, err := os.Readlink(filepath.Join(home, ".claude", "skills", "agentbox-cli")); err != nil || target != "../../.agents/skills/agentbox-cli" {
		t.Fatalf("claude link = %q (%v)", target, err)
	}
	if target, err := os.Readlink(filepath.Join(home, ".config", "opencode", "skills", "agentbox-cli")); err != nil || target != "../../../.agents/skills/agentbox-cli" {
		t.Fatalf("opencode link = %q (%v)", target, err)
	}
	if entries, err := os.ReadDir(filepath.Join(home, ".codex", "skills")); err == nil && len(entries) > 0 {
		t.Fatalf("install wrote into ~/.codex/skills: %v", entries)
	}

	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "list", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var statuses []skills.Status
	if err := json.Unmarshal([]byte(output), &statuses); err != nil {
		t.Fatalf("skills list output is not JSON: %s (%v)", output, err)
	}
	if len(statuses) != len(skills.Names()) {
		t.Fatalf("statuses = %d", len(statuses))
	}
	if statuses[0].Installations[0].State != "current" {
		t.Fatalf("installation = %+v", statuses[0].Installations[0])
	}
	for _, status := range statuses {
		if status.Name != "agentbox-orchestrator" {
			continue
		}
		if len(status.Files) != 4 {
			t.Fatalf("orchestrator template statuses = %+v", status.Files)
		}
		for _, file := range status.Files {
			if file.State != "current" {
				t.Fatalf("template %s state = %+v", file.Name, file)
			}
		}
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "list"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "file") || !strings.Contains(output, "issue.md") {
		t.Fatalf("text list does not report templates: %s", output)
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "show", "agentbox-api"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "curl -fsS") {
		t.Fatalf("shown skill lacks API examples: %s", output[:200])
	}

	// The orchestrator skill is packaged, installed, and shown like the others.
	orchestratorDir := filepath.Join(home, ".agents", "skills", "agentbox-orchestrator")
	for _, file := range []string{"SKILL.md", "issue.md", "pr.md", "verdict.md", "state.md"} {
		if _, err := os.Stat(filepath.Join(orchestratorDir, file)); err != nil {
			t.Fatalf("%s was not installed: %v", file, err)
		}
	}
	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "show", "agentbox-orchestrator"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "orchestrator-state") || !strings.Contains(output, "agentbox agent start") {
		t.Fatalf("shown orchestrator skill lacks the flow content: %s", output[:200])
	}
}

func TestSkillsInstallRefusesDifferingFileWithoutForce(t *testing.T) {
	dir := t.TempDir()
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	if err := dispatch(context.Background(), a, []string{"skills", "install", "--dir", dir}); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"issue.md", "pr.md", "verdict.md", "state.md"} {
		if _, err := os.Stat(filepath.Join(dir, "agentbox-orchestrator", file)); err != nil {
			t.Fatalf("--dir did not write %s: %v", file, err)
		}
	}
	path := filepath.Join(dir, "agentbox-cli", "SKILL.md")
	if err := os.WriteFile(path, []byte("edited locally\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := dispatch(context.Background(), a, []string{"skills", "install", "--dir", dir}); err == nil {
		t.Fatal("install overwrote a differing file without --force")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "edited locally\n" {
		t.Fatalf("refused install changed the file: %q", body)
	}
	template := filepath.Join(dir, "agentbox-orchestrator", "pr.md")
	if err := os.WriteFile(template, []byte("edited template\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := dispatch(context.Background(), a, []string{"skills", "install", "--dir", dir}); err == nil {
		t.Fatal("install overwrote a differing template without --force")
	}
	if body, err := os.ReadFile(template); err != nil || string(body) != "edited template\n" {
		t.Fatalf("refused template install changed the file: %q (%v)", body, err)
	}
}

func TestSkillsLifecycleJSONAndFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{".claude", ".codex", filepath.Join(".config", "opencode")} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}

	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "install", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var results []skills.Result
	if err := json.Unmarshal([]byte(output), &results); err != nil || len(results) == 0 {
		t.Fatalf("install output is not a results JSON array: %s (%v)", output, err)
	}
	for _, result := range results {
		if result.Skill == "" || result.Target == "" || result.Path == "" || result.Action == "" {
			t.Fatalf("install result is missing a stable field: %+v", result)
		}
		if result.Target == "agents" && result.File == "" {
			t.Fatalf("canonical install result is missing its file: %+v", result)
		}
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "update", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(output), &results); err != nil || len(results) == 0 {
		t.Fatalf("update output is not a results JSON array: %s (%v)", output, err)
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "show", "--json", "agentbox-api"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var shown struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}
	if err := json.Unmarshal([]byte(output), &shown); err != nil {
		t.Fatalf("show output is not JSON: %s (%v)", output, err)
	}
	if shown.Name != "agentbox-api" || shown.Version != binaryVersion() || shown.Description == "" || !strings.Contains(shown.Content, "curl -fsS") {
		t.Fatalf("show payload = %+v", shown)
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "remove", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(output), &results); err != nil || len(results) == 0 {
		t.Fatalf("remove output is not a results JSON array: %s (%v)", output, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "agentbox-cli", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("remove left a canonical copy behind")
	}

	// The deprecated --target alias still selects destinations.
	if err := dispatch(context.Background(), a, []string{"skills", "install", "--target", "claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "agentbox-cli")); err != nil {
		t.Fatal("--target claude did not create the claude link")
	}
	if err := dispatch(context.Background(), a, []string{"skills", "update", "--target", "claude"}); err == nil {
		t.Fatal("update accepted the deprecated --target flag")
	}
}

func TestSkillsLocalInstall(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	home := t.TempDir()
	t.Setenv("HOME", home)
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	if err := dispatch(context.Background(), a, []string{"skills", "install", "--local", "--all"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "agentbox-cli", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(filepath.Join(project, ".claude", "skills", "agentbox-cli")); err != nil || target != "../../.agents/skills/agentbox-cli" {
		t.Fatalf("local claude link = %q (%v)", target, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents")); !os.IsNotExist(err) {
		t.Fatal("--local wrote into the user home directory")
	}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "list", "--local", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var statuses []skills.Status
	if err := json.Unmarshal([]byte(output), &statuses); err != nil {
		t.Fatalf("local list output is not JSON: %s (%v)", output, err)
	}
	for _, installation := range statuses[0].Installations {
		if installation.Target == "agents" && installation.State != "current" {
			t.Fatalf("local installation = %+v", installation)
		}
	}
	if err := dispatch(context.Background(), a, []string{"skills", "remove", "--local"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "agentbox-cli")); !os.IsNotExist(err) {
		t.Fatal("local remove left a copy behind")
	}
}

func TestUsageListsSkillsCommand(t *testing.T) {
	output, err := captureStdout(t, func() error {
		usage()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "skills install|update|remove|list|show") {
		t.Fatalf("usage is missing the skills command: %s", output)
	}
}

func TestImageRegistryShowSetClear(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	if err := dispatch(context.Background(), a, []string{"image", "registry", "set", "quay.io/team/agentbox"}); err != nil {
		t.Fatal(err)
	}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"image", "registry", "show", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var registry app.Registry
	if err := json.Unmarshal([]byte(output), &registry); err != nil || registry.Reference != "quay.io/team/agentbox" {
		t.Fatalf("registry output = %s, %v", output, err)
	}
	if err := dispatch(context.Background(), a, []string{"image", "registry", "clear"}); err != nil {
		t.Fatal(err)
	}
	registry, err = a.Registry()
	if err != nil || registry.Reference != "" {
		t.Fatalf("registry after clear = %+v, %v", registry, err)
	}
}

func TestImageBuildHelpReturnsFlagHelp(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	err := dispatch(context.Background(), a, []string{"image", "build", "--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("image build --help error = %v; want flag.ErrHelp", err)
	}
}

func TestImageHelpDoesNotRunCommand(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error { return dispatch(context.Background(), a, []string{"image", "--help"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "image build") {
		t.Fatalf("image help output = %q", output)
	}
}

type imageListRunner struct{}

func (imageListRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	if name == "podman" && len(args) > 0 && args[0] == "images" {
		return []byte(`[{"Names":["localhost/openshell:codex-0.167.0-r3"],"Digest":"sha256:image","Arch":"arm64","Os":"linux","Labels":{"agentbox":"true","agentbox.agent":"codex","agentbox.version":"0.167.0","agentbox.recipe":"3"}}]`), nil
	}
	return []byte(`{}`), nil
}

func (imageListRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func TestImageListDefaultOutputIsStructured(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: imageListRunner{}}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"image", "list"})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"AGENT", "VERSION", "RECIPE", "PLATFORM", "REFERENCE", "codex", "0.167.0", "linux/arm64", "localhost/openshell:codex-0.167.0-r3"} {
		if !strings.Contains(output, want) {
			t.Fatalf("structured image list missing %q:\n%s", want, output)
		}
	}
	if strings.HasPrefix(output, "[") || strings.HasPrefix(output, "{") {
		t.Fatalf("image list returned JSON without --json:\n%s", output)
	}
}

func TestProjectCreateListAndShow(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.name", "Tester"},
		{"config", "user.email", "test@example.invalid"},
	} {
		command := exec.Command("git", args...)
		command.Dir = repo
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-qm", "base"}} {
		command := exec.Command("git", args...)
		command.Dir = repo
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	a := &app.App{Store: app.Store{Base: filepath.Join(base, "state")}, Runner: &appfakeRunner{}}
	a.Runner = app.OSRunner{}
	if _, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"project", "create", "--root", repo, "--no-worktree", "demo"})
	}); err != nil {
		t.Fatal(err)
	}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"project", "list", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var projects []app.Project
	if err := json.Unmarshal([]byte(output), &projects); err != nil || len(projects) != 1 {
		t.Fatalf("project list = %s, %v", output, err)
	}
	if projects[0].Name != "demo" || projects[0].UseWorktree() {
		t.Fatalf("unexpected project: %+v", projects[0])
	}
	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"project", "show", "demo"})
	})
	if err != nil || !strings.Contains(output, "Isolation: direct") {
		t.Fatalf("project show = %q, %v", output, err)
	}
}

func TestSettingsSetAndShow(t *testing.T) {
	base := t.TempDir()
	a := &app.App{Store: app.Store{Base: base}, Runner: &appfakeRunner{}}
	root := filepath.Join(base, "trees")
	if err := dispatch(context.Background(), a, []string{"settings", "set", "--worktree-root", root, "--branch-prefix", "runs"}); err != nil {
		t.Fatal(err)
	}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"settings", "show", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var settings app.Settings
	if err := json.Unmarshal([]byte(output), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Worktrees.Root != root || settings.Worktrees.BranchPrefix != "runs" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}

type skillsRunner struct{}

func (*skillsRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	switch {
	case name == "openshell" && len(args) > 0 && args[0] == "gateway":
		return []byte(`{"name":"openshell","type":"podman"}`), nil
	case name == "openshell" && len(args) > 0 && args[0] == "provider":
		return []byte(`{}`), nil
	case name == "podman":
		return []byte(`[]`), nil
	case name == "git":
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		return []byte(filepath.Clean(filepath.Join(dir, "..", ".."))), nil
	}
	return []byte(`{}`), nil
}

func (*skillsRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func TestSkillsCommandsAndAgentStartFlag(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	policy, err := store.CreatePolicyConfiguration("unit-policy")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(app.Profile{
		Version: 1, Name: "codex", Agent: "codex", Image: "localhost/openshell-codex:1.2.3-r3",
		Mode: "standard", Provider: "unit-provider", Policy: policy.Path, Approval: "prompted",
	}); err != nil {
		t.Fatal(err)
	}
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	skillDir := filepath.Join(codexHome, "skills", "orca-cli")
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Orca command line\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Store: store, Runner: &skillsRunner{}}

	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skill", "create", "--agent", "codex", "--skills", "orca-cli", "unit-skills"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "unit-skills codex") {
		t.Fatalf("create output = %q", output)
	}
	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skill", "available", "--agent", "codex"})
	})
	if err != nil || !strings.Contains(output, "orca-cli") || !strings.Contains(output, "Orca command line") {
		t.Fatalf("available output = %q, %v", output, err)
	}
	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "start", "--profile", "codex", "--gateway", "openshell", "--skills", "unit-skills", "--dry-run", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var run app.Run
	if err := json.Unmarshal([]byte(output), &run); err != nil {
		t.Fatalf("start output is not JSON: %s (%v)", output, err)
	}
	if run.Skills != "unit-skills" {
		t.Fatalf("run.Skills = %q", run.Skills)
	}
	if err := dispatch(context.Background(), a, []string{"skill", "delete", "unit-skills"}); err != nil {
		t.Fatal(err)
	}
}
