package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	done := make(chan struct{})
	var body []byte
	var readErr error
	go func() {
		body, readErr = io.ReadAll(read)
		close(done)
	}()
	callErr := action()
	_ = write.Close()
	os.Stdout = previous
	<-done
	defer read.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(bytes.TrimSpace(body)), callErr
}

type agentSyncCLIRunner struct {
	run       app.Run
	head      string
	artifacts map[string]string
}

func (runner agentSyncCLIRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "git" {
		return app.OSRunner{}.Output(ctx, "git", args...)
	}
	if name != "openshell" {
		return nil, nil
	}
	if argPresent(args, "node") && argPresent(args, "-e") {
		return []byte(fmt.Sprintf(`{"head":%q,"branch_tip":%q,"diff_sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","untracked":[]}`, runner.head, runner.head)), nil
	}
	if argPresent(args, "/bin/sh") && strings.Contains(strings.Join(args, " "), "/workspace/export.bundle") {
		return []byte(strings.Join([]string{runner.head, runner.run.Branch, runner.head}, "\n")), nil
	}
	for index, arg := range args {
		if arg != "download" || index+3 >= len(args) {
			continue
		}
		if local, ok := runner.artifacts[args[index+2]]; ok {
			body, err := os.ReadFile(local)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(args[index+3], body, 0600); err != nil {
				return nil, err
			}
			return nil, nil
		}
	}
	return []byte(`{}`), nil
}

func (agentSyncCLIRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func agentSyncCLIFixture(t *testing.T) (*app.App, app.Run) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	runGit := func(dir string, args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	runGit(source, "init", "-q")
	runGit(source, "config", "user.name", "Unit Tester")
	runGit(source, "config", "user.email", "unit@example.invalid")
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(source, "add", ".")
	runGit(source, "commit", "-qm", "base")
	store := app.Store{Base: filepath.Join(root, "state")}
	a := &app.App{Store: store, Runner: app.OSRunner{}}
	run, err := a.CreateWorktree(ctx, source, "run-sync-cli")
	if err != nil {
		t.Fatal(err)
	}
	run.Status = app.StatusWorking
	run.Gateway = "gateway-one"
	run.Sandbox = "sandbox-sync-cli"
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}

	sandbox := filepath.Join(root, "sandbox")
	runGit(root, "clone", "-q", run.Worktree, sandbox)
	runGit(sandbox, "config", "user.name", "Sandbox Agent")
	runGit(sandbox, "config", "user.email", "sandbox@example.invalid")
	if err := os.WriteFile(filepath.Join(sandbox, "tracked.txt"), []byte("agent edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(sandbox, "add", ".")
	runGit(sandbox, "commit", "-qm", "agent edit")
	headOutput, err := app.OSRunner{}.Output(ctx, "git", "-C", sandbox, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(string(headOutput))

	bundle := filepath.Join(root, "export.bundle")
	runGit(sandbox, "bundle", "create", bundle, run.Branch)
	archive := filepath.Join(root, "export.tar")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := tar.NewWriter(file).Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	a.Runner = agentSyncCLIRunner{
		run:  run,
		head: head,
		artifacts: map[string]string{
			"/workspace/export.bundle": bundle,
			"/workspace/export.patch":  filepath.Join(root, "empty.patch"),
			"/workspace/export.tar":    archive,
		},
	}
	if err := os.WriteFile(filepath.Join(root, "empty.patch"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	return a, run
}

func TestDoctorJSONIsParseable(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error { return dispatch(context.Background(), a, []string{"doctor", "--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var checks []struct {
		Name   string  `json:"name"`
		OK     *bool   `json:"ok"`
		State  *string `json:"state"`
		Detail *string `json:"detail"`
	}
	if err := json.Unmarshal([]byte(output), &checks); err != nil || len(checks) < 5 {
		t.Fatalf("doctor output is not a checks JSON array: %s (%v)", output, err)
	}
	hasClock := false
	for _, check := range checks {
		if check.Name == "podman VM clock" {
			hasClock = true
		}
		if check.OK == nil || check.State == nil || check.Detail == nil {
			t.Fatalf("check %q lacks ok, state, or detail: %s", check.Name, output)
		}
		switch *check.State {
		case "ok", "fail", "unavailable":
		default:
			t.Fatalf("check %q has invalid state %q: %s", check.Name, *check.State, output)
		}
	}
	if !hasClock {
		t.Fatalf("doctor output lacks the Podman VM clock check: %s", output)
	}
}

func TestAgentPruneRejectsDryRunWithYes(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	err := dispatch(context.Background(), a, []string{"agent", "prune", "--completed", "--dry-run", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "--dry-run and --yes are mutually exclusive") {
		t.Fatalf("flag conflict = %v", err)
	}
}

func TestAgentPruneJSONListsSelectedRuns(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	a := &app.App{Store: store, Runner: &appfakeRunner{}}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(app.Run{ID: "run-failed1", Name: "failed-one", Project: "demo", Status: app.StatusFailed, Sandbox: "sandbox-failed1", Gateway: "gateway-one", Synced: true}); err != nil {
		t.Fatal(err)
	}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "prune", "--failed", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var result app.RunPruneResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("prune output is not a result object: %s (%v)", output, err)
	}
	if !result.DryRun || result.Deleted || len(result.Runs) != 1 || result.Runs[0].ID != "run-failed1" {
		t.Fatalf("prune result = %+v", result)
	}
	if _, err := store.LoadRun("run-failed1"); err != nil {
		t.Fatalf("json dry-run removed the run: %v", err)
	}
}

func TestAgentPruneTextAsksForConfirmation(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	a := &app.App{Store: store, Runner: &appfakeRunner{}}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(app.Run{ID: "run-failed1", Name: "failed-one", Project: "demo", Status: app.StatusFailed, Sandbox: "sandbox-failed1", Gateway: "gateway-one", Synced: true}); err != nil {
		t.Fatal(err)
	}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "prune", "--failed"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Would delete 1 run(s)") || !strings.Contains(output, "Pass --yes to delete these runs.") {
		t.Fatalf("prune text = %q", output)
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
	if record.Agent != "codex" || record.Version != "0.167.0" || record.Tag != "localhost/openshell:codex-0.167.0-r5" {
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

type providerListRunner struct {
	output []byte
	args   []string
}

func (runner *providerListRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	runner.args = args
	return runner.output, nil
}

func (*providerListRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func TestProviderListPrintsProviderRecords(t *testing.T) {
	runner := &providerListRunner{output: []byte(`{"providers":[{"name":"model-gateway","type":"gateway-bearer"},{"name":"direct-openai","provider_type":"openai"}]}`)}
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: runner}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"provider", "list", "--gateway", "unit-gateway"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if output != "model-gateway        gateway-bearer\ndirect-openai        openai" {
		t.Fatalf("provider list output = %q", output)
	}
	want := []string{"provider", "list", "-o", "json", "--gateway", "unit-gateway"}
	if !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("provider list command = %#v; want %#v", runner.args, want)
	}
}

func TestProviderListJSONIsParseable(t *testing.T) {
	body := `{"providers":[{"name":"model-gateway","type":"gateway-bearer"}]}`
	runner := &providerListRunner{output: []byte(body)}
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: runner}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"provider", "list", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Providers []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"providers"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("provider list JSON is invalid: %s (%v)", output, err)
	}
	if len(payload.Providers) != 1 || payload.Providers[0].Name != "model-gateway" || payload.Providers[0].Type != "gateway-bearer" {
		t.Fatalf("provider list JSON = %+v", payload)
	}
}

func TestProviderCommandRequiresKnownSubcommand(t *testing.T) {
	for _, command := range [][]string{{"provider"}, {"provider", "unknown"}} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
			err := dispatch(context.Background(), a, command)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "setup") || !strings.Contains(err.Error(), "list") {
				t.Fatalf("provider usage error = %v", err)
			}
			if strings.Contains(err.Error(), "provider setup required") {
				t.Fatalf("provider usage error used the old message: %v", err)
			}
		})
	}
}

type pruneOutputRunner struct{}

func (pruneOutputRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	if name == "podman" && len(args) > 0 && args[0] == "images" {
		return []byte(`[
			{"Id":"image-free","Names":["localhost/openshell:codex-1.0.0-r4"],"Labels":{"agentbox":"true","agentbox.agent":"codex","agentbox.version":"1.0.0","agentbox.recipe":"4"}},
			{"Id":"image-dangling","Names":[],"Labels":{"agentbox":"true","agentbox.agent":"claude","agentbox.version":"2.0.0","agentbox.recipe":"4"}},
			{"Id":"image-other","Names":[],"Labels":{}}
		]`), nil
	}
	return nil, nil
}

func (pruneOutputRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func TestImagePruneOutputCountsNamedAndDanglingImages(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: pruneOutputRunner{}}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"image", "prune"})
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "Removed 2 AgentBox image(s):\nlocalhost/openshell:codex-1.0.0-r4\nimage-dangling"
	if output != want {
		t.Fatalf("prune output = %q; want %q", output, want)
	}
}

