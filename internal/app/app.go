package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/dmartinol/agentbox/internal/skills"
)

type App struct {
	Store  Store
	Runner CommandRunner
	// Version is the running binary version, set by the CLI entry point.
	// When empty, the doctor output omits the installed usage-skill check.
	Version string
}

func New() (*App, error) {
	s, err := DefaultStore()
	if err != nil {
		return nil, err
	}
	return &App{Store: s, Runner: OSRunner{}}, nil
}

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func (a *App) Doctor(ctx context.Context) []Check {
	out := []Check{{Name: "platform", OK: runtime.GOOS == "darwin" && runtime.GOARCH == "arm64", Detail: runtime.GOOS + "/" + runtime.GOARCH + " (spike target: darwin/arm64)"}}
	for _, name := range []string{"git", "podman", "openshell", "npm"} {
		path, err := exec.LookPath(name)
		out = append(out, Check{Name: name, OK: err == nil, Detail: path})
	}
	if b, err := a.Runner.Output(ctx, "podman", "info", "--format", "{{.Host.Arch}}"); err != nil {
		out = append(out, Check{Name: "podman connection", Detail: err.Error()})
	} else {
		out = append(out, Check{Name: "podman connection", OK: true, Detail: strings.TrimSpace(string(b))})
	}
	if b, err := a.Runner.Output(ctx, "openshell", "status", "-o", "json"); err != nil {
		out = append(out, Check{Name: "gateway", Detail: err.Error()})
	} else {
		out = append(out, Check{Name: "gateway", OK: true, Detail: strings.TrimSpace(string(b))})
	}
	out = append(out, a.skillChecks()...)
	return out
}

func (a *App) skillChecks() []Check {
	if a.Version == "" {
		return nil
	}
	var problems []string
	for _, name := range skills.Names() {
		content, err := skills.Embedded(name, a.Version)
		if err != nil {
			problems = append(problems, fmt.Sprintf("embedded skill %q: %v; upgrade AgentBox", name, err))
			continue
		}
		if _, err := skills.ParseFrontMatter(content); err != nil {
			problems = append(problems, fmt.Sprintf("embedded skill %q has invalid front matter (%v); upgrade AgentBox", name, err))
		}
	}
	statuses, err := skills.List(skills.ListOptions{Version: a.Version})
	if err != nil {
		return []Check{{Name: "skills", Detail: err.Error()}}
	}
	installed := 0
	var (
		stale, staleWithoutSkill           []string
		modified, modifiedWithoutSkill     []string
		missing                            []string
		unreadable, unreadableWithoutSkill []string
		invalid, broken, legacy            []string
	)
	for _, status := range statuses {
		canonicalPresent := false
		for _, installation := range status.Installations {
			if installation.Target == "agents" && installation.Present {
				canonicalPresent = true
			}
			switch installation.State {
			case "current":
				if installation.Target == "agents" {
					installed++
				}
			case "stale":
				stale = append(stale, installation.Path)
			case "modified":
				modified = append(modified, installation.Path)
			case "invalid":
				invalid = append(invalid, installation.Path)
			case "broken-link":
				broken = append(broken, installation.Path)
			case "legacy":
				legacy = append(legacy, installation.Path)
			}
		}
		for _, file := range status.Files {
			switch file.State {
			case "stale":
				if canonicalPresent {
					stale = append(stale, file.Path)
				} else {
					staleWithoutSkill = append(staleWithoutSkill, file.Path)
				}
			case "modified":
				if canonicalPresent {
					modified = append(modified, file.Path)
				} else {
					modifiedWithoutSkill = append(modifiedWithoutSkill, file.Path)
				}
			case "missing":
				// Without a canonical copy, list reports every template as
				// missing; only call that a problem when the skill is installed.
				if canonicalPresent {
					missing = append(missing, file.Path)
				}
			case "unreadable":
				if canonicalPresent {
					unreadable = append(unreadable, file.Path)
				} else {
					unreadableWithoutSkill = append(unreadableWithoutSkill, file.Path)
				}
			}
		}
	}
	// A template without its canonical SKILL.md cannot be repaired by update:
	// update reports the skill as not installed and changes nothing. Install
	// creates the missing SKILL.md first, so those templates are grouped under
	// install instead of the update command the other states use.
	appendFix := func(label string, paths []string, command, note string) {
		if len(paths) == 0 {
			return
		}
		problems = append(problems, fmt.Sprintf("%s: %s; run `%s`%s", label, strings.Join(paths, ", "), command, note))
	}
	appendFix("invalid front matter", invalid, "agentbox skills install --force", "")
	appendFix("stale", stale, "agentbox skills update", "")
	appendFix("stale", staleWithoutSkill, "agentbox skills install", "")
	appendFix("modified", modified, "agentbox skills update --force", " (the previous copy is backed up)")
	appendFix("modified", modifiedWithoutSkill, "agentbox skills install --force", " (the previous copy is backed up)")
	appendFix("missing files", missing, "agentbox skills update", "")
	appendFix("unreadable files", unreadable, "agentbox skills update --force", " (the previous copy is backed up)")
	appendFix("unreadable files", unreadableWithoutSkill, "agentbox skills install --force", " (the previous copy is backed up)")
	if len(broken) > 0 {
		problems = append(problems, fmt.Sprintf("broken links: %s; run `agentbox skills install`", strings.Join(broken, ", ")))
	}
	if len(legacy) > 0 {
		problems = append(problems, fmt.Sprintf("legacy Codex copies: %s; run `agentbox skills update`", strings.Join(legacy, ", ")))
	}
	if len(problems) > 0 {
		return []Check{{Name: "skills", Detail: strings.Join(problems, "; ")}}
	}
	if installed == 0 {
		return []Check{{Name: "skills", OK: true, Detail: "no installed usage skills"}}
	}
	return []Check{{Name: "skills", OK: true, Detail: fmt.Sprintf("%d installed, up to date", installed)}}
}

func PrintJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (a *App) ProviderList(ctx context.Context, gateway string) ([]byte, error) {
	args := []string{"provider", "list", "-o", "json"}
	if gateway != "" {
		args = append(args, "--gateway", gateway)
	}
	return a.Runner.Output(ctx, "openshell", args...)
}

func (a *App) GatewayList(ctx context.Context) ([]byte, error) {
	return a.Runner.Output(ctx, "openshell", "gateway", "list", "-o", "json")
}

func (a *App) GatewayInfo(ctx context.Context, name string) ([]byte, error) {
	args := []string{"gateway", "info", "-o", "json"}
	if name != "" {
		args = append(args, "--gateway", name)
	}
	return a.Runner.Output(ctx, "openshell", args...)
}

func (a *App) GatewayConnect(ctx context.Context, url, name string, dry bool) error {
	if url == "" || name == "" || !safeName.MatchString(name) {
		return fmt.Errorf("gateway URL and simple name required")
	}
	args := []string{"gateway", "add", url, "--name", name}
	if strings.HasPrefix(url, "https://localhost:") || strings.HasPrefix(url, "https://127.0.0.1:") || strings.HasPrefix(url, "http://localhost:") || strings.HasPrefix(url, "http://127.0.0.1:") {
		args = append(args, "--local")
	}
	if dry {
		fmt.Println("openshell", strings.Join(args, " "))
		return nil
	}
	_, err := a.Runner.Output(ctx, "openshell", args...)
	return err
}

func (a *App) GatewayStart(ctx context.Context, dry bool) error {
	_, err := exec.LookPath("brew")
	command, args, err := gatewayStartCommand(runtime.GOOS, err == nil)
	if err != nil {
		return err
	}
	if dry {
		fmt.Println(command, strings.Join(args, " "))
		return nil
	}
	_, err = a.Runner.Output(ctx, command, args...)
	return err
}

func gatewayStartCommand(goos string, homebrewAvailable bool) (string, []string, error) {
	if goos != "darwin" {
		return "", nil, fmt.Errorf("gateway start currently supports installed Homebrew service on macOS only")
	}
	if !homebrewAvailable {
		return "", nil, fmt.Errorf("Homebrew OpenShell service is not installed")
	}
	return "brew", []string{"services", "start", "openshell"}, nil
}
