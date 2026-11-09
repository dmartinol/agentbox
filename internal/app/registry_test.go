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
			{"Name":"quay.io/team/agentbox","Tag":"v0.1.0","Description":""},
			{"Name":"quay.io/team/agentbox","Tag":"v0.2.0","Description":""}
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
	if want := []string{"quay.io/team/agentbox:v0.1.0", "quay.io/team/agentbox:v0.2.0"}; !reflect.DeepEqual(images, want) {
		t.Fatalf("images = %#v; want %#v", images, want)
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
