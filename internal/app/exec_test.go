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
		nil,
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

func TestOSRunnerStreamTerminalAppliesResizes(t *testing.T) {
	var output bytes.Buffer
	resizes := make(chan TerminalSize, 1)
	resizes <- TerminalSize{Columns: 100, Rows: 30}
	close(resizes)
	err := OSRunner{}.StreamTerminal(
		context.Background(),
		strings.NewReader(""),
		&output,
		nil,
		120,
		36,
		resizes,
		"sh",
		"-c",
		"sleep 1; stty size",
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != "30 100" {
		t.Fatalf("terminal size after resize = %q", output.String())
	}
}
