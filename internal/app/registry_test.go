package app

import (
	"context"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRegistryStoreValidation(t *testing.T) {
	store := Store{Base: t.TempDir()}
	registry, err := store.LoadRegistry()
	if err != nil || registry.Reference != "" {
		t.Fatalf("default registry = %+v, %v", registry, err)
	}
	for _, invalid := range []string{"", "quay.io", "https://quay.io/team/agentbox", "quay.io/team/agentbox:latest", "quay.io/team/agentbox@sha256:abc"} {
		if err := store.SaveRegistry(Registry{Reference: invalid}); err == nil {
			t.Fatalf("accepted invalid registry reference %q", invalid)
		}
	}
	if err := store.SaveRegistry(Registry{Reference: "quay.io/team/agentbox"}); err != nil {
		t.Fatal(err)
	}
	registry, err = store.LoadRegistry()
	if err != nil || registry.NormalizedReference() != "quay.io/team/agentbox" {
		t.Fatalf("loaded registry = %+v, %v", registry, err)
	}
	if err := store.ClearRegistry(); err != nil {
		t.Fatal(err)
	}
	registry, err = store.LoadRegistry()
	if err != nil || registry.Reference != "" {
		t.Fatalf("cleared registry = %+v, %v", registry, err)
	}
}

func TestRegistryDiscoverParsesPodmanTags(t *testing.T) {
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if !reflect.DeepEqual(call.args, []string{"search", "--list-tags", "--format", "json", "quay.io/team/agentbox"}) {
			t.Fatalf("unexpected podman args: %#v", call.args)
		}
		return []byte(`[
			{"Name":"quay.io/team/openshell","Tag":"codex-0.167.0-r3-linux-arm64","Description":""},
			{"Name":"quay.io/team/openshell","Tag":"claude-2.1.319-r3","Description":""}
		]`), nil
	}}
	store := Store{Base: t.TempDir()}
	if err := store.SaveRegistry(Registry{Reference: "quay.io/team/agentbox"}); err != nil {
		t.Fatal(err)
	}
	app := App{Store: store, Runner: runner}
	images, err := app.RegistryDiscover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 {
		t.Fatalf("images = %#v; want 2", images)
	}
	if images[0].Agent != "claude" || images[0].Version != "2.1.319" || images[0].Recipe != 3 || images[0].Reference != "quay.io/team/openshell:claude-2.1.319-r3" {
		t.Fatalf("unexpected claude image: %+v", images[0])
	}
	if images[1].Agent != "codex" || images[1].Version != "0.167.0" || images[1].Recipe != 3 || images[1].OS != "linux" || images[1].Arch != "arm64" || images[1].Reference != "quay.io/team/openshell:codex-0.167.0-r3-linux-arm64" {
		t.Fatalf("unexpected codex image: %+v", images[1])
	}
}

func TestRegistryFileContainsNoSecrets(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.SaveRegistry(Registry{Reference: "quay.io/team/agentbox"}); err != nil {
		t.Fatal(err)
	}
	b, err := yaml.Marshal(Registry{Reference: "quay.io/team/agentbox"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("empty registry YAML")
	}
}
