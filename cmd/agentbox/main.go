package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"text/tabwriter"

	"github.com/dmartinol/agentbox/internal/app"
	"github.com/dmartinol/agentbox/internal/gui"
	"github.com/dmartinol/agentbox/internal/skills"
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
	a.Version = binaryVersion()
	if err := dispatch(context.Background(), a, os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
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
	case "settings":
		return settingsCmd(ctx, a, args[1:])
	case "project":
		return projectCmd(ctx, a, args[1:])
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
	case "mcp":
		return mcpCmd(ctx, a, args[1:])
	case "skill":
		return skillCmd(ctx, a, args[1:])
	case "skills":
		return skillsCmd(args[1:])
	case "agent":
		return agentCmd(ctx, a, args[1:])
	case "gui":
		return guiCmd(ctx, a, args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func guiCmd(ctx context.Context, a *app.App, args []string) error {
	f := flags("gui")
	address := f.String("address", "127.0.0.1:8080", "GUI listen address")
	if err := f.Parse(args); err != nil {
		return err
	}
	return gui.Run(ctx, a, *address)
}

func usage() {
	fmt.Println("AgentBox — OpenShell agent runner")
	fmt.Println("  version")
	fmt.Println("  doctor [--json]")
	fmt.Println("  settings show|set")
	fmt.Println("  project create|edit|show|list")
	fmt.Println("  profile create|edit|show|list")
	fmt.Println("  image versions|build|list|push|pull|registry")
	fmt.Println("  gateway list|connect|start|info")
	fmt.Println("  provider setup")
	fmt.Println("  policy show|edit")
	fmt.Println("  mcp list|show|create|delete|servers")
	fmt.Println("  skill list|show|create|delete|available")
	fmt.Println("  skills install|update|remove|list|show")
	fmt.Println("  agent start|list|show|attach|logs|stop|sync|diff|delete")
	fmt.Println("  gui [--address host:port]")
}

func versionString() string {
	v := binaryVersion()
	if commit != "" && commit != "none" {
		return fmt.Sprintf("%s (commit %s, built %s)", v, commit, date)
	}
	return v
}

func binaryVersion() string {
	v := version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				v = info.Main.Version
			}
		}
	}
	return v
}

func need(pos []string, n int) error {
	if len(pos) < n {
		return fmt.Errorf("expected %d positional argument(s)", n)
	}
	return nil
}

func settingsCmd(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 {
		args = []string{"show"}
	}
	switch args[0] {
	case "show":
		f := flags("settings show")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		settings, err := a.EffectiveSettings()
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(settings)
		}
		fmt.Printf("Worktree root: %s\nBranch prefix: %s\n", settings.Worktrees.Root, settings.Worktrees.BranchPrefix)
		return nil
	case "set":
		f := flags("settings set")
		root := f.String("worktree-root", "", "root directory for Git worktrees")
		prefix := f.String("branch-prefix", "", "branch namespace for run branches")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		current, err := a.EffectiveSettings()
		if err != nil {
			return err
		}
		if *root == "" {
			*root = current.Worktrees.Root
		}
		if *prefix == "" {
			*prefix = current.Worktrees.BranchPrefix
		}
		_, err = a.SaveGlobalSettings(*root, *prefix)
		return err
	default:
		return fmt.Errorf("unknown settings command")
	}
}

