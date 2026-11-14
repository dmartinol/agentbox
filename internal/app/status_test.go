package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeRunStatusMigratesLegacyValues(t *testing.T) {
	cases := map[string]RunStatus{
		"":               StatusCreating,
		"preview":        StatusCreating,
		"prepared":       StatusCreating,
		"created":        StatusCreating,
		"running":        StatusWorking,
		"idle":           StatusReady,
		"finished":       StatusCompleted,
		"stopped":        StatusStopped,
		"error":          StatusFailed,
		"create-failed":  StatusFailed,
		"policy-blocked": StatusFailed,
		"unreachable":    StatusUnreachable,
		"unreadable":     StatusUnreachable,
		"ready":          StatusReady,
		"working":        StatusWorking,
		"waiting":        StatusWaiting,
		"completed":      StatusCompleted,
		"failed":         StatusFailed,
	}
	for raw, want := range cases {
		got, _ := NormalizeRunStatus(raw, "")
		if got != want {
			t.Errorf("NormalizeRunStatus(%q) = %q; want %q", raw, got, want)
		}
	}
	got, detail := NormalizeRunStatus("mystery", "")
	if got != StatusUnreachable || !strings.Contains(detail, "mystery") {
		t.Fatalf("unknown status = %q, %q; want Unreachable naming the value", got, detail)
	}
	if got, detail := NormalizeRunStatus("policy-blocked", ""); got != StatusFailed || detail == "" {
		t.Fatalf("legacy failure detail missing: %q, %q", got, detail)
	}
}

