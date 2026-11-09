package tui

import (
	"reflect"
	"testing"
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
