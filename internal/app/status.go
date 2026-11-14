package app

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RunStatus is the single derived, read-only operational status of a run.
// Persisted values are a cache; reconcile is authoritative.
type RunStatus string

const (
	StatusCreating    RunStatus = "creating"
	StatusReady       RunStatus = "ready"
	StatusWorking     RunStatus = "working"
	StatusWaiting     RunStatus = "waiting"
	StatusFailed      RunStatus = "failed"
	StatusUnreachable RunStatus = "unreachable"
	StatusStopped     RunStatus = "stopped"
	StatusCompleted   RunStatus = "completed"
)

// openCodeWorkingWindow is how recently agent.log must have changed for a
// hook-less OpenCode run to count as working. The constant is fixed; there is
// no configurable threshold.
const openCodeWorkingWindow = 120 * time.Second

const statusDetailLimit = 240

// Label renders the user-facing name of a status.
func (s RunStatus) Label() string {
	switch s {
	case StatusCreating:
		return "Creating"
	case StatusReady:
		return "Ready"
	case StatusWorking:
		return "Working"
	case StatusWaiting:
		return "Waiting for input"
	case StatusFailed:
		return "Failed"
	case StatusUnreachable:
		return "Unreachable"
	case StatusStopped:
		return "Stopped"
	case StatusCompleted:
		return "Completed"
	case "":
		return "Unknown"
	default:
		return string(s)
	}
}

// IsActive reports whether a run still occupies its worktree.
func (s RunStatus) IsActive() bool {
	switch s {
	case StatusCreating, StatusReady, StatusWorking, StatusWaiting:
		return true
	default:
		return false
	}
}

// IsAttachable reports whether the agent session can be attached to.
func (s RunStatus) IsAttachable() bool {
	switch s {
	case StatusReady, StatusWorking, StatusWaiting:
		return true
	default:
		return false
	}
}

// IsStoppable reports whether Stop applies to the run.
func (s RunStatus) IsStoppable() bool { return s.IsAttachable() }

// NormalizeRunStatus maps a persisted value, including legacy values, onto the
// documented status set. Unknown values are reported as Unreachable with a
// detail naming the value so the problem stays visible.
func NormalizeRunStatus(raw, detail string) (RunStatus, string) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(StatusCreating):
		return StatusCreating, detail
	case string(StatusReady):
		return StatusReady, detail
	case string(StatusWorking):
		return StatusWorking, detail
	case string(StatusWaiting):
		return StatusWaiting, detail
	case string(StatusFailed):
		return StatusFailed, detail
	case string(StatusUnreachable):
		return StatusUnreachable, detail
	case string(StatusStopped):
		return StatusStopped, detail
	case string(StatusCompleted):
		return StatusCompleted, detail
	case "", "preview", "prepared", "created":
		return StatusCreating, detail
	case "running":
		return StatusWorking, detail
	case "idle":
		return StatusReady, detail
	case "finished":
		return StatusCompleted, detail
	case "error":
		return StatusFailed, firstDetail(detail, "sandbox reported an error")
	case "create-failed":
		return StatusFailed, firstDetail(detail, "sandbox creation failed")
	case "policy-blocked":
		return StatusFailed, firstDetail(detail, "policy check failed")
	case "unreadable":
		return StatusUnreachable, firstDetail(detail, "sandbox response could not be parsed")
	default:
		return StatusUnreachable, fmt.Sprintf("unrecognized status %q", raw)
	}
}

type activitySignal struct {
	State  string
	Detail string
}

// runEvidence is everything classifyRunStatus needs. It is pure input so the
// classification table can be tested without a sandbox.
type runEvidence struct {
	InspectErr     string
	Unreadable     bool
	Phase          string
	Session        bool
	PaneDead       bool
	PaneCommand    string
	IdleMarker     bool
	FinishedMarker bool
	ExitCode       *int
	Activity       *activitySignal
	PlanPrompt     bool
	LogMtime       time.Time
	LogMtimeSet    bool
	Agent          string
}

func (e runEvidence) agentAlive() bool { return e.Session && !e.PaneDead }