func projectCmd(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("project create|edit|show|list required")
	}
	switch args[0] {
	case "create":
		f := flags("project create")
		root := f.String("root", ".", "Git project root")
		noWorktree := f.Bool("no-worktree", false, "run directly in the checkout instead of an isolated worktree")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		project, err := a.RegisterProject(ctx, f.Args()[0], *root, !*noWorktree)
		if err != nil {
			return err
		}
		fmt.Println(project.Root)
		return nil
	case "edit":
		f := flags("project edit")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		return a.Store.EditProject(ctx, f.Args()[0])
	case "list":
		f := flags("project list")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		projects, err := a.Store.ListProjects()
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(projects)
		}
		for _, project := range projects {
			isolation := "worktree"
			if !project.UseWorktree() {
				isolation = "direct"
			}
			fmt.Printf("%-20s %-10s %s\n", project.Name, isolation, project.Root)
		}
		return nil
	case "show":
		f := flags("project show")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		project, err := a.Store.LoadProject(f.Args()[0])
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(project)
		}
		isolation := "worktree"
		if !project.UseWorktree() {
			isolation = "direct"
		}
		fmt.Printf("Name: %s\nRoot: %s\nIsolation: %s\n", project.Name, project.Root, isolation)
		return nil
	default:
		return fmt.Errorf("unknown project command")
	}
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
		mode := f.String("mode", "standard", "standard or gateway")
		model := f.String("model", "", "exact default model")
		provider := f.String("provider", "", "OpenShell provider name")
		policy := f.String("policy", "", "sandbox policy path")
		approval := f.String("approval", "prompted", "prompted or unattended")
		baseURL := f.String("base-url", "", "gateway model API base URL")
		apiKeyEnv := f.String("api-key-env", "", "environment variable holding the gateway API key")
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
		p := app.Profile{Version: 1, Name: name, Agent: *agent, Image: *image, Mode: *mode, Model: *model, Provider: *provider, Policy: *policy, Approval: *approval, BaseURL: *baseURL, APIKeyEnv: *apiKeyEnv}
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
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		imageUsage()
		return nil
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
		f.Usage = func() { imageBuildUsage(f) }
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
		images, err := a.LocalImages(ctx)
		if err != nil {
			return err
		}
		printImageList(images)
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

func printImageList(images []app.ImageArtifact) {
	if len(images) == 0 {
		fmt.Println("No AgentBox images found.")
		return
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "AGENT\tVERSION\tRECIPE\tPLATFORM\tREFERENCE")
	for _, image := range images {
		platform := strings.Trim(strings.Join([]string{image.OS, image.Arch}, "/"), "/")
		if platform == "" {
			platform = "-"
		}
		fmt.Fprintf(writer, "%s\t%s\t%d\t%s\t%s\n", image.Agent, image.Version, image.Recipe, platform, image.Reference)
	}
	_ = writer.Flush()
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
			arch := strings.Trim(strings.Join([]string{image.OS, image.Arch}, "/"), "/")
			fmt.Printf("%-8s %-16s r%-4d %-16s %s\n", image.Agent, image.Version, image.Recipe, arch, image.Reference)
		}
		return nil
	case "push":
		f := flags("image registry push")
		dry := f.Bool("dry-run", false, "preview")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		registry, err := a.Registry()
		if err != nil {
			return err
		}
		if registry.Reference == "" {
			return fmt.Errorf("no external registry configured")
		}
		artifact, err := a.LocalImage(ctx, f.Args()[0])
		if err != nil {
			return err
		}
		return a.ImagePush(ctx, artifact.Reference, app.RemoteReference(registry.NormalizedReference(), artifact), *dry)
	default:
		return fmt.Errorf("unknown image registry command")
	}
}

func imageUsage() {
	fmt.Println("AgentBox image commands")
	fmt.Println("  image versions [--json] <claude|codex|opencode>")
	fmt.Println("  image build [--version EXACT] [--dry-run] <claude|codex|opencode>")
	fmt.Println("  image list [--json]")
	fmt.Println("  image push [--dry-run] <local-image> <remote-reference>")
	fmt.Println("  image pull [--dry-run] <remote-reference>")
	fmt.Println("  image registry show|set|clear|discover|push")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  agentbox image versions codex")
	fmt.Println("  agentbox image build --version 0.167.0 codex")
	fmt.Println("  agentbox image build --version 0.167.0 --dry-run codex")
	fmt.Println("  agentbox image list --json")
}

