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

func TestNavigationHistoryBackForward(t *testing.T) {
	history := navigationHistory{}
	history = pushNavigation(history, pageDashboard, pageRuns)
	if len(history.past) != 1 || history.past[0] != pageDashboard || len(history.future) != 0 {
		t.Fatalf("unexpected push: %+v", history)
	}
	history = pushNavigation(history, pageRuns, "run-form")
	if len(history.past) != 2 || history.past[1] != pageRuns || len(history.future) != 0 {
		t.Fatalf("unexpected second push: %+v", history)
	}

	next, page, ok := backNavigation(history, "run-form")
	if !ok || page != pageRuns || len(next.past) != 1 || len(next.future) != 1 || next.future[0] != "run-form" {
		t.Fatalf("unexpected back: page=%q history=%+v ok=%v", page, next, ok)
	}

	next, page, ok = forwardNavigation(next, page)
	if !ok || page != "run-form" || len(next.past) != 2 || len(next.future) != 0 {
		t.Fatalf("unexpected forward: page=%q history=%+v ok=%v", page, next, ok)
	}
}

func TestPushNavigationCoalescesAndBoundsHistory(t *testing.T) {
	history := navigationHistory{}
	history = pushNavigation(history, pageDashboard, pageDashboard)
	if len(history.past) != 0 {
		t.Fatalf("identical navigation was pushed: %+v", history)
	}

	current := pageDashboard
	for i := 0; i < 60; i++ {
		next := pageRuns
		if i%2 == 0 {
			next = pageImages
		}
		history = pushNavigation(history, current, next)
		current = next
	}
	if len(history.past) > 50 {
		t.Fatalf("history was not bounded: %d", len(history.past))
	}
}

func TestBreadcrumbAndParentSection(t *testing.T) {
	cases := map[string]string{
		pageDashboard:        "AgentBox / Dashboard",
		"run-form":           "AgentBox / Runs / Start run",
		"image-build-form":   "AgentBox / Images / Build image",
		"gateway-form":       "AgentBox / Gateways / Connect gateway",
		"provider-form":      "AgentBox / Profiles / Set up provider",
		"policy-host-form":   "AgentBox / Policies / Allow network host",
		"logs:run-123":       "AgentBox / Runs / Logs",
		"diff:run-123":       "AgentBox / Runs / Diff",
		"gateway-info:local": "AgentBox / Gateways / Gateway info",
	}
	for page, want := range cases {
		if got := breadcrumb(page); got != want {
			t.Errorf("breadcrumb(%q) = %q, want %q", page, got, want)
		}
	}
}