// classifyRunStatus implements the documented precedence. It never guesses
// "waiting" from output: only an explicit signal or the session prompt can
// produce it.
func classifyRunStatus(e runEvidence, now time.Time) (RunStatus, string) {
	if e.InspectErr != "" {
		return StatusUnreachable, truncateDetail(e.InspectErr)
	}
	if e.Unreadable {
		return StatusUnreachable, "sandbox response could not be parsed"
	}
	switch e.Phase {
	case "error":
		return StatusFailed, "sandbox reported an error"
	case "stopped", "exited":
		return StatusStopped, "sandbox stopped"
	}
	if !e.Session && !e.FinishedMarker {
		return StatusCreating, ""
	}
	if e.Phase == "completed" || e.FinishedMarker {
		if e.ExitCode != nil {
			return StatusCompleted, fmt.Sprintf("exit code %d", *e.ExitCode)
		}
		return StatusCompleted, "agent exited"
	}
	if e.agentAlive() && e.PlanPrompt {
		return StatusWaiting, "plan ready for review"
	}
	if e.agentAlive() && e.Activity != nil {
		switch e.Activity.State {
		case "working":
			return StatusWorking, truncateDetail(e.Activity.Detail)
		case "waiting":
			return StatusWaiting, firstDetail(truncateDetail(e.Activity.Detail), "agent is waiting for input")
		case "done", "idle":
			return StatusReady, ""
		}
	}
	if e.IdleMarker || shellCommand(e.PaneCommand) {
		return StatusReady, ""
	}
	if e.agentAlive() {
		if e.Agent == "opencode" {
			if e.LogMtimeSet && now.Sub(e.LogMtime) < openCodeWorkingWindow {
				return StatusWorking, ""
			}
			return StatusReady, ""
		}
		if e.PaneCommand != "" {
			return StatusWorking, ""
		}
	}
	return StatusReady, ""
}

var terminalShells = map[string]bool{
	"sh": true, "bash": true, "dash": true, "ash": true, "zsh": true,
	"ksh": true, "fish": true, "csh": true, "tcsh": true,
}

func shellCommand(command string) bool {
	name := strings.ToLower(strings.TrimSpace(command))
	if fields := strings.Fields(name); len(fields) > 0 {
		name = fields[0]
	}
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	return terminalShells[name]
}

func parseActivitySignal(raw string) *activitySignal {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var doc struct {
		State  string `json:"state"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil
	}
	state := strings.ToLower(strings.TrimSpace(doc.State))
	switch state {
	case "working", "waiting", "done", "idle":
		return &activitySignal{State: state, Detail: strings.TrimSpace(doc.Detail)}
	default:
		return nil
	}
}

func firstDetail(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return truncateDetail(value)
		}
	}
	return ""
}

func truncateDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) <= statusDetailLimit {
		return detail
	}
	return detail[:statusDetailLimit-1] + "…"
}

// statusProbeScript gathers every signal classifyRunStatus needs in one exec.
// It uses tmux's own view of the pane, never /proc traversal of our own.
const statusProbeScript = `session=0
dead=1
command=
plan=0
idle=0
finished=0
exit=
activity=
mtime=
if tmux has-session -t agent 2>/dev/null; then
  session=1
  dead=$(tmux display-message -p -t agent '#{pane_dead}' 2>/dev/null) || dead=1
  command=$(tmux display-message -p -t agent '#{pane_current_command}' 2>/dev/null) || command=
  if tmux capture-pane -p -t agent 2>/dev/null | tail -n 12 | grep -q 'Implement this plan?'; then
    plan=1
  fi
fi
if [ -f /workspace/idle ]; then idle=1; fi
if [ -f /workspace/finished ]; then finished=1; fi
if [ -f /workspace/exit-code ]; then exit=$(tr -d '[:space:]' < /workspace/exit-code 2>/dev/null) || exit=; fi
if [ -f /workspace/state/activity.json ]; then activity=$(tr -d '\r\n' < /workspace/state/activity.json 2>/dev/null) || activity=; fi
if [ -f /workspace/state/agent.log ]; then mtime=$(stat -c %Y /workspace/state/agent.log 2>/dev/null) || mtime=; fi
printf 'session=%s\n' "$session"
printf 'dead=%s\n' "$dead"
printf 'command=%s\n' "$command"
printf 'plan=%s\n' "$plan"
printf 'idle=%s\n' "$idle"
printf 'finished=%s\n' "$finished"
printf 'exit=%s\n' "$exit"
printf 'activity=%s\n' "$activity"
printf 'mtime=%s\n' "$mtime"
`

// parseRunProbe turns the probe output into evidence. Unknown or malformed
// fields fall back to their zero value rather than failing the whole probe.
func parseRunProbe(out []byte) (runEvidence, error) {
	var e runEvidence
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		seen[key] = true
		switch key {
		case "session":
			e.Session = value == "1"
		case "dead":
			e.PaneDead = value != "0"
		case "command":
			e.PaneCommand = strings.TrimSpace(value)
		case "plan":
			e.PlanPrompt = value == "1"
		case "idle":
			e.IdleMarker = value == "1"
		case "finished":
			e.FinishedMarker = value == "1"
		case "exit":
			if value != "" {
				code, err := strconv.Atoi(value)
				if err == nil {
					e.ExitCode = &code
				}
			}
		case "activity":
			e.Activity = parseActivitySignal(value)
		case "mtime":
			if value != "" {
				seconds, err := strconv.ParseInt(value, 10, 64)
				if err == nil {
					e.LogMtime = time.Unix(seconds, 0)
					e.LogMtimeSet = true
				}
			}
		}
	}
	if !seen["session"] {
		return e, fmt.Errorf("sandbox probe returned no session state")
	}
	return e, nil
}
