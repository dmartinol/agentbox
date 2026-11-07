package main

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dmartinol/agentbox/internal/app"
)

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestTUIRequiresConfirmationBeforeDiscardingRun(t *testing.T) {
	m := uiModel{tab: 0, cursor: 0, runIDs: []string{"run-unit123"}}
	if cmd := m.action("D"); cmd != nil {
		t.Fatal("delete action executed before confirmation")
	}
	if m.confirm == nil || m.confirm.args[0] != "run" || m.confirm.args[1] != "delete" || m.confirm.args[2] != "--discard" {
		t.Fatalf("discard confirmation was not prepared: %+v", m.confirm)
	}
	view := m.View()
	if view == "" || m.confirm.message == "" {
		t.Fatal("confirmation prompt is not visible")
	}
	updated, cmd := m.Update(keyRune('n'))
	m = updated.(uiModel)
	if cmd != nil || m.confirm != nil {
		t.Fatal("negative confirmation did not cancel")
	}
	_ = m.action("D")
	updated, cmd = m.Update(keyRune('y'))
	m = updated.(uiModel)
	if cmd == nil || m.confirm != nil {
		t.Fatal("positive confirmation did not dispatch the guarded delete")
	}
}

func TestTUITabNavigationAndProfileForm(t *testing.T) {
	m := uiModel{app: &app.App{Store: app.Store{Base: t.TempDir()}, Runner: &app.OSRunner{}}, rows: []string{"one"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(uiModel)
	if m.tab != 1 {
		t.Fatalf("tab index = %d; want 1", m.tab)
	}
	m.tab = 4
	_ = m.action("c")
	if m.form == nil || m.form.title != "Create profile" || len(m.form.fields) != 8 {
		t.Fatalf("profile form not available: %+v", m.form)
	}
	view := m.View()
	if !containsText(view, "Create profile") || !containsText(view, "Agent: codex") {
		t.Fatalf("profile form not rendered: %s", view)
	}
}

func TestTUIFormsBuildCLICommands(t *testing.T) {
	m := uiModel{tab: 0, cursor: 0, runIDs: []string{"run-unit123"}}
	_ = m.action("n")
	if m.form == nil {
		t.Fatal("new-run form was not opened")
	}
	got := m.form.build([]string{"dev", ".", "gateway-one", "enmaas", "model-x", "unattended", "do work"})
	want := []string{"run", "start", "--profile", "dev", "--source", ".", "--gateway", "gateway-one", "--mode", "enmaas", "--model", "model-x", "--approval", "unattended", "--prompt", "do work"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("run form command = %#v; want %#v", got, want)
	}

	m.tab = 1
	_ = m.action("b")
	got = m.form.build([]string{"opencode", "2.2.5"})
	want = []string{"image", "build", "--version", "2.2.5", "opencode"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("image form command = %#v; want %#v", got, want)
	}
}

func containsText(s, part string) bool {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
