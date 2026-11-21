package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
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

func TestProjectRepoStatusPrefersTheBuildMatchingTheCurrentManifest(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	staleBase := fmt.Sprintf("localhost/openshell:codex-0.168.0-r%d", f.recipe)
	stale := f.repoRecord(t, staleBase, "0.169.0", f.digest())
	current := f.repoRecord(t, f.baseRef, "0.169.0", f.digest())
	stale["Id"] = "repo-stale"
	current["Id"] = "repo-current"
	app, _ := f.app(t, f.baseRecord("0.169.0"), stale, current)

	status := app.ProjectRepoStatus(context.Background(), f.root)
	var codex *ProjectWorkerImageStatus
	for index := range status.WorkerImages {
		if status.WorkerImages[index].Agent == "codex" {
			codex = &status.WorkerImages[index]
		}
	}
	if codex == nil || codex.Status != "current" || codex.Reference != f.repoTag(t, f.baseRef) {
		t.Fatalf("codex worker status = %+v, want the tag matching the current manifest", codex)
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

func TestProjectRepoStatusSkipsInventoryWithoutUsableManifestOrIdentity(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name == "git" && commandHasArg(call, "remote") {
			return nil, errors.New("origin unavailable")
		}
		if call.name == "podman" {
			t.Errorf("image inventory called without repository identity: %#v", call)
		}
		return nil, nil
	}}
	app := &App{Store: f.store, Runner: runner}

	status := app.ProjectRepoStatus(context.Background(), f.root)
	if status.ManifestDigest != f.digest() {
		t.Fatalf("manifest digest = %q, want %q", status.ManifestDigest, f.digest())
	}
	for _, worker := range status.WorkerImages {
		if worker.Status != "unknown" {
			t.Fatalf("worker status = %+v, want unknown", status.WorkerImages)
		}
	}
	if !sort.StringsAreSorted(status.Warnings) {
		t.Fatalf("warnings are not sorted: %v", status.Warnings)
	}

	f = newRunImageFixture(t, "")
	app, runner = f.app(t)
	status = app.ProjectRepoStatus(context.Background(), f.root)
	if status.ManifestPresent || status.ManifestDigest != "" {
		t.Fatalf("absent manifest reported as present: %+v", status)
	}
	for _, worker := range status.WorkerImages {
		if worker.Status != "missing" {
			t.Fatalf("worker status = %+v, want missing", status.WorkerImages)
		}
	}
	for _, call := range runner.outputs {
		if call.name == "podman" {
			t.Fatalf("image inventory called without a manifest: %#v", call)
		}
	}
}

func TestProjectRepoStatusInvalidManifestDoesNotReportCurrentImage(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name == "git" && commandHasArg(call, "remote") {
			return nil, errors.New("origin unavailable")
		}
		return nil, nil
	}}
	app := &App{Store: f.store, Runner: runner}
	if err := writeRepoManifest(t, f.root, "schema: 1\nunknown_field: true\n"); err != nil {
		t.Fatal(err)
	}

	status := app.ProjectRepoStatus(context.Background(), f.root)
	if status.ManifestDigest != "" {
		t.Fatalf("invalid manifest digest = %q, want empty", status.ManifestDigest)
	}
	for _, worker := range status.WorkerImages {
		if worker.Status != "missing" {
			t.Fatalf("worker status = %+v, want missing", status.WorkerImages)
		}
	}
	if len(status.Warnings) != 2 || !strings.Contains(status.Warnings[0], "repository identity unavailable") || !strings.Contains(status.Warnings[1], "invalid") {
		t.Fatalf("warnings = %v, want sorted identity and invalid-manifest warnings", status.Warnings)
	}
	for _, call := range runner.outputs {
		if call.name == "podman" {
			t.Fatalf("image inventory called with an invalid manifest: %#v", call)
		}
	}
}