func imageBuildUsage(f *flag.FlagSet) {
	fmt.Fprintln(f.Output(), "Build a revisioned AgentBox image for one agent.")
	fmt.Fprintln(f.Output(), "Usage: agentbox image build [--version EXACT] [--dry-run] <claude|codex|opencode>")
	fmt.Fprintln(f.Output(), "If --version is omitted, the latest official release is proposed and used.")
	fmt.Fprintln(f.Output(), "The base image is pinned to a digest and the build is tagged as localhost/openshell:<agent>-<version>-r3.")
	fmt.Fprintln(f.Output())
	fmt.Fprintln(f.Output(), "Flags:")
	f.PrintDefaults()
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
	kind := f.String("type", "gateway-bearer", "gateway-bearer, gateway-header, or built-in openai/anthropic")
	host := f.String("host", "", "gateway host (gateway provider types)")
	credential := f.String("credential", "AGENTBOX_MODEL_KEY", "environment variable holding the API key")
	gateway := f.String("gateway", "", "gateway name")
	dry := f.Bool("dry-run", false, "preview")
	update := f.Bool("update", false, "update an existing AgentBox provider profile")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if err := need(f.Args(), 1); err != nil {
		return err
	}
	return a.ProviderSetup(ctx, *kind, f.Args()[0], *host, *credential, *gateway, *dry, *update)
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

func mcpCmd(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("mcp list|show|create|delete|servers required")
	}
	switch args[0] {
	case "list":
		f := flags("mcp list")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		entries, err := a.Store.ListMCPs()
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(entries)
		}
		for _, entry := range entries {
			fmt.Printf("%-20s %-10s %s\n", entry.Name, entry.Agent, strings.Join(entry.Servers, ","))
		}
		return nil
	case "show":
		f := flags("mcp show")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		entry, err := a.Store.LoadMCP(f.Args()[0])
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(entry)
		}
		fmt.Printf("%s %s %s\n", entry.Name, entry.Agent, strings.Join(entry.Servers, ","))
		return nil
	case "create":
		f := flags("mcp create")
		agent := f.String("agent", "", "agent the entry selects servers for (claude, codex, opencode)")
		servers := f.String("servers", "", "comma-separated global MCP server names")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		var list []string
		for _, part := range strings.Split(*servers, ",") {
			if part = strings.TrimSpace(part); part != "" {
				list = append(list, part)
			}
		}
		entry := app.MCPTools{Version: 1, Name: f.Args()[0], Agent: strings.TrimSpace(*agent), Servers: list}
		if err := a.Store.SaveMCP(entry); err != nil {
			return err
		}
		fmt.Printf("%s %s %s\n", entry.Name, entry.Agent, strings.Join(entry.Servers, ","))
		return nil
	case "delete":
		f := flags("mcp delete")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		return a.Store.DeleteMCP(f.Args()[0])
	case "servers":
		f := flags("mcp servers")
		agent := f.String("agent", "", "agent to list global MCP servers for (claude, codex, opencode)")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*agent) == "" {
			return fmt.Errorf("--agent is required")
		}
		names, err := a.GlobalMCPServers(strings.TrimSpace(*agent))
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(names)
		}
		for _, name := range names {
			fmt.Println(name)
		}
		return nil
	}
	return fmt.Errorf("unknown mcp command %q", args[0])
}

