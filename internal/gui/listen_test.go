package gui

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"
	"unicode/utf8"
)

func TestCloseReasonFitsAControlFrame(t *testing.T) {
	if got := closeReason(errors.New("short reason")); got != "short reason" {
		t.Fatalf("short reason = %q", got)
	}
	for _, input := range []string{strings.Repeat("x", 400), strings.Repeat("é", 400)} {
		got := closeReason(errors.New(input))
		if len(got) > 123 {
			t.Fatalf("reason is %d bytes, want at most 123", len(got))
		}
		if !utf8.ValidString(got) {
			t.Fatalf("reason is not valid UTF-8: %q", got)
		}
		if !strings.HasSuffix(got, "…") {
			t.Fatalf("truncated reason %q does not end with an ellipsis", got)
		}
	}
}

func TestGUIListenErrorNamesStaleProcess(t *testing.T) {
	err := guiListenError("127.0.0.1:8080", fmt.Errorf("listen tcp 127.0.0.1:8080: bind: %w", syscall.EADDRINUSE))
	message := err.Error()
	for _, want := range []string{"127.0.0.1:8080", "address already in use", "old embedded UI", "lsof -nP -iTCP:8080 -sTCP:LISTEN"} {
		if !strings.Contains(message, want) {
			t.Errorf("message %q does not contain %q", message, want)
		}
	}
}

func TestGUIListenErrorKeepsOtherFailures(t *testing.T) {
	err := guiListenError("127.0.0.1:8080", syscall.EACCES)
	if !strings.Contains(err.Error(), "cannot start the AgentBox GUI on http://127.0.0.1:8080") {
		t.Errorf("unexpected message: %q", err)
	}
	if strings.Contains(err.Error(), "old embedded UI") {
		t.Errorf("permission failure should not be described as a stale process: %q", err)
	}
}
