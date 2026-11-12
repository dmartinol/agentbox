package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentPackageAndVersionValidation(t *testing.T) {
	for agent, want := range map[string]string{
		"claude":   "@anthropic-ai/claude-code",
		"codex":    "@openai/codex",
		"opencode": "@opencode/cli",
	} {
		got, err := agentPackage(agent)
		if err != nil || got != want {
			t.Fatalf("agentPackage(%q) = %q, %v; want %q", agent, got, err, want)
		}
	}
	if _, err := agentPackage("unknown"); err == nil {
		t.Fatal("unknown agent was accepted")
	}
	if !exactVersion.MatchString("2.2.5") || exactVersion.MatchString("latest") || exactVersion.MatchString("2.2") {
		t.Fatal("exact version expression accepted an invalid release")
	}
}

func TestImageVersionUsesAgentChannelAndRejectsWrongOpenCodeMajor(t *testing.T) {
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name != "npm" || len(call.args) < 3 || call.args[0] != "view" {
			t.Fatalf("unexpected lookup command: %#v", call)
		}
		if call.args[1] != "@opencode/cli" {
			t.Fatalf("wrong package: %#v", call.args)
		}
		return []byte(`"2.2.5"`), nil
	}}
	app := App{Runner: runner}
	version, err := app.ImageVersion(context.Background(), "opencode")
	if err != nil || version != "2.2.5" {
		t.Fatalf("ImageVersion = %q, %v", version, err)
	}
	runner.output = func(recordedCommand) ([]byte, error) { return []byte(`"1.9.0"`), nil }
	if _, err := app.ImageVersion(context.Background(), "opencode"); err == nil {
		t.Fatal("non-v2 OpenCode release was accepted")
	}
	runner.output = func(recordedCommand) ([]byte, error) { return nil, errors.New("npm unavailable") }
	if _, err := app.ImageVersion(context.Background(), "codex"); err == nil || !strings.Contains(err.Error(), "npm unavailable") {
		t.Fatalf("dependency failure not propagated: %v", err)
	}
}

func TestImageBuildUsesPinnedDigestAndRecipeTag(t *testing.T) {
	runner := &fakeRunner{
		output: func(call recordedCommand) ([]byte, error) {
			if commandHasArg(call, "docker.io/library/node:22-bookworm-slim") {
				return []byte("sha256:base123\n"), nil
			}
			return []byte("sha256:image-id sha256:image-digest\n"), nil
		},
		stream: func(call recordedCommand) error {
			if call.name == "podman" && len(call.args) > 0 && call.args[0] == "build" {
				contextDir := call.args[len(call.args)-1]
				dockerfile, err := os.ReadFile(filepath.Join(contextDir, "Dockerfile"))
				if err != nil {
					return err
				}
				if !strings.Contains(string(dockerfile), "FROM docker.io/library/node@sha256:base123") {
					return errors.New("Dockerfile did not pin the resolved base digest")
				}
				for _, want := range []string{"ENV LANG=C.UTF-8", "ENV LC_ALL=C.UTF-8"} {
					if !strings.Contains(string(dockerfile), want) {
						return fmt.Errorf("Dockerfile does not set %s", want)
					}
				}
			}
			return nil
		},
	}
	app := App{Runner: runner}
	record, err := app.ImageBuild(context.Background(), "codex", "0.167.0", false)
	if err != nil {
		t.Fatal(err)
	}
	if record.Tag != "localhost/openshell:codex-0.167.0-r3" || record.BaseDigest != "sha256:base123" || record.ImageID != "sha256:image-id" || record.Digest != "sha256:image-digest" {
		t.Fatalf("unexpected build record: %+v", record)
	}
	if len(runner.streams) != 2 || !commandHasArg(runner.streams[1], record.Tag) {
		t.Fatalf("expected base pull and tagged build, got %#v", runner.streams)
	}
}

