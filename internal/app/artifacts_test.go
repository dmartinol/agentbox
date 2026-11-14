package app

import (
	"context"
	"errors"
	"testing"
)

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