func skillCmd(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("skill list|show|create|delete|available required")
	}
	switch args[0] {
	case "list":
		f := flags("skill list")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		entries, err := a.Store.ListSkills()
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(entries)
		}
		for _, entry := range entries {
			fmt.Printf("%-20s %-10s skills=%s plugins=%s\n", entry.Name, entry.Agent, strings.Join(entry.Skills, ","), strings.Join(entry.Plugins, ","))
		}
		return nil
	case "show":
		f := flags("skill show")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		entry, err := a.Store.LoadSkills(f.Args()[0])
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(entry)
		}
		fmt.Printf("%s %s skills=%s plugins=%s\n", entry.Name, entry.Agent, strings.Join(entry.Skills, ","), strings.Join(entry.Plugins, ","))
		return nil
	case "create":
		f := flags("skill create")
		agent := f.String("agent", "", "agent the entry selects skills for (claude, codex, opencode)")
		skills := f.String("skills", "", "comma-separated standalone skill names")
		plugins := f.String("plugins", "", "comma-separated plugin@marketplace bundles")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		entry := app.Skills{Version: 1, Name: f.Args()[0], Agent: strings.TrimSpace(*agent), Skills: splitList(*skills), Plugins: splitList(*plugins)}
		if err := a.Store.SaveSkills(entry); err != nil {
			return err
		}
		fmt.Printf("%s %s skills=%s plugins=%s\n", entry.Name, entry.Agent, strings.Join(entry.Skills, ","), strings.Join(entry.Plugins, ","))
		return nil
	case "delete":
		f := flags("skill delete")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		return a.Store.DeleteSkills(f.Args()[0])
	case "available":
		f := flags("skill available")
		agent := f.String("agent", "", "agent to list available skills for (claude, codex, opencode)")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*agent) == "" {
			return fmt.Errorf("--agent is required")
		}
		available, err := a.DiscoverSkills(strings.TrimSpace(*agent))
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(available)
		}
		for _, skill := range available.Skills {
			if skill.Description == "" {
				fmt.Println(skill.Name)
				continue
			}
			fmt.Printf("%s\t%s\n", skill.Name, skill.Description)
		}
		for _, plugin := range available.Plugins {
			fmt.Printf("%s\t%d bundled skills\n", plugin.ID, plugin.Skills)
		}
		return nil
	}
	return fmt.Errorf("unknown skill command %q", args[0])
}

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func skillsCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("skills install|update|remove|list|show required")
	}
	switch args[0] {
	case "install":
		f := flags("skills install")
		agent := f.String("agent", "", "comma-separated harnesses to install for (claude, codex, opencode; default: detected harnesses)")
		target := f.String("target", "", "deprecated alias for --agent: agents, codex, claude, opencode, or all")
		all := f.Bool("all", false, "install for every harness regardless of detection")
		local := f.Bool("local", false, "install into ./.agents/skills for the current project")
		dir := f.String("dir", "", "write plain copies under this directory (no links or manifest)")
		force := f.Bool("force", false, "replace modified copies and foreign destinations, backing up first")
		dry := f.Bool("dry-run", false, "report the planned changes without touching the filesystem")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if *target != "" {
			fmt.Fprintln(os.Stderr, "warning: --target is deprecated; use --agent")
		}
		results, err := skills.Install(skills.Options{
			Agents: splitList(*agent), Target: strings.TrimSpace(*target), All: *all,
			Local: *local, Dir: *dir, Force: *force, DryRun: *dry, Version: binaryVersion(),
		})
		return printSkillsOutput(results, err, *jsonOut)
	case "update":
		f := flags("skills update")
		agent := f.String("agent", "", "comma-separated harnesses to relink for (claude, codex, opencode; default: detected harnesses)")
		all := f.Bool("all", false, "relink for every harness regardless of detection")
		local := f.Bool("local", false, "update the project-scoped ./.agents/skills root")
		force := f.Bool("force", false, "replace modified copies, backing up first")
		dry := f.Bool("dry-run", false, "report the planned changes without touching the filesystem")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		results, err := skills.Update(skills.Options{
			Agents: splitList(*agent), All: *all, Local: *local,
			Force: *force, DryRun: *dry, Version: binaryVersion(),
		})
		return printSkillsOutput(results, err, *jsonOut)
	case "remove":
		f := flags("skills remove")
		agent := f.String("agent", "", "remove only the links of these harnesses (claude, codex, opencode); default removes everything recorded")
		all := f.Bool("all", false, "remove every recorded copy and link")
		local := f.Bool("local", false, "remove from the project-scoped ./.agents/skills root")
		force := f.Bool("force", false, "remove modified copies, backing up first")
		dry := f.Bool("dry-run", false, "report the planned removals without touching the filesystem")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		results, err := skills.Remove(skills.Options{
			Agents: splitList(*agent), All: *all, Local: *local,
			Force: *force, DryRun: *dry, Version: binaryVersion(),
		})
		return printSkillsOutput(results, err, *jsonOut)
	case "list":
		f := flags("skills list")
		local := f.Bool("local", false, "read the project-scoped ./.agents/skills root")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		statuses, err := skills.List(skills.ListOptions{Version: binaryVersion(), Local: *local})
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(statuses)
		}
		for _, status := range statuses {
			fmt.Printf("%s %s\n", status.Name, status.Version)
			if len(status.Installations) == 0 {
				fmt.Println("  home directory unavailable; no installed copies checked")
				continue
			}
			for _, installation := range status.Installations {
				version := installation.Version
				if version == "" {
					version = "-"
				}
				fmt.Printf("  %-9s %-12s %-8s %s\n", installation.Target, installation.State, version, installation.Path)
			}
			for _, file := range status.Files {
				fmt.Printf("  %-9s %-12s %-8s %s\n", "file:"+file.Name, file.State, "-", file.Path)
			}
		}
		return nil
	case "show":
		f := flags("skills show")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		content, err := skills.Embedded(f.Args()[0], binaryVersion())
		if err != nil {
			return err
		}
		if *jsonOut {
			fm, err := skills.ParseFrontMatter(content)
			if err != nil {
				return err
			}
			return app.PrintJSON(struct {
				Name        string `json:"name"`
				Version     string `json:"version"`
				Description string `json:"description"`
				Content     string `json:"content"`
			}{Name: fm.Name, Version: fm.Version, Description: fm.Description, Content: string(content)})
		}
		_, err = os.Stdout.Write(content)
		return err
	}
	return fmt.Errorf("unknown skills command %q", args[0])
}

