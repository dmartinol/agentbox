package tui

import (
	"context"
	"io"
	"path/filepath"
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
	worktree := true
	if err := store.SaveProject(app.Project{Version: 1, Name: "repo", Root: t.TempDir(), Worktree: &worktree}); err != nil {
		t.Fatal(err)
	}
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

func TestImageBuildFormIncludesDiscoverLatestVersion(t *testing.T) {
	ui := &UI{}
	form, ok := ui.imageBuildForm().(*tview.Form)
	if !ok {
		t.Fatal("build form is not a tview form")
	}
	if form.GetFormItemByLabel("Exact version") == nil {
		t.Fatal("exact version field is missing")
	}
	help, ok := form.GetFormItemByLabel("Version help").(*tview.TextView)
	if !ok || !strings.Contains(help.GetText(true), "Discover latest version") {
		t.Fatalf("version help does not explain discovery: %q", help.GetText(true))
	}
	labels := make([]string, 0, form.GetButtonCount())
	for i := 0; i < form.GetButtonCount(); i++ {
		labels = append(labels, form.GetButton(i).GetLabel())
	}
	found := false
	for _, label := range labels {
		if label == "Discover latest version" {
			found = true
		}
	}
	if !found {
		t.Fatalf("build form buttons = %v; missing Discover latest version", labels)
	}
}

func TestRunFormUsesProjectInsteadOfSourceCheckout(t *testing.T) {
	t.Setenv("ENMAAS_KEY", "")
	store := app.Store{Base: t.TempDir()}
	worktree := false
	if err := store.SaveProject(app.Project{Version: 1, Name: "repo", Root: t.TempDir(), Worktree: &worktree}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(app.Profile{Version: 1, Name: "dev", Agent: "codex", Image: "image", Mode: "standard", Provider: "provider", Policy: "/tmp/policy.yaml", Approval: "prompted"}); err != nil {
		t.Fatal(err)
	}
	ui := &UI{ctx: context.Background(), app: &app.App{Store: store, Runner: runFormRunner{}}}
	form, ok := ui.runForm().(*tview.Form)
	if !ok {
		t.Fatal("run form is not a tview form")
	}
	project := form.GetFormItemByLabel("Project").(*tview.DropDown)
	if index, value := project.GetCurrentOption(); index != 0 || value != "repo" {
		t.Fatalf("project = %d %q; want repo", index, value)
	}
	if form.GetFormItemByLabel("Source checkout") != nil {
		t.Fatal("source checkout should be derived from the project")
	}
	details := form.GetFormItemByLabel("Project details").(*tview.TextView).GetText(true)
	if !strings.Contains(details, "Isolation: direct checkout") {
		t.Fatalf("project details do not show isolation: %q", details)
	}
}

func TestProjectFormDefaultsToWorktreeIsolation(t *testing.T) {
	ui := &UI{app: &app.App{Store: app.Store{Base: t.TempDir()}}}
	form, ok := ui.projectForm().(*tview.Form)
	if !ok {
		t.Fatal("project form is not a tview form")
	}
	checkbox, ok := form.GetFormItemByLabel("Worktree isolation").(*tview.Checkbox)
	if !ok || !checkbox.IsChecked() {
		t.Fatal("new projects should default to worktree isolation")
	}
	help := form.GetFormItemByLabel("Isolation help").(*tview.TextView).GetText(true)
	if !strings.Contains(help, "one active run") {
		t.Fatalf("isolation help is unclear: %q", help)
	}
}

func TestSettingsPageExposesGlobalWorktreeSettings(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	ui := &UI{app: &app.App{Store: store}}
	form, ok := ui.settingsPage().(*tview.Form)
	if !ok {
		t.Fatal("settings page is not a tview form")
	}
	root := form.GetFormItemByLabel("Worktree root").(*tview.InputField).GetText()
	if root != filepath.Join(store.Base, "worktrees") {
		t.Fatalf("worktree root = %q", root)
	}
	if prefix := form.GetFormItemByLabel("Branch prefix").(*tview.InputField).GetText(); prefix != "agentbox" {
		t.Fatalf("branch prefix = %q", prefix)
	}
}

func TestImageNamesUsesCanonicalLocalReferences(t *testing.T) {
	ui := &UI{
		ctx: context.Background(),
		app: &app.App{Runner: runFormRunner{}},
	}
	names := ui.imageNames()
	if len(names) != 1 || names[0] != "localhost/agentbox-codex:1.0.0-r3" {
		t.Fatalf("image names = %#v; want one canonical local reference", names)
	}
}

func TestPoliciesPageRegistersReachableActions(t *testing.T) {
	ui := &UI{
		ctx: context.Background(),
		app: &app.App{Store: app.Store{Base: t.TempDir()}},
	}
	primitive := ui.policiesPage()
	if primitive == nil {
		t.Fatal("policies page is nil")
	}
	if ui.contentFocus == nil {
		t.Fatal("policies page did not register content focus")
	}
	if ui.currentActions == nil {
		t.Fatal("policies page did not register action focus")
	}
	if ui.currentActions.GetButtonCount() == 0 {
		t.Fatal("policies page has no action buttons")
	}
}
