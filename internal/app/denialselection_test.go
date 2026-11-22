package app

import (
	"errors"
	"strings"
	"testing"
)

func selectionDenials() []PolicyDenial {
	return []PolicyDenial{
		{Host: "example.com", Port: 443, Binary: "/usr/bin/curl", Reason: "transparent_tcp_policy_denied", Valid: true},
		{Host: "relative.test", Port: 80, Binary: "curl", Valid: false, InvalidReason: "binary path must be absolute"},
		{Host: "other.com", Port: 22, Binary: "/usr/bin/ssh", Valid: true},
	}
}

func TestResolveDenialSelectionByRow(t *testing.T) {
	selections, err := ResolveDenialSelection("1", selectionDenials())
	if err != nil {
		t.Fatal(err)
	}
	if len(selections) != 1 {
		t.Fatalf("selections = %+v", selections)
	}
	want := DenialSelection{Row: 1, Request: DenialRequest{Host: "example.com", Port: 443, Binary: "/usr/bin/curl"}}
	if selections[0] != want {
		t.Fatalf("selection = %+v; want %+v", selections[0], want)
	}
}

func TestResolveDenialSelectionAcceptsMultipleRowsAndCollapsesDuplicates(t *testing.T) {
	selections, err := ResolveDenialSelection("3, 1,3", selectionDenials())
	if err != nil {
		t.Fatal(err)
	}
	if len(selections) != 2 {
		t.Fatalf("selections = %+v", selections)
	}
	if selections[0].Row != 3 || selections[1].Row != 1 {
		t.Fatalf("selection rows = %+v; want 3 then 1", selections)
	}
	if selections[0].Request.Host != "other.com" || selections[1].Request.Host != "example.com" {
		t.Fatalf("selection hosts = %+v", selections)
	}
}

func TestResolveDenialSelectionAllSkipsInvalidRows(t *testing.T) {
	selections, err := ResolveDenialSelection("all", selectionDenials())
	if err != nil {
		t.Fatal(err)
	}
	if len(selections) != 2 || selections[0].Row != 1 || selections[1].Row != 3 {
		t.Fatalf("selections = %+v; want rows 1 and 3", selections)
	}
	for _, selection := range selections {
		if selection.Request.Persist {
			t.Fatalf("selection persists: %+v", selection)
		}
	}
}

func TestResolveDenialSelectionEmptySkips(t *testing.T) {
	for _, input := range []string{"", "   ", "\n"} {
		selections, err := ResolveDenialSelection(input, selectionDenials())
		if err != nil || len(selections) != 0 {
			t.Fatalf("input %q: selections = %+v, err = %v", input, selections, err)
		}
	}
}

func TestResolveDenialSelectionRefusesInvalidRow(t *testing.T) {
	_, err := ResolveDenialSelection("2", selectionDenials())
	if err == nil || !strings.Contains(err.Error(), "cannot be allowed") || !strings.Contains(err.Error(), "binary path must be absolute") {
		t.Fatalf("err = %v", err)
	}
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("err = %T; want ValidationError", err)
	}
}

func TestResolveDenialSelectionRefusesOutOfRangeAndGarbage(t *testing.T) {
	denials := selectionDenials()
	for _, input := range []string{"0", "4", "yes", "1,x"} {
		if _, err := ResolveDenialSelection(input, denials); err == nil {
			t.Fatalf("input %q was accepted", input)
		}
	}
	if _, err := ResolveDenialSelection("4", denials); !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("out-of-range error = %v", err)
	}
}

func TestResolveDenialSelectionAllWithoutValidDenials(t *testing.T) {
	denials := []PolicyDenial{{Host: "relative.test", Port: 80, Binary: "curl", Valid: false, InvalidReason: "binary path must be absolute"}}
	if _, err := ResolveDenialSelection("all", denials); err == nil || !strings.Contains(err.Error(), "No denial can be turned into a host rule") {
		t.Fatalf("err = %v", err)
	}
}
