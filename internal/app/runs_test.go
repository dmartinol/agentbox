package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifySandboxStatus(t *testing.T) {
	cases := []struct {
		phase    string
		finished bool
		want     string
		sync     bool
	}{
		{phase: "running", want: "running"},
		{phase: "RUNNING", finished: true, want: "finished", sync: true},
		{phase: "completed", finished: true, want: "finished", sync: true},
		{phase: "stopped", want: "stopped"},
		{phase: "error", finished: true, want: "error"},
	}
	for _, tc := range cases {
		got, sync := classifySandboxStatus(tc.phase, tc.finished)
		if got != tc.want || sync != tc.sync {
			t.Errorf("classifySandboxStatus(%q,%v) = %q,%v; want %q,%v", tc.phase, tc.finished, got, sync, tc.want, tc.sync)
		}
	}
}

func TestDeleteProtectsActiveAndReviewableRunData(t *testing.T) {
	for _, tc := range []struct {
		name   string
		active bool
		edit   bool
		want   string
	}{
		{name: "active", active: true},
		{name: "unsynced", active: false, want: "unsynced"},
		{name: "review changes", active: false, edit: true, want: "review changes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, run, _ := createSyncFixture(t)
			run.Synced = true
			if tc.name == "unsynced" {
				run.Synced = false
			}
			if !tc.active {
				run.Status = "stopped"
			}
			if tc.edit {
				if err := os.WriteFile(filepath.Join(run.Worktree, "tracked.txt"), []byte("review me\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := app.Store.SaveRun(run); err != nil {
				t.Fatal(err)
			}
			runner := newSyncRunner(t, nil)
			app.Runner = runner
			err := app.Delete(context.Background(), run.ID, false)
			if err == nil {
				t.Fatal("run with protected data was deleted")
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected delete refusal: %v", err)
			}
			if tc.active && !strings.Contains(err.Error(), "still active") {
				t.Fatalf("active run refusal was unclear: %v", err)
			}
			for _, call := range runner.outputs {
				if call.name == "openshell" {
					t.Fatalf("sandbox delete attempted before guard: %#v", call)
				}
			}
		})
	}
}

func TestStopRetainsSandboxWhenSyncRefusesLocalConflict(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	if err := os.WriteFile(filepath.Join(run.Worktree, "tracked.txt"), []byte("local edit during run\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := newSyncRunner(t, nil)
	app.Runner = runner
	err := app.Stop(context.Background(), run.ID)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected sync refusal, got %v", err)
	}
	for _, call := range runner.outputs {
		if call.name == "openshell" && commandHasArg(call, "stop") {
			t.Fatal("sandbox stopped despite sync failure")
		}
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || stored.Status != "running" {
		t.Fatalf("run state did not retain recovery status: %+v, %v", stored, err)
	}
}

func TestStopSyncsBeforeStoppingSandbox(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	artifacts := createAgentExport(t, run, t.TempDir())
	runner := newSyncRunner(t, artifacts)
	app.Runner = runner
	if err := app.Stop(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := app.Store.LoadRun(run.ID)
	if err != nil || stored.Status != "stopped" || !stored.Synced {
		t.Fatalf("run did not record successful stop/sync: %+v, %v", stored, err)
	}
	stopIndex, syncDownloadIndex := -1, -1
	for i, call := range runner.outputs {
		if call.name == "openshell" && commandHasArg(call, "stop") {
			stopIndex = i
		}
		if call.name == "openshell" && commandHasArg(call, "download") {
			syncDownloadIndex = i
		}
	}
	if syncDownloadIndex < 0 || stopIndex < syncDownloadIndex {
		t.Fatalf("sandbox stop did not follow sync: download=%d stop=%d", syncDownloadIndex, stopIndex)
	}
}

func TestListAndReconcileAttemptsSyncWhenCompletedMarkerExists(t *testing.T) {
	app, run, _ := createSyncFixture(t)
	store := app.Store
	run.Synced = false
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "git" {
			return (OSRunner{}).Output(context.Background(), "git", call.args...)
		}
		if call.name == "openshell" && commandHasArg(call, "get") {
			return []byte(`{"phase":"completed"}`), nil
		}
		return nil, nil
	}
	app.Runner = runner
	got, err := app.ListAndReconcile(context.Background())
	if err != nil || len(got) != 1 || got[0].Status != "finished" {
		t.Fatalf("reconcile result = %#v, %v", got, err)
	}
	foundExport := false
	for _, call := range runner.outputs {
		if call.name == "openshell" && strings.Contains(strings.Join(call.args, " "), "/workspace/export.bundle") {
			foundExport = true
		}
	}
	if !foundExport {
		t.Fatal("completed run was not offered to the sync path")
	}
}

func TestStartReportsMissingProfileAndPolicy(t *testing.T) {
	ctx := context.Background()
	store := Store{Base: t.TempDir()}
	app := App{Store: store, Runner: &fakeRunner{}}

	if _, err := app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell"}); err == nil || !strings.Contains(err.Error(), `profile "mycodex" not found`) {
		t.Fatalf("missing profile error = %v", err)
	}

	policy := filepath.Join(store.PolicyDir(), "missing.yaml")
	if err := store.SaveProfile(Profile{
		Version:  1,
		Name:     "mycodex",
		Agent:    "codex",
		Image:    "localhost/openshell-codex:1.2.3-r3",
		Mode:     "standard",
		Provider: "provider",
		Policy:   policy,
		Approval: "prompted",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Start(ctx, StartOptions{Profile: "mycodex", Gateway: "openshell"}); err == nil || !strings.Contains(err.Error(), "references missing policy file") {
		t.Fatalf("missing policy error = %v", err)
	}
}

func TestBootstrapKillsAgentSessionWhenAgentExits(t *testing.T) {
	for _, want := range []string{"touch /workspace/finished", "tmux kill-session -t agent", "exec sleep infinity"} {
		if !strings.Contains(bootstrapScript, want) {
			t.Fatalf("bootstrap script missing %q:\n%s", want, bootstrapScript)
		}
	}
}
