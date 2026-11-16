package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dmartinol/agentbox/internal/app"
	"github.com/dmartinol/agentbox/internal/skills"
)

type guiRunner struct {
	gitRoot         string
	images          string
	gateways        string
	phases          map[string]string
	finishedMarkers map[string]bool
	idleMarkers     map[string]bool
	probes          map[string]string
	search          string
	logs            string
	policySet       func() error
}

func (r guiRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	switch {
	case name == "git" && containsCommand(args, "rev-parse", "--show-toplevel"):
		return []byte(r.gitRoot), nil
	case name == "podman" && len(args) > 0 && args[0] == "images":
		return []byte(r.images), nil
	case name == "podman" && len(args) > 0 && args[0] == "search":
		return []byte(r.search), nil
	case name == "podman" && len(args) > 0 && args[0] == "info":
		return []byte("arm64"), nil
	case name == "openshell" && len(args) > 0 && args[0] == "gateway" && args[1] == "list":
		return []byte(r.gateways), nil
	case name == "openshell" && len(args) > 0 && args[0] == "status":
		return []byte(`{"gateway":"ready"}`), nil
	case name == "openshell" && containsCommand(args, "sandbox", "get"):
		for sandbox, phase := range r.phases {
			if len(args) > 2 && args[2] == sandbox {
				return []byte(fmt.Sprintf(`{"phase":%q}`, phase)), nil
			}
		}
		return nil, fmt.Errorf("sandbox not found")
	case name == "openshell" && containsCommand(args, "sandbox", "exec"):
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "tmux has-session -t agent") {
			for sandbox, probe := range r.probes {
				if strings.Contains(joined, sandbox) {
					return []byte(probe), nil
				}
			}
			return []byte("session=0\ndead=1\n"), nil
		}
		if strings.HasSuffix(joined, "test -f /workspace/finished") || strings.HasSuffix(joined, "test -f /workspace/idle") {
			marker := "finished"
			markers := r.finishedMarkers
			if strings.HasSuffix(joined, "test -f /workspace/idle") {
				marker = "idle"
				markers = r.idleMarkers
			}
			found := false
			for sandbox, present := range markers {
				if strings.Contains(joined, sandbox) {
					found = true
					if present {
						return []byte{}, nil
					}
					return nil, fmt.Errorf("%s marker not found", marker)
				}
			}
			if !found {
				return nil, fmt.Errorf("sandbox not found")
			}
		}
		if strings.Contains(joined, "tmux list-sessions") {
			return []byte("agent\nshell-1\n"), nil
		}
		if strings.Contains(joined, "tail -n") && strings.Contains(joined, "/workspace/state/agent.log") {
			return []byte(r.logs), nil
		}
		if strings.Contains(joined, "cat /workspace/state/agent.log") {
			return []byte(r.logs), nil
		}
		return []byte{}, nil
	case name == "openshell" && containsCommand(args, "policy", "set"):
		if r.policySet != nil {
			return nil, r.policySet()
		}
		return nil, nil
	default:
		return []byte(`{}`), nil
	}
}

func containsCommand(args []string, first, second string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == first && args[i+1] == second {
			return true
		}
	}
	return false
}

