package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dmartinol/agentbox/internal/app"
)

var tabs = []string{"Runs", "Images", "Gateways", "Policies", "Profiles", "Doctor"}

type field struct{ label, value string }
type form struct {
	title  string
	fields []field
	at     int
	build  func([]string) []string
}
type actionDone struct{ err error }
type confirmation struct {
	message string
	args    []string
}
type uiModel struct {
	app     *app.App
	tab     int
	cursor  int
	rows    []string
	runIDs  []string
	message string
	form    *form
	confirm *confirmation
	width   int
}

func runTUI(ctx context.Context, a *app.App) error {
	m := uiModel{app: a, width: 100}
	m.refresh(ctx)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m *uiModel) refresh(ctx context.Context) {
	m.rows = nil
	m.runIDs = nil
	switch tabs[m.tab] {
	case "Runs":
		runs, err := m.app.ListAndReconcile(ctx)
		if err != nil {
			m.rows = []string{err.Error()}
			break
		}
		for _, r := range runs {
			m.runIDs = append(m.runIDs, r.ID)
			m.rows = append(m.rows, fmt.Sprintf("%-20s %-9s %-9s %-11s %s", r.ID, r.Agent, r.Mode, r.Status, r.Worktree))
		}
	case "Images":
		b, err := m.app.ImageList(ctx)
		if err != nil {
			m.rows = []string{err.Error()}
		} else {
			m.rows = strings.Split(strings.TrimSpace(string(b)), "\n")
		}
	case "Gateways":
		b, err := m.app.GatewayList(ctx)
		if err != nil {
			m.rows = []string{err.Error()}
		} else {
			m.rows = strings.Split(strings.TrimSpace(string(b)), "\n")
		}
	case "Policies":
		runs, _ := m.app.Store.ListRuns()
		if len(runs) == 0 {
			m.rows = []string{"No runs yet."}
			break
		}
		p, err := m.app.PolicyShow(ctx, runs[0].ID)
		if err != nil {
			m.rows = []string{err.Error()}
		} else {
			m.rows = []string{"Run: " + runs[0].ID, "Base:", string(p["base"]), "Effective:", string(p["effective"]), "Global (read only):", string(p["global"])}
		}
	case "Profiles":
		ps, err := m.app.Store.ListProfiles()
		if err != nil {
			m.rows = []string{err.Error()}
		} else {
			for _, p := range ps {
				m.rows = append(m.rows, fmt.Sprintf("%-20s %-9s %-9s %s", p.Name, p.Agent, p.Mode, p.Model))
			}
		}
	case "Doctor":
		for _, c := range m.app.Doctor(ctx) {
			status := "FAIL"
			if c.OK {
				status = "OK"
			}
			m.rows = append(m.rows, fmt.Sprintf("%-5s %-18s %s", status, c.Name, c.Detail))
		}
	}
	if len(m.rows) == 0 {
		m.rows = []string{"No items."}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
}

func (m uiModel) Init() tea.Cmd { return nil }

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
	case actionDone:
		if v.err != nil {
			m.message = v.err.Error()
		} else {
			m.message = "Action completed."
		}
		m.refresh(context.Background())
	case tea.KeyMsg:
		if m.confirm != nil {
			switch v.String() {
			case "y", "Y":
				args := m.confirm.args
				m.confirm = nil
				return m, m.command(args...)
			case "n", "N", "esc":
				m.confirm = nil
				m.message = "Action cancelled."
			}
			return m, nil
		}
		if m.form != nil {
			f := m.form
			switch v.String() {
			case "esc":
				m.form = nil
				m.message = "Cancelled."
				return m, nil
			case "enter":
				f.at++
				if f.at >= len(f.fields) {
					vals := make([]string, len(f.fields))
					for i, x := range f.fields {
						vals[i] = x.value
					}
					args := f.build(vals)
					m.form = nil
					return m, m.command(args...)
				}
			case "backspace", "ctrl+h":
				s := f.fields[f.at].value
				if len(s) > 0 {
					f.fields[f.at].value = s[:len(s)-1]
				}
			default:
				if v.Type == tea.KeyRunes {
					f.fields[f.at].value += v.String()
				}
			}
			return m, nil
		}
		switch v.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab", "right":
			m.tab = (m.tab + 1) % len(tabs)
			m.cursor = 0
			m.refresh(context.Background())
		case "shift+tab", "left":
			m.tab = (m.tab + len(tabs) - 1) % len(tabs)
			m.cursor = 0
			m.refresh(context.Background())
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor+1 < len(m.rows) {
				m.cursor++
			}
		case "r":
			m.refresh(context.Background())
		default:
			return m, m.action(v.String())
		}
	}
	return m, nil
}

func (m uiModel) command(args ...string) tea.Cmd {
	if len(args) == 0 {
		return nil
	}
	cmd := exec.Command(os.Args[0], args...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return actionDone{err} })
}

func (m *uiModel) startForm(title string, labels, defaults []string, build func([]string) []string) {
	fields := make([]field, len(labels))
	for i := range labels {
		fields[i] = field{labels[i], defaults[i]}
	}
	m.form = &form{title: title, fields: fields, build: build}
}

func (m *uiModel) selectedRun() string {
	if m.cursor >= 0 && m.cursor < len(m.runIDs) {
		return m.runIDs[m.cursor]
	}
	return ""
}

