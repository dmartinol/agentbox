package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunArtifactPathsUseExplicitNames(t *testing.T) {
	store := Store{Base: t.TempDir()}
	got := runArtifactPaths(store, "run-unit123")
	want := []string{
		filepath.Join(store.RunsDir(), "run-unit123-source.bundle"),
		filepath.Join(store.RunsDir(), "run-unit123-config.tar"),
		filepath.Join(store.RunsDir(), "run-unit123-initial.patch"),
		filepath.Join(store.RunsDir(), "run-unit123-mirror.patch"),
		filepath.Join(store.RunsDir(), "run-unit123-mirror.tar"),
		filepath.Join(store.RunsDir(), "run-unit123-config"),
	}
	if len(got) != len(want) {
		t.Fatalf("artifact path count = %d; want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("artifact path %d = %q; want %q", index, got[index], want[index])
		}
	}
}

func TestPruneRunArtifactsDryRunListsWithoutDeleting(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(store.RunsDir(), "run-orphan1-config")
	if err := os.MkdirAll(filepath.Join(configDir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	configBody := []byte("orphan\n")
	if err := os.WriteFile(filepath.Join(configDir, "nested", "file"), configBody, 0600); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(store.RunsDir(), "run-orphan1-source.bundle")
	sourceBody := []byte("data")
	if err := os.WriteFile(sourcePath, sourceBody, 0600); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(store.RunsDir(), "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("keep\n"), 0600); err != nil {
		t.Fatal(err)
	}

	result, err := (&App{Store: store}).PruneRunArtifacts(true)
	if err != nil {
		t.Fatal(err)
	}
	want := []PrunedRunArtifact{
		{Path: configDir, Bytes: int64(len(configBody))},
		{Path: sourcePath, Bytes: int64(len(sourceBody))},
	}
	if !result.DryRun || len(result.Artifacts) != len(want) || result.FreedBytes != int64(len(configBody)+len(sourceBody)) {
		t.Fatalf("dry-run result = %+v; want %#v", result, want)
	}
	for index, artifact := range want {
		if result.Artifacts[index] != artifact {
			t.Fatalf("dry-run artifact %d = %+v; want %+v", index, result.Artifacts[index], artifact)
		}
		if _, err := os.Stat(artifact.Path); err != nil {
			t.Fatalf("dry-run removed artifact: %s (%v)", artifact.Path, err)
		}
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("dry-run removed unrelated file: %v", err)
	}
}

func TestPruneRunArtifactsRemovesOnlyOrphansAndIsIdempotent(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(Run{ID: "run-live123"}); err != nil {
		t.Fatal(err)
	}
	livePath := filepath.Join(store.RunsDir(), "run-live123-source.bundle")
	if err := os.WriteFile(livePath, []byte("live\n"), 0600); err != nil {
		t.Fatal(err)
	}
	orphanConfig := filepath.Join(store.RunsDir(), "run-orphan2-config")
	if err := os.MkdirAll(filepath.Join(orphanConfig, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphanConfig, "nested", "file"), []byte("orphan\n"), 0600); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(store.RunsDir(), "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("keep\n"), 0600); err != nil {
		t.Fatal(err)
	}

	result, err := (&App{Store: store}).PruneRunArtifacts(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Path != orphanConfig || result.Artifacts[0].Bytes != int64(len("orphan\n")) {
		t.Fatalf("prune result = %+v; want only %q", result, orphanConfig)
	}
	if _, err := os.Stat(orphanConfig); !os.IsNotExist(err) {
		t.Fatalf("orphan config still present: %v", err)
	}
	if _, err := os.Stat(livePath); err != nil {
		t.Fatalf("live run artifact was removed: %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}

	again, err := (&App{Store: store}).PruneRunArtifacts(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Artifacts) != 0 || again.Artifacts == nil || again.FreedBytes != 0 {
		t.Fatalf("second prune result = %+v; want an initialized empty list", again)
	}
}

func savePruneRun(t *testing.T, app *App, id, project string, status RunStatus) {
	t.Helper()
	run := Run{
		ID:          id,
		Name:        id,
		Project:     project,
		Status:      status,
		Sandbox:     "sandbox-" + id,
		Gateway:     "gateway-one",
		Agent:       "codex",
		ProfileName: "codex",
		Mode:        "worktree",
		Synced:      true,
	}
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
}

func pruneRunner() *fakeRunner {
	runner := &fakeRunner{}
	runner.output = func(call recordedCommand) ([]byte, error) {
		joined := strings.Join(call.args, " ")
		switch {
		case call.name == "openshell" && containsCommand(call.args, "sandbox", "get"):
			switch {
			case strings.Contains(joined, "sandbox-run-done"):
				return []byte(`{"phase":"completed"}`), nil
			case strings.Contains(joined, "sandbox-run-stopped"):
				return []byte(`{"phase":"stopped"}`), nil
			case strings.Contains(joined, "sandbox-run-working"):
				return []byte(`{"phase":"running"}`), nil
			default:
				return nil, errors.New("sandbox not found")
			}
		case call.name == "openshell" && strings.Contains(joined, probeMarker):
			if strings.Contains(joined, "sandbox-run-working") {
				return []byte(workingProbe), nil
			}
			return []byte(exitedProbe), nil
		case call.name == "openshell" && containsCommand(call.args, "sandbox", "delete"):
			return nil, errors.New("sandbox not found")
		case call.name == "git":
			return (OSRunner{}).Output(context.Background(), "git", call.args...)
		}
		return nil, nil
	}
	return runner
}

func TestPruneRunsRequiresAStatusSelector(t *testing.T) {
	app := &App{Store: Store{Base: t.TempDir()}, Runner: pruneRunner()}
	if _, err := app.PruneRuns(context.Background(), PruneRunsOptions{DryRun: true}); err == nil {
		t.Fatal("prune without a status selector did not fail")
	}
}

func TestPruneRunsDryRunSelectsOnlyFinishedRuns(t *testing.T) {
	app := &App{Store: Store{Base: t.TempDir()}, Runner: pruneRunner()}
	if err := app.Store.Ensure(); err != nil {
		t.Fatal(err)
	}
	savePruneRun(t, app, "run-working", "beta", StatusWorking)
	savePruneRun(t, app, "run-stopped", "alpha", StatusStopped)
	savePruneRun(t, app, "run-done", "beta", StatusCompleted)
	savePruneRun(t, app, "run-failed", "alpha", StatusFailed)

	result, err := app.PruneRuns(context.Background(), PruneRunsOptions{Completed: true, Failed: true, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.Deleted {
		t.Fatalf("dry-run flags = %+v", result)
	}
	want := []PrunedRunTarget{
		{ID: "run-failed", Name: "run-failed", Project: "alpha", Status: string(StatusFailed)},
		{ID: "run-done", Name: "run-done", Project: "beta", Status: string(StatusCompleted)},
	}
	if len(result.Runs) != len(want) {
		t.Fatalf("selected runs = %+v; want %+v", result.Runs, want)
	}
	for index, target := range want {
		if result.Runs[index] != target {
			t.Fatalf("selected run %d = %+v; want %+v", index, result.Runs[index], target)
		}
	}
	for _, id := range []string{"run-done", "run-failed", "run-working", "run-stopped"} {
		if _, err := app.Store.LoadRun(id); err != nil {
			t.Fatalf("dry-run removed %s: %v", id, err)
		}
	}
}

func TestPruneRunsDeletesSelectedRunsAndIsIdempotent(t *testing.T) {
	app := &App{Store: Store{Base: t.TempDir()}, Runner: pruneRunner()}
	if err := app.Store.Ensure(); err != nil {
		t.Fatal(err)
	}
	savePruneRun(t, app, "run-failed-one", "alpha", StatusFailed)
	savePruneRun(t, app, "run-failed-two", "alpha", StatusFailed)
	savePruneRun(t, app, "run-working", "alpha", StatusWorking)

	result, err := app.PruneRuns(context.Background(), PruneRunsOptions{Failed: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Deleted || result.DryRun || len(result.Runs) != 2 {
		t.Fatalf("delete result = %+v", result)
	}
	for _, id := range []string{"run-failed-one", "run-failed-two"} {
		if _, err := app.Store.LoadRun(id); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s record still present: %v", id, err)
		}
	}
	if _, err := app.Store.LoadRun("run-working"); err != nil {
		t.Fatalf("active run was removed: %v", err)
	}
	again, err := app.PruneRuns(context.Background(), PruneRunsOptions{Failed: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Runs) != 0 {
		t.Fatalf("second prune selected %+v; want none", again.Runs)
	}
}

func TestPruneRunsReportsPartialFailureAndKeepsTheRecord(t *testing.T) {
	app := &App{Store: Store{Base: t.TempDir()}}
	if err := app.Store.Ensure(); err != nil {
		t.Fatal(err)
	}
	savePruneRun(t, app, "run-failed-one", "alpha", StatusFailed)
	savePruneRun(t, app, "run-failed-two", "alpha", StatusFailed)
	runner := pruneRunner()
	base := runner.output
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "openshell" && containsCommand(call.args, "sandbox", "delete") && strings.Contains(strings.Join(call.args, " "), "sandbox-run-failed-two") {
			return nil, errors.New("business error: sandbox locked")
		}
		return base(call)
	}
	app.Runner = runner

	result, err := app.PruneRuns(context.Background(), PruneRunsOptions{Failed: true})
	if err == nil || !strings.Contains(err.Error(), "deleted 1 of 2 selected runs") || !strings.Contains(err.Error(), "run-failed-two") {
		t.Fatalf("partial failure = %v", err)
	}
	if !result.Deleted || len(result.Runs) != 2 || len(result.Failed) != 1 || result.Failed[0].ID != "run-failed-two" {
		t.Fatalf("partial result = %+v", result)
	}
	if _, err := app.Store.LoadRun("run-failed-two"); err != nil {
		t.Fatalf("failed cleanup removed the record: %v", err)
	}
	if _, err := app.Store.LoadRun("run-failed-one"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful cleanup kept the record: %v", err)
	}
}