func TestAgentPruneDryRunJSONAndIdempotence(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Store: store, Runner: &appfakeRunner{}}
	path := filepath.Join(store.RunsDir(), "run-cli123-source.bundle")
	body := []byte("orphan\n")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}

	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "prune", "--dry-run"})
	})
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("Would remove 1 orphaned run artifact(s), freeing %d bytes:\n%s", len(body), path)
	if output != want {
		t.Fatalf("dry-run text = %q; want %q", output, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("dry-run removed artifact: %v", err)
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "prune", "--json", "--dry-run"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var result app.RunArtifactPruneResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("dry-run JSON is invalid: %s (%v)", output, err)
	}
	if !result.DryRun || result.FreedBytes != int64(len(body)) || len(result.Artifacts) != 1 {
		t.Fatalf("dry-run JSON = %+v", result)
	}
	if result.Artifacts[0].Path != path || result.Artifacts[0].Bytes != int64(len(body)) {
		t.Fatalf("dry-run JSON artifact = %+v", result.Artifacts[0])
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "prune"})
	})
	if err != nil {
		t.Fatal(err)
	}
	want = fmt.Sprintf("Removed 1 orphaned run artifact(s), freeing %d bytes:\n%s", len(body), path)
	if output != want {
		t.Fatalf("real prune text = %q; want %q", output, want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("real prune left artifact: %v", err)
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "prune"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if output != "No orphaned run artifacts to remove." {
		t.Fatalf("second prune text = %q", output)
	}
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

func TestProfileCreateDefaultsToUnattendedApproval(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	a := &app.App{Store: store, Runner: &appfakeRunner{}}
	if _, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{
			"profile", "create", "--agent", "codex", "--provider", "unit-provider", "default-approval",
		})
	}); err != nil {
		t.Fatal(err)
	}
	created, err := store.LoadProfile("default-approval")
	if err != nil {
		t.Fatal(err)
	}
	if app.DefaultApproval != "unattended" || created.Approval != app.DefaultApproval {
		t.Fatalf("default profile approval = %q; want unattended", created.Approval)
	}

	if _, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{
			"profile", "create", "--agent", "codex", "--provider", "unit-provider", "--approval", "prompted", "prompted-profile",
		})
	}); err != nil {
		t.Fatal(err)
	}
	explicit, err := store.LoadProfile("prompted-profile")
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Approval != "prompted" {
		t.Fatalf("explicit profile approval = %q; want prompted", explicit.Approval)
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
	if len(statuses) != len(skills.Names())+len(skills.RoleNames()) {
		t.Fatalf("statuses = %d", len(statuses))
	}
	if statuses[0].Installations[0].State != "current" {
		t.Fatalf("installation = %+v", statuses[0].Installations[0])
	}
	roles := 0
	for _, status := range statuses {
		if status.Kind == "role" {
			roles++
		}
	}
	if roles != len(skills.RoleNames()) {
		t.Fatalf("role statuses = %d", roles)
	}
	for _, status := range statuses {
		if status.Kind == "role" || status.Name != "agentbox-orchestrator" {
			continue
		}
		if len(status.Files) != 7 {
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
	if !strings.Contains(output, "agentbox-planner") || !strings.Contains(output, "(role)") {
		t.Fatalf("text list does not report roles: %s", output)
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
	for _, file := range []string{"SKILL.md", "issue.md", "pr.md", "verdict.md", "state.md", "plan.md", "review-task.md", "verify-task.md"} {
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
	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "show", "--role", "agentbox-orchestrator"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "name: agentbox-orchestrator") || !strings.Contains(output, "dispatch: true") {
		t.Fatalf("shown orchestrator role lacks its canonical definition: %s", output[:200])
	}
}

func TestSkillStatusVersionWidth(t *testing.T) {
	longVersion := "v0.3.0-11-g5d89751"
	statuses := []skills.Status{
		{Installations: []skills.Installation{{Version: longVersion}}},
	}
	if got := skillStatusVersionWidth(statuses); got != 18 {
		t.Fatalf("version width = %d; want 18", got)
	}

	statuses = []skills.Status{
		{Installations: []skills.Installation{{Version: "0.2.0"}}},
	}
	if got := skillStatusVersionWidth(statuses); got != 8 {
		t.Fatalf("short version width = %d; want 8", got)
	}

	statuses = []skills.Status{
		{Files: []skills.FileStatus{{Name: "issue.md"}}},
	}
	if got := skillStatusVersionWidth(statuses); got != 8 {
		t.Fatalf("template-only version width = %d; want 8", got)
	}
}

func TestSkillsListDerivesLongVersionWidth(t *testing.T) {
	const longVersion = "v0.3.0-11-g5d89751"
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{".claude", ".codex", filepath.Join(".config", "opencode")} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := skills.Install(skills.Options{
		Home: home, All: true, Version: longVersion, SkipRoles: true,
	}); err != nil {
		t.Fatal(err)
	}

	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "list"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, longVersion) {
		t.Fatalf("skills list lacks long version: %s", output)
	}

	var longRows, shortRows, templateRows int
	var pathOffset int
	for _, line := range strings.Split(output, "\n") {
		offset := strings.Index(line, home)
		if offset < 0 {
			continue
		}
		if pathOffset == 0 {
			pathOffset = offset
		}
		if offset != pathOffset {
			t.Fatalf("path column differs: line=%q offset=%d want=%d", line, offset, pathOffset)
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			t.Fatalf("skills list row is not parseable: %q", line)
		}
		switch fields[2] {
		case longVersion:
			longRows++
		case "-":
			shortRows++
			if strings.HasPrefix(fields[0], "file:") {
				templateRows++
			}
		}
	}
	if longRows == 0 || shortRows == 0 || templateRows == 0 {
		t.Fatalf("skills list row versions: long=%d short=%d template=%d; output:\n%s", longRows, shortRows, templateRows, output)
	}
}

func TestSkillsListRowsKeepTemplateColumnsAligned(t *testing.T) {
	installation := formatSkillStatusRow("agents", "current", "0.2.0", "/tmp/skill/SKILL.md", 8)
	template := formatSkillStatusRow("file:review-task.md", "modified", "-", "/tmp/skill/review-task.md", 8)
	if !strings.HasPrefix(template, "  file:review-task.md ") {
		t.Fatalf("template target was truncated: %q", template)
	}
	installationState := strings.Index(installation, "current")
	templateState := strings.Index(template, "modified")
	if installationState == 0 || templateState != installationState {
		t.Fatalf("state columns differ: installation=%q template=%q", installation, template)
	}
	installationPath := strings.Index(installation, "/tmp/skill/SKILL.md")
	templatePath := strings.Index(template, "/tmp/skill/review-task.md")
	if installationPath == 0 || templatePath != installationPath {
		t.Fatalf("path columns differ: installation=%q template=%q", installation, template)
	}
}

func TestFormatSkillStatusRowLongVersion(t *testing.T) {
	statuses := []skills.Status{
		{Installations: []skills.Installation{{Version: "0.2.0"}}},
		{Installations: []skills.Installation{{Version: "v0.3.0-11-g5d89751"}}},
		{Files: []skills.FileStatus{{Name: "review-task.md"}}},
	}
	versionWidth := skillStatusVersionWidth(statuses)
	rows := []string{
		formatSkillStatusRow("agents", "current", "0.2.0", "/tmp/skill/SKILL.md", versionWidth),
		formatSkillStatusRow("agents", "current", "v0.3.0-11-g5d89751", "/tmp/long/SKILL.md", versionWidth),
		formatSkillStatusRow("file:review-task.md", "modified", "-", "/tmp/skill/review-task.md", versionWidth),
	}
	stateOffsets := make([]int, len(rows))
	pathOffsets := make([]int, len(rows))
	for i, row := range rows {
		stateOffsets[i] = strings.Index(row, "current")
		if i == len(rows)-1 {
			stateOffsets[i] = strings.Index(row, "modified")
		}
		pathOffsets[i] = strings.Index(row, "/tmp")
	}
	for i := 1; i < len(rows); i++ {
		if stateOffsets[i] != stateOffsets[0] || pathOffsets[i] != pathOffsets[0] {
			t.Fatalf("columns differ: rows=%q stateOffsets=%v pathOffsets=%v", rows, stateOffsets, pathOffsets)
		}
	}
	if !strings.Contains(rows[1], "v0.3.0-11-g5d89751") {
		t.Fatalf("long version was truncated: %q", rows[1])
	}
}

// TestSkillsOutputAlignsLongTemplateLabels covers lifecycle output, where the
// skill and template form one combined label. A long template label must not
// shift the path column.
func TestSkillsOutputAlignsLongTemplateLabels(t *testing.T) {
	results := []skills.Result{
		{Skill: "agentbox-orchestrator", File: "SKILL.md", Target: "agents", Path: "/tmp/skill/SKILL.md", Action: "unchanged"},
		{Skill: "agentbox-orchestrator", File: "verdict.md", Target: "agents", Path: "/tmp/skill/verdict.md", Action: "updated"},
	}
	output, err := captureStdout(t, func() error { return printSkillsOutput(results, nil, false) })
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(output, "\n")
	if len(lines) != 2 {
		t.Fatalf("output lines = %d: %q", len(lines), output)
	}
	if !strings.Contains(lines[1], "agentbox-orchestrator/verdict.md") {
		t.Fatalf("template label missing: %q", lines[1])
	}
	installationPath := strings.Index(lines[0], "/tmp/skill/SKILL.md")
	templatePath := strings.Index(lines[1], "/tmp/skill/verdict.md")
	if installationPath == 0 || templatePath != installationPath {
		t.Fatalf("path columns differ: installation=%q template=%q", lines[0], lines[1])
	}
}

func TestSkillsInstallRefusesDifferingSkillFileWithoutForce(t *testing.T) {
	dir := t.TempDir()
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	if err := dispatch(context.Background(), a, []string{"skills", "install", "--dir", dir}); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"issue.md", "pr.md", "verdict.md", "state.md", "plan.md", "review-task.md", "verify-task.md"} {
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
}

// TestSkillsInstallRefusesDifferingTemplateWithoutForce isolates the template
// case in its own --dir so the failure can only come from the template.
func TestSkillsInstallRefusesDifferingTemplateWithoutForce(t *testing.T) {
	dir := t.TempDir()
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	if err := dispatch(context.Background(), a, []string{"skills", "install", "--dir", dir}); err != nil {
		t.Fatal(err)
	}
	template := filepath.Join(dir, "agentbox-orchestrator", "pr.md")
	if err := os.WriteFile(template, []byte("edited template\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := dispatch(context.Background(), a, []string{"skills", "install", "--dir", dir})
	if err == nil {
		t.Fatal("install overwrote a differing template without --force")
	}
	if !strings.Contains(err.Error(), template) {
		t.Fatalf("failure was not caused by the modified template: %v", err)
	}
	if strings.Contains(err.Error(), filepath.Join(dir, "agentbox-cli")) {
		t.Fatalf("failure also reported an untouched skill: %v", err)
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

func TestUsageListsProviderListCommand(t *testing.T) {
	output, err := captureStdout(t, func() error {
		usage()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "provider setup|list") {
		t.Fatalf("usage is missing the provider list command: %s", output)
	}
}

func TestUsageListsOrchestratorCommand(t *testing.T) {
	output, err := captureStdout(t, func() error {
		usage()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "orchestrator config path|show|validate · gate pr-body|pr-merge") {
		t.Fatalf("usage is missing the orchestrator command: %s", output)
	}
}

func captureStderr(t *testing.T, action func() error) (string, error) {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = write
	done := make(chan struct{})
	var body []byte
	var readErr error
	go func() {
		body, readErr = io.ReadAll(read)
		close(done)
	}()
	callErr := action()
	_ = write.Close()
	os.Stderr = previous
	<-done
	defer read.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(bytes.TrimSpace(body)), callErr
}

func TestUsageListsLifecycleCommands(t *testing.T) {
	output, err := captureStdout(t, func() error {
		usage()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "sleep|resume|finish|stop") {
		t.Fatalf("usage is missing the lifecycle commands: %s", output)
	}
}

func TestUsageListsAgentDenialsCommand(t *testing.T) {
	output, err := captureStdout(t, func() error {
		usage()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "agent start|list|show|attach|logs|denials|sleep|resume|finish|stop|sync|diff|delete") {
		t.Fatalf("usage is missing the agent denials command: %s", output)
	}
}

type logsCommandRunner struct {
	commands []string
}

func (runner *logsCommandRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	command := name + " " + strings.Join(args, " ")
	runner.commands = append(runner.commands, command)
	if strings.Contains(command, "capture-pane") {
		return []byte("agentbox-source:screen\nrendered\n"), nil
	}
	return []byte(strings.Repeat("raw-", 50)), nil
}

func (*logsCommandRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func TestAgentLogsRawFlagReadsRawStream(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(app.Run{ID: "run-logs-raw", Sandbox: "sandbox-raw", Gateway: "gateway-raw", Status: app.StatusReady}); err != nil {
		t.Fatal(err)
	}
	runner := &logsCommandRunner{}
	a := &app.App{Store: store, Runner: runner}
	want := strings.Repeat("raw-", 50)
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "logs", "--raw", "run-logs-raw"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if output != want {
		t.Fatalf("raw output = %q, want %q", output, want)
	}
	if len(runner.commands) != 2 || !strings.Contains(runner.commands[1], "cat /workspace/state/agent.log") {
		t.Fatalf("raw logs commands = %#v", runner.commands)
	}
	for _, command := range runner.commands {
		if strings.Contains(command, "capture-pane") {
			t.Fatalf("raw logs used terminal rendering: %s", command)
		}
	}
}

func TestAgentLogsDefaultDoesNotReadRawStream(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(app.Run{ID: "run-logs-default", Sandbox: "sandbox-default", Gateway: "gateway-default", Status: app.StatusReady}); err != nil {
		t.Fatal(err)
	}
	runner := &logsCommandRunner{}
	a := &app.App{Store: store, Runner: runner}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "logs", "run-logs-default"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if output != "rendered" {
		t.Fatalf("default output = %q", output)
	}
	if len(runner.commands) != 1 || !strings.Contains(runner.commands[0], "capture-pane") {
		t.Fatalf("default logs commands = %#v", runner.commands)
	}
}

func TestAgentStartRejectsRemovedModeFlag(t *testing.T) {
	ctx := context.Background()
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	err := dispatch(ctx, a, []string{"agent", "start", "--mode", "gateway"})
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined: -mode") {
		t.Fatalf("removed mode flag error = %v", err)
	}

	stderr, err := captureStderr(t, func() error {
		return dispatch(ctx, a, []string{"agent", "start", "--help"})
	})
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		t.Fatal(err)
	}
	if strings.Contains(stderr, "--mode") || strings.Contains(stderr, "  -mode ") {
		t.Fatalf("agent start help still mentions mode: %s", stderr)
	}

	usageOutput, err := captureStdout(t, func() error {
		usage()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(usageOutput, "--mode") {
		t.Fatalf("top-level help still mentions mode: %s", usageOutput)
	}
}

func TestAgentStartHelpListsRequireHead(t *testing.T) {
	stderr, err := captureStderr(t, func() error {
		return dispatch(context.Background(), &app.App{}, []string{"agent", "start", "--help"})
	})
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "-require-head") || !strings.Contains(stderr, "full source commit SHA required before run creation") {
		t.Fatalf("agent start help is missing --require-head:\n%s", stderr)
	}
}

func TestAgentSyncHelpExplainsDestination(t *testing.T) {
	for _, helpFlag := range []string{"--help", "-h"} {
		t.Run(helpFlag, func(t *testing.T) {
			output, err := captureStdout(t, func() error {
				return dispatch(context.Background(), &app.App{}, []string{"agent", "sync", helpFlag})
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{
				"Usage: agentbox agent sync ID",
				"~/.agentbox/worktrees/agentbox/<name>",
				"The --source option to",
				"names only the input checkout",
			} {
				if !strings.Contains(output, want) {
					t.Fatalf("agent sync help missing %q:\n%s", want, output)
				}
			}
		})
	}
}

func TestAgentSyncPrintsDestinationWorktree(t *testing.T) {
	a, run := agentSyncCLIFixture(t)
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "sync", run.ID})
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "synced to " + run.Worktree; output != want {
		t.Fatalf("output = %q; want %q", output, want)
	}
}

func TestAgentStopWarnsAndDelegatesToSleep(t *testing.T) {
	ctx := context.Background()
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	run := app.Run{ID: "run-deprecated-stop", Agent: "codex", Status: app.StatusCompleted, Sandbox: "sandbox-deprecated", Gateway: "gateway-unit"}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Store: store, Runner: &appfakeRunner{}}
	var callErr error
	stderr, err := captureStderr(t, func() error {
		_, callErr = captureStdout(t, func() error {
			return dispatch(ctx, a, []string{"agent", "stop", run.ID})
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "deprecated") || !strings.Contains(stderr, "agent sleep") {
		t.Fatalf("stop warning = %q", stderr)
	}
	if callErr == nil || !strings.Contains(callErr.Error(), "sleep applies to") {
		t.Fatalf("stop did not delegate to sleep: %v", callErr)
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
		Mode: "standard", Provider: "openai", Policy: policy.Path, Approval: "prompted",
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

// TestSkillsRolesRenderAndShow covers the role surface of the CLI: the
// canonical definition is shown, and render emits a harness file on stdout
// without touching the installed copy.
func TestSkillsRolesRenderAndShow(t *testing.T) {
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
	installed := filepath.Join(home, ".codex", "agents", "agentbox-reviewer.toml")
	before, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}

	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "show", "agentbox-planner"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "name: agentbox-planner") || !strings.Contains(output, "write: false") {
		t.Fatalf("shown role lacks its canonical definition: %s", output[:120])
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "roles", "render", "agentbox-reviewer", "--harness", "codex", "--model", "override-model", "--effort", "high"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `model = "override-model"`) || !strings.Contains(output, `model_reasoning_effort = "high"`) {
		t.Fatalf("rendered role lacks the override: %s", output)
	}
	after, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("roles render modified the installed role")
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "roles", "render", "agentbox-implementer", "--harness", "claude"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "tools:") || !strings.Contains(output, "Edit, Write") {
		t.Fatalf("writable claude role lacks write tools: %s", output)
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "roles", "render", "agentbox-triage", "--harness", "claude"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "Bash") || strings.Contains(output, "Edit") || strings.Contains(output, "Write") {
		t.Fatalf("triage claude role has host tools: %s", output)
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "roles", "render", "agentbox-retro", "--harness", "opencode"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "action: shell") || !strings.Contains(output, "action: edit") || !strings.Contains(output, "effect: deny") {
		t.Fatalf("retro opencode role does not deny shell and edit: %s", output)
	}
}

// TestSkillsInstallSkipRoles installs only the usage skills.
func TestSkillsInstallSkipRoles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{".claude", ".codex", filepath.Join(".config", "opencode")} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	if err := dispatch(context.Background(), a, []string{"skills", "install", "--skip-roles"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "agentbox-cli", "SKILL.md")); err != nil {
		t.Fatalf("skills were not installed: %v", err)
	}
	for _, dir := range []string{filepath.Join(".claude", "agents"), filepath.Join(".codex", "agents")} {
		if entries, err := os.ReadDir(filepath.Join(home, dir)); err == nil && len(entries) > 0 {
			t.Fatalf("--skip-roles wrote agent files: %v", entries)
		}
	}
}

// TestSkillsLocalReportsGlobalRoles keeps a project-scoped install from
// writing global agent files and reports why.
func TestSkillsLocalReportsGlobalRoles(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "install", "--local"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "agent roles are global-only") {
		t.Fatalf("local install did not report skipped roles: %s", output)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "agents")); !os.IsNotExist(err) {
		t.Fatalf("local install wrote agent files: %v", err)
	}
}

// TestSkillsInstallJSONReportsOneRoleRowPerHarness covers the install report
// contract: one row per role and harness, in text and JSON.
func TestSkillsInstallJSONReportsOneRoleRowPerHarness(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{".claude", ".codex", filepath.Join(".config", "opencode")} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"skills", "install", "--all", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var results []skills.Result
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		t.Fatalf("install output is not JSON: %v", err)
	}
	count := 0
	for _, result := range results {
		if result.Kind == "role" {
			count++
			if !skills.ValidRoleName(result.Skill) {
				t.Fatalf("role row has an unknown role name: %+v", result)
			}
			if result.Target != "claude" && result.Target != "codex" && result.Target != "opencode" {
				t.Fatalf("role row target = %+v", result)
			}
			if result.Action != "installed" {
				t.Fatalf("role row action = %+v", result)
			}
		}
	}
	if want := len(skills.RoleNames()) * 3; count != want {
		t.Fatalf("role rows = %d, want %d", count, want)
	}
}

type policyCLIRunner struct{ reject bool }

func argPresent(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func (r policyCLIRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	switch {
	case name == "openshell" && argPresent(args, "sandbox") && argPresent(args, "get"):
		return []byte(`{"phase":"running"}`), nil
	case name == "openshell" && argPresent(args, "sandbox") && argPresent(args, "exec"):
		return []byte("session=1\ndead=0\ncommand=codex\nplan=0\nidle=0\nfinished=0\nexit=\nactivity=\nmtime=\n"), nil
	case name == "openshell" && argPresent(args, "policy") && argPresent(args, "set"):
		if r.reject {
			return nil, errors.New("policy rejected")
		}
		return nil, nil
	default:
		return []byte(`{}`), nil
	}
}

func (policyCLIRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func policyCLIFixture(t *testing.T) (*app.App, string) {
	t.Helper()
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	policy, err := store.CreatePolicyConfiguration("unit-policy")
	if err != nil {
		t.Fatal(err)
	}
	run := app.Run{
		ID: "run-policy", Policy: policy.Path, Sandbox: "sandbox-policy",
		Gateway: "gateway-one", Agent: "codex", Status: app.StatusWorking,
	}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	return &app.App{Store: store, Runner: &appfakeRunner{}}, run.ID
}

func TestPolicyEditCLIPrintsNetworkPreviewDiff(t *testing.T) {
	a, runID := policyCLIFixture(t)
	preview, err := a.PolicyEdit(context.Background(), runID, "api.example.test", 443, "/usr/bin/curl", true)
	if err != nil {
		t.Fatal(err)
	}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"policy", "edit", "--allow-host", "api.example.test", "--binary", "/usr/bin/curl", "--dry-run", runID})
	})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(preview.Diff) + "\nnetwork change: apply live on save"
	if output != want {
		t.Fatalf("stdout = %q; want %q", output, want)
	}
}

func TestPolicyEditCLIPrintsStaticPreviewDiff(t *testing.T) {
	a, runID := policyCLIFixture(t)
	editor := filepath.Join(t.TempDir(), "editor.sh")
	script := "#!/bin/sh\nperl -0pi -e 's|    - /tmp\\n|    - /tmp\\n    - /srv\\n|' \"$1\"\n"
	if err := os.WriteFile(editor, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editor)
	preview, err := a.PolicyEdit(context.Background(), runID, "", 0, "", true)
	if err != nil {
		t.Fatal(err)
	}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"policy", "edit", "--dry-run", runID})
	})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(preview.Diff) + "\nstatic change: pending for a future sandbox"
	if output != want {
		t.Fatalf("stdout = %q; want %q", output, want)
	}
}

func TestPolicyEditCLIPrintsDiffBeforeOpenShellError(t *testing.T) {
	a, runID := policyCLIFixture(t)
	a.Runner = policyCLIRunner{reject: true}
	preview, err := a.PolicyEdit(context.Background(), runID, "api.example.test", 443, "/usr/bin/curl", true)
	if err != nil {
		t.Fatal(err)
	}
	a.Runner = policyCLIRunner{reject: true}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"policy", "edit", "--allow-host", "api.example.test", "--binary", "/usr/bin/curl", runID})
	})
	if err == nil || !strings.Contains(err.Error(), "the local policy was restored") {
		t.Fatalf("error = %v", err)
	}
	if output != strings.TrimSpace(preview.Diff) {
		t.Fatalf("stdout = %q; want diff %q", output, strings.TrimSpace(preview.Diff))
	}
}

