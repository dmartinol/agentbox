package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const denialLog = `[1795092202.467] [sandbox] [OCSF ] [ocsf] NET:OPEN [MED] DENIED /usr/bin/curl(0) -> example.com:443 [reason:transparent_tcp_policy_denied]
`

func newDenialTestApp(t *testing.T) (App, Run, Store, *fakeRunner) {
	t.Helper()
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agentbox"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agentbox", "config.yaml"), []byte(repoSetupManifest), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadRepoConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := RepoPolicyFragment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	fragmentPath := filepath.Join(store.PolicyDir(), "repo-example.yaml")
	if err := os.WriteFile(fragmentPath, fragment, 0600); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(store.PolicyDir(), "run-unit123.yaml")
	if err := os.WriteFile(policyPath, DefaultPolicy(), 0600); err != nil {
		t.Fatal(err)
	}
	run := Run{
		ID:             "run-unit123",
		Policy:         policyPath,
		PolicyFragment: fragmentPath,
		Sandbox:        "sandbox-unit123",
		Gateway:        "gateway-one",
		Worktree:       root,
		Source:         root,
		Status:         StatusWorking,
	}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && len(call.args) > 0 && call.args[0] == "logs" {
			return []byte(denialLog), nil
		}
		return nil, nil
	}}
	return App{Store: store, Runner: runner}, run, store, runner
}

func denialRequest(persist bool) DenialRequest {
	return DenialRequest{Host: "example.com", Port: 443, Binary: "/usr/bin/curl", Persist: persist}
}

func TestDenialPreviewDoesNotWriteAndShowsBothFiles(t *testing.T) {
	app, run, store, _ := newDenialTestApp(t)
	beforePolicy, _ := os.ReadFile(run.Policy)
	beforeManifest, _ := os.ReadFile(RepoConfigPath(run.Worktree))
	beforeFragment, _ := os.ReadFile(run.PolicyFragment)

	preview, err := app.PreviewDenial(context.Background(), run.ID, denialRequest(true))
	if err != nil {
		t.Fatal(err)
	}
	if !preview.PersistAvailable || preview.PersistReason != "" {
		t.Fatalf("persist availability = %+v", preview)
	}
	if preview.Entry.Host != "example.com" || preview.Entry.Port != 443 || preview.Entry.Binaries[0] != "/usr/bin/curl" {
		t.Fatalf("entry = %+v", preview.Entry)
	}
	for _, diff := range []string{preview.PolicyDiff, preview.ManifestDiff, preview.FragmentDiff} {
		if !strings.Contains(diff, "+") {
			t.Fatalf("preview diff was empty: %q", diff)
		}
	}
	afterPolicy, _ := os.ReadFile(run.Policy)
	afterManifest, _ := os.ReadFile(RepoConfigPath(run.Worktree))
	afterFragment, _ := os.ReadFile(run.PolicyFragment)
	if !bytes.Equal(beforePolicy, afterPolicy) || !bytes.Equal(beforeManifest, afterManifest) || !bytes.Equal(beforeFragment, afterFragment) {
		t.Fatal("preview wrote a file")
	}
	if stored, _ := store.LoadRun(run.ID); len(stored.RuntimeHosts) != 0 {
		t.Fatalf("preview recorded provenance: %+v", stored.RuntimeHosts)
	}
}

func TestApplyDenialPatchesOnlyTheRunWhenNotPersisting(t *testing.T) {
	app, run, store, runner := newDenialTestApp(t)
	beforeManifest, _ := os.ReadFile(RepoConfigPath(run.Worktree))

	result, err := app.ApplyDenial(context.Background(), run.ID, denialRequest(false))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != PolicyEditLiveApplied || result.Persisted || result.RuntimeHost.Source != "run" {
		t.Fatalf("result = %+v", result)
	}
	policy, err := os.ReadFile(run.Policy)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	rule, ok := parsed.Network[result.RuntimeHost.Rule]
	if !ok || len(rule.Binaries) != 1 || rule.Binaries[0].Path != "/usr/bin/curl" {
		t.Fatalf("run rule = %+v", rule)
	}
	afterManifest, _ := os.ReadFile(RepoConfigPath(run.Worktree))
	if !bytes.Equal(beforeManifest, afterManifest) {
		t.Fatal("run-only apply changed the manifest")
	}
	stored, err := store.LoadRun(run.ID)
	if err != nil || len(stored.RuntimeHosts) != 1 || stored.RuntimeHosts[0].Source != "run" {
		t.Fatalf("provenance = %+v, %v", stored.RuntimeHosts, err)
	}
	for _, call := range runner.outputs {
		if call.name == "podman" {
			t.Fatalf("denial remediation invoked an image build: %#v", call.args)
		}
	}
}