func TestClassifyRunStatusPrecedence(t *testing.T) {
	now := time.Now()
	exitThree := 3
	working := &activitySignal{State: "working", Detail: "running a tool"}
	waiting := &activitySignal{State: "waiting", Detail: "permission requested"}

	cases := []struct {
		name       string
		evidence   runEvidence
		want       RunStatus
		wantDetail string
	}{
		{
			name:       "inspect error",
			evidence:   runEvidence{InspectErr: "gateway unreachable", Phase: "error"},
			want:       StatusUnreachable,
			wantDetail: "gateway unreachable",
		},
		{
			name:     "unparsable sandbox response",
			evidence: runEvidence{Unreadable: true, Phase: "running"},
			want:     StatusUnreachable,
		},
		{
			name:     "sandbox phase error beats a finished marker",
			evidence: runEvidence{Phase: "error", FinishedMarker: true},
			want:     StatusFailed,
		},
		{
			name:     "stopped sandbox",
			evidence: runEvidence{Phase: "stopped", FinishedMarker: true},
			want:     StatusStopped,
		},
		{
			name:     "exited sandbox",
			evidence: runEvidence{Phase: "exited"},
			want:     StatusStopped,
		},
		{
			name:     "no session yet",
			evidence: runEvidence{Phase: "running"},
			want:     StatusCreating,
		},
		{
			name:       "finished marker with exit code",
			evidence:   runEvidence{Phase: "running", FinishedMarker: true, ExitCode: &exitThree},
			want:       StatusCompleted,
			wantDetail: "exit code 3",
		},
		{
			name:       "finished marker without exit code",
			evidence:   runEvidence{Phase: "running", FinishedMarker: true},
			want:       StatusCompleted,
			wantDetail: "agent exited",
		},
		{
			name:     "completed phase",
			evidence: runEvidence{Phase: "completed", Session: true, PaneCommand: "codex"},
			want:     StatusCompleted,
		},
		{
			name:       "explicit working signal",
			evidence:   runEvidence{Phase: "running", Session: true, PaneCommand: "codex", Activity: working},
			want:       StatusWorking,
			wantDetail: "running a tool",
		},
		{
			name:       "explicit waiting signal beats a busy pane",
			evidence:   runEvidence{Phase: "running", Session: true, PaneCommand: "bash", Activity: waiting},
			want:       StatusWaiting,
			wantDetail: "permission requested",
		},
		{
			// Codex ends a plan-mode turn with Stop, so the activity signal
			// says done; the approval prompt is what the user actually sees.
			name:       "plan approval prompt waits for the decision",
			evidence:   runEvidence{Phase: "running", Session: true, PaneCommand: "sh", PlanPrompt: true, Activity: &activitySignal{State: "done", Detail: "turn finished"}},
			want:       StatusWaiting,
			wantDetail: "plan ready for review",
		},
		{
			name:     "plan prompt is ignored once the pane is gone",
			evidence: runEvidence{Phase: "running", Session: true, PaneDead: true, PlanPrompt: true},
			want:     StatusReady,
		},
		{
			name:     "explicit idle signal beats the agent in the foreground",
			evidence: runEvidence{Phase: "running", Session: true, PaneCommand: "codex", Activity: &activitySignal{State: "idle"}},
			want:     StatusReady,
		},
		{
			// OpenCode has no hooks of its own; a waiting signal can only come
			// from the session prompt, which is a genuine Needs You state.
			name:     "session prompt waits for every agent",
			evidence: runEvidence{Phase: "running", Session: true, PaneCommand: "sh", Agent: "opencode", Activity: waiting},
			want:     StatusWaiting,
		},
		{
			name:     "stale signal with a dead pane falls through",
			evidence: runEvidence{Phase: "running", Session: true, PaneDead: true, Activity: working, IdleMarker: true},
			want:     StatusReady,
		},
		{
			name:     "idle marker",
			evidence: runEvidence{Phase: "running", Session: true, PaneCommand: "codex", IdleMarker: true},
			want:     StatusReady,
		},
		{
			name:     "shell in the foreground",
			evidence: runEvidence{Phase: "running", Session: true, PaneCommand: "sh"},
			want:     StatusReady,
		},
		{
			name:     "agent in the foreground",
			evidence: runEvidence{Phase: "running", Session: true, PaneCommand: "codex"},
			want:     StatusWorking,
		},
		{
			name:     "opencode with a fresh log",
			evidence: runEvidence{Phase: "running", Session: true, PaneCommand: "opencode", Agent: "opencode", LogMtime: now.Add(-10 * time.Second), LogMtimeSet: true},
			want:     StatusWorking,
		},
		{
			name:     "opencode with a quiet log",
			evidence: runEvidence{Phase: "running", Session: true, PaneCommand: "opencode", Agent: "opencode", LogMtime: now.Add(-10 * time.Minute), LogMtimeSet: true},
			want:     StatusReady,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, detail := classifyRunStatus(tc.evidence, now)
			if got != tc.want {
				t.Fatalf("status = %q; want %q (detail %q)", got, tc.want, detail)
			}
			if tc.wantDetail != "" && detail != tc.wantDetail {
				t.Fatalf("detail = %q; want %q", detail, tc.wantDetail)
			}
		})
	}
}

