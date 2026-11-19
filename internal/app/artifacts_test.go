package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestParseLocalImagesResolvesRepositoryBase(t *testing.T) {
	records := []map[string]any{
		{
			"Id":     "base",
			"Names":  []string{"localhost/openshell:codex-1.2.3-r4"},
			"Labels": map[string]string{"agentbox.agent": "codex", "agentbox.version": "1.2.3", "agentbox.recipe": "4"},
		},
		{
			"Id":    "repo",
			"Names": []string{"localhost/openshell:repo-example-codex-abcd1234"},
			"Labels": map[string]string{
				"agentbox.agent":           "codex",
				"agentbox.version":         "1.2.3",
				"agentbox.recipe":          "4",
				"agentbox.worker":          "true",
				"agentbox.repo":            "github.com/example/demo",
				"agentbox.manifest-digest": "sha256:manifest",
			},
		},
	}
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	images, err := parseLocalImages(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 {
		t.Fatalf("images = %#v", images)
	}
	var repo ImageArtifact
	for _, image := range images {
		if image.Worker {
			repo = image
		}
	}
	if repo.Reference != "localhost/openshell:repo-example-codex-abcd1234" {
		t.Fatalf("repo image = %#v", repo)
	}
	if repo.BaseReference != "localhost/openshell:codex-1.2.3-r4" {
		t.Fatalf("base reference = %q; want the matching local base", repo.BaseReference)
	}
	if repo.Repo != "github.com/example/demo" || repo.ManifestDigest != "sha256:manifest" {
		t.Fatalf("repo provenance = %#v", repo)
	}
}

func TestParseLocalImagesPrefersRecordedBaseLabel(t *testing.T) {
	data := []byte(`[
		{"Id":"base","Names":["localhost/openshell:codex-1.2.3-r4"],"Labels":{"agentbox.agent":"codex","agentbox.version":"1.2.3","agentbox.recipe":"4"}},
		{"Id":"repo","Names":["localhost/openshell:repo-example-codex-abcd1234"],"Labels":{"agentbox.agent":"codex","agentbox.version":"1.2.3","agentbox.recipe":"4","agentbox.worker":"true","agentbox.base":"quay.io/team/openshell:codex-1.2.3-r4"}}
	]`)
	images, err := parseLocalImages(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, image := range images {
		if image.Worker && image.BaseReference != "quay.io/team/openshell:codex-1.2.3-r4" {
			t.Fatalf("recorded base label was not used: %#v", image)
		}
	}
}

func TestImageArtifactRemoteNaming(t *testing.T) {
	img := ImageArtifact{Agent: "codex", Version: "0.167.0", Recipe: 3, OS: "linux", Arch: "arm64"}
	if got := img.RemoteTag(); got != "codex-0.167.0-r3-linux-arm64" {
		t.Fatalf("RemoteTag = %q", got)
	}
	if got := RemoteReference("quay.io/dmartino/openshell", img); got != "quay.io/dmartino/openshell:codex-0.167.0-r3-linux-arm64" {
		t.Fatalf("RemoteReference = %q", got)
	}
	parsed, ok := ParseRemoteTag("codex-0.167.0-r3-linux-arm64")
	if !ok || parsed.Agent != "codex" || parsed.Version != "0.167.0" || parsed.Recipe != 3 || parsed.OS != "linux" || parsed.Arch != "arm64" {
		t.Fatalf("parsed = %+v, %v", parsed, ok)
	}
	parsed, ok = ParseRemoteTag("claude-2.1.319-r3")
	if !ok || parsed.Agent != "claude" || parsed.OS != "" || parsed.Arch != "" {
		t.Fatalf("architecture-free tag parsed = %+v, %v", parsed, ok)
	}
	if _, ok := ParseRemoteTag("v1.0.0"); ok {
		t.Fatal("unrelated tag was accepted")
	}
}

func TestImageRemoveConflictExposesDependents(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	reference := "localhost/openshell:codex-1.0.0-r1"
	if err := store.SaveProfile(Profile{Version: 1, Name: "codex-default", Agent: "codex", Image: reference, Mode: "standard", Provider: "provider", Policy: "/tmp/policy.yaml", Approval: "prompted"}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{output: func(recordedCommand) ([]byte, error) {
		return []byte(`[{"Id":"image","Names":["` + reference + `"],"Labels":{"agentbox":"true","agentbox.agent":"codex","agentbox.version":"1.0.0","agentbox.recipe":"1"}}]`), nil
	}}
	app := App{Store: store, Runner: runner}
	err := app.ImageRemove(context.Background(), reference)
	var conflict *ConflictError
	if err == nil || !errors.As(err, &conflict) {
		t.Fatalf("error = %v; want conflict", err)
	}
	if len(conflict.Dependents) != 1 || conflict.Dependents[0] != "profile codex-default" {
		t.Fatalf("dependents = %#v", conflict.Dependents)
	}
}
