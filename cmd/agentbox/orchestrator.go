package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/dmartinol/agentbox/internal/app"
)

func orchestratorCmd(a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("orchestrator config path|show|validate · gate pr-body|pr-merge required")
	}
	switch args[0] {
	case "config":
		return orchestratorConfigCmd(a, args)
	case "gate":
		return orchestratorGateCmd(args[1:])
	default:
		return fmt.Errorf("orchestrator config path|show|validate · gate pr-body|pr-merge required")
	}
}

func orchestratorConfigCmd(a *app.App, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("orchestrator config path|show|validate required")
	}
	registryPath := app.OrchestrationRegistryPath(a.Store.Base)
	switch args[1] {
	case "path":
		fmt.Println(registryPath)
		return nil
	case "show":
		f := flags("orchestrator config show")
		path := f.String("file", registryPath, "registry file to resolve")
		project := f.String("project", "", "repository root to match a project entry")
		remote := f.String("remote", "", "Git remote URL to match a project entry")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[2:]); err != nil {
			return err
		}
		resolved, err := app.ResolveOrchestrationFile(strings.TrimSpace(*path), app.OrchestrationProjectRef{
			Root:   strings.TrimSpace(*project),
			Remote: strings.TrimSpace(*remote),
		})
		if err != nil {
			return err
		}
		if *jsonOut {
			return app.PrintJSON(resolved)
		}
		printResolvedOrchestration(resolved)
		return nil
	case "validate":
		f := flags("orchestrator config validate")
		path := f.String("file", registryPath, "registry file to validate")
		jsonOut := f.Bool("json", false, "JSON output")
		if err := f.Parse(args[2:]); err != nil {
			return err
		}
		registry, err := app.LoadOrchestrationRegistry(strings.TrimSpace(*path))
		if err != nil {
			return err
		}
		if registry == nil {
			return fmt.Errorf("orchestration registry %s does not exist", strings.TrimSpace(*path))
		}
		if *jsonOut {
			return app.PrintJSON(struct {
				Path  string `json:"path"`
				Valid bool   `json:"valid"`
			}{Path: strings.TrimSpace(*path), Valid: true})
		}
		fmt.Printf("%s: valid (version %d, default %s)\n", strings.TrimSpace(*path), registry.Version, registry.Default)
		return nil
	}
	return fmt.Errorf("unknown orchestrator config command %q", args[1])
}

func orchestratorGateCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("orchestrator gate pr-body|pr-merge required")
	}
	switch args[0] {
	case "pr-body":
		f := flags("orchestrator gate pr-body")
		prBodyPath := f.String("pr-body", "", "pull request body file")
		issueBodyPath := f.String("issue-body", "", "issue body file")
		issueNumber := f.Int("issue", 0, "issue number")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if len(f.Args()) != 0 {
			return fmt.Errorf("orchestrator gate pr-body takes no positional arguments")
		}
		if strings.TrimSpace(*prBodyPath) == "" {
			return fmt.Errorf("orchestrator gate pr-body requires --pr-body")
		}
		if strings.TrimSpace(*issueBodyPath) == "" {
			return fmt.Errorf("orchestrator gate pr-body requires --issue-body")
		}
		if *issueNumber <= 0 {
			return fmt.Errorf("orchestrator gate pr-body requires --issue")
		}
		prBody, err := os.ReadFile(*prBodyPath)
		if err != nil {
			return err
		}
		issueBody, err := os.ReadFile(*issueBodyPath)
		if err != nil {
			return err
		}
		if err := app.ValidateReviewPRBody(string(prBody), string(issueBody), *issueNumber); err != nil {
			return err
		}
		fmt.Println("pr-body: valid")
		return nil
	case "pr-merge":
		f := flags("orchestrator gate pr-merge")
		prBodyPath := f.String("pr-body", "", "pull request body file")
		headSHA := f.String("head", "", "merged head SHA")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if len(f.Args()) != 0 {
			return fmt.Errorf("orchestrator gate pr-merge takes no positional arguments")
		}
		if strings.TrimSpace(*prBodyPath) == "" {
			return fmt.Errorf("orchestrator gate pr-merge requires --pr-body")
		}
		if strings.TrimSpace(*headSHA) == "" {
			return fmt.Errorf("orchestrator gate pr-merge requires --head")
		}
		prBody, err := os.ReadFile(*prBodyPath)
		if err != nil {
			return err
		}
		if err := app.ValidateReviewChecks(string(prBody), strings.TrimSpace(*headSHA)); err != nil {
			return err
		}
		fmt.Println("pr-merge: valid")
		return nil
	default:
		return fmt.Errorf("unknown orchestrator gate command %q", args[0])
	}
}

func printResolvedOrchestration(resolved app.ResolvedOrchestration) {
	fmt.Printf("registry: %s\n", resolved.Path)
	fmt.Printf("present: %t\n", resolved.Present)
	if !resolved.Present {
		fmt.Println("scope: none")
		fmt.Println("no applicable configuration; ask for the missing orchestration, worker, profile, and model choices")
		return
	}
	fmt.Printf("default: %s\n", resolved.Default)
	scope := resolved.Scope
	if resolved.Project != "" {
		scope += " (" + resolved.Project + ")"
	}
	fmt.Printf("scope: %s\n", scope)
	if len(resolved.Sources) > 0 {
		fmt.Printf("sources: %s\n", strings.Join(resolved.Sources, ", "))
	}
	fmt.Println("orchestrator:")
	printOrchestrationSelection("  selection", resolved.Orchestrator)
	fmt.Println("stages:")
	for _, stage := range app.OrchestrationStageNames() {
		printOrchestrationSelection("  "+stage, resolved.Stages[stage])
	}
	if len(resolved.Warnings) > 0 {
		fmt.Println("warnings:")
		for _, warning := range resolved.Warnings {
			fmt.Printf("  - %s\n", warning)
		}
	}
}

func printOrchestrationSelection(label string, selection app.ResolvedOrchestrationSelection) {
	parts := []string{dash(selection.Runtime) + "/" + dash(selection.Agent)}
	if selection.Profile != "" {
		parts = append(parts, "profile="+selection.Profile)
	}
	if selection.Model != "" {
		parts = append(parts, "model="+selection.Model)
	}
	if selection.Effort != "" {
		parts = append(parts, "effort="+selection.Effort)
	}
	if selection.Source != "" {
		parts = append(parts, "source="+selection.Source)
	}
	fmt.Printf("%-12s %s\n", label, strings.Join(parts, " "))
}

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