func TestParseRunProbe(t *testing.T) {
	evidence, err := parseRunProbe([]byte("session=1\ndead=0\ncommand=codex\nplan=1\nidle=0\nfinished=1\nexit=0\nactivity={\"state\":\"waiting\",\"detail\":\"permission requested\"}\nmtime=1700000000\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.Session || evidence.PaneDead || evidence.PaneCommand != "codex" || !evidence.PlanPrompt || evidence.IdleMarker || !evidence.FinishedMarker {
		t.Fatalf("probe evidence = %+v", evidence)
	}
	if evidence.ExitCode == nil || *evidence.ExitCode != 0 {
		t.Fatalf("exit code = %v", evidence.ExitCode)
	}
	if evidence.Activity == nil || evidence.Activity.State != "waiting" || evidence.Activity.Detail != "permission requested" {
		t.Fatalf("activity = %+v", evidence.Activity)
	}
	if !evidence.LogMtimeSet || evidence.LogMtime.Unix() != 1700000000 {
		t.Fatalf("log mtime = %v (%v)", evidence.LogMtime, evidence.LogMtimeSet)
	}

	if _, err := parseRunProbe([]byte("garbage\n")); err == nil {
		t.Fatal("probe without session state should fail")
	}
	for _, raw := range []string{`{"state":"thinking"}`, `{`, ``, `{"state":"waiting"`} {
		if signal := parseActivitySignal(raw); signal != nil {
			t.Fatalf("activity %q should be ignored, got %+v", raw, signal)
		}
	}
	if signal := parseActivitySignal(`{"state":"WORKING","detail":"x"}`); signal == nil || signal.State != "working" {
		t.Fatalf("activity state should be case-insensitive: %+v", signal)
	}
}

func TestStatusPredicates(t *testing.T) {
	for _, status := range []RunStatus{StatusCreating, StatusReady, StatusWorking, StatusWaiting} {
		if !status.IsActive() {
			t.Errorf("%q should be active", status)
		}
	}
	for _, status := range []RunStatus{StatusFailed, StatusStopped, StatusCompleted, StatusUnreachable} {
		if status.IsActive() {
			t.Errorf("%q should not be active", status)
		}
	}
	for _, status := range []RunStatus{StatusReady, StatusWorking, StatusWaiting} {
		if !status.IsAttachable() || !status.IsStoppable() {
			t.Errorf("%q should be attachable and stoppable", status)
		}
	}
	if StatusCreating.IsAttachable() || StatusStopped.IsAttachable() || StatusCompleted.IsAttachable() {
		t.Error("creating/stopped/completed must not be attachable")
	}
	if StatusWaiting.Label() != "Waiting for input" {
		t.Errorf("waiting label = %q", StatusWaiting.Label())
	}
}

func TestInspectRunCachesDerivedStatus(t *testing.T) {
	ctx := context.Background()
	store := Store{Base: t.TempDir()}
	app := App{Store: store}
	run := Run{ID: "run-probe123", Sandbox: "sandbox-probe", Gateway: "gw", Status: StatusCreating}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}

	// A record whose sandbox does not exist yet stays Creating.
	app.Runner = &fakeRunner{output: func(recordedCommand) ([]byte, error) { return nil, errors.New("sandbox not found") }}
	got, err := app.GetRun(ctx, run.ID)
	if err != nil || got.Status != StatusCreating {
		t.Fatalf("missing sandbox status = %q, %v; want creating", got.Status, err)
	}

	// Failed is terminal for the record: a show must not probe it again.
	run.Status = StatusFailed
	run.StatusDetail = "prepare failed"
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	app.Runner = &fakeRunner{output: func(recordedCommand) ([]byte, error) {
		t.Fatal("failed run was inspected")
		return nil, nil
	}}
	got, err = app.GetRun(ctx, run.ID)
	if err != nil || got.Status != StatusFailed || got.StatusDetail != "prepare failed" {
		t.Fatalf("failed run = %q/%q, %v", got.Status, got.StatusDetail, err)
	}

	// A live explicit signal wins and is persisted for the next render.
	run.Status = StatusWorking
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	app.Runner = statusRunnerFor("running", "session=1\ndead=0\ncommand=codex\nactivity={\"state\":\"waiting\",\"detail\":\"permission requested\"}\n")
	got, err = app.GetRun(ctx, run.ID)
	if err != nil || got.Status != StatusWaiting || got.StatusDetail != "permission requested" {
		t.Fatalf("live run = %q/%q, %v", got.Status, got.StatusDetail, err)
	}
	stored, err := store.LoadRun(run.ID)
	if err != nil || stored.Status != StatusWaiting {
		t.Fatalf("derived status not persisted: %+v, %v", stored, err)
	}
}

func TestStopRefusesTerminalStatuses(t *testing.T) {
	ctx := context.Background()
	store := Store{Base: t.TempDir()}
	app := App{Store: store}
	run := Run{ID: "run-done123", Sandbox: "sandbox-done", Gateway: "gw", Status: StatusCompleted}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	app.Runner = statusRunnerFor("running", "session=0\ndead=1\nfinished=1\nexit=0\n")
	if err := app.Stop(ctx, run.ID); err == nil || !strings.Contains(err.Error(), "stop applies to") {
		t.Fatalf("completed run stop = %v", err)
	}
}
