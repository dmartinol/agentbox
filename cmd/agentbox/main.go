package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/dmartinol/agentbox/internal/app"
	"github.com/dmartinol/agentbox/internal/tui"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	a, err := app.New()
	if err != nil {
		fail(err)
	}
	if err := dispatch(context.Background(), a, os.Args[1:]); err != nil {
		fail(err)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "error:", err); os.Exit(1) }

func flags(name string) *flag.FlagSet {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(os.Stderr)
	return f
}

func dispatch(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		usage()
		return nil
	}
	if args[0] == "version" || args[0] == "--version" || args[0] == "-version" {
		fmt.Println(versionString())
		return nil
	}
	switch args[0] {
	case "doctor":
		f := flags("doctor")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		checks := a.Doctor(ctx)
		if *jsonOut {
			return app.PrintJSON(checks)
		}
		for _, c := range checks {
			state := "FAIL"
			if c.OK {
				state = "OK"
			}
			fmt.Printf("%-5s %-18s %s\n", state, c.Name, c.Detail)
		}
		return nil
	case "profile":
		return profileCmd(ctx, a, args[1:])
	case "image":
		return imageCmd(ctx, a, args[1:])
	case "gateway":
		return gatewayCmd(ctx, a, args[1:])
	case "provider":
		return providerCmd(ctx, a, args[1:])
	case "policy":
		return policyCmd(ctx, a, args[1:])
	case "run":
		return runCmd(ctx, a, args[1:])
	case "tui":
		return tui.Run(ctx, a)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Println("AgentBox — OpenShell agent runner")
	fmt.Println("  version")
	fmt.Println("  doctor [--json]")
	fmt.Println("  profile create|edit|show|list")
	fmt.Println("  image versions|build|list|push|pull|registry")
	fmt.Println("  gateway list|connect|start|info")
	fmt.Println("  provider setup")
	fmt.Println("  policy show|edit")
	fmt.Println("  run start|list|show|attach|logs|stop|sync|diff|delete")
	fmt.Println("  tui")
}

func versionString() string {
	v := version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				v = info.Main.Version
			}
		}
	}
	if commit != "" && commit != "none" {
		return fmt.Sprintf("%s (commit %s, built %s)", v, commit, date)
	}
	return v
}

func need(pos []string, n int) error {
	if len(pos) < n {
		return fmt.Errorf("expected %d positional argument(s)", n)
	}
	return nil
}

func profileCmd(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("profile create|edit|show|list required")
	}
	switch args[0] {
	case "create":
		f := flags("profile create")
		agent := f.String("agent", "", "claude, codex, opencode")
		image := f.String("image", "", "local image reference")
		mode := f.String("mode", "standard", "standard or enmaas")
		model := f.String("model", "", "exact default model")
		provider := f.String("provider", "", "OpenShell provider name")
		policy := f.String("policy", "", "sandbox policy path")
		approval := f.String("approval", "prompted", "prompted or unattended")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		name := f.Args()[0]
		if *policy == "" {
			if err := a.Store.Ensure(); err != nil {
				return err
			}
			*policy = filepath.Join(a.Store.PolicyDir(), name+".yaml")
			if _, err := os.Stat(*policy); os.IsNotExist(err) {
				if err := os.WriteFile(*policy, app.DefaultPolicy(), 0600); err != nil {
					return err
				}
			}
		}
		if _, err := os.Stat(*policy); err != nil {
			return fmt.Errorf("policy file does not exist: %s", *policy)
		}
		p := app.Profile{Version: 1, Name: name, Agent: *agent, Image: *image, Mode: *mode, Model: *model, Provider: *provider, Policy: *policy, Approval: *approval}
		if err := a.Store.SaveProfile(p); err != nil {
			return err
		}
		fmt.Println(a.Store.ProfilePath(name))
		return nil
	case "edit":
		f := flags("profile edit")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		name := f.Args()[0]
		return a.Store.EditProfile(ctx, name)
	case "list":
		f := flags("profile list")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		p, err := a.Store.ListProfiles()
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(p)
		}
		for _, v := range p {
			fmt.Printf("%-20s %-10s %-9s %s\n", v.Name, v.Agent, v.Mode, v.Model)
		}
		return nil
	case "show":
		f := flags("profile show")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		p, err := a.Store.LoadProfile(f.Args()[0])
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(p)
		}
		fmt.Printf("%+v\n", p)
		return nil
	default:
		return fmt.Errorf("unknown profile command")
	}
}

