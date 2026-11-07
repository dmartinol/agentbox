package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestSectionForFunctionKey(t *testing.T) {
	cases := map[tcell.Key]int{
		tcell.KeyF1: 0,
		tcell.KeyF2: 1,
		tcell.KeyF3: 2,
		tcell.KeyF4: 3,
		tcell.KeyF5: 4,
		tcell.KeyF6: 5,
		tcell.KeyF7: 6,
	}
	for key, want := range cases {
		got, ok := sectionForFunctionKey(key)
		if !ok || got != want {
			t.Errorf("sectionForFunctionKey(%v) = %d,%v; want %d,true", key, got, ok, want)
		}
	}
	if _, ok := sectionForFunctionKey(tcell.KeyEnter); ok {
		t.Error("sectionForFunctionKey accepted Enter")
	}
}

func TestNextSectionCycles(t *testing.T) {
	if got := nextSection(pageDashboard, -1); got != pageDoctor {
		t.Errorf("previous from dashboard = %q, want %q", got, pageDoctor)
	}
	if got := nextSection(pageDoctor, 1); got != pageDashboard {
		t.Errorf("next from doctor = %q, want %q", got, pageDashboard)
	}
	if got := nextSection(pageRuns, 1); got != pageImages {
		t.Errorf("next from runs = %q, want %q", got, pageImages)
	}
	if got := nextSection("unknown-form", 1); got != pageRuns {
		t.Errorf("next from unknown = %q, want %q", got, pageRuns)
	}
}
