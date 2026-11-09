package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/dmartinol/agentbox/internal/app"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const taskOutputPage = "task-output"

type taskLogWriter struct {
	app  *tview.Application
	view *tview.TextView
	mu   sync.Mutex
}

func (w *taskLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	text := string(p)
	w.app.QueueUpdateDraw(func() {
		_, _ = fmt.Fprint(w.view, text)
		w.view.ScrollToEnd()
	})
	return len(p), nil
}

type taskRunner struct {
	base app.CommandRunner
	out  io.Writer
}

func (r taskRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	_, _ = fmt.Fprintf(r.out, "$ %s\n", displayCommand(name, args))
	body, err := r.base.Output(ctx, name, args...)
	if err != nil {
		_, _ = fmt.Fprintf(r.out, "[failed] %v\n", err)
	}
	return body, err
}

func (r taskRunner) Stream(ctx context.Context, in io.Reader, _, _ io.Writer, name string, args ...string) error {
	_, _ = fmt.Fprintf(r.out, "$ %s\n", displayCommand(name, args))
	return r.base.Stream(ctx, in, r.out, r.out, name, args...)
}

func displayCommand(name string, args []string) string {
	parts := []string{name}
	redactNext := false
	for _, arg := range args {
		if redactNext {
			parts = append(parts, "<redacted>")
			redactNext = false
			continue
		}
		lower := strings.ToLower(arg)
		secretFlag := strings.Contains(lower, "credential") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "api-key")
		if secretFlag {
			if key, _, ok := strings.Cut(arg, "="); ok {
				parts = append(parts, key+"=<redacted>")
			} else {
				parts = append(parts, arg)
				redactNext = true
			}
			continue
		}
		if strings.ContainsAny(arg, " \t\n") {
			parts = append(parts, fmt.Sprintf("%q", arg))
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

func (ui *UI) runTask(title, returnPage string, cancelable bool, action func(context.Context, *app.App, io.Writer) error) {
	ctx, cancel := context.WithCancel(ui.ctx)
	logView := tview.NewTextView().
		SetDynamicColors(false).
		SetScrollable(true).
		SetWrap(true).
		SetMaxLines(10000)
	logView.SetBorder(true).SetTitle(" Command output ")
	_, _ = fmt.Fprintf(logView, "Starting: %s\n\n", title)
	logView.ScrollToEnd()

	status := tview.NewTextView().SetDynamicColors(true).SetText("[yellow]Running:[white] " + title)
	status.SetBorder(true).SetTitle(" Task ")

	var (
		done      bool
		closeTask func()
	)
	actionForm := tview.NewForm().SetButtonsAlign(tview.AlignCenter)
	buttonLabel := "Close"
	if cancelable {
		buttonLabel = "Stop"
	}
	actionForm.AddButton(buttonLabel, func() {
		if !done {
			cancel()
			ui.tapp.QueueUpdateDraw(func() {
				status.SetText("[yellow]Stopping:[white] " + title)
				actionForm.GetButton(0).SetDisabled(true).SetLabel("Stopping...")
			})
			return
		}
		closeTask()
	})
	stopButton := actionForm.GetButton(0)
	if !cancelable {
		stopButton.SetDisabled(true)
	}
	actionForm.SetBorder(true).SetTitle(" Actions ")

	page := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(status, 3, 0, false).
		AddItem(logView, 0, 1, true).
		AddItem(actionForm, 5, 0, false)

	closeTask = func() {
		ui.pages.RemovePage(taskOutputPage)
		ui.modalActive = false
		ui.currentActions = nil
		ui.contentFocus = nil
		if returnPage != "" {
			ui.show(returnPage)
		} else {
			ui.refresh()
		}
	}
	logView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
			ui.tapp.SetFocus(actionForm.GetButton(0))
			return nil
		}
		return event
	})
	actionForm.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyBacktab, tcell.KeyEscape:
			ui.tapp.SetFocus(logView)
			return nil
		}
		return event
	})

	ui.modalActive = true
	ui.currentActions = actionForm
	ui.contentFocus = logView
	ui.pages.AddPage(taskOutputPage, page, true, true)
	ui.tapp.SetFocus(logView)
	ui.header.SetText("[::b]AgentBox — Task")
	if cancelable {
		ui.footer.SetText("Running command • Tab actions • Stop cancels this command")
	} else {
		ui.footer.SetText("Running command • Tab actions • Close is enabled when finished")
	}

	started := time.Now()
	writer := &taskLogWriter{app: ui.tapp, view: logView}
	go func() {
		taskApp := *ui.app
		taskApp.Runner = taskRunner{base: ui.app.Runner, out: writer}
		err := action(ctx, &taskApp, writer)
		ui.tapp.QueueUpdateDraw(func() {
			done = true
			stopButton.SetDisabled(false).SetLabel("Close")
			if err != nil {
				if ctx.Err() != nil {
					status.SetText("[yellow]Stopped:[white] " + title)
					_, _ = fmt.Fprintf(logView, "\n[stopped] %v\n", err)
				} else {
					status.SetText("[red]Failed:[white] " + title)
					_, _ = fmt.Fprintf(logView, "\n[failed] %v\n", err)
				}
			} else {
				status.SetText(fmt.Sprintf("[green]Completed:[white] %s [gray](%s)", title, time.Since(started).Round(time.Second)))
			}
			logView.ScrollToEnd()
			ui.tapp.SetFocus(stopButton)
			ui.footer.SetText("Task finished • Enter Close • Tab output")
		})
	}()
}