func imageCmd(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("image versions|build|list|push|pull required")
	}
	switch args[0] {
	case "versions":
		f := flags("image versions")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		v, err := a.ImageVersion(ctx, f.Args()[0])
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(map[string]string{"agent": f.Args()[0], "proposed": v})
		}
		fmt.Println(v)
		return nil
	case "build":
		f := flags("image build")
		version := f.String("version", "", "exact agent version (default: proposed latest)")
		dry := f.Bool("dry-run", false, "preview")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		r, err := a.ImageBuild(ctx, f.Args()[0], *version, *dry)
		if err != nil {
			return err
		}
		return app.PrintJSON(r)
	case "list":
		f := flags("image list")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		b, err := a.ImageList(ctx)
		if err != nil {
			return err
		}
		if *jsonOut {
			_, err = os.Stdout.Write(b)
			return err
		}
		fmt.Print(string(b))
		return nil
	case "push":
		f := flags("image push")
		dry := f.Bool("dry-run", false, "preview")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 2); err != nil {
			return err
		}
		return a.ImagePush(ctx, f.Args()[0], f.Args()[1], *dry)
	case "pull":
		f := flags("image pull")
		dry := f.Bool("dry-run", false, "preview")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		return a.ImagePull(ctx, f.Args()[0], *dry)
	case "registry":
		return imageRegistryCmd(ctx, a, args[1:])
	default:
		return fmt.Errorf("unknown image command")
	}
}

func imageRegistryCmd(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 {
		args = []string{"show"}
	}
	switch args[0] {
	case "show":
		f := flags("image registry show")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		registry, err := a.Registry()
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(registry)
		}
		if registry.Reference == "" {
			fmt.Println("no external registry configured")
			return nil
		}
		fmt.Println(registry.NormalizedReference())
		return nil
	case "set":
		f := flags("image registry set")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		return a.SetRegistry(f.Args()[0])
	case "clear":
		f := flags("image registry clear")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		return a.ClearRegistry()
	case "discover":
		f := flags("image registry discover")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		images, err := a.RegistryDiscover(ctx)
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(images)
		}
		for _, image := range images {
			fmt.Println(image)
		}
		return nil
	default:
		return fmt.Errorf("unknown image registry command")
	}
}

func gatewayCmd(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("gateway list|connect|start|info required")
	}
	switch args[0] {
	case "list":
		b, err := a.GatewayList(ctx)
		if err != nil {
			return err
		}
		fmt.Print(string(b))
		return nil
	case "info":
		f := flags("gateway info")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		name := ""
		if len(f.Args()) > 0 {
			name = f.Args()[0]
		}
		b, err := a.GatewayInfo(ctx, name)
		if err != nil {
			return err
		}
		if *jsonOut {
			_, err = os.Stdout.Write(b)
			return err
		}
		var parsed any
		if json.Unmarshal(b, &parsed) == nil {
			return app.PrintJSON(parsed)
		}
		fmt.Print(string(b))
		return nil
	case "connect":
		f := flags("gateway connect")
		name := f.String("name", "", "registration name")
		dry := f.Bool("dry-run", false, "preview")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		return a.GatewayConnect(ctx, f.Args()[0], *name, *dry)
	case "start":
		f := flags("gateway start")
		dry := f.Bool("dry-run", false, "preview")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		return a.GatewayStart(ctx, *dry)
	default:
		return fmt.Errorf("unknown gateway command")
	}
}

