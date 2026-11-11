package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestOSRunnerStreamTerminalSetsSize(t *testing.T) {
	var output bytes.Buffer
	err := OSRunner{}.StreamTerminal(
		context.Background(),
		strings.NewReader(""),
		&output,
		nil,
		120,
		36,
		"sh",
		"-c",
		"stty size",
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != "36 120" {
		t.Fatalf("terminal size output = %q", output.String())
	}
}