func (m *uiModel) action(key string) tea.Cmd {
	switch tabs[m.tab] {
	case "Runs":
		id := m.selectedRun()
		switch key {
		case "n":
			m.startForm("New run", []string{"Profile", "Source checkout", "Gateway", "Mode override", "Model override", "Approval override", "Prompt (optional)"}, []string{"", ".", "openshell", "", "", "", ""}, func(v []string) []string {
				args := []string{"run", "start", "--profile", v[0], "--source", v[1], "--gateway", v[2]}
				for i, name := range []string{"mode", "model", "approval", "prompt"} {
					if v[i+3] != "" {
						args = append(args, "--"+name, v[i+3])
					}
				}
				return args
			})
			return nil
		case "a":
			if id != "" {
				return m.command("run", "attach", id)
			}
		case "l":
			if id != "" {
				return m.command("run", "logs", id)
			}
		case "x":
			if id != "" {
				return m.command("run", "stop", id)
			}
		case "s":
			if id != "" {
				return m.command("run", "sync", id)
			}
		case "d":
			if id != "" {
				return m.command("run", "diff", id)
			}
		case "D":
			if id != "" {
				m.confirm = &confirmation{
					message: "Delete this run and discard its review results?",
					args:    []string{"run", "delete", "--discard", id},
				}
				return nil
			}
		}
	case "Images":
		switch key {
		case "b":
			m.startForm("Build image", []string{"Agent", "Exact version (empty: proposed latest)"}, []string{"codex", ""}, func(v []string) []string {
				args := []string{"image", "build"}
				if v[1] != "" {
					args = append(args, "--version", v[1])
				}
				return append(args, v[0])
			})
			return nil
		case "v":
			m.startForm("Discover version", []string{"Agent"}, []string{"codex"}, func(v []string) []string { return []string{"image", "versions", v[0]} })
			return nil
		case "p":
			m.startForm("Push image", []string{"Local tag", "Registry reference"}, []string{"", ""}, func(v []string) []string { return []string{"image", "push", v[0], v[1]} })
			return nil
		case "u":
			m.startForm("Pull image", []string{"Registry reference"}, []string{""}, func(v []string) []string { return []string{"image", "pull", v[0]} })
			return nil
		}
	case "Gateways":
		switch key {
		case "s":
			return m.command("gateway", "start")
		case "i":
			return m.command("gateway", "info")
		case "c":
			m.startForm("Connect gateway", []string{"Endpoint URL", "Name"}, []string{"https://localhost:17670", "openshell"}, func(v []string) []string { return []string{"gateway", "connect", "--name", v[1], v[0]} })
			return nil
		}
	case "Policies":
		runs, _ := m.app.Store.ListRuns()
		if len(runs) == 0 {
			return nil
		}
		id := runs[0].ID
		switch key {
		case "e":
			return m.command("policy", "edit", id)
		case "a":
			m.startForm("Allow HTTPS host", []string{"Run ID", "Host", "Binary path (optional)"}, []string{id, "", ""}, func(v []string) []string {
				args := []string{"policy", "edit", "--allow-host", v[1]}
				if v[2] != "" {
					args = append(args, "--binary", v[2])
				}
				return append(args, v[0])
			})
			return nil
		}
	case "Profiles":
		switch key {
		case "c":
			m.startForm("Create profile", []string{"Name", "Agent", "Image", "Mode", "Model", "Provider", "Policy file (empty: default)", "Approval"}, []string{"", "codex", "", "standard", "", "", "", "prompted"}, func(v []string) []string {
				args := []string{"profile", "create", "--agent", v[1], "--image", v[2], "--mode", v[3], "--model", v[4], "--provider", v[5], "--approval", v[7]}
				if v[6] != "" {
					args = append(args, "--policy", v[6])
				}
				return append(args, v[0])
			})
			return nil
		case "e":
			m.startForm("Edit profile", []string{"Name"}, []string{""}, func(v []string) []string { return []string{"profile", "edit", v[0]} })
			return nil
		case "p":
			m.startForm("Set up OpenShell provider", []string{"Type", "Name", "Gateway"}, []string{"enmaas-bearer", "", "openshell"}, func(v []string) []string {
				return []string{"provider", "setup", "--type", v[0], "--gateway", v[2], v[1]}
			})
			return nil
		}
	}
	return nil
}

func (m uiModel) View() string {
	var b strings.Builder
	b.WriteString("AgentBox — OpenShell prototype\n")
	for i, name := range tabs {
		if i == m.tab {
			b.WriteString("[" + name + "] ")
		} else {
			b.WriteString(" " + name + "  ")
		}
	}
	b.WriteString("\n\n")
	if m.form != nil {
		b.WriteString(m.form.title + "\n")
		for i, f := range m.form.fields {
			marker := " "
			if i == m.form.at {
				marker = ">"
			}
			b.WriteString(fmt.Sprintf("%s %s: %s\n", marker, f.label, f.value))
		}
		b.WriteString("\nEnter next • Esc cancel\n")
		return b.String()
	}
	if m.confirm != nil {
		b.WriteString(m.confirm.message + "\n")
		b.WriteString("y confirm • n cancel\n")
		return b.String()
	}
	max := len(m.rows)
	if max > 25 {
		max = 25
	}
	for i := 0; i < max; i++ {
		marker := " "
		if i == m.cursor {
			marker = ">"
		}
		b.WriteString(marker + " " + m.rows[i] + "\n")
	}
	b.WriteString("\n")
	switch tabs[m.tab] {
	case "Runs":
		b.WriteString("n new • a attach • l logs • x stop • s sync • d diff • D delete/discard")
	case "Images":
		b.WriteString("b build • v versions • p push • u pull")
	case "Gateways":
		b.WriteString("s start • c connect • i inspect")
	case "Policies":
		b.WriteString("e edit YAML • a allow host")
	case "Profiles":
		b.WriteString("c create • e edit • p provider setup")
	}
	b.WriteString("\nTab switch • r refresh • q quit\n")
	if m.message != "" {
		b.WriteString(m.message + "\n")
	}
	return b.String()
}
