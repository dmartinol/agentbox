package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// RunDenials reads the sandbox policy log and returns structured denials. The
// agent log is a terminal stream; OpenShell's sandbox log is the authoritative
// source for host, port, binary, and deny reason.
func (a *App) RunDenials(ctx context.Context, runID string) ([]PolicyDenial, error) {
	run, err := a.Store.LoadRun(runID)
	if err != nil {
		return nil, err
	}
	if run.Sandbox == "" {
		return []PolicyDenial{}, nil
	}
	args := []string{"logs", run.Sandbox, "--source", "sandbox", "-n", "5000"}
	if run.Gateway != "" {
		args = append(args, "--gateway", run.Gateway)
	}
	out, err := a.Runner.Output(ctx, "openshell", args...)
	if err != nil {
		return []PolicyDenial{}, fmt.Errorf("sandbox policy log unavailable: %w", err)
	}
	return ParsePolicyDenials(out), nil
}

// PreviewDenial validates one exact denial entry and returns the policy and
// optional manifest diffs without writing anything.
func (a *App) PreviewDenial(ctx context.Context, runID string, request DenialRequest) (DenialPreview, error) {
	return a.denialPreview(ctx, runID, request)
}

// ApplyDenial patches the run's materialized policy through the same pending
// machinery as other live edits, and optionally appends the rule to the
// repository manifest and regenerates its policy fragment. It never builds an
// image and never commits.
func (a *App) ApplyDenial(ctx context.Context, runID string, request DenialRequest) (DenialApplyResult, error) {
	var result DenialApplyResult
	preview, err := a.denialPreview(ctx, runID, request)
	if err != nil {
		return result, err
	}
	result.DenialPreview = preview
	if request.Persist && !preview.PersistAvailable {
		return result, &ValidationError{Message: preview.PersistReason}
	}
	run, err := a.Store.LoadRun(runID)
	if err != nil {
		return result, err
	}
	old, err := os.ReadFile(run.Policy)
	if err != nil {
		return result, err
	}
	next, ruleName, err := addHostRule(old, preview.Entry)
	if err != nil {
		return result, err
	}
	saved, err := a.savePolicyEdit(ctx, run, old, next)
	result.Outcome = saved.Outcome
	result.RolledBack = saved.RolledBack
	if saved.PolicyPath != "" {
		result.PolicyPath = saved.PolicyPath
	}
	if err != nil {
		return result, err
	}
	runtimeHost := RuntimeHost{
		Host:    preview.Entry.Host,
		Port:    preview.Entry.Port,
		Binary:  preview.Entry.Binaries[0],
		Rule:    ruleName,
		Source:  "run",
		AddedAt: time.Now().UTC(),
	}
	var persistErr error
	if request.Persist {
		manifest, err := a.readManifestPreview(run, preview.Entry)
		if err != nil {
			persistErr = fmt.Errorf("run policy updated, but the manifest could not be prepared: %w", err)
		} else {
			mode := os.FileMode(0644)
			if info, statErr := os.Stat(manifest.Path); statErr == nil {
				mode = info.Mode().Perm()
			}
			if err := atomicWriteMode(manifest.Path, manifest.Content, mode); err != nil {
				persistErr = fmt.Errorf("run policy updated, but %s could not be written: %w", manifest.Path, err)
			} else {
				result.Persisted = true
				result.ManifestPath = manifest.Path
				runtimeHost.Source = "run+manifest"
				status, err := applyRepoFragment(run.PolicyFragment, manifest.Fragment, false)
				result.FragmentStatus = status
				if err != nil {
					persistErr = fmt.Errorf("run policy and manifest updated, but the policy fragment %s could not be regenerated: %w", run.PolicyFragment, err)
				}
			}
		}
	}
	result.RuntimeHost = runtimeHost
	replaced := false
	for i := range run.RuntimeHosts {
		existing := &run.RuntimeHosts[i]
		if strings.EqualFold(existing.Host, runtimeHost.Host) && existing.Port == runtimeHost.Port && existing.Binary == runtimeHost.Binary {
			existing.Source = runtimeHost.Source
			existing.Rule = ruleName
			replaced = true
			break
		}
	}
	if !replaced {
		run.RuntimeHosts = append(run.RuntimeHosts, runtimeHost)
	}
	if err := a.Store.SaveRun(run); err != nil {
		return result, err
	}
	if persistErr != nil {
		return result, persistErr
	}
	return result, nil
}