func TestApplyDenialPersistsManifestAndRegeneratesFragment(t *testing.T) {
	app, run, _, runner := newDenialTestApp(t)
	result, err := app.ApplyDenial(context.Background(), run.ID, denialRequest(true))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Persisted || result.FragmentStatus == "" || result.RuntimeHost.Source != "run+manifest" {
		t.Fatalf("result = %+v", result)
	}
	manifest, err := os.ReadFile(RepoConfigPath(run.Worktree))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseRepoConfig(manifest)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rule := range cfg.Hosts {
		if rule.Host == "example.com" && rule.Port == 443 && len(rule.Binaries) == 1 && rule.Binaries[0] == "/usr/bin/curl" {
			found = true
		}
	}
	if !found {
		t.Fatalf("manifest hosts = %+v", cfg.Hosts)
	}
	fragment, err := os.ReadFile(run.PolicyFragment)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fragment), "example.com") {
		t.Fatalf("fragment was not regenerated:\n%s", fragment)
	}
	for _, call := range runner.outputs {
		if call.name == "podman" {
			t.Fatalf("persist invoked an image build: %#v", call.args)
		}
	}
}

func TestApplyDenialRequiresALiveSurfacedDenial(t *testing.T) {
	app, run, store, runner := newDenialTestApp(t)
	terminal := run
	terminal.Status = StatusCompleted
	if err := store.SaveRun(terminal); err != nil {
		t.Fatal(err)
	}
	if _, err := app.PreviewDenial(context.Background(), run.ID, denialRequest(false)); !errors.As(err, new(*NoLiveSandboxError)) {
		t.Fatalf("terminal preview error = %v", err)
	}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner.output = func(recordedCommand) ([]byte, error) { return []byte("no denials here\n"), nil }
	_, err := app.ApplyDenial(context.Background(), run.ID, denialRequest(false))
	var validation *ValidationError
	if !errors.As(err, &validation) || !strings.Contains(err.Error(), "No active denial") {
		t.Fatalf("missing denial error = %v", err)
	}
}

func TestApplyDenialDoesNotPersistWhenOpenShellRejects(t *testing.T) {
	app, run, _, runner := newDenialTestApp(t)
	beforeManifest, _ := os.ReadFile(RepoConfigPath(run.Worktree))
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && len(call.args) > 0 && call.args[0] == "logs" {
			return []byte(denialLog), nil
		}
		if call.name == "openshell" && len(call.args) > 0 && call.args[0] == "policy" {
			return nil, errors.New("policy rejected")
		}
		return nil, nil
	}
	result, err := app.ApplyDenial(context.Background(), run.ID, denialRequest(true))
	if err == nil || result.Outcome != PolicyEditOpenShellRejected || !result.RolledBack {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	afterManifest, _ := os.ReadFile(RepoConfigPath(run.Worktree))
	if !bytes.Equal(beforeManifest, afterManifest) {
		t.Fatal("manifest changed after a rejected run policy")
	}
}

func TestApplyDenialRecordsRunProvenanceWhenPersistWriteFails(t *testing.T) {
	app, run, store, _ := newDenialTestApp(t)
	agentboxDir := filepath.Join(run.Worktree, ".agentbox")
	if err := os.Chmod(agentboxDir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(agentboxDir, 0700) })

	result, err := app.ApplyDenial(context.Background(), run.ID, denialRequest(true))
	if err == nil || !strings.Contains(err.Error(), "could not be written") {
		t.Fatalf("err = %v", err)
	}
	if result.Persisted || result.RuntimeHost.Source != "run" {
		t.Fatalf("result = %+v", result)
	}
	stored, err := store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.RuntimeHosts) != 1 || stored.RuntimeHosts[0].Source != "run" {
		t.Fatalf("provenance = %+v", stored.RuntimeHosts)
	}
	policy, err := os.ReadFile(run.Policy)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Network) == 0 {
		t.Fatal("run policy was not updated before the manifest failure")
	}
}
