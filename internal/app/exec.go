package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
)

type CommandRunner interface {
	Output(context.Context, string, ...string) ([]byte, error)
	Stream(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error
}

type TerminalCommandRunner interface {
	StreamTerminal(context.Context, io.Reader, io.Writer, io.Writer, int, int, string, ...string) error
}

type OSRunner struct{}

func (OSRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func (OSRunner) Stream(ctx context.Context, in io.Reader, out, errout io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, errout
	return cmd.Run()
}

func RunEditor(ctx context.Context, path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return fmt.Errorf("EDITOR is empty")
	}
	parts = append(parts[1:], path)
	cmd := exec.CommandContext(ctx, strings.Fields(editor)[0], parts...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func (OSRunner) StreamTerminal(ctx context.Context, in io.Reader, out, _ io.Writer, columns, rows int, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(columns), Rows: uint16(rows)})
	if err != nil {
		return err
	}
	defer terminal.Close()

	go func() {
		_, _ = io.Copy(terminal, in)
	}()
	_, _ = io.Copy(out, terminal)
	return cmd.Wait()
}
