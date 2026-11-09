package tui

import (
	"context"
	"io"
	"testing"

	"github.com/dmartinol/agentbox/internal/app"
	"github.com/rivo/tview"
)

type pushFormRunner struct{ images string }

func (r pushFormRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	if name == "podman" && len(args) > 0 && args[0] == "images" {
		return []byte(r.images), nil
	}
	return []byte(`{}`), nil
}

func (pushFormRunner) Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func TestImagePushUsesDropdownForLocalImages(t *testing.T) {
	store := app.Store{Base: t.TempDir()}
	ui := &UI{
		ctx: context.Background(),
		app: &app.App{Store: store, Runner: pushFormRunner{images: `[
			{"Id":"one","Names":["localhost/openshell-codex:1.0.0-r3"],"Labels":{"agentbox":"true","agentbox.agent":"codex","agentbox.version":"1.0.0","agentbox.recipe":"3"}},
			{"Id":"two","Names":["localhost/openshell-claude:2.0.0-r3"],"Labels":{"agentbox":"true","agentbox.agent":"claude","agentbox.version":"2.0.0","agentbox.recipe":"3"}}
		]`}},
	}
	primitive := ui.imagePushForm()
	form, ok := primitive.(*tview.Form)
	if !ok {
		t.Fatalf("push form type = %T; want *tview.Form", primitive)
	}
	dropdown, ok := form.GetFormItemByLabel("Local tag").(*tview.DropDown)
	if !ok {
		t.Fatalf("local tag is %T; want *tview.DropDown", form.GetFormItemByLabel("Local tag"))
	}
	index, value := dropdown.GetCurrentOption()
	if index != 0 || value != "localhost/openshell-claude:2.0.0-r3" {
		t.Fatalf("current local tag = %d %q", index, value)
	}
}
