package tui

import (
	"reflect"
	"testing"

	"github.com/rivo/tview"
)

func TestUniqueSorted(t *testing.T) {
	got := uniqueSorted([]string{"b", "a", "b", " ", "c"})
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueSorted = %#v; want %#v", got, want)
	}
}

func TestOptionIndex(t *testing.T) {
	options := []string{"", "standard", "enmaas"}
	if got := optionIndex(options, "enmaas"); got != 2 {
		t.Fatalf("optionIndex = %d; want 2", got)
	}
	if got := optionIndex(options, "missing"); got != 0 {
		t.Fatalf("optionIndex fallback = %d; want 0", got)
	}
}

func TestEditingPrimitivesDoNotTriggerQuitShortcut(t *testing.T) {
	if !isEditingPrimitive(tview.NewInputField()) {
		t.Fatal("input field is not considered editable")
	}
	if !isEditingPrimitive(tview.NewTextArea()) {
		t.Fatal("text area is not considered editable")
	}
	if !isEditingPrimitive(tview.NewDropDown()) {
		t.Fatal("drop-down is not considered editable")
	}
	if isEditingPrimitive(tview.NewTable()) {
		t.Fatal("table was considered editable")
	}
}