func TestImageBuildPreviewAndRegistryReferences(t *testing.T) {
	runner := &fakeRunner{}
	app := App{Runner: runner}
	record, err := app.ImageBuild(context.Background(), "claude", "2.1.319", true)
	if err != nil || record.Tag != "localhost/openshell:claude-2.1.319-r3" {
		t.Fatalf("unexpected preview: %+v, %v", record, err)
	}
	if len(runner.outputs)+len(runner.streams) != 0 {
		t.Fatalf("preview invoked commands: %#v %#v", runner.outputs, runner.streams)
	}
	if err := app.ImagePush(context.Background(), record.Tag, "registry.example/team/claude:2.1.319", false); err != nil {
		t.Fatal(err)
	}
	if len(runner.outputs) != 2 || runner.outputs[0].args[0] != "image" || runner.outputs[1].args[0] != "tag" {
		t.Fatalf("push did not inspect and tag the explicit reference: %#v", runner.outputs)
	}
	if len(runner.streams) != 1 || runner.streams[0].name != "podman" || runner.streams[0].args[0] != "push" || runner.streams[0].args[1] != "registry.example/team/claude:2.1.319" {
		t.Fatalf("push used an unexpected command: %#v", runner.streams)
	}
	if err := app.ImagePull(context.Background(), "registry.example/team/claude:2.1.319", false); err != nil {
		t.Fatal(err)
	}
	if got := runner.streams[len(runner.streams)-1].args; len(got) != 2 || got[0] != "pull" || got[1] != "registry.example/team/claude:2.1.319" {
		t.Fatalf("pull used an unexpected reference: %#v", got)
	}
	if err := app.ImagePush(context.Background(), "", "", false); err == nil {
		t.Fatal("empty push references were accepted")
	}
}

func TestImageListFiltersCurrentAndLegacyLabels(t *testing.T) {
	raw := []byte(`[
		{"Id":"new","Labels":{"agentbox":"true"}},
		{"Id":"old","Labels":{"ai-sandbox.spike":"true"}},
		{"Id":"other","Labels":{"other":"true"}}
	]`)
	runner := &fakeRunner{output: func(recordedCommand) ([]byte, error) { return raw, nil }}
	app := App{Runner: runner}
	got, err := app.ImageList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.outputs) != 1 || runner.outputs[0].name != "podman" || !commandHasArg(runner.outputs[0], "--format") {
		t.Fatalf("image list command had unexpected shape: %#v", runner.outputs)
	}
	for _, id := range []string{"new", "old"} {
		if !strings.Contains(string(got), `"Id":"`+id+`"`) {
			t.Fatalf("filtered images missing %s: %s", id, got)
		}
	}
	if strings.Contains(string(got), `"Id":"other"`) {
		t.Fatalf("unrelated image was included: %s", got)
	}
	if len(filteredImages(t, got)) != 2 {
		t.Fatalf("filtered images = %#v; want new and old records", filteredImages(t, got))
	}
}

func TestImageListDeduplicatesPodmanRowsForOneImage(t *testing.T) {
	record := `{"Id":"image-1","Names":["quay.io/team/openshell:codex-1.0.0-r3","localhost/openshell:codex-1.0.0-r3"],"Labels":{"agentbox":"true","agentbox.agent":"codex","agentbox.version":"1.0.0","agentbox.recipe":"3"}}`
	raw := []byte("[" + record + "," + record + "]")
	runner := &fakeRunner{output: func(recordedCommand) ([]byte, error) { return raw, nil }}
	app := App{Runner: runner}
	got, err := app.ImageList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var images []struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(got, &images); err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0].ID != "image-1" {
		t.Fatalf("images = %#v; want one image-1 record", images)
	}
}

func TestLocalImagesReturnsOneCanonicalReferencePerImage(t *testing.T) {
	raw := []byte(`[
		{
			"Id":"image-1",
			"Names":["quay.io/team/openshell:codex-1.0.0-r3-linux-arm64","localhost/openshell:codex-1.0.0-r3"],
			"Digest":"sha256:image-1",
			"Arch":"arm64",
			"Os":"linux",
			"Labels":{"agentbox":"true","agentbox.agent":"codex","agentbox.version":"1.0.0","agentbox.recipe":"3"}
		}
	]`)
	runner := &fakeRunner{output: func(recordedCommand) ([]byte, error) { return raw, nil }}
	app := App{Runner: runner}
	images, err := app.LocalImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 {
		t.Fatalf("images = %#v; want one image", images)
	}
	if images[0].Reference != "localhost/openshell:codex-1.0.0-r3" {
		t.Fatalf("reference = %q; want the canonical local tag", images[0].Reference)
	}
}

func filteredImages(t *testing.T, data []byte) []struct {
	ID string `json:"Id"`
} {
	t.Helper()
	var images []struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(data, &images); err != nil {
		t.Fatal(err)
	}
	return images
}