func (guiRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func testRunner() guiRunner {
	return guiRunner{
		images:   `[{"Names":["localhost/openshell:codex-0.167.0-r3"],"Digest":"sha256:image","Arch":"arm64","Os":"linux","Labels":{"agentbox":"true","agentbox.agent":"codex","agentbox.version":"0.167.0","agentbox.recipe":"3"}}]`,
		gateways: `[{"name":"openshell","active":true,"endpoint":"https://localhost:17670","type":"podman","auth":"bearer","source":"local"}]`,
		phases: map[string]string{
			"sandbox-active":   "running",
			"sandbox-idle":     "running",
			"sandbox-finished": "running",
		},
		finishedMarkers: map[string]bool{
			"sandbox-finished": true,
		},
		idleMarkers: map[string]bool{
			"sandbox-idle": true,
		},
		probes: map[string]string{
			"sandbox-active":   "session=1\ndead=0\ncommand=codex\nidle=0\nfinished=0\nactivity={\"state\":\"working\"}\n",
			"sandbox-idle":     "session=1\ndead=0\ncommand=sh\nidle=1\nfinished=0\n",
			"sandbox-finished": "session=0\ndead=1\ncommand=\nidle=0\nfinished=1\nexit=0\n",
		},
		search: `[{"Name":"quay.io/team/openshell","Tag":"codex-0.167.0-r3-linux-arm64"}]`,
		logs:   "agent log output\n",
	}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return newTestServerWithRunner(t, testRunner())
}

func newTestServerWithRunner(t *testing.T, runner guiRunner) *httptest.Server {
	t.Helper()
	server, _ := newTestServerAndStore(t, runner)
	return server
}

func newTestServerAndStore(t *testing.T, runner guiRunner) (*httptest.Server, app.Store) {
	t.Helper()
	base := t.TempDir()
	store := app.Store{Base: base}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProject(app.Project{Version: 1, Name: "demo", Root: "/repo", Worktree: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	policy, err := store.CreatePolicyConfiguration("default")
	if err != nil {
		t.Fatal(err)
	}
	profile := app.Profile{
		Version: 1, Name: "codex", Agent: "codex", Image: "localhost/openshell:codex-0.167.0-r3",
		Mode: "gateway", Model: "model-x", Provider: "model-gateway", Policy: policy.Path, Approval: "unattended", BaseURL: "https://models.example.test/v1", APIKeyEnv: "UNIT_MODEL_KEY",
	}
	if err := store.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	now := app.Run{
		ID: "run-active", Name: "active-run", Created: time.Now().UTC(), Source: "/repo", Project: "demo",
		Worktree: "/trees/demo/active-run", WorktreeIsolated: true, Branch: "agentbox/demo/active-run",
		Sandbox: "sandbox-active", Gateway: "openshell", Agent: "codex", Image: profile.Image, Mode: profile.Mode,
		Model: profile.Model, Approval: profile.Approval, Status: "working", Synced: true, Policy: policy.Path,
	}
	idle := now
	idle.ID, idle.Name, idle.Sandbox, idle.Status, idle.Synced = "run-idle", "idle-run", "sandbox-idle", "ready", false
	finished := now
	finished.ID, finished.Name, finished.Sandbox, finished.Status, finished.Synced = "run-finished", "finished-run", "sandbox-finished", "completed", true
	for _, run := range []app.Run{now, idle, finished} {
		if err := store.SaveRun(run); err != nil {
			t.Fatal(err)
		}
	}
	server, err := New(&app.App{Store: store, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return httpServer, store
}

func boolPtr(value bool) *bool { return &value }

func requestJSON(t *testing.T, method, url string, body any, target any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	if target != nil {
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			t.Fatalf("decode response from %s: %v", url, err)
		}
	}
	return response
}

func TestOverviewAggregatesFleetAndProjectTree(t *testing.T) {
	server := newTestServer(t)
	var response overviewResponse
	requestJSON(t, http.MethodGet, server.URL+"/api/overview", nil, &response)
	if response.Summary["active"] != 2 || response.Summary["finished"] != 1 || response.Summary["unsynced"] != 1 || response.Summary["waiting"] != 0 {
		t.Fatalf("summary = %#v", response.Summary)
	}
	if response.Summary["images"] != 1 || response.Summary["gateways"] != 1 || response.Summary["profiles"] != 1 || response.Summary["projects"] != 1 {
		t.Fatalf("resource summary = %#v", response.Summary)
	}
	if len(response.Tree) != 1 || response.Tree[0].Project != "demo" {
		t.Fatalf("tree = %#v", response.Tree)
	}
	if len(response.Tree[0].ActiveRuns) != 2 || len(response.Tree[0].IdleRuns) != 1 || len(response.Tree[0].FinishedRuns) != 1 {
		t.Fatalf("project run buckets = %#v", response.Tree[0])
	}
	if response.Tree[0].Isolation != "worktree" {
		t.Fatalf("isolation = %q", response.Tree[0].Isolation)
	}
}

func TestRunsListReconcilesStatuses(t *testing.T) {
	server := newTestServer(t)
	var runs []app.Run
	requestJSON(t, http.MethodGet, server.URL+"/api/runs", nil, &runs)
	statuses := map[string]string{}
	filled := 0
	for _, run := range runs {
		statuses[run.ID] = string(run.Status)
		if run.StatusDetail != "" {
			filled++
		}
	}
	if statuses["run-active"] != "working" || statuses["run-idle"] != "ready" || statuses["run-finished"] != "completed" {
		t.Fatalf("reconciled statuses = %#v", statuses)
	}
	if filled == 0 {
		t.Fatal("completed run should carry an exit-code detail")
	}
}

type attachRunner struct {
	commands [][]string
	resizes  <-chan app.TerminalSize
}

func (r *attachRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, append([]string{name}, args...))
	return []byte(`{}`), nil
}

func (r *attachRunner) StreamTerminal(_ context.Context, in io.Reader, out io.Writer, _ io.Writer, columns, rows int, resizes <-chan app.TerminalSize, name string, args ...string) error {
	r.commands = append(r.commands, append([]string{name}, args...), []string{"columns", strconv.Itoa(columns), "rows", strconv.Itoa(rows)})
	r.resizes = resizes
	buffer := make([]byte, 128)
	_, _ = in.Read(buffer)
	_, _ = io.WriteString(out, "attach terminal output")
	return nil
}

func (r *attachRunner) Stream(_ context.Context, in io.Reader, out io.Writer, _ io.Writer, name string, args ...string) error {
	r.commands = append(r.commands, append([]string{name}, args...))
	buffer := make([]byte, 128)
	_, _ = in.Read(buffer)
	_, _ = io.WriteString(out, "fallback stream output")
	return nil
}

func TestRunAttachWebSocket(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	run := app.Run{
		ID: "run-attach", Sandbox: "sandbox-attach", Gateway: "openshell", Agent: "codex",
		Status: "running", Source: "/repo", Worktree: "/trees/run-attach", Branch: "main",
		Baseline: "abc", LastSynced: "abc", Created: time.Now(), Synced: true, Policy: "/policy",
	}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := &attachRunner{}
	server, err := New(&app.App{Store: store, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	connection, _, err := websocket.DefaultDialer.Dial(strings.Replace(httpServer.URL, "http://", "ws://", 1)+"/api/runs/run-attach/attach?cols=100&rows=30", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	if err := connection.WriteMessage(websocket.TextMessage, []byte("terminal input")); err != nil {
		t.Fatal(err)
	}
	_, message, err := connection.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(message) != "attach terminal output" {
		t.Fatalf("terminal output = %q", message)
	}
	// The attach waits for the tmux session before streaming, so the last two
	// recorded commands are the attach itself and its initial size.
	if len(runner.commands) < 2 {
		t.Fatalf("attach commands = %#v", runner.commands)
	}
	attach := runner.commands[len(runner.commands)-2]
	size := runner.commands[len(runner.commands)-1]
	if !slices.Contains(size, "100") || !slices.Contains(size, "30") {
		t.Fatalf("terminal size = %#v", size)
	}
	for _, want := range []string{"sandbox", "exec", "-n", "sandbox-attach", "--tty", "tmux", "attach", "--gateway", "openshell"} {
		if !slices.Contains(attach, want) {
			t.Fatalf("attach command missing %q: %#v", want, attach)
		}
	}
}

func TestRunAttachWebSocketAppliesResizeMessages(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	run := app.Run{
		ID: "run-attach", Sandbox: "sandbox-attach", Gateway: "openshell", Agent: "codex",
		Status: "running", Source: "/repo", Worktree: "/trees/run-attach", Branch: "main",
		Baseline: "abc", LastSynced: "abc", Created: time.Now(), Synced: true, Policy: "/policy",
	}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := &attachRunner{}
	server, err := New(&app.App{Store: store, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	connection, _, err := websocket.DefaultDialer.Dial(strings.Replace(httpServer.URL, "http://", "ws://", 1)+"/api/runs/run-attach/attach?cols=100&rows=30", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	// A resize control message must not leak into the terminal input.
	if err := connection.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":80,"rows":24}`)); err != nil {
		t.Fatal(err)
	}
	if err := connection.WriteMessage(websocket.TextMessage, []byte("terminal input")); err != nil {
		t.Fatal(err)
	}
	_, message, err := connection.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(message) != "attach terminal output" {
		t.Fatalf("terminal output = %q", message)
	}
	select {
	case size := <-runner.resizes:
		if size.Columns != 80 || size.Rows != 24 {
			t.Fatalf("resize = %+v", size)
		}
	case <-time.After(time.Second):
		t.Fatal("resize message was not applied")
	}
}

func TestRunLogsEndpoint(t *testing.T) {
	server := newTestServer(t)
	var response struct {
		Logs string `json:"logs"`
	}
	requestJSON(t, http.MethodGet, server.URL+"/api/runs/run-active/logs", nil, &response)
	if response.Logs != "agent log output\n" {
		t.Fatalf("logs = %q", response.Logs)
	}
}

func TestRunLogsTailEndpoint(t *testing.T) {
	server := newTestServer(t)
	var response struct {
		Logs string `json:"logs"`
	}
	requestJSON(t, http.MethodGet, server.URL+"/api/runs/run-active/logs?tail=50", nil, &response)
	if response.Logs != "agent log output\n" {
		t.Fatalf("logs = %q", response.Logs)
	}
}

func TestRunShellEndpoints(t *testing.T) {
	server := newTestServer(t)

	var shells []string
	requestJSON(t, http.MethodGet, server.URL+"/api/runs/run-active/shells", nil, &shells)
	if !slices.Equal(shells, []string{"shell-1"}) {
		t.Fatalf("shells = %#v", shells)
	}

	var created struct {
		Name string `json:"name"`
	}
	requestJSON(t, http.MethodPost, server.URL+"/api/runs/run-active/shells", nil, &created)
	if created.Name != "shell-2" {
		t.Fatalf("created shell = %q, want shell-2", created.Name)
	}

	var deleted struct {
		Deleted bool `json:"deleted"`
	}
	requestJSON(t, http.MethodPost, server.URL+"/api/runs/run-active/shells/shell-1/delete", nil, &deleted)
	if !deleted.Deleted {
		t.Fatal("shell was not deleted")
	}
}

func TestProjectsEndpoint(t *testing.T) {
	server := newTestServer(t)
	var projects []app.Project
	requestJSON(t, http.MethodGet, server.URL+"/api/projects", nil, &projects)
	if len(projects) != 1 || projects[0].Name != "demo" {
		t.Fatalf("projects = %#v", projects)
	}
}

func TestProfilesEndpointCreatesDefaultPolicyReference(t *testing.T) {
	server := newTestServer(t)
	profile := app.Profile{
		Version: 1, Name: "claude", Agent: "claude", Image: "image", Mode: "standard",
		Provider: "provider", Approval: "prompted",
	}
	var saved app.Profile
	requestJSON(t, http.MethodPost, server.URL+"/api/profiles", profile, &saved)
	if !strings.Contains(saved.Policy, "/policies/claude.yaml") {
		t.Fatalf("default policy = %q", saved.Policy)
	}
	var profiles []app.Profile
	requestJSON(t, http.MethodGet, server.URL+"/api/profiles", nil, &profiles)
	if len(profiles) != 2 {
		t.Fatalf("profiles = %#v", profiles)
	}
}

func TestProfilesEndpointRepairsLegacyPolicyReference(t *testing.T) {
	server := newTestServer(t)
	profile := app.Profile{
		Version: 1, Name: "codex-enmaas", Agent: "codex", Image: "image", Mode: "gateway",
		Model: "model", Provider: "provider", Approval: "unattended",
		BaseURL: "https://models.example.test/v1", APIKeyEnv: "UNIT_MODEL_KEY",
		Policy: "~/.agentbox/policies/codex-enmaas.yaml",
	}
	var saved app.Profile
	requestJSON(t, http.MethodPost, server.URL+"/api/profiles", profile, &saved)
	if saved.Policy == profile.Policy || !strings.HasSuffix(saved.Policy, "/policies/codex-enmaas.yaml") {
		t.Fatalf("policy = %q", saved.Policy)
	}
	if _, err := os.Stat(saved.Policy); err != nil {
		t.Fatalf("policy file: %v", err)
	}
}

func TestProfilesEndpointMigratesLegacyPolicyWithoutManualEdit(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	profile := app.Profile{
		Version: 1, Name: "codex-enmaas", Agent: "codex", Image: "image", Mode: "gateway",
		Model: "model", Provider: "provider", Approval: "unattended",
		BaseURL: "https://models.example.test/v1", APIKeyEnv: "UNIT_MODEL_KEY",
		Policy: "~/.agentbox/policies/codex-enmaas.yaml",
	}
	if err := store.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	server, err := New(&app.App{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	var profiles []app.Profile
	requestJSON(t, http.MethodGet, httpServer.URL+"/api/profiles", nil, &profiles)
	if len(profiles) != 1 || profiles[0].Policy == profile.Policy || !strings.HasSuffix(profiles[0].Policy, "/policies/codex-enmaas.yaml") {
		t.Fatalf("profiles = %#v", profiles)
	}
	if _, err := os.Stat(profiles[0].Policy); err != nil {
		t.Fatalf("policy file: %v", err)
	}
	reloaded, err := store.LoadProfile(profile.Name)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Policy != profiles[0].Policy {
		t.Fatalf("stored policy = %q", reloaded.Policy)
	}
}

func TestPolicyContentEndpoints(t *testing.T) {
	server := newTestServer(t)
	var created app.PolicyConfiguration
	requestJSON(t, http.MethodPost, server.URL+"/api/policies", map[string]string{"name": "edited"}, &created)
	if created.Name != "edited" {
		t.Fatalf("created policy = %#v", created)
	}
	var content struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	requestJSON(t, http.MethodGet, server.URL+"/api/policies/edited", nil, &content)
	if !strings.Contains(content.Content, "hard_requirement") {
		t.Fatalf("policy content = %q", content.Content)
	}
	content.Content = strings.Replace(content.Content, "network_policies: {}", "network_policies: {}", 1)
	var saved app.PolicyConfiguration
	requestJSON(t, http.MethodPost, server.URL+"/api/policies/edited", content, &saved)
	if saved.Name != "edited" {
		t.Fatalf("saved policy = %#v", saved)
	}
}

func TestSettingsEndpoints(t *testing.T) {
	server := newTestServer(t)
	var before app.Settings
	requestJSON(t, http.MethodGet, server.URL+"/api/settings", nil, &before)
	root := filepath.Join(t.TempDir(), "worktrees")
	var after app.Settings
	requestJSON(t, http.MethodPost, server.URL+"/api/settings", app.Settings{
		Version:   1,
		Worktrees: app.WorktreeSettings{Root: root, BranchPrefix: "agentbox"},
	}, &after)
	if after.Worktrees.Root != root || after.Worktrees.BranchPrefix != "agentbox" {
		t.Fatalf("saved settings = %#v", after)
	}
}

func TestRegistryEndpointsDiscoverArtifacts(t *testing.T) {
	server := newTestServer(t)
	var registry app.Registry
	requestJSON(t, http.MethodPost, server.URL+"/api/registry", map[string]string{"reference": "quay.io/team/openshell"}, &registry)
	if registry.Reference != "quay.io/team/openshell" {
		t.Fatalf("registry = %#v", registry)
	}
	var images []app.ImageArtifact
	requestJSON(t, http.MethodPost, server.URL+"/api/registry/discover", nil, &images)
	if len(images) != 1 || images[0].Agent != "codex" || images[0].Version != "0.167.0" {
		t.Fatalf("discovered images = %#v", images)
	}
}

func TestGatewayEndpointParsesOpenShellJSON(t *testing.T) {
	server := newTestServer(t)
	var gateways []gatewayRecord
	requestJSON(t, http.MethodGet, server.URL+"/api/gateways", nil, &gateways)
	if len(gateways) != 1 || !gateways[0].Active || gateways[0].Type != "podman" {
		t.Fatalf("gateways = %#v", gateways)
	}
}

func TestDoctorEndpointReturnsChecks(t *testing.T) {
	server := newTestServer(t)
	var checks []app.Check
	requestJSON(t, http.MethodGet, server.URL+"/api/doctor", nil, &checks)
	if len(checks) < 5 {
		t.Fatalf("checks = %#v", checks)
	}
}

func TestSPAFallbackServesEmbeddedApplication(t *testing.T) {
	server := newTestServer(t)
	response, err := http.Get(server.URL + "/runs")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if !strings.Contains(string(body), "<div id=\"root\">") {
		t.Fatalf("SPA fallback body = %s", body)
	}
}

func TestInvalidJSONReturnsBadRequest(t *testing.T) {
	server := newTestServer(t)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/projects", strings.NewReader("{"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	commands := [][]string{
		{"init", "-q"},
		{"config", "user.name", "AgentBox Test"},
		{"config", "user.email", "test@example.invalid"},
	}
	for _, args := range commands {
		command := exec.Command("git", args...)
		command.Dir = repo
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-qm", "base"}} {
		command := exec.Command("git", args...)
		command.Dir = repo
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	return repo
}

func TestProjectCreateEndpointRegistersGitRoot(t *testing.T) {
	repo := initGitRepo(t)
	runner := testRunner()
	runner.gitRoot = repo
	server := newTestServerWithRunner(t, runner)
	var project app.Project
	response := requestJSON(t, http.MethodPost, server.URL+"/api/projects", map[string]any{
		"name": "registered", "root": repo, "worktree": false,
	}, &project)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	canonical, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != "registered" || project.Root != canonical || project.UseWorktree() {
		t.Fatalf("project = %#v", project)
	}
	var projects []app.Project
	requestJSON(t, http.MethodGet, server.URL+"/api/projects", nil, &projects)
	if len(projects) != 2 {
		t.Fatalf("projects = %#v", projects)
	}
}

func TestImagesEndpointReturnsLocalArtifacts(t *testing.T) {
	server := newTestServer(t)
	var images []app.ImageArtifact
	requestJSON(t, http.MethodGet, server.URL+"/api/images", nil, &images)
	if len(images) != 1 || images[0].Agent != "codex" || images[0].Recipe != 3 {
		t.Fatalf("images = %#v", images)
	}
}

func TestGetRunEndpoint(t *testing.T) {
	server := newTestServer(t)
	var run struct {
		app.Run
		AgentBinary string `json:"agent_binary"`
	}
	requestJSON(t, http.MethodGet, server.URL+"/api/runs/run-active", nil, &run)
	if run.ID != "run-active" || run.Project != "demo" {
		t.Fatalf("run = %#v", run)
	}
	if run.AgentBinary != "/usr/local/bin/codex" {
		t.Fatalf("agent_binary = %q", run.AgentBinary)
	}
}

func postRunHost(t *testing.T, url, body string) (*http.Response, []byte) {
	t.Helper()
	response, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, content
}

func TestWriteRunHostResultMapsExactSuccessBodies(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeRunHostResult(recorder, app.PolicyEditResult{Outcome: app.PolicyEditLiveApplied, Diff: "computed diff"}, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("live status = %d", recorder.Code)
	}
	if got, want := recorder.Body.String(), "{\"outcome\":\"live-applied\",\"diff\":\"computed diff\"}\n"; got != want {
		t.Fatalf("live body = %q; want %q", got, want)
	}

	recorder = httptest.NewRecorder()
	writeRunHostResult(recorder, app.PolicyEditResult{Outcome: app.PolicyEditStaticPending, Diff: "computed diff"}, nil)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("static status = %d", recorder.Code)
	}
	if got, want := recorder.Body.String(), "{\"outcome\":\"static-pending\",\"diff\":\"computed diff\"}\n"; got != want {
		t.Fatalf("static body = %q; want %q", got, want)
	}
}

func TestRunHostEndpointAppliesLivePolicy(t *testing.T) {
	server := newTestServer(t)
	response, body := postRunHost(t, server.URL+"/api/runs/run-active/policy/host", `{"host":"api.example.com"}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 2 || payload["outcome"] != "live-applied" {
		t.Fatalf("body = %s", body)
	}
	diff, _ := payload["diff"].(string)
	if !strings.Contains(diff, "host-api-example-com") {
		t.Fatalf("diff = %q", diff)
	}
}

func TestRunHostEndpointRejectsTerminalRunWithoutMutation(t *testing.T) {
	calls := 0
	runner := testRunner()
	runner.policySet = func() error {
		calls++
		return nil
	}
	server, store := newTestServerAndStore(t, runner)
	run, err := store.LoadRun("run-finished")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(run.Policy)
	if err != nil {
		t.Fatal(err)
	}
	response, body := postRunHost(t, server.URL+"/api/runs/run-finished/policy/host", `{"host":"api.example.com"}`)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := "{\"outcome\":\"no-live-sandbox\",\"error\":\"Allow host is available only for Ready, Working, and Waiting agents.\"}\n"
	if string(body) != want {
		t.Fatalf("body = %q; want %q", body, want)
	}
	after, err := os.ReadFile(run.Policy)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("policy mutated: %v", err)
	}
	if calls != 0 {
		t.Fatalf("policy set calls = %d", calls)
	}
}

func TestRunHostEndpointReportsOpenShellRejectionWithRollback(t *testing.T) {
	runner := testRunner()
	runner.policySet = func() error { return errors.New("policy rejected") }
	server, store := newTestServerAndStore(t, runner)
	run, err := store.LoadRun("run-active")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(run.Policy)
	if err != nil {
		t.Fatal(err)
	}
	response, body := postRunHost(t, server.URL+"/api/runs/run-active/policy/host", `{"host":"api.example.com"}`)
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	var payload struct {
		Outcome    string `json:"outcome"`
		Error      string `json:"error"`
		Diff       string `json:"diff"`
		RolledBack *bool  `json:"rolledBack"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Outcome != "openshell-rejected" || payload.RolledBack == nil || !*payload.RolledBack {
		t.Fatalf("body = %s", body)
	}
	wantErr := "OpenShell rejected the policy update; the local policy was restored: policy rejected"
	if payload.Error != wantErr || !strings.Contains(payload.Diff, "host-api-example-com") {
		t.Fatalf("payload = %+v", payload)
	}
	after, err := os.ReadFile(run.Policy)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rollback did not restore policy: %v", err)
	}
}

func TestRunHostEndpointReportsRollbackFailure(t *testing.T) {
	var policyPath string
	runner := testRunner()
	runner.policySet = func() error {
		if err := os.Remove(policyPath); err != nil {
			return err
		}
		if err := os.Mkdir(policyPath, 0700); err != nil {
			return err
		}
		return errors.New("policy rejected")
	}
	server, store := newTestServerAndStore(t, runner)
	run, err := store.LoadRun("run-active")
	if err != nil {
		t.Fatal(err)
	}
	policyPath = run.Policy
	response, body := postRunHost(t, server.URL+"/api/runs/run-active/policy/host", `{"host":"api.example.com"}`)
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	var payload struct {
		Error      string `json:"error"`
		RolledBack *bool  `json:"rolledBack"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.RolledBack == nil || *payload.RolledBack {
		t.Fatalf("rolledBack = %v", payload.RolledBack)
	}
	if !strings.Contains(payload.Error, "local policy rollback failed: policy rejected") {
		t.Fatalf("error = %q", payload.Error)
	}
}

func TestInvalidPolicyContentIsRejected(t *testing.T) {
	server := newTestServer(t)
	var created app.PolicyConfiguration
	requestJSON(t, http.MethodPost, server.URL+"/api/policies", map[string]string{"name": "bad"}, &created)
	response := requestJSON(t, http.MethodPost, server.URL+"/api/policies/bad", map[string]string{"content": "version: 2"}, nil)
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestPolicyHostEndpointAddsNetworkRule(t *testing.T) {
	server := newTestServer(t)
	var policy app.PolicyConfiguration
	response := requestJSON(t, http.MethodPost, server.URL+"/api/policies/default/host", map[string]string{
		"agent": "codex", "host": "example.com",
	}, &policy)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	content, err := os.ReadFile(policy.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "example.com") {
		t.Fatalf("updated policy = %s", content)
	}
}

func TestEmptyCollectionEndpointsReturnArrays(t *testing.T) {
	base := t.TempDir()
	store := app.Store{Base: base}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	runner := testRunner()
	runner.images = "[]"
	server, err := New(&app.App{Store: store, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	for _, path := range []string{"/api/runs", "/api/projects", "/api/profiles", "/api/policies", "/api/images"} {
		response, err := http.Get(httpServer.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusOK || string(body) != "[]\n" {
			t.Fatalf("%s status=%d body=%q", path, response.StatusCode, body)
		}
	}
}

type buildTaskRunner struct{}

func (buildTaskRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	if name == "podman" && len(args) > 1 && args[0] == "image" && args[1] == "inspect" {
		if args[2] == "docker.io/library/node:22-bookworm-slim" {
			return []byte("sha256:base-digest"), nil
		}
		return []byte("sha256:image-id sha256:image-digest"), nil
	}
	return []byte(`{}`), nil
}

func (buildTaskRunner) Stream(_ context.Context, _ io.Reader, out, _ io.Writer, _ string, _ ...string) error {
	_, _ = out.Write([]byte("simulated build output\n"))
	return nil
}

func TestImageBuildEndpointStartsCapturedTask(t *testing.T) {
	server, err := New(&app.App{Store: app.Store{Base: t.TempDir()}, Runner: buildTaskRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	var task Task
	response := requestJSON(t, http.MethodPost, httpServer.URL+"/api/images/build", map[string]string{
		"agent": "codex", "version": "0.167.0",
	}, &task)
	if response.StatusCode != http.StatusAccepted || task.ID == "" || task.Status != "running" {
		t.Fatalf("response=%d task=%#v", response.StatusCode, task)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		requestJSON(t, http.MethodGet, httpServer.URL+"/api/tasks/"+task.ID, nil, &task)
		if task.Status == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("build task did not finish: %#v", task)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(task.Output, "simulated build output") || !strings.Contains(task.Output, "localhost/openshell:codex-0.167.0-r4") {
		t.Fatalf("build task output = %q", task.Output)
	}
}

func TestTaskManagerStopCancelsRunningWork(t *testing.T) {
	manager := newTaskManager()
	task, err := manager.start("test", func(ctx context.Context, output io.Writer) error {
		_, _ = output.Write([]byte("working\n"))
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for task.Output == "" {
		task, _ = manager.get(task.ID)
		if time.Now().After(deadline) {
			t.Fatal("task output was not captured")
		}
		time.Sleep(time.Millisecond)
	}
	stopped, ok := manager.stop(task.ID)
	if !ok || !stopped.StopRequested {
		t.Fatalf("stopped task = %#v, %v", stopped, ok)
	}
	deadline = time.Now().Add(time.Second)
	for stopped.Status != "canceled" {
		stopped, _ = manager.get(task.ID)
		if time.Now().After(deadline) {
			t.Fatalf("task did not cancel: %#v", stopped)
		}
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(stopped.Output, "working") {
		t.Fatalf("stopped output = %q", stopped.Output)
	}
}

func TestProviderSetupEndpointUsesGatewayHostAndCredentialName(t *testing.T) {
	t.Setenv("UNIT_MODEL_KEY", "unit-secret")
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	server, err := New(&app.App{Store: store, Runner: testRunner()})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	response := requestJSON(t, http.MethodPost, httpServer.URL+"/api/providers", map[string]string{
		"kind": "gateway-bearer", "name": "unit-provider", "host": "models.example.test",
		"credential": "UNIT_MODEL_KEY", "gateway": "openshell",
	}, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	providerPath := filepath.Join(store.ProfilesDir(), "agentbox-gateway-bearer-models-example-test-provider.yaml")
	provider, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(provider), "UNIT_MODEL_KEY") || strings.Contains(string(provider), "unit-secret") {
		t.Fatalf("provider credential reference = %s", provider)
	}
}

func TestProviderSetupEndpointReturnsActionableError(t *testing.T) {
	t.Setenv("UNIT_MODEL_KEY", "")
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	server, err := New(&app.App{Store: store, Runner: testRunner()})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	var response struct {
		Error string `json:"error"`
	}
	httpResponse := requestJSON(t, http.MethodPost, httpServer.URL+"/api/providers", map[string]string{
		"kind": "gateway-bearer", "name": "unit-provider", "host": "models.example.test",
		"credential": "UNIT_MODEL_KEY", "gateway": "openshell",
	}, &response)
	if httpResponse.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d", httpResponse.StatusCode)
	}
	if !strings.Contains(response.Error, "provider setup failed: UNIT_MODEL_KEY environment reference is missing") {
		t.Fatalf("error = %q", response.Error)
	}
}

func TestProvidersEndpointParsesOpenShellJSON(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	providerProfile := `id: agentbox-gateway-bearer-models-example-com
credentials:
  - env_vars: [MY_GATEWAY_KEY]
endpoints:
  - host: models.example.com
    port: 443
    path: /v1/**
`
	if err := os.WriteFile(filepath.Join(store.ProfilesDir(), "agentbox-gateway-bearer-models-example-com-provider.yaml"), []byte(providerProfile), 0600); err != nil {
		t.Fatal(err)
	}
	server, err := New(&app.App{Store: store, Runner: providerListRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	var providers []providerRecord
	requestJSON(t, http.MethodGet, httpServer.URL+"/api/providers?gateway=openshell", nil, &providers)
	var selected providerRecord
	for _, provider := range providers {
		if provider.Name == "model-gateway" {
			selected = provider
		}
	}
	if selected.Name != "model-gateway" || selected.Type != "gateway-bearer" || selected.Credential != "MY_GATEWAY_KEY" || selected.BaseURL != "https://models.example.com/v1" || selected.Gateway != "openshell" {
		t.Fatalf("providers = %#v", providers)
	}
}

type providerListRunner struct{}

func (providerListRunner) Output(context.Context, string, ...string) ([]byte, error) {
	return []byte(`{"providers":[{"name":"model-gateway","type":"gateway-bearer","profile":"agentbox-gateway-bearer-models-example-com","credential_keys":["MY_GATEWAY_KEY"]}]}`), nil
}

func (providerListRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func TestClarifyProviderSetupErrorExplainsUnreachableGateway(t *testing.T) {
	original := errors.New("openshell: exit status 1: transport error: tcp connect error: Connection refused (os error 61)")
	clarified := clarifyProviderSetupError(original, "openshell")
	for _, want := range []string{
		`OpenShell gateway "openshell" is unreachable`,
		"podman machine start",
		"brew services restart openshell",
		"Doctor page",
		"Connection refused",
	} {
		if !strings.Contains(clarified.Error(), want) {
			t.Fatalf("clarified error missing %q: %v", want, clarified)
		}
	}
}

type providerCommandRunner struct {
	commands [][]string
}

func (r *providerCommandRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, append([]string{name}, args...))
	if name == "openshell" && len(args) > 1 && args[0] == "provider" && args[1] == "list" {
		return []byte(`{"providers":[{"name":"model-gateway","type":"gateway-bearer","credential":"OLD_KEY"}]}`), nil
	}
	return []byte(`{}`), nil
}

func (providerCommandRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

type modelProviderRunner struct {
	baseURL string
}

func (r modelProviderRunner) Output(context.Context, string, ...string) ([]byte, error) {
	return []byte(`{"providers":[{"name":"model-gateway","type":"gateway-bearer","credential_keys":["UNIT_MODEL_KEY"],"base_url":"` + r.baseURL + `"}]}`), nil
}

func (modelProviderRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func TestProviderModelsEndpoint(t *testing.T) {
	t.Setenv("UNIT_MODEL_KEY", "test-key")
	models := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("model discovery path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization header = %q", got)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"zeta"},{"id":"alpha"}]}`))
	}))
	t.Cleanup(models.Close)

	server, err := New(&app.App{Store: app.Store{Base: t.TempDir()}, Runner: modelProviderRunner{baseURL: models.URL}})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	var ids []string
	requestJSON(t, http.MethodGet, httpServer.URL+"/api/providers/model-gateway/models?gateway=openshell", nil, &ids)
	if len(ids) != 2 || ids[0] != "alpha" || ids[1] != "zeta" {
		t.Fatalf("models = %#v", ids)
	}
}

func TestProviderListIncludesProfileUsage(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(app.Profile{
		Version: 1, Name: "codex-gateway", Agent: "codex", Image: "image", Mode: "gateway",
		Model: "model", Provider: "model-gateway", Policy: "/tmp/policy.yaml", Approval: "prompted",
		BaseURL: "https://models.example.test/v1", APIKeyEnv: "UNIT_MODEL_KEY",
	}); err != nil {
		t.Fatal(err)
	}
	server, err := New(&app.App{Store: store, Runner: providerListRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	var providers []providerRecord
	requestJSON(t, http.MethodGet, httpServer.URL+"/api/providers?gateway=openshell", nil, &providers)
	var gatewayProvider *providerRecord
	for i := range providers {
		if providers[i].Name == "model-gateway" {
			gatewayProvider = &providers[i]
		}
	}
	if gatewayProvider == nil || len(gatewayProvider.UsedBy) != 1 || gatewayProvider.UsedBy[0] != "codex-gateway" {
		t.Fatalf("providers = %#v", providers)
	}
}

func TestProviderListIncludesBuiltinProviders(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	server, err := New(&app.App{Store: store, Runner: providerListRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	var providers []providerRecord
	requestJSON(t, http.MethodGet, httpServer.URL+"/api/providers?gateway=openshell", nil, &providers)
	byName := map[string]providerRecord{}
	for _, provider := range providers {
		byName[provider.Name] = provider
	}
	for _, name := range []string{"openai", "anthropic"} {
		provider, ok := byName[name]
		if !ok || !provider.Builtin || !provider.Registered || provider.Gateway != "openshell" {
			t.Fatalf("built-in %q = %#v (all providers: %#v)", name, provider, providers)
		}
	}
}

func TestProviderUpdateCredentialEndpoint(t *testing.T) {
	runner := &providerCommandRunner{}
	server, err := New(&app.App{Store: app.Store{Base: t.TempDir()}, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	response := requestJSON(t, http.MethodPost, httpServer.URL+"/api/providers/model-gateway/update", map[string]string{
		"credential": "NEW_MODEL_KEY", "gateway": "openshell",
	}, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %#v", runner.commands)
	}
	command := runner.commands[0]
	for _, want := range []string{"provider", "update", "model-gateway", "--credential", "NEW_MODEL_KEY", "--gateway", "openshell"} {
		if !slices.Contains(command, want) {
			t.Fatalf("update command missing %q: %#v", want, command)
		}
	}
}

func TestProviderDeleteBlockedWhenUsedByProfile(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(app.Profile{
		Version: 1, Name: "codex-gateway", Agent: "codex", Image: "image", Mode: "gateway",
		Model: "model", Provider: "model-gateway", Policy: "/tmp/policy.yaml", Approval: "prompted",
		BaseURL: "https://models.example.test/v1", APIKeyEnv: "UNIT_MODEL_KEY",
	}); err != nil {
		t.Fatal(err)
	}
	runner := &providerCommandRunner{}
	server, err := New(&app.App{Store: store, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	var response struct {
		Error string `json:"error"`
	}
	httpResponse := requestJSON(t, http.MethodPost, httpServer.URL+"/api/providers/model-gateway/delete?gateway=openshell", nil, &response)
	if httpResponse.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d", httpResponse.StatusCode)
	}
	if !strings.Contains(response.Error, "used by profiles: codex-gateway") {
		t.Fatalf("error = %q", response.Error)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("delete command unexpectedly issued: %#v", runner.commands)
	}
}

func TestProviderDeleteEndpoint(t *testing.T) {
	runner := &providerCommandRunner{}
	server, err := New(&app.App{Store: app.Store{Base: t.TempDir()}, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	response := requestJSON(t, http.MethodPost, httpServer.URL+"/api/providers/model-gateway/delete?gateway=openshell", nil, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %#v", runner.commands)
	}
	command := runner.commands[0]
	for _, want := range []string{"provider", "delete", "model-gateway", "--gateway", "openshell"} {
		if !slices.Contains(command, want) {
			t.Fatalf("delete command missing %q: %#v", want, command)
		}
	}
}

func TestBuiltinProviderDeleteIsRejected(t *testing.T) {
	runner := &providerCommandRunner{}
	server, err := New(&app.App{Store: app.Store{Base: t.TempDir()}, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	var response struct {
		Error string `json:"error"`
	}
	httpResponse := requestJSON(t, http.MethodPost, httpServer.URL+"/api/providers/openai/delete?gateway=openshell", nil, &response)
	if httpResponse.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d", httpResponse.StatusCode)
	}
	if !strings.Contains(response.Error, "built in") {
		t.Fatalf("error = %q", response.Error)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("delete command unexpectedly issued: %#v", runner.commands)
	}
}

func TestMCPEndpointsManageEntriesAndDiscovery(t *testing.T) {
	server := newTestServer(t)
	t.Setenv("CODEX_HOME", t.TempDir())

	var names []string
	response := requestJSON(t, http.MethodGet, server.URL+"/api/mcps/servers?agent=codex", nil, &names)
	if response.StatusCode != http.StatusOK || len(names) != 0 {
		t.Fatalf("empty discovery: status=%d names=%v", response.StatusCode, names)
	}

	var apiError struct {
		Error string `json:"error"`
	}
	response = requestJSON(t, http.MethodGet, server.URL+"/api/mcps/servers?agent=", nil, &apiError)
	if response.StatusCode != http.StatusInternalServerError || apiError.Error == "" {
		t.Fatalf("missing agent parameter: status=%d error=%q", response.StatusCode, apiError.Error)
	}

	var entry app.MCPTools
	response = requestJSON(t, http.MethodPost, server.URL+"/api/mcps", map[string]any{"name": "unit-set", "agent": "codex", "servers": []string{"alpha"}}, &entry)
	if response.StatusCode != http.StatusCreated || entry.Name != "unit-set" || entry.Agent != "codex" || len(entry.Servers) != 1 {
		t.Fatalf("create: status=%d entry=%+v", response.StatusCode, entry)
	}

	var invalid struct {
		Error string `json:"error"`
	}
	response = requestJSON(t, http.MethodPost, server.URL+"/api/mcps", map[string]any{"name": "bad", "agent": "codex"}, &invalid)
	if response.StatusCode != http.StatusInternalServerError || invalid.Error == "" {
		t.Fatalf("invalid create: status=%d error=%q", response.StatusCode, invalid.Error)
	}

	var entries []app.MCPTools
	response = requestJSON(t, http.MethodGet, server.URL+"/api/mcps", nil, &entries)
	if response.StatusCode != http.StatusOK || len(entries) != 1 || entries[0].Name != "unit-set" {
		t.Fatalf("list: status=%d entries=%+v", response.StatusCode, entries)
	}

	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	if err := os.MkdirAll(codexHome, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte("[mcp_servers.alpha]\ncommand = \"/usr/bin/true\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	response = requestJSON(t, http.MethodGet, server.URL+"/api/mcps/servers?agent=codex", nil, &names)
	if response.StatusCode != http.StatusOK || len(names) != 1 || names[0] != "alpha" {
		t.Fatalf("discovery: status=%d names=%v", response.StatusCode, names)
	}

	var deleted struct {
		Deleted bool `json:"deleted"`
	}
	response = requestJSON(t, http.MethodPost, server.URL+"/api/mcps/unit-set/delete", nil, &deleted)
	if response.StatusCode != http.StatusOK || !deleted.Deleted {
		t.Fatalf("delete: status=%d body=%+v", response.StatusCode, deleted)
	}
	response = requestJSON(t, http.MethodGet, server.URL+"/api/mcps", nil, &entries)
	if response.StatusCode != http.StatusOK || len(entries) != 0 {
		t.Fatalf("list after delete: status=%d entries=%+v", response.StatusCode, entries)
	}
}

func TestSkillsEndpointsManageEntriesAndDiscovery(t *testing.T) {
	server := newTestServer(t)
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	skillDir := filepath.Join(codexHome, "skills", "orca-cli")
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Orca command line\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var apiError struct {
		Error string `json:"error"`
	}
	response := requestJSON(t, http.MethodGet, server.URL+"/api/skills/available?agent=", nil, &apiError)
	if response.StatusCode != http.StatusInternalServerError || apiError.Error == "" {
		t.Fatalf("missing agent parameter: status=%d error=%q", response.StatusCode, apiError.Error)
	}

	var available app.AvailableSkills
	response = requestJSON(t, http.MethodGet, server.URL+"/api/skills/available?agent=codex", nil, &available)
	if response.StatusCode != http.StatusOK || len(available.Skills) != 1 || available.Skills[0].Name != "orca-cli" || available.Skills[0].Description != "Orca command line" {
		t.Fatalf("discovery: status=%d available=%+v", response.StatusCode, available)
	}

	var entry app.Skills
	response = requestJSON(t, http.MethodPost, server.URL+"/api/skills", map[string]any{"name": "unit-skills", "agent": "codex", "skills": []string{"orca-cli"}, "plugins": []string{"browser@openai-bundled"}}, &entry)
	if response.StatusCode != http.StatusCreated || entry.Name != "unit-skills" || len(entry.Skills) != 1 || len(entry.Plugins) != 1 {
		t.Fatalf("create: status=%d entry=%+v", response.StatusCode, entry)
	}

	response = requestJSON(t, http.MethodPost, server.URL+"/api/skills", map[string]any{"name": "bad", "agent": "codex"}, &apiError)
	if response.StatusCode != http.StatusInternalServerError || apiError.Error == "" {
		t.Fatalf("invalid create: status=%d error=%q", response.StatusCode, apiError.Error)
	}

	var entries []app.Skills
	response = requestJSON(t, http.MethodGet, server.URL+"/api/skills", nil, &entries)
	if response.StatusCode != http.StatusOK || len(entries) != 1 || entries[0].Name != "unit-skills" {
		t.Fatalf("list: status=%d entries=%+v", response.StatusCode, entries)
	}

	var deleted struct {
		Deleted bool `json:"deleted"`
	}
	response = requestJSON(t, http.MethodPost, server.URL+"/api/skills/unit-skills/delete", nil, &deleted)
	if response.StatusCode != http.StatusOK || !deleted.Deleted {
		t.Fatalf("delete: status=%d body=%+v", response.StatusCode, deleted)
	}
	response = requestJSON(t, http.MethodGet, server.URL+"/api/skills", nil, &entries)
	if response.StatusCode != http.StatusOK || len(entries) != 0 {
		t.Fatalf("list after delete: status=%d entries=%+v", response.StatusCode, entries)
	}

	t.Setenv("HOME", t.TempDir())
	var status []skills.Status
	response = requestJSON(t, http.MethodGet, server.URL+"/api/skills/status", nil, &status)
	if response.StatusCode != http.StatusOK || len(status) != len(skills.Names())+len(skills.RoleNames()) {
		t.Fatalf("status: code=%d status=%+v", response.StatusCode, status)
	}
	roles := 0
	for _, entry := range status {
		if entry.Kind == "role" {
			roles++
		}
	}
	if roles != len(skills.RoleNames()) {
		t.Fatalf("role statuses = %d", roles)
	}
}

func TestSkillsEndpointsEmitArraysForMissingSelections(t *testing.T) {
	server := newTestServer(t)
	var created app.Skills
	response := requestJSON(t, http.MethodPost, server.URL+"/api/skills", map[string]any{
		"name": "plugin-only", "agent": "codex", "plugins": []string{"browser@openai-bundled"},
	}, &created)
	if response.StatusCode != http.StatusCreated || created.Skills == nil || created.Plugins == nil {
		t.Fatalf("plugin-only create: status=%d entry=%+v", response.StatusCode, created)
	}

	var entries []app.Skills
	response = requestJSON(t, http.MethodGet, server.URL+"/api/skills", nil, &entries)
	if response.StatusCode != http.StatusOK || len(entries) != 1 {
		t.Fatalf("list: status=%d entries=%+v", response.StatusCode, entries)
	}
	if entries[0].Skills == nil || entries[0].Plugins == nil {
		t.Fatalf("list emitted nil selection: %+v", entries[0])
	}

	response = requestJSON(t, http.MethodPost, server.URL+"/api/skills", map[string]any{
		"name": "skill-only", "agent": "codex", "skills": []string{"orca-cli"},
	}, &created)
	if response.StatusCode != http.StatusCreated || created.Skills == nil || created.Plugins == nil {
		t.Fatalf("skill-only create: status=%d entry=%+v", response.StatusCode, created)
	}
}

func TestImageLifecycleEndpointsGuardHolders(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(app.Profile{
		Version: 1, Name: "codex-default", Agent: "codex", Image: "quay.io/team/openshell:codex-0.167.0-r4",
		Mode: "standard", Provider: "openai", Policy: "/tmp/unit-policy.yaml", Approval: "prompted",
	}); err != nil {
		t.Fatal(err)
	}
	runner := testRunner()
	runner.images = `[
			{"Id":"image-held","Names":["localhost/openshell:codex-0.167.0-r4","quay.io/team/openshell:codex-0.167.0-r4"],"Digest":"sha256:held","Arch":"arm64","Os":"linux","Labels":{"agentbox":"true","agentbox.agent":"codex","agentbox.version":"0.167.0","agentbox.recipe":"4"}},
		{"Id":"image-free","Names":["localhost/openshell:opencode-2.0.0-r4"],"Digest":"sha256:free","Arch":"arm64","Os":"linux","Labels":{"agentbox":"true","agentbox.agent":"opencode","agentbox.version":"2.0.0","agentbox.recipe":"4"}}
	]`
	server, err := New(&app.App{Store: store, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	var images []struct {
		Reference string   `json:"reference"`
		UsedBy    []string `json:"used_by"`
	}
	response := requestJSON(t, http.MethodGet, httpServer.URL+"/api/images", nil, &images)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", response.StatusCode)
	}
	used := map[string][]string{}
	for _, image := range images {
		used[image.Reference] = image.UsedBy
	}
	if !slices.Equal(used["localhost/openshell:codex-0.167.0-r4"], []string{"profile codex-default"}) {
		t.Fatalf("held image usage = %#v", used)
	}
	if len(used["localhost/openshell:opencode-2.0.0-r4"]) != 0 {
		t.Fatalf("free image usage = %#v", used)
	}

	response = requestJSON(t, http.MethodPost, httpServer.URL+"/api/images/delete", map[string]string{"reference": "localhost/openshell:codex-0.167.0-r4"}, nil)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("held delete status = %d; want conflict", response.StatusCode)
	}
	response = requestJSON(t, http.MethodPost, httpServer.URL+"/api/images/delete", map[string]string{"reference": "localhost/openshell:opencode-2.0.0-r4"}, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("free delete status = %d", response.StatusCode)
	}
	response = requestJSON(t, http.MethodPost, httpServer.URL+"/api/images/delete", map[string]string{"reference": "localhost/openshell:missing-1.0.0-r4"}, nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing delete status = %d; want not found", response.StatusCode)
	}
	var pruned struct {
		Removed []string `json:"removed"`
	}
	response = requestJSON(t, http.MethodPost, httpServer.URL+"/api/images/prune", nil, &pruned)
	if response.StatusCode != http.StatusOK || !slices.Contains(pruned.Removed, "localhost/openshell:opencode-2.0.0-r4") {
		t.Fatalf("prune status = %d removed = %#v", response.StatusCode, pruned.Removed)
	}
}

type imageRefreshRunner struct {
	mutex   sync.Mutex
	outputs [][]string
	streams [][]string
}

func (r *imageRefreshRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.outputs = append(r.outputs, append([]string{name}, args...))
	if name == "npm" {
		versions := map[string]string{
			"@openai/codex":             "0.167.0",
			"@anthropic-ai/claude-code": "2.1.321",
			"@opencode/cli":             "2.2.6",
		}
		version, ok := versions[args[1]]
		if !ok {
			return nil, fmt.Errorf("unexpected package %q", args[1])
		}
		return []byte(`"` + version + `"` + "\n"), nil
	}
	if name != "podman" || len(args) < 2 || args[0] != "image" || args[1] != "inspect" {
		return nil, fmt.Errorf("unexpected output command: %s %v", name, args)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "docker.io/library/node:22-bookworm-slim") {
		return []byte("sha256:base\n"), nil
	}
	for _, agent := range []string{"codex", "claude", "opencode"} {
		if strings.Contains(joined, agent+"-") {
			return []byte("sha256:" + agent + "-id sha256:" + agent + "-digest\n"), nil
		}
	}
	return nil, fmt.Errorf("unexpected image inspection: %v", args)
}

func (r *imageRefreshRunner) Stream(_ context.Context, _ io.Reader, _, _ io.Writer, name string, args ...string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.streams = append(r.streams, append([]string{name}, args...))
	return nil
}

func (r *imageRefreshRunner) commandContains(want string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	commands := append(append([][]string(nil), r.outputs...), r.streams...)
	for _, command := range commands {
		if slices.Contains(command, want) {
			return true
		}
	}
	return false
}

func TestImageRefreshEndpointBuildsAllAgentsWithoutDeletingImages(t *testing.T) {
	runner := &imageRefreshRunner{}
	server, err := New(&app.App{Store: app.Store{Base: t.TempDir()}, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	var started Task
	response := requestJSON(t, http.MethodPost, httpServer.URL+"/api/images/refresh", nil, &started)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("refresh status = %d; want accepted", response.StatusCode)
	}
	var task Task
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		requestJSON(t, http.MethodGet, httpServer.URL+"/api/tasks/"+started.ID, nil, &task)
		if task.Status != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if task.Status != "succeeded" {
		t.Fatalf("refresh task = %#v", task)
	}
	for _, tag := range []string{
		"localhost/openshell:codex-0.167.0-r4",
		"localhost/openshell:claude-2.1.321-r4",
		"localhost/openshell:opencode-2.2.6-r4",
	} {
		if !strings.Contains(task.Output, "Built image: "+tag+"\n") {
			t.Fatalf("refresh output does not include %q:\n%s", tag, task.Output)
		}
		if !runner.commandContains(tag) {
			t.Fatalf("refresh did not build %q", tag)
		}
	}
	if runner.commandContains("rm") {
		t.Fatal("refresh deleted an image")
	}
}

type lifecycleRunner struct {
	commands [][]string
}

func (r *lifecycleRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, append([]string{name}, args...))
	switch {
	case name == "openshell" && len(args) > 0 && args[0] == "gateway" && len(args) > 1 && args[1] == "list":
		return []byte(`[{"name":"gateway-one","active":true,"endpoint":"https://gateway.example","type":"podman","auth":"bearer","source":"remote"}]`), nil
	case name == "podman" && len(args) > 0 && args[0] == "images":
		return []byte(`[]`), nil
	case name == "openshell":
		return []byte(`{}`), nil
	default:
		return []byte(`{}`), nil
	}
}

func (r *lifecycleRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func newLifecycleServer(t *testing.T, store app.Store, runner *lifecycleRunner) *httptest.Server {
	t.Helper()
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	server, err := New(&app.App{Store: store, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return httpServer
}

func lifecycleStore(t *testing.T) app.Store {
	t.Helper()
	store := app.Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestResourceListsExposeUsage(t *testing.T) {
	store := lifecycleStore(t)
	defaultPolicy, err := store.CreatePolicyConfiguration("profile-default")
	if err != nil {
		t.Fatal(err)
	}
	otherPolicy, err := store.CreatePolicyConfiguration("other")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProject(app.Project{Version: 1, Name: "demo", Root: "/repo"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(app.Profile{Version: 1, Name: "profile-default", Agent: "codex", Image: "image", Mode: "standard", Provider: "provider", Policy: otherPolicy.Path, Approval: "prompted"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(app.Profile{Version: 1, Name: "profile-second", Agent: "codex", Image: "image", Mode: "standard", Provider: "provider", Policy: otherPolicy.Path, Approval: "prompted"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(app.Run{ID: "run-live123", Name: "live-run", Project: "demo", ProfileName: "profile-default", Gateway: "gateway-one", Status: app.StatusWorking}); err != nil {
		t.Fatal(err)
	}
	server := newLifecycleServer(t, store, &lifecycleRunner{})

	var projects []struct {
		Name   string   `json:"name"`
		UsedBy []string `json:"used_by"`
	}
	if response := requestJSON(t, http.MethodGet, server.URL+"/api/projects", nil, &projects); response.StatusCode != http.StatusOK || !slices.Equal(projects[0].UsedBy, []string{"live-run"}) {
		t.Fatalf("projects = %#v", projects)
	}
	var profiles []struct {
		Name         string   `json:"name"`
		UsedBy       []string `json:"used_by"`
		LastForAgent bool     `json:"last_for_agent"`
	}
	if response := requestJSON(t, http.MethodGet, server.URL+"/api/profiles", nil, &profiles); response.StatusCode != http.StatusOK {
		t.Fatalf("profiles status = %d", response.StatusCode)
	}
	if !slices.Equal(profiles[0].UsedBy, []string{"live-run"}) || profiles[0].LastForAgent {
		t.Fatalf("profiles = %#v", profiles)
	}
	if profiles[1].LastForAgent {
		t.Fatalf("second profile marked last: %#v", profiles)
	}
	var gateways []struct {
		Name   string   `json:"name"`
		UsedBy []string `json:"used_by"`
	}
	if response := requestJSON(t, http.MethodGet, server.URL+"/api/gateways", nil, &gateways); response.StatusCode != http.StatusOK || !slices.Equal(gateways[0].UsedBy, []string{"live-run"}) {
		t.Fatalf("gateways = %#v", gateways)
	}
	var policies []struct {
		Name       string   `json:"name"`
		UsedBy     []string `json:"used_by"`
		DefaultFor string   `json:"default_for"`
	}
	if response := requestJSON(t, http.MethodGet, server.URL+"/api/policies", nil, &policies); response.StatusCode != http.StatusOK {
		t.Fatalf("policies status = %d", response.StatusCode)
	}
	if !slices.Equal(policies[0].UsedBy, []string{"profile-default", "profile-second"}) || policies[0].DefaultFor != "" {
		t.Fatalf("other policy = %#v", policies[0])
	}
	if policies[1].Name != "profile-default" || policies[1].DefaultFor != "profile-default" {
		t.Fatalf("default policy = %#v", policies[1])
	}
	if defaultPolicy.Name == "" {
		t.Fatal("unreachable default policy assertion")
	}
}

func TestProjectDeleteEndpoint(t *testing.T) {
	store := lifecycleStore(t)
	if err := store.SaveProject(app.Project{Version: 1, Name: "unused", Root: "/repo"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProject(app.Project{Version: 1, Name: "held", Root: "/held"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(app.Run{ID: "run-held123", Name: "held-run", Project: "held", Status: app.StatusCompleted}); err != nil {
		t.Fatal(err)
	}
	server := newLifecycleServer(t, store, &lifecycleRunner{})

	var deleted struct {
		Deleted bool `json:"deleted"`
	}
	if response := requestJSON(t, http.MethodPost, server.URL+"/api/projects/unused/delete", nil, &deleted); response.StatusCode != http.StatusOK || !deleted.Deleted {
		t.Fatalf("delete status=%d deleted=%v", response.StatusCode, deleted)
	}
	var conflict struct {
		Error      string   `json:"error"`
		Dependents []string `json:"dependents"`
	}
	response := requestJSON(t, http.MethodPost, server.URL+"/api/projects/held/delete", nil, &conflict)
	if response.StatusCode != http.StatusConflict || !slices.Equal(conflict.Dependents, []string{"held-run"}) {
		t.Fatalf("conflict status=%d body=%+v", response.StatusCode, conflict)
	}
	response = requestJSON(t, http.MethodPost, server.URL+"/api/projects/missing/delete", nil, &conflict)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", response.StatusCode)
	}
}

func TestProfileDeleteEndpoint(t *testing.T) {
	store := lifecycleStore(t)
	policy, err := store.CreatePolicyConfiguration("policy")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if err := store.SaveProfile(app.Profile{Version: 1, Name: name, Agent: "codex", Image: "image", Mode: "standard", Provider: "provider", Policy: policy.Path, Approval: "prompted"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveRun(app.Run{ID: "run-live123", Name: "live-run", ProfileName: "one", Status: app.StatusWorking}); err != nil {
		t.Fatal(err)
	}
	server := newLifecycleServer(t, store, &lifecycleRunner{})

	var conflict struct {
		Dependents []string `json:"dependents"`
	}
	if response := requestJSON(t, http.MethodPost, server.URL+"/api/profiles/one/delete", nil, &conflict); response.StatusCode != http.StatusConflict || !slices.Equal(conflict.Dependents, []string{"live-run"}) {
		t.Fatalf("active status=%d body=%+v", response.StatusCode, conflict)
	}
	if err := store.SaveRun(app.Run{ID: "run-live123", Status: app.StatusCompleted}); err != nil {
		t.Fatal(err)
	}
	var deleted struct {
		Deleted bool `json:"deleted"`
	}
	if response := requestJSON(t, http.MethodPost, server.URL+"/api/profiles/one/delete", nil, &deleted); response.StatusCode != http.StatusOK || !deleted.Deleted {
		t.Fatalf("delete status=%d deleted=%v", response.StatusCode, deleted)
	}
	if response := requestJSON(t, http.MethodPost, server.URL+"/api/profiles/two/delete", nil, &conflict); response.StatusCode != http.StatusConflict {
		t.Fatalf("last-agent status = %d", response.StatusCode)
	}
	if response := requestJSON(t, http.MethodPost, server.URL+"/api/profiles/missing/delete", nil, &conflict); response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", response.StatusCode)
	}
}

func TestPolicyDeleteEndpoint(t *testing.T) {
	store := lifecycleStore(t)
	if _, err := store.CreatePolicyConfiguration("orphan"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreatePolicyConfiguration("held"); err != nil {
		t.Fatal(err)
	}
	held, err := store.LoadPolicyConfiguration("held")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(app.Profile{Version: 1, Name: "profile", Agent: "codex", Image: "image", Mode: "standard", Provider: "provider", Policy: held.Path, Approval: "prompted"}); err != nil {
		t.Fatal(err)
	}
	liveRunPolicy, err := store.CreatePolicyConfiguration("run-live123")
	if err != nil {
		t.Fatal(err)
	}
	pendingRunPolicy, err := store.CreatePolicyConfiguration("run-pending123-pending")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(app.Run{ID: "run-live123", Name: "live-run", Policy: liveRunPolicy.Path}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(app.Run{ID: "run-pending123", Name: "pending-run", PendingPolicy: pendingRunPolicy.Path}); err != nil {
		t.Fatal(err)
	}
	server := newLifecycleServer(t, store, &lifecycleRunner{})

	var deleted struct {
		Deleted bool `json:"deleted"`
	}
	if response := requestJSON(t, http.MethodPost, server.URL+"/api/policies/orphan/delete", nil, &deleted); response.StatusCode != http.StatusOK || !deleted.Deleted {
		t.Fatalf("delete status=%d deleted=%v", response.StatusCode, deleted)
	}
	var conflict struct {
		Dependents []string `json:"dependents"`
	}
	if response := requestJSON(t, http.MethodPost, server.URL+"/api/policies/held/delete", nil, &conflict); response.StatusCode != http.StatusConflict || !slices.Equal(conflict.Dependents, []string{"profile"}) {
		t.Fatalf("conflict status=%d body=%+v", response.StatusCode, conflict)
	}
	if response := requestJSON(t, http.MethodPost, server.URL+"/api/policies/run-live123/delete", nil, &conflict); response.StatusCode != http.StatusConflict || !slices.Equal(conflict.Dependents, []string{"live-run"}) {
		t.Fatalf("live run conflict status=%d body=%+v", response.StatusCode, conflict)
	}
	if response := requestJSON(t, http.MethodPost, server.URL+"/api/policies/run-pending123-pending/delete", nil, &conflict); response.StatusCode != http.StatusConflict || !slices.Equal(conflict.Dependents, []string{"pending-run"}) {
		t.Fatalf("pending run conflict status=%d body=%+v", response.StatusCode, conflict)
	}
	if response := requestJSON(t, http.MethodPost, server.URL+"/api/policies/missing/delete", nil, &conflict); response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", response.StatusCode)
	}
}

func TestGatewayDeleteEndpoint(t *testing.T) {
	store := lifecycleStore(t)
	if err := store.SaveRun(app.Run{ID: "run-live123", Name: "live-run", Gateway: "gateway-one", Status: app.StatusWorking}); err != nil {
		t.Fatal(err)
	}
	runner := &lifecycleRunner{}
	server := newLifecycleServer(t, store, runner)

	var conflict struct {
		Dependents []string `json:"dependents"`
	}
	if response := requestJSON(t, http.MethodPost, server.URL+"/api/gateways/gateway-one/delete", nil, &conflict); response.StatusCode != http.StatusConflict || !slices.Equal(conflict.Dependents, []string{"live-run"}) {
		t.Fatalf("conflict status=%d body=%+v", response.StatusCode, conflict)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("blocked request issued commands: %#v", runner.commands)
	}
	if err := store.SaveRun(app.Run{ID: "run-live123", Status: app.StatusCompleted}); err != nil {
		t.Fatal(err)
	}
	runner.commands = nil
	var deleted struct {
		Deleted bool `json:"deleted"`
	}
	if response := requestJSON(t, http.MethodPost, server.URL+"/api/gateways/gateway-one/delete", nil, &deleted); response.StatusCode != http.StatusOK || !deleted.Deleted {
		t.Fatalf("delete status=%d deleted=%v", response.StatusCode, deleted)
	}
	if len(runner.commands) != 1 || !slices.Equal(runner.commands[0], []string{"openshell", "gateway", "remove", "gateway-one"}) {
		t.Fatalf("commands = %#v", runner.commands)
	}
}

func TestRegistryClearEndpoint(t *testing.T) {
	store := lifecycleStore(t)
	if err := store.SaveRegistry(app.Registry{Reference: "registry.example/team/agentbox"}); err != nil {
		t.Fatal(err)
	}
	runner := &lifecycleRunner{}
	server := newLifecycleServer(t, store, runner)

	for range 2 {
		var cleared struct {
			Cleared bool `json:"cleared"`
		}
		if response := requestJSON(t, http.MethodPost, server.URL+"/api/registry/clear", nil, &cleared); response.StatusCode != http.StatusOK || !cleared.Cleared {
			t.Fatalf("clear status=%d cleared=%v", response.StatusCode, cleared.Cleared)
		}
	}
	for _, command := range runner.commands {
		if command[0] == "podman" {
			t.Fatalf("registry clear issued Podman command: %#v", command)
		}
	}
	if _, err := os.Stat(store.RegistryPath()); !os.IsNotExist(err) {
		t.Fatalf("registry file still exists: %v", err)
	}
}
