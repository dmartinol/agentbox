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
	args := []string{"run", "start", "--dry-run", "--json", "--profile", profile, "--gateway", liveGateway(), "--source", liveSource()}
	if err := json.Unmarshal(runLive(t, args...), &run); err != nil {
		t.Fatal(err)
	}
	if run.Status != "preview" || run.ID == "" {
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
	if err := json.Unmarshal(runLive(t, "run", "show", "--json", runID), &run); err != nil {
		t.Fatal(err)
	}
	if run.ID != runID {
		t.Fatalf("run show returned ID %q; want %q", run.ID, runID)
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
}
