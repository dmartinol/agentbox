package app

import (
	"os"
	"path/filepath"
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
