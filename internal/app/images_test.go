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
	if record.Tag != "localhost/openshell:codex-0.167.0-r4" || record.BaseDigest != "sha256:base123" || record.ImageID != "sha256:image-id" || record.Digest != "sha256:image-digest" {
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
	if err != nil || record.Tag != "localhost/openshell:claude-2.1.319-r4" {
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

func TestRecipeDockerfileInstallsPinnedToolset(t *testing.T) {
	record := ImageRecord{Agent: "codex", Version: "1.2.3", BaseDigest: "sha256:base123"}
	got := imageDockerfile(record, "@openai/codex")
	if recipeRevision != "4" {
		t.Fatalf("recipeRevision = %q; want 4", recipeRevision)
	}
	if !strings.Contains(got, "FROM docker.io/library/node@sha256:base123") {
		t.Fatalf("base image is not pinned by digest:\n%s", got)
	}
	if !strings.Contains(got, strings.Join(recipePackages, " ")) {
		t.Fatalf("Dockerfile lacks the fixed recipe toolset:\n%s", got)
	}
	for _, pkg := range []string{"curl", "jq", "ripgrep", "unzip", "less", "git-lfs", "procps"} {
		if !strings.Contains(got, " "+pkg) {
			t.Fatalf("Dockerfile lacks package %q:\n%s", pkg, got)
		}
	}
	// The recipe is a local-work toolset, not a forge client: gh and
	// openssh-client stay out so no sandbox gains forge reachability.
	for _, forbidden := range []string{"openssh-client", "cli.github.com", " gh ", " gh\n", "github-cli"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("Dockerfile contains %q:\n%s", forbidden, got)
		}
	}
}

func TestImageRemoveAndPruneRespectProfileAndActiveRunHolders(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	profileImage := "localhost/openshell:codex-1.0.0-r4"
	runImage := "localhost/openshell:claude-1.0.0-r4"
	freeImage := "localhost/openshell:opencode-2.0.0-r4"
	profile := testProfile("codex", "standard", "prompted")
	profile.Image = profileImage
	if err := store.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(Run{ID: "run-active", Name: "active-run", Image: runImage, Status: StatusWorking}); err != nil {
		t.Fatal(err)
	}
	terminalImage := "localhost/openshell:opencode-1.0.0-r4"
	for _, status := range []RunStatus{"finished", "stopped", "completed", "exited", "failed", "canceled"} {
		label := string(status)
		if err := store.SaveRun(Run{ID: "run-" + label, Name: "run-" + label, Image: terminalImage, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	raw := []byte(`[
		{"Id":"image-free","Names":["` + freeImage + `"],"Digest":"sha256:free","Arch":"arm64","Os":"linux","Labels":{"agentbox":"true","agentbox.agent":"opencode","agentbox.version":"2.0.0","agentbox.recipe":"4"}},
		{"Id":"image-profile","Names":["` + profileImage + `"],"Digest":"sha256:profile","Arch":"arm64","Os":"linux","Labels":{"agentbox":"true","agentbox.agent":"codex","agentbox.version":"1.0.0","agentbox.recipe":"4"}},
		{"Id":"image-run","Names":["` + runImage + `"],"Digest":"sha256:run","Arch":"arm64","Os":"linux","Labels":{"agentbox":"true","agentbox.agent":"claude","agentbox.version":"1.0.0","agentbox.recipe":"4"}}
		,{"Id":"image-terminal","Names":["` + terminalImage + `"],"Digest":"sha256:terminal","Arch":"arm64","Os":"linux","Labels":{"agentbox":"true","agentbox.agent":"opencode","agentbox.version":"1.0.0","agentbox.recipe":"4"}}
			,{"Id":"image-dangling","Names":[],"Labels":{"agentbox":"true","agentbox.agent":"opencode","agentbox.version":"2.1.0","agentbox.recipe":"4"}}
			,{"Id":"image-legacy-dangling","Names":[],"Labels":{"ai-sandbox.spike":"true","ai-sandbox.agent":"opencode","ai-sandbox.version":"2.0.0","ai-sandbox.recipe":"3"}}
			,{"Id":"image-unlabeled-dangling","Names":[],"Labels":{}}
	]`)
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name == "podman" && len(call.args) > 0 && call.args[0] == "images" {
			return raw, nil
		}
		return nil, nil
	}}
	app := App{Store: store, Runner: runner}

	if err := app.ImageRemove(context.Background(), profileImage); err == nil || !strings.Contains(err.Error(), "profile unit-profile") {
		t.Fatalf("profile-held removal error = %v", err)
	}
	if err := app.ImageRemove(context.Background(), runImage); err == nil || !strings.Contains(err.Error(), "run active-run (working)") {
		t.Fatalf("run-held removal error = %v", err)
	}
	usage, err := app.ImageUsage()
	if err != nil {
		t.Fatal(err)
	}
	if len(usage[freeImage]) != 0 || len(usage[profileImage]) != 1 || len(usage[runImage]) != 1 {
		t.Fatalf("usage = %#v", usage)
	}
	if len(usage[terminalImage]) != 0 {
		t.Fatalf("terminal failed/canceled runs should not hold image %s: %#v", terminalImage, usage[terminalImage])
	}

	removed, err := app.ImagePrune(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 3 {
		t.Fatalf("pruned = %#v; want two named images and one dangling image", removed)
	}
	for _, reference := range []string{freeImage, terminalImage, "image-dangling"} {
		found := false
		for _, got := range removed {
			if got == reference {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("pruned = %#v; want %s", removed, reference)
		}
	}
	if err := app.ImageRemove(context.Background(), freeImage); err != nil {
		t.Fatalf("unreferenced removal failed: %v", err)
	}
	for _, call := range runner.outputs {
		if call.name == "podman" && len(call.args) >= 3 && call.args[0] == "image" && call.args[1] == "rm" && call.args[2] == "image-dangling" {
			return
		}
	}
	t.Fatalf("dangling AgentBox image was not removed by image ID: %#v", runner.outputs)
}

func TestImageRemoveProtectsEveryNamedAlias(t *testing.T) {
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	profile := testProfile("codex", "standard", "prompted")
	profile.Image = "quay.io/team/openshell:codex-1.0.0-r4"
	if err := store.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`[
		{"Id":"image-alias","Names":["localhost/openshell:codex-1.0.0-r4","quay.io/team/openshell:codex-1.0.0-r4"],"Digest":"sha256:alias","Arch":"arm64","Os":"linux","Labels":{"agentbox":"true","agentbox.agent":"codex","agentbox.version":"1.0.0","agentbox.recipe":"4"}}
	]`)
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		if call.name == "podman" && len(call.args) > 0 && call.args[0] == "images" {
			return raw, nil
		}
		return nil, nil
	}}
	app := App{Store: store, Runner: runner}

	err := app.ImageRemove(context.Background(), "localhost/openshell:codex-1.0.0-r4")
	if err == nil || !strings.Contains(err.Error(), "profile unit-profile") {
		t.Fatalf("alias-held removal error = %v", err)
	}
	for _, call := range runner.outputs {
		if call.name == "podman" && len(call.args) > 1 && call.args[0] == "image" && call.args[1] == "rm" {
			t.Fatalf("held image was removed: %#v", call)
		}
	}
}
