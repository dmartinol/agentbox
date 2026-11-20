//go:build live

package it_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmartinol/agentbox/internal/app"
)

var liveBinary string

func TestMain(m *testing.M) {
	if os.Getenv("AGENTBOX_LIVE") != "1" {
		fmt.Fprintln(os.Stderr, "AGENTBOX_LIVE is not set; skipping live integration tests")
		os.Exit(0)
	}
	root, err := filepath.Abs("..")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	tmp, err := os.MkdirTemp("", "agentbox-live-it-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)
	liveBinary = filepath.Join(tmp, "agentbox")
	build := exec.Command("go", "build", "-o", liveBinary, "./cmd/agentbox")
	build.Dir = root
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build agentbox:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func liveGateway() string {
	if gateway := os.Getenv("AGENTBOX_LIVE_GATEWAY"); gateway != "" {
		return gateway
	}
	return "openshell"
}

func liveSource() string {
	if source := os.Getenv("AGENTBOX_LIVE_SOURCE"); source != "" {
		return source
	}
	return "."
}

func runLive(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(liveBinary, args...)
	cmd.Dir = ".."
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("agentbox %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.Bytes()
}

func TestLiveDoctor(t *testing.T) {
	var checks []app.Check
	if err := json.Unmarshal(runLive(t, "doctor", "--json"), &checks); err != nil {
		t.Fatal(err)
	}
	byName := map[string]app.Check{}
	for _, check := range checks {
		byName[check.Name] = check
	}
	for _, name := range []string{"openshell", "podman", "podman connection", "gateway"} {
		check, ok := byName[name]
		if !ok || !check.OK {
			t.Fatalf("doctor check %q failed: %+v", name, check)
		}
	}
}

func TestLiveGatewayUsesPodman(t *testing.T) {
	var payload struct {
		Status         string `json:"status"`
		ComputeDrivers []struct {
			Name string `json:"name"`
		} `json:"compute_drivers"`
	}
	if err := json.Unmarshal(runLive(t, "gateway", "info", "--json", liveGateway()), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "" && payload.Status != "healthy" && payload.Status != "connected" {
		t.Fatalf("unexpected gateway status %q", payload.Status)
	}
	for _, driver := range payload.ComputeDrivers {
		if driver.Name == "podman" {
			return
		}
	}
	t.Fatalf("gateway %q does not expose a Podman compute driver: %+v", liveGateway(), payload)
}

func TestLiveImageInventory(t *testing.T) {
	var images []struct {
		Names  []string          `json:"Names"`
		Labels map[string]string `json:"Labels"`
	}
	if err := json.Unmarshal(runLive(t, "image", "list", "--json"), &images); err != nil {
		t.Fatal(err)
	}
	if len(images) == 0 {
		t.Fatal("no AgentBox images found; build at least one image before running the live IT")
	}
	for _, image := range images {
		if len(image.Names) > 0 {
			t.Logf("live image: %s", strings.Join(image.Names, ", "))
			return
		}
	}
	t.Fatal("AgentBox image records do not contain image names")
}

func TestLiveProfileDryRun(t *testing.T) {
	profile := os.Getenv("AGENTBOX_LIVE_PROFILE")
	if profile == "" {
		t.Skip("set AGENTBOX_LIVE_PROFILE to validate a real profile and dry-run a launch")
	}
	var saved app.Profile
	if err := json.Unmarshal(runLive(t, "profile", "show", "--json", profile), &saved); err != nil {
		t.Fatal(err)
	}
	if err := saved.Validate(); err != nil {
		t.Fatalf("live profile is invalid: %v", err)
	}
	var run app.Run
	args := []string{"agent", "start", "--dry-run", "--json", "--profile", profile, "--gateway", liveGateway(), "--source", liveSource()}
	if err := json.Unmarshal(runLive(t, args...), &run); err != nil {
		t.Fatal(err)
	}
	if run.Status != app.StatusCreating || run.ID == "" || run.StatusDetail != "dry run preview" {
		t.Fatalf("unexpected dry-run result: %+v", run)
	}
	t.Logf("profile %q is launchable in preview mode", profile)
}

func TestLivePolicyAndRunState(t *testing.T) {
	runID := os.Getenv("AGENTBOX_LIVE_RUN")
	if runID == "" {
		t.Skip("set AGENTBOX_LIVE_RUN to validate policy and run state for an existing run")
	}
	var run app.Run
	if err := json.Unmarshal(runLive(t, "agent", "show", "--json", runID), &run); err != nil {
		t.Fatal(err)
	}
	if run.ID != runID {
		t.Fatalf("agent show returned ID %q; want %q", run.ID, runID)
	}
	var policy map[string]json.RawMessage
	if err := json.Unmarshal(runLive(t, "policy", "show", "--json", runID), &policy); err != nil {
		t.Fatal(err)
	}
	if len(policy) == 0 {
		t.Fatal("policy show returned no policy sections")
	}
	for _, key := range []string{"base", "effective"} {
		if _, ok := policy[key]; !ok {
			t.Fatalf("policy show missing %q section", key)
		}
	}
	var effective struct {
		ActiveVersion  int    `json:"active_version"`
		ConfigRevision uint64 `json:"config_revision"`
		Status         string `json:"status"`
	}
	if err := json.Unmarshal(policy["effective"], &effective); err != nil {
		t.Fatal(err)
	}
	if effective.ActiveVersion <= 0 || effective.ConfigRevision == 0 {
		t.Fatalf("effective policy has no loaded revision: %+v", effective)
	}
}

func TestLiveImageRecipes(t *testing.T) {
	var images []struct {
		Names  []string          `json:"Names"`
		Digest string            `json:"Digest"`
		Labels map[string]string `json:"Labels"`
	}
	if err := json.Unmarshal(runLive(t, "image", "list", "--json"), &images); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"claude", "codex", "opencode"} {
		found := false
		for _, image := range images {
			label := func(name string) string {
				if value := image.Labels["agentbox."+name]; value != "" {
					return value
				}
				return image.Labels["ai-sandbox."+name]
			}
			if label("agent") != agent {
				continue
			}
			if label("recipe") == app.CurrentImageRecipe() && label("version") != "" && label("base-digest") != "" && image.Digest != "" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("no r%s %s image with version, base digest, and image digest found", app.CurrentImageRecipe(), agent)
		}
		var proposed struct {
			Agent    string `json:"agent"`
			Proposed string `json:"proposed"`
		}
		if err := json.Unmarshal(runLive(t, "image", "versions", "--json", agent), &proposed); err != nil {
			t.Fatal(err)
		}
		if proposed.Agent != agent || proposed.Proposed == "" {
			t.Fatalf("unexpected version proposal for %s: %+v", agent, proposed)
		}
	}
}

func TestLiveRegistryRoundTrip(t *testing.T) {
	local := os.Getenv("AGENTBOX_LIVE_LOCAL_IMAGE")
	remote := os.Getenv("AGENTBOX_LIVE_REGISTRY_REF")
	if local == "" || remote == "" {
		t.Skip("blocked: set AGENTBOX_LIVE_LOCAL_IMAGE and AGENTBOX_LIVE_REGISTRY_REF to verify registry push/pull")
	}
	runLive(t, "image", "push", local, remote)
	runLive(t, "image", "pull", remote)
}

func liveApproval() string {
	if approval := os.Getenv("AGENTBOX_LIVE_APPROVAL"); approval != "" {
		return approval
	}
	return "unattended"
}

func livePrompt() string {
	if prompt := os.Getenv("AGENTBOX_LIVE_PROMPT"); prompt != "" {
		return prompt
	}
	return "Reply with exactly OK. Do not modify any files."
}

func startLiveRun(t *testing.T, profile string) app.Run {
	t.Helper()
	args := []string{
		"agent", "start", "--json",
		"--profile", profile,
		"--gateway", liveGateway(),
		"--source", liveSource(),
		"--prompt", livePrompt(),
		"--approval", liveApproval(),
	}
	var run app.Run
	if err := json.Unmarshal(runLive(t, args...), &run); err != nil {
		t.Fatal(err)
	}
	if run.ID == "" || run.Status == app.StatusFailed {
		t.Fatalf("unexpected started run: %+v", run)
	}
	return run
}

func liveRunStatus(t *testing.T, id string) app.Run {
	t.Helper()
	var runs []app.Run
	if err := json.Unmarshal(runLive(t, "agent", "list", "--json"), &runs); err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		if run.ID == id {
			return run
		}
	}
	t.Fatalf("agent %s not found in agent list", id)
	return app.Run{}
}

func liveRunLogs(t *testing.T, id string) string {
	t.Helper()
	return string(runLive(t, "agent", "logs", id))
}

// waitForExitedAgent waits until the harness exits and the run stays resumable
// instead of finishing itself.
func waitForExitedAgent(t *testing.T, run app.Run, timeout time.Duration) app.Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		current := liveRunStatus(t, run.ID)
		switch {
		case current.Status == app.StatusFailed:
			t.Fatalf("run %s failed; logs:\n%s", run.ID, liveRunLogs(t, run.ID))
		case current.Status == app.StatusCompleted:
			t.Fatalf("run %s finished without an explicit finish: %+v", run.ID, current)
		case current.Status == app.StatusReady && current.Exited && strings.Contains(current.StatusDetail, "resume available"):
			return current
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("run %s did not exit within %s; logs:\n%s", run.ID, timeout, liveRunLogs(t, run.ID))
	return app.Run{}
}

func runLiveAgent(t *testing.T, args ...string) {
	t.Helper()
	runLive(t, append([]string{"agent"}, args...)...)
}

func waitForLiveStatus(t *testing.T, id string, timeout time.Duration, want ...app.RunStatus) app.Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		current := liveRunStatus(t, id)
		if current.Status == app.StatusFailed {
			t.Fatalf("run %s failed; logs:\n%s", id, liveRunLogs(t, id))
		}
		for _, status := range want {
			if current.Status == status {
				return current
			}
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("run %s did not reach %v within %s; logs:\n%s", id, want, timeout, liveRunLogs(t, id))
	return app.Run{}
}

func deleteLiveRun(t *testing.T, id string) {
	t.Helper()
	if os.Getenv("AGENTBOX_LIVE_KEEP_RUN") == "1" {
		t.Logf("keeping live run %s because AGENTBOX_LIVE_KEEP_RUN=1", id)
		return
	}
	_ = runLiveNoFail(t, "agent", "delete", "--discard", id)
}

func runLiveNoFail(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(liveBinary, args...)
	cmd.Dir = ".."
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Logf("agentbox %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.Bytes()
}

func TestLiveNewRunLifecycle(t *testing.T) {
	if os.Getenv("AGENTBOX_LIVE_NEW_RUN") != "1" {
		t.Skip("set AGENTBOX_LIVE_NEW_RUN=1 to start and finish a real agent run")
	}
	profile := os.Getenv("AGENTBOX_LIVE_PROFILE")
	if profile == "" {
		t.Fatal("AGENTBOX_LIVE_PROFILE is required with AGENTBOX_LIVE_NEW_RUN=1")
	}
	timeout := liveTimeout()
	run := startLiveRun(t, profile)
	defer deleteLiveRun(t, run.ID)
	exited := waitForExitedAgent(t, run, timeout)
	if exited.StatusDetail != "agent exited (code 0); resume available" {
		t.Logf("agent exited with detail %q", exited.StatusDetail)
	}
	if os.Getenv("AGENTBOX_LIVE_SLEEP_RESUME") == "1" {
		runLiveAgent(t, "sleep", run.ID)
		slept := waitForLiveStatus(t, run.ID, timeout, app.StatusStopped)
		if slept.StatusDetail != "asleep; resume available" {
			t.Fatalf("sleep detail = %q", slept.StatusDetail)
		}
		runLiveAgent(t, "resume", "--no-attach", run.ID)
		waitForLiveStatus(t, run.ID, timeout, app.StatusReady, app.StatusWorking, app.StatusWaiting)
		// The continued conversation may still be working; force ends the turn
		// before sleeping again so the test never races the agent.
		runLiveAgent(t, "sleep", "--force", run.ID)
		waitForLiveStatus(t, run.ID, timeout, app.StatusStopped)
	}
	runLiveAgent(t, "finish", run.ID)
	finished := waitForLiveStatus(t, run.ID, timeout, app.StatusCompleted)
	if !finished.Synced {
		t.Fatalf("finished run was not synced: %+v", finished)
	}
	logs := liveRunLogs(t, run.ID)
	if strings.TrimSpace(logs) == "" {
		t.Fatal("run logs are empty")
	}
	_ = runLive(t, "agent", "diff", run.ID)
	t.Logf("run %s logs:\n%s", run.ID, logs)
}

func TestLiveConcurrentRuns(t *testing.T) {
	profilesValue := os.Getenv("AGENTBOX_LIVE_CONCURRENT_PROFILES")
	if profilesValue == "" {
		t.Skip("set AGENTBOX_LIVE_CONCURRENT_PROFILES to a comma-separated list to verify concurrent runs")
	}
	profiles := strings.Split(profilesValue, ",")
	if len(profiles) < 2 {
		t.Fatal("AGENTBOX_LIVE_CONCURRENT_PROFILES must contain at least two profiles")
	}
	timeout := liveTimeout()
	var runs []app.Run
	for _, profile := range profiles {
		profile = strings.TrimSpace(profile)
		if profile == "" {
			continue
		}
		run := startLiveRun(t, profile)
		runs = append(runs, run)
		defer deleteLiveRun(t, run.ID)
	}
	if len(runs) < 2 {
		t.Fatal("fewer than two concurrent runs started")
	}
	seenWorktrees := map[string]bool{}
	seenBranches := map[string]bool{}
	for _, run := range runs {
		if seenWorktrees[run.Worktree] {
			t.Fatalf("duplicate worktree %s", run.Worktree)
		}
		if seenBranches[run.Branch] {
			t.Fatalf("duplicate branch %s", run.Branch)
		}
		seenWorktrees[run.Worktree] = true
		seenBranches[run.Branch] = true
	}
	for _, run := range runs {
		waitForExitedAgent(t, run, timeout)
		runLiveAgent(t, "finish", run.ID)
		waitForLiveStatus(t, run.ID, timeout, app.StatusCompleted)
	}
}

func liveTimeout() time.Duration {
	if value := os.Getenv("AGENTBOX_LIVE_TIMEOUT"); value != "" {
		if timeout, err := time.ParseDuration(value); err == nil {
			return timeout
		}
	}
	return 5 * time.Minute
}

func TestLiveSecretScan(t *testing.T) {
	if _, err := exec.LookPath("gitleaks"); err != nil {
		t.Skip("gitleaks is not installed")
	}
	store, err := app.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("gitleaks", "detect", "--no-git", "--source", store.Base, "--redact", "--exit-code", "1")
	cmd.Dir = ".."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gitleaks failed for %s: %v\n%s", store.Base, err, output)
	}
}

func TestLivePolicyAllowDeny(t *testing.T) {
	runID := os.Getenv("AGENTBOX_LIVE_RUN")
	if runID == "" {
		t.Skip("set AGENTBOX_LIVE_RUN to verify live policy allow/deny and revision loading")
	}
	var run app.Run
	if err := json.Unmarshal(runLive(t, "agent", "show", "--json", runID), &run); err != nil {
		t.Fatal(err)
	}
	allowed := "example.com"
	denied := "example.org"
	binary := "/usr/local/bin/node"
	if value := os.Getenv("AGENTBOX_LIVE_POLICY_BINARY"); value != "" {
		binary = value
	}
	nodeCheck := exec.Command("openshell", "--gateway", liveGateway(), "sandbox", "exec", "-n", run.Sandbox, "--no-login-shell", "--", "sh", "-c", "command -v "+binary)
	if output, err := nodeCheck.CombinedOutput(); err != nil {
		t.Skipf("policy network binary %s unavailable in sandbox: %v\n%s", binary, err, output)
	}
	before := livePolicyVersion(t, runID)
	runLive(t, "policy", "edit", "--allow-host", allowed, "--binary", binary, runID)
	after := livePolicyVersion(t, runID)
	if after <= before && !livePolicyContains(t, runID, allowed) {
		t.Fatalf("policy version did not advance after live network edit and policy lacks %s: before=%d after=%d", allowed, before, after)
	}
	allowedScript := fmt.Sprintf("fetch('https://%s').then(r=>process.exit(r.ok?0:1)).catch(e=>{console.error(e);process.exit(1)})", allowed)
	runSandboxNode(t, run, binary, allowedScript, true)
	deniedScript := fmt.Sprintf("fetch('https://%s').then(r=>process.exit(r.ok?0:1)).catch(e=>{console.error(e);process.exit(1)})", denied)
	runSandboxNode(t, run, binary, deniedScript, false)
}

func livePolicyContains(t *testing.T, runID, value string) bool {
	t.Helper()
	var policy map[string]json.RawMessage
	if err := json.Unmarshal(runLive(t, "policy", "show", "--json", runID), &policy); err != nil {
		t.Fatal(err)
	}
	return bytes.Contains(policy["effective"], []byte(value))
}

func livePolicyVersion(t *testing.T, runID string) int {
	t.Helper()
	var policy map[string]json.RawMessage
	if err := json.Unmarshal(runLive(t, "policy", "show", "--json", runID), &policy); err != nil {
		t.Fatal(err)
	}
	var effective struct {
		ActiveVersion int `json:"active_version"`
	}
	if err := json.Unmarshal(policy["effective"], &effective); err != nil {
		t.Fatal(err)
	}
	return effective.ActiveVersion
}

func runSandboxNode(t *testing.T, run app.Run, binary, script string, wantSuccess bool) {
	t.Helper()
	cmd := exec.Command("openshell", "--gateway", liveGateway(), "sandbox", "exec", "-n", run.Sandbox, "--no-login-shell", "--", binary, "-e", script)
	cmd.Dir = ".."
	output, err := cmd.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("expected sandbox request to succeed: %v\n%s", err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("expected sandbox request to be denied, but it succeeded:\n%s", output)
	}
}