func providerCmd(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 || args[0] != "setup" {
		return fmt.Errorf("provider setup required")
	}
	f := flags("provider setup")
	kind := f.String("type", "enmaas-bearer", "enmaas-bearer, enmaas-header, openai, or anthropic")
	gateway := f.String("gateway", "", "gateway name")
	dry := f.Bool("dry-run", false, "preview")
	update := f.Bool("update", false, "update an existing AgentBox provider profile")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if err := need(f.Args(), 1); err != nil {
		return err
	}
	return a.ProviderSetup(ctx, *kind, f.Args()[0], *gateway, *dry, *update)
}

func policyCmd(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("policy show|edit required")
	}
	switch args[0] {
	case "show":
		f := flags("policy show")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		m, err := a.PolicyShow(ctx, f.Args()[0])
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(m)
		}
		for _, key := range []string{"base", "effective", "global"} {
			fmt.Printf("%s:\n%s\n", key, m[key])
		}
		return nil
	case "edit":
		f := flags("policy edit")
		host := f.String("allow-host", "", "add HTTPS host rule")
		binary := f.String("binary", "", "agent binary path")
		dry := f.Bool("dry-run", false, "preview")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		return a.PolicyEdit(ctx, f.Args()[0], *host, *binary, *dry)
	default:
		return fmt.Errorf("unknown policy command")
	}
}

func runCmd(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("run start|list|show|attach|logs|stop|sync|diff|delete required")
	}
	switch args[0] {
	case "start":
		f := flags("run start")
		profile := f.String("profile", "", "saved profile name")
		source := f.String("source", ".", "Git source checkout")
		gateway := f.String("gateway", "", "OpenShell gateway name")
		image := f.String("image", "", "image override")
		mode := f.String("mode", "", "mode override")
		model := f.String("model", "", "model override")
		approval := f.String("approval", "", "approval override")
		prompt := f.String("prompt", "", "task prompt; empty opens interactive agent")
		dry := f.Bool("dry-run", false, "preview")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		r, err := a.Start(ctx, app.StartOptions{Profile: *profile, Source: *source, Gateway: *gateway, Image: *image, Mode: *mode, Model: *model, Approval: *approval, Prompt: *prompt, Dry: *dry})
		if err != nil {
			return err
		}
		if *jsonOut || *dry {
			return app.PrintJSON(r)
		}
		fmt.Printf("%s %s %s\n", r.ID, r.Agent, r.Worktree)
		return nil
	case "list":
		f := flags("run list")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		runs, err := a.ListAndReconcile(ctx)
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(runs)
		}
		for _, r := range runs {
			fmt.Printf("%-20s %-9s %-10s %-12s %s\n", r.ID, r.Agent, r.Mode, r.Status, r.Worktree)
		}
		return nil
	case "show":
		f := flags("run show")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		r, err := a.Store.LoadRun(f.Args()[0])
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(r)
		}
		fmt.Printf("%+v\n", r)
		return nil
	case "attach":
		if err := need(args[1:], 1); err != nil {
			return err
		}
		return a.Attach(ctx, args[1])
	case "logs":
		if err := need(args[1:], 1); err != nil {
			return err
		}
		b, err := a.Logs(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Print(string(b))
		return nil
	case "stop":
		if err := need(args[1:], 1); err != nil {
			return err
		}
		return a.Stop(ctx, args[1])
	case "sync":
		if err := need(args[1:], 1); err != nil {
			return err
		}
		return a.Sync(ctx, args[1])
	case "diff":
		if err := need(args[1:], 1); err != nil {
			return err
		}
		r, err := a.Store.LoadRun(args[1])
		if err != nil {
			return err
		}
		b, err := a.Diff(ctx, r)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(b)
		return err
	case "delete":
		f := flags("run delete")
		discard := f.Bool("discard", false, "acknowledge discarding run results")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		return a.Delete(ctx, f.Args()[0], *discard)
	default:
		return fmt.Errorf("unknown run command %q", strings.Join(args, " "))
	}
}