func TestOrchestratorConfigPathShowAndValidate(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	path := app.OrchestrationRegistryPath(a.Store.Base)
	body := []byte(`
version: 1
default: base
configs:
  base:
    workers:
      default: {runtime: orca, agent: codex, model: base-model}
projects:
  - name: repo
    root: /repo
    workers:
      stages:
        implement: {model: repo-model}
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"orchestrator", "config", "path"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if output != path {
		t.Fatalf("path = %q; want %q", output, path)
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"orchestrator", "config", "show", "--project", "/repo/worktree", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var resolved app.ResolvedOrchestration
	if err := json.Unmarshal([]byte(output), &resolved); err != nil {
		t.Fatalf("show output is not JSON: %s (%v)", output, err)
	}
	if !resolved.Present || resolved.Scope != "project" || resolved.Project != "repo" {
		t.Fatalf("resolved = %+v", resolved)
	}
	if got := resolved.Stages["implement"].Model; got != "repo-model" {
		t.Fatalf("implement model = %q", got)
	}

	output, err = captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"orchestrator", "config", "validate", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var validation struct {
		Path  string `json:"path"`
		Valid bool   `json:"valid"`
	}
	if err := json.Unmarshal([]byte(output), &validation); err != nil {
		t.Fatalf("validate output is not JSON: %s (%v)", output, err)
	}
	if validation.Path != path || !validation.Valid {
		t.Fatalf("validation = %+v", validation)
	}
}

func TestOrchestratorConfigShowWithoutRegistry(t *testing.T) {
	a := &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &appfakeRunner{}}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"orchestrator", "config", "show", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var resolved app.ResolvedOrchestration
	if err := json.Unmarshal([]byte(output), &resolved); err != nil {
		t.Fatalf("show output is not JSON: %s (%v)", output, err)
	}
	if resolved.Present || resolved.Scope != "none" || len(resolved.Stages) != len(app.OrchestrationStageNames()) {
		t.Fatalf("resolved = %+v", resolved)
	}
	text, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"orchestrator", "config", "show"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "scope: none") || !strings.Contains(text, "no applicable configuration") {
		t.Fatalf("text output = %s", text)
	}
	if err := dispatch(context.Background(), a, []string{"orchestrator", "config", "validate"}); err == nil {
		t.Fatal("validate accepted a missing registry")
	}
}

const denialSandboxLog = `[1795252309.297] [sandbox] [OCSF ] [ocsf] NET:REFUSE [MED] DENIED ab.chatgpt.com [reason:policy_dns_ineligible]
[1795252309.302] [sandbox] [OCSF ] [ocsf] NET:OPEN [MED] DENIED /usr/local/bin/codex(0) -> ab.chatgpt.com:443 [reason:transparent_tcp_policy_denied]
[1795252309.310] [sandbox] [OCSF ] [ocsf] NET:OPEN [MED] DENIED curl(0) -> relative.test:80 [reason:transparent_tcp_policy_denied]
`

type denialsRunner struct{ reject bool }

func (r denialsRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	switch {
	case name == "openshell" && len(args) > 0 && args[0] == "logs":
		return []byte(denialSandboxLog), nil
	case name == "openshell" && len(args) > 0 && args[0] == "policy" && r.reject:
		return nil, errors.New("policy rejected")
	default:
		return []byte(`{}`), nil
	}
}

func (denialsRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func denialsCLIFixture(t *testing.T) (*app.App, string) {
	t.Helper()
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	policy, err := store.CreatePolicyConfiguration("unit-policy")
	if err != nil {
		t.Fatal(err)
	}
	run := app.Run{
		ID: "run-denials", Name: "unit-denials", Policy: policy.Path, Sandbox: "sandbox-denials",
		Gateway: "gateway-one", Agent: "codex", Status: app.StatusWorking,
	}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	return &app.App{Store: store, Runner: denialsRunner{}}, run.ID
}

func denialRows(t *testing.T, a *app.App, runID string) []app.PolicyDenial {
	t.Helper()
	denials, err := a.RunDenials(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	return denials
}

func runDenialSelection(t *testing.T, a *app.App, runID, input string) (string, error) {
	t.Helper()
	return captureStdout(t, func() error {
		return runDenialsCommand(context.Background(), a, runID, denialRows(t, a, runID), true, strings.NewReader(input))
	})
}

// TestAgentDenialsListsRowsWithoutPrompting covers the non-interactive path:
// piped or redirected stdin must print the listing and exit without blocking.
func TestAgentDenialsListsRowsWithoutPrompting(t *testing.T) {
	a, runID := denialsCLIFixture(t)
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "denials", runID})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"unit-denials (run-denials)", "2 network denial(s)",
		"ab.chatgpt.com", "443", "transparent_tcp_policy_denied",
		"relative.test", "invalid", "must be an absolute path",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("listing missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Allow which?") {
		t.Fatalf("non-interactive listing prompted:\n%s", output)
	}
}

func TestAgentDenialsJSONMatchesAPISchema(t *testing.T) {
	a, runID := denialsCLIFixture(t)
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "denials", "--json", runID})
	})
	if err != nil {
		t.Fatal(err)
	}
	var denials []app.PolicyDenial
	if err := json.Unmarshal([]byte(output), &denials); err != nil || len(denials) != 2 {
		t.Fatalf("denials JSON = %s (%v)", output, err)
	}
	if denials[0].Host != "ab.chatgpt.com" || denials[0].Port != 443 || !denials[0].Valid {
		t.Fatalf("first denial = %+v", denials[0])
	}
	if !strings.HasSuffix(denials[0].Binary, "/bin/codex") {
		t.Fatalf("first denial binary = %q", denials[0].Binary)
	}
	if denials[1].Valid || denials[1].InvalidReason == "" {
		t.Fatalf("second denial = %+v; want invalid with a reason", denials[1])
	}
}

func TestAgentDenialsEmptyListing(t *testing.T) {
	a, runID := denialsCLIFixture(t)
	a.Runner = &appfakeRunner{}
	output, err := captureStdout(t, func() error {
		return dispatch(context.Background(), a, []string{"agent", "denials", runID})
	})
	if err != nil {
		t.Fatal(err)
	}
	if output != "No network denials." {
		t.Fatalf("output = %q", output)
	}
}

func TestAgentDenialsPromptAppliesSelectedRow(t *testing.T) {
	a, runID := denialsCLIFixture(t)
	output, err := runDenialSelection(t, a, runID, "1\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Allow which?", "ab.chatgpt.com:443 live-applied", "run-only:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	stored, err := a.Store.LoadRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.RuntimeHosts) != 1 || stored.RuntimeHosts[0].Host != "ab.chatgpt.com" || stored.RuntimeHosts[0].Source != "run" {
		t.Fatalf("runtime hosts = %+v", stored.RuntimeHosts)
	}
	policy, err := os.ReadFile(stored.Policy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(policy), "ab.chatgpt.com") || strings.Contains(string(policy), "relative.test") {
		t.Fatalf("run policy was not patched with the selected denial only:\n%s", policy)
	}
}

func TestAgentDenialsPromptAllAppliesOnlyValidRows(t *testing.T) {
	a, runID := denialsCLIFixture(t)
	output, err := runDenialSelection(t, a, runID, "all\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "relative.test:80 live-applied") {
		t.Fatalf("invalid row was applied:\n%s", output)
	}
	if !strings.Contains(output, "ab.chatgpt.com:443 live-applied") {
		t.Fatalf("valid row was not applied:\n%s", output)
	}
	stored, _ := a.Store.LoadRun(runID)
	if len(stored.RuntimeHosts) != 1 {
		t.Fatalf("runtime hosts = %+v", stored.RuntimeHosts)
	}
}

func TestAgentDenialsPromptSkipsOnEmptyInput(t *testing.T) {
	a, runID := denialsCLIFixture(t)
	output, err := runDenialSelection(t, a, runID, "\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Skipped.") {
		t.Fatalf("output = %q", output)
	}
	stored, _ := a.Store.LoadRun(runID)
	if len(stored.RuntimeHosts) != 0 {
		t.Fatalf("skip applied host rules: %+v", stored.RuntimeHosts)
	}
}

func TestAgentDenialsPromptReportsOpenShellRejection(t *testing.T) {
	a, runID := denialsCLIFixture(t)
	a.Runner = denialsRunner{reject: true}
	output, err := runDenialSelection(t, a, runID, "1\n")
	if err == nil || !strings.Contains(err.Error(), "1 of 1 host rule(s) failed") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(output, "openshell-rejected") || !strings.Contains(output, "rolled back") {
		t.Fatalf("output missing the rejection outcome:\n%s", output)
	}
}

func TestAgentDenialsPromptReportsNoLiveSandbox(t *testing.T) {
	a, runID := denialsCLIFixture(t)
	stored, err := a.Store.LoadRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Status = app.StatusCompleted
	if err := a.Store.SaveRun(stored); err != nil {
		t.Fatal(err)
	}
	output, err := runDenialSelection(t, a, runID, "1\n")
	if err == nil || !strings.Contains(err.Error(), "1 of 1 host rule(s) failed") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(output, "Add host is available only for Ready, Working, and Waiting agents.") {
		t.Fatalf("output missing the no-live-sandbox reason:\n%s", output)
	}
}