func (a *App) denialPreview(ctx context.Context, runID string, request DenialRequest) (DenialPreview, error) {
	var preview DenialPreview
	run, err := a.Store.LoadRun(runID)
	if err != nil {
		return preview, err
	}
	if !run.Status.IsAttachable() {
		return preview, &NoLiveSandboxError{}
	}
	rule := RepoHostRule{
		Host:     strings.ToLower(strings.TrimSpace(request.Host)),
		Port:     request.Port,
		Binaries: []string{strings.TrimSpace(request.Binary)},
	}
	if err := ValidateHostRule(rule); err != nil {
		return preview, &ValidationError{Message: err.Error()}
	}
	denial, err := a.findDenial(ctx, runID, rule)
	if err != nil {
		return preview, err
	}
	if !denial.Valid {
		return preview, &ValidationError{Message: "This denial cannot be turned into a host rule: " + denial.InvalidReason}
	}
	old, err := os.ReadFile(run.Policy)
	if err != nil {
		return preview, err
	}
	next, _, err := addHostRule(old, rule)
	if err != nil {
		return preview, err
	}
	preview.Entry = rule
	preview.PolicyPath = run.Policy
	preview.PolicyDiff = simpleDiff(old, next)
	if err := a.describePersist(&preview, run, rule, request.Persist); err != nil {
		return preview, err
	}
	return preview, nil
}

func (a *App) findDenial(ctx context.Context, runID string, rule RepoHostRule) (PolicyDenial, error) {
	denials, err := a.RunDenials(ctx, runID)
	if err != nil {
		return PolicyDenial{}, err
	}
	for _, denial := range denials {
		if strings.EqualFold(denial.Host, rule.Host) && denial.Port == rule.Port && denial.Binary == rule.Binaries[0] {
			return denial, nil
		}
	}
	return PolicyDenial{}, &ValidationError{Message: fmt.Sprintf("No active denial for %s:%d from %s in the current run output. Refresh the logs and try again.", rule.Host, rule.Port, rule.Binaries[0])}
}

type manifestPreview struct {
	Path     string
	Content  []byte
	Fragment []byte
}

func (a *App) readManifestPreview(run Run, rule RepoHostRule) (manifestPreview, error) {
	root := run.Worktree
	if root == "" {
		root = run.Source
	}
	if root == "" {
		return manifestPreview{}, &ValidationError{Message: "This run has no checkout to persist the rule into."}
	}
	path := RepoConfigPath(root)
	old, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return manifestPreview{}, &ValidationError{Message: fmt.Sprintf("No repository manifest at %s. Author .agentbox/config.yaml and run `agentbox repo setup` before persisting host rules.", path)}
	}
	if err != nil {
		return manifestPreview{}, err
	}
	next, err := AppendRepoHostRule(old, rule)
	if err != nil {
		return manifestPreview{}, err
	}
	cfg, err := ParseRepoConfig(next)
	if err != nil {
		return manifestPreview{}, err
	}
	fragment, err := RepoPolicyFragment(cfg)
	if err != nil {
		return manifestPreview{}, err
	}
	return manifestPreview{Path: path, Content: next, Fragment: fragment}, nil
}

// describePersist fills the persist availability and, when the dialog has the
// persist option selected, the exact manifest and fragment diffs.
func (a *App) describePersist(preview *DenialPreview, run Run, rule RepoHostRule, withDiff bool) error {
	if run.PolicyFragment == "" {
		preview.PersistAvailable = false
		preview.PersistReason = "This run uses its profile policy. Start a run from a repository image to persist host rules in .agentbox/config.yaml."
		return nil
	}
	preview.FragmentPath = run.PolicyFragment
	manifest, err := a.readManifestPreview(run, rule)
	if err != nil {
		// A run-only fix is still useful, so an unreadable or unusable
		// manifest disables the persist option instead of failing the dialog.
		preview.PersistAvailable = false
		preview.PersistReason = err.Error()
		return nil
	}
	preview.PersistAvailable = true
	preview.ManifestPath = manifest.Path
	if !withDiff {
		return nil
	}
	oldManifest, err := os.ReadFile(manifest.Path)
	if err != nil {
		return err
	}
	preview.ManifestDiff = fileDiff(oldManifest, manifest.Content)
	oldFragment, err := os.ReadFile(run.PolicyFragment)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	preview.FragmentDiff = fileDiff(oldFragment, manifest.Fragment)
	return nil
}

func fileDiff(old, next []byte) string {
	if bytes.Equal(old, next) {
		return "No file changes.\n"
	}
	return fmt.Sprintf("--- current\n+++ proposed\n-%s\n+%s\n",
		strings.ReplaceAll(strings.TrimSuffix(string(old), "\n"), "\n", "\n-"),
		strings.ReplaceAll(strings.TrimSuffix(string(next), "\n"), "\n", "\n+"))
}
