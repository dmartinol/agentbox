package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type App struct {
	Store  Store
	Runner CommandRunner
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
	return out
}

func PrintJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
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