// printSkillsOutput renders lifecycle results as text or JSON and returns the
// collected error after the report, so a refusal still shows every action.
func printSkillsOutput(results []skills.Result, err error, jsonOut bool) error {
	if jsonOut {
		if printErr := app.PrintJSON(results); printErr != nil {
			return printErr
		}
		return err
	}
	for _, result := range results {
		detail := ""
		if result.Detail != "" {
			detail = " (" + result.Detail + ")"
		}
		label := result.Skill
		if result.File != "" && result.File != "SKILL.md" {
			label += "/" + result.File
		}
		fmt.Printf("%-14s %-20s %s%s\n", result.Action, label, result.Path, detail)
	}
	return err
}

func agentCmd(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("agent start|list|show|attach|logs|stop|sync|diff|delete required")
	}
	switch args[0] {
	case "start":
		f := flags("agent start")
		profile := f.String("profile", "", "saved profile name")
		project := f.String("project", "", "registered project name")
		name := f.String("name", "", "optional unique agent name")
		source := f.String("source", ".", "Git source checkout (when --project is omitted)")
		gateway := f.String("gateway", "", "OpenShell gateway name")
		image := f.String("image", "", "image override")
		mode := f.String("mode", "", "mode override")
		model := f.String("model", "", "model override")
		approval := f.String("approval", "", "approval override")
		prompt := f.String("prompt", "", "task prompt; empty opens interactive agent")
		mcp := f.String("mcp", "", "MCP tools entry name (see `agentbox mcp`)")
		skills := f.String("skills", "", "skills entry name (see `agentbox skill`)")
		dry := f.Bool("dry-run", false, "preview")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		r, err := a.Start(ctx, app.StartOptions{Profile: *profile, Project: *project, Name: *name, Source: *source, Gateway: *gateway, Image: *image, Mode: *mode, Model: *model, Approval: *approval, Prompt: *prompt, MCP: *mcp, Skills: *skills, Dry: *dry})
		if err != nil {
			return err
		}
		if *jsonOut || *dry {
			return app.PrintJSON(r)
		}
		fmt.Printf("%s %s %s\n", r.ID, r.Agent, r.Worktree)
		for _, warning := range r.MCPWarnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
		}
		for _, warning := range r.SkillsWarnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
		}
		return nil
	case "list":
		f := flags("agent list")
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
			isolation := "direct"
			if r.Isolated() {
				isolation = "worktree"
			}
			name := r.Name
			if name == "" {
				name = r.ID
			}
			fmt.Printf("%-16s %-20s %-14s %-9s %-9s %-10s %-12s %s\n", name, r.ID, r.Project, r.Agent, isolation, r.Mode, r.Status, r.Worktree)
		}
		return nil
	case "show":
		f := flags("agent show")
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
		f := flags("agent delete")
		discard := f.Bool("discard", false, "acknowledge discarding agent results")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if err := need(f.Args(), 1); err != nil {
			return err
		}
		return a.Delete(ctx, f.Args()[0], *discard)
	default:
		return fmt.Errorf("unknown agent command %q", strings.Join(args, " "))
	}
}
