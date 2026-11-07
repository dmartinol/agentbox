package app

import (
	"context"
	"io"
)

type recordedCommand struct {
	name string
	args []string
}

type fakeRunner struct {
	outputs []recordedCommand
	streams []recordedCommand
	output  func(recordedCommand) ([]byte, error)
	stream  func(recordedCommand) error
}

func (f *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	call := recordedCommand{name: name, args: append([]string(nil), args...)}
	f.outputs = append(f.outputs, call)
	if f.output != nil {
		return f.output(call)
	}
	return nil, nil
}

func (f *fakeRunner) Stream(_ context.Context, _ io.Reader, _, _ io.Writer, name string, args ...string) error {
	call := recordedCommand{name: name, args: append([]string(nil), args...)}
	f.streams = append(f.streams, call)
	if f.stream != nil {
		return f.stream(call)
	}
	return nil
}

func commandHasArg(call recordedCommand, arg string) bool {
	for _, got := range call.args {
		if got == arg {
			return true
		}
	}
	return false
}
