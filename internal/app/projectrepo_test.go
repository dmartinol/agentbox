package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestProjectRepoStatusReportsRepositoryState(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app, _ := f.app(t, f.baseRecord("0.169.0"), f.repoRecord(t, f.baseRef, "0.169.0", f.digest()))

	status := app.ProjectRepoStatus(context.Background(), f.root)
	if !status.ManifestPresent || status.ManifestPath != RepoConfigPath(f.root) || status.ManifestDigest != f.digest() {
		t.Fatalf("manifest status = %+v", status)
	}
	if status.FragmentPath != f.fragmentPath() || status.FragmentDigest != f.fragmentDigest(t) || status.FragmentStatus != ProjectRepoFragmentCurrent {
		t.Fatalf("fragment status = %+v", status)
	}
	if !reflect.DeepEqual(status.HostRules, []RepoHostRule{{Host: "proxy.golang.org", Port: 443, Binaries: []string{"/usr/local/go/bin/go"}}}) {
		t.Fatalf("host rules = %+v", status.HostRules)
	}
	var codex *ProjectWorkerImageStatus
	for index := range status.WorkerImages {
		if status.WorkerImages[index].Agent == "codex" {
			codex = &status.WorkerImages[index]
		}
	}
	if codex == nil || codex.Status != "current" || codex.Reference != f.repoTag(t, f.baseRef) {
		t.Fatalf("codex worker status = %+v", codex)
	}
}

func TestProjectRepoStatusReportsDriftAndInventoryErrors(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app, _ := f.app(t, f.baseRecord("0.169.0"), f.repoRecord(t, f.baseRef, "0.169.0", "sha256:old"))

	status := app.ProjectRepoStatus(context.Background(), f.root)
	if status.FragmentStatus != ProjectRepoFragmentCurrent || status.WorkerImages[1].Status != "stale" {
		t.Fatalf("drift status = %+v", status)
	}
	if len(status.Warnings) != 0 {
		t.Fatalf("warnings = %v", status.Warnings)
	}

	app = &App{Store: f.store, Runner: &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name == "git" && commandHasArg(call, "remote") {
			return []byte("git@github.com:owner/repo.git"), nil
		}
		return nil, errors.New("inventory unavailable")
	}}}
	status = app.ProjectRepoStatus(context.Background(), f.root)
	for _, worker := range status.WorkerImages {
		if worker.Status != "unknown" {
			t.Fatalf("worker status = %+v", status.WorkerImages)
		}
	}
}
