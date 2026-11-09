package tui

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/dmartinol/agentbox/internal/app"
	"github.com/rivo/tview"
)

type runFormRunner struct{}

func (runFormRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	switch name {
	case "podman":
		if len(args) > 0 && args[0] == "images" {
			return []byte(`[{"Id":"image-1","Names":["localhost/agentbox-codex:1.0.0-r3"],"Labels":{"agentbox":"true","agentbox.agent":"codex","agentbox.version":"1.0.0","agentbox.recipe":"3"}}]`), nil
		}
	case "openshell":
		if len(args) > 1 && args[0] == "gateway" && args[1] == "list" {
			return []byte(`[{"name":"openshell","active":true}]`), nil
		}
	}
	return []byte(`{}`), nil
}

func (runFormRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func TestRunFormShowsReadOnlyProfileValuesAndNoneOverrides(t *testing.T) {
	t.Setenv("ENMAAS_KEY", "")
	store := app.Store{Base: t.TempDir()}
	for _, profile := range []app.Profile{
		{Version: 1, Name: "alpha", Agent: "codex", Image: "image-a", Mode: "standard", Model: "model-a", Provider: "provider-a", Policy: "/tmp/alpha.yaml", Approval: "prompted"},
		{Version: 1, Name: "beta", Agent: "claude", Image: "image-b", Mode: "enmaas", Model: "model-b", Provider: "provider-b", Policy: "/tmp/beta.yaml", Approval: "unattended"},
	} {
		if err := store.SaveProfile(profile); err != nil {
			t.Fatal(err)
		}
	}
	ui := &UI{ctx: context.Background(), app: &app.App{Store: store, Runner: runFormRunner{}}}
	primitive := ui.runForm()
	form, ok := primitive.(*tview.Form)
	if !ok {
		t.Fatalf("run form type = %T; want *tview.Form", primitive)
	}

	details, ok := form.GetFormItemByLabel("Profile values (RO)").(*tview.TextView)
	if !ok {
		t.Fatal("missing read-only profile values")
	}
	text := details.GetText(true)
	for _, want := range []string{"Agent: codex", "Image: image-a", "Mode: standard", "Model: model-a", "Provider: provider-a", "Approval: prompted"} {
		if !strings.Contains(text, want) {
			t.Fatalf("profile summary %q does not contain %q", text, want)
		}
	}

	for _, label := range []string{"Image override", "Mode override", "Model override", "Approval override"} {
		dropdown, ok := form.GetFormItemByLabel(label).(*tview.DropDown)
		if !ok {
			t.Fatalf("%s is not a dropdown", label)
		}
		index, value := dropdown.GetCurrentOption()
		if index != 0 || value != "<None>" {
			t.Fatalf("%s initial selection = %d %q; want 0 %q", label, index, value, "<None>")
		}
	}

	override := form.GetFormItemByLabel("Override profile").(*tview.Checkbox)
	if override.IsChecked() {
		t.Fatal("Override profile must be disabled initially")
	}
	image := form.GetFormItemByLabel("Image override").(*tview.DropDown)
	form.Focus(func(tview.Primitive) {})
	form.SetFocus(form.GetFormItemIndex("Image override"))
	form.Focus(func(tview.Primitive) {})
	if image.HasFocus() {
		t.Fatal("disabled override field received focus")
	}
	override.SetChecked(true)
	form.SetFocus(form.GetFormItemIndex("Image override"))
	form.Focus(func(tview.Primitive) {})
	if !image.HasFocus() {
		t.Fatal("enabled override field did not receive focus")
	}

	mode := form.GetFormItemByLabel("Mode override").(*tview.DropDown)
	mode.SetCurrentOption(2)
	if _, value := mode.GetCurrentOption(); value != "enmaas" {
		t.Fatalf("mode override = %q; want enmaas", value)
	}

	profile := form.GetFormItemByLabel("Profile").(*tview.DropDown)
	if index, value := profile.GetCurrentOption(); index != 0 || value != "alpha" {
		t.Fatalf("profile initial selection = %d %q; want alpha", index, value)
	}
	profile.SetCurrentOption(1)
	text = details.GetText(true)
	for _, want := range []string{"Agent: claude", "Image: image-b", "Mode: enmaas", "Model: model-b", "Approval: unattended"} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated profile summary %q does not contain %q", text, want)
		}
	}
	if index, value := mode.GetCurrentOption(); index != 0 || value != "<None>" {
		t.Fatalf("profile change did not reset mode override: %d %q", index, value)
	}
}

func TestProfileEditFormIsPrefilled(t *testing.T) {
	t.Setenv("ENMAAS_KEY", "")
	store := app.Store{Base: t.TempDir()}
	profile := app.Profile{
		Version: 1, Name: "dev", Agent: "claude", Image: "image-dev",
		Mode: "enmaas", Model: "model-dev", Provider: "provider-dev",
		Policy: "/tmp/dev-policy.yaml", Approval: "unattended", Args: []string{"--flag"},
	}
	if err := store.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	ui := &UI{ctx: context.Background(), app: &app.App{Store: store, Runner: runFormRunner{}}}
	primitive := ui.profileEditForm(profile)
	form, ok := primitive.(*tview.Form)
	if !ok {
		t.Fatalf("edit form type = %T; want *tview.Form", primitive)
	}
	if title := form.GetTitle(); title != " Edit profile: dev " {
		t.Fatalf("edit form title = %q", title)
	}
	if got := form.GetFormItemByLabel("Name").(*tview.InputField).GetText(); got != "dev" {
		t.Fatalf("name = %q; want dev", got)
	}
	if got := form.GetFormItemByLabel("Image").(*tview.InputField).GetText(); got != "image-dev" {
		t.Fatalf("image = %q", got)
	}
	if got := form.GetFormItemByLabel("Model").(*tview.InputField).GetText(); got != "model-dev" {
		t.Fatalf("model = %q", got)
	}
	if got := form.GetFormItemByLabel("Provider").(*tview.InputField).GetText(); got != "provider-dev" {
		t.Fatalf("provider = %q", got)
	}
	if got := form.GetFormItemByLabel("Policy").(*tview.InputField).GetText(); got != "/tmp/dev-policy.yaml" {
		t.Fatalf("policy = %q", got)
	}
	for label, want := range map[string]string{"Agent": "claude", "Mode": "enmaas", "Approval": "unattended"} {
		if _, got := form.GetFormItemByLabel(label).(*tview.DropDown).GetCurrentOption(); got != want {
			t.Fatalf("%s = %q; want %q", label, got, want)
		}
	}

}
