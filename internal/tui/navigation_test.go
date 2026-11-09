package tui

import "testing"

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
		"project-form":       "AgentBox / Projects / Create project",
		"project-edit:dev":   "AgentBox / Projects / Edit project",
		"profile-edit:dev":   "AgentBox / Profiles / Edit profile",
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
