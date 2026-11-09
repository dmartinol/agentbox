package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const recipeRevision = "3"
const baseImage = "docker.io/library/node:22-bookworm-slim"

var exactVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-.][a-zA-Z0-9.-]+)?$`)

func agentPackage(agent string) (string, error) {
	switch agent {
	case "claude":
		return "@anthropic-ai/claude-code", nil
	case "codex":
		return "@openai/codex", nil
	case "opencode":
		return "@opencode/cli", nil
	default:
		return "", fmt.Errorf("agent must be claude, codex, or opencode")
	}
}

func (a *App) ImageVersion(ctx context.Context, agent string) (string, error) {
	pkg, err := agentPackage(agent)
	if err != nil {
		return "", err
	}
	b, err := a.Runner.Output(ctx, "npm", "view", pkg, "version", "--json")
	if err != nil {
		return "", err
	}
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return "", err
	}
	if !exactVersion.MatchString(v) {
		return "", fmt.Errorf("unrecognized release version %q", v)
	}
	if agent == "opencode" && !strings.HasPrefix(v, "2.") {
		return "", fmt.Errorf("OpenCode selected channel returned %q, expected v2", v)
	}
	return v, nil
}

type ImageRecord struct {
	Agent      string `json:"agent"`
	Version    string `json:"version"`
	Tag        string `json:"tag"`
	Base       string `json:"base"`
	BaseDigest string `json:"base_digest"`
	ImageID    string `json:"image_id"`
	Digest     string `json:"digest,omitempty"`
	Recipe     string `json:"recipe"`
}

func imageTag(agent, version string) string {
	return fmt.Sprintf("localhost/openshell:%s-%s-r%s", agent, version, recipeRevision)
}

func (a *App) ImageBuild(ctx context.Context, agent, version string, dry bool) (ImageRecord, error) {
	pkg, err := agentPackage(agent)
	if err != nil {
		return ImageRecord{}, err
	}
	if version == "" {
		version, err = a.ImageVersion(ctx, agent)
		if err != nil {
			return ImageRecord{}, err
		}
	}
	if !exactVersion.MatchString(version) || (agent == "opencode" && !strings.HasPrefix(version, "2.")) {
		return ImageRecord{}, fmt.Errorf("exact compatible version required")
	}
	rec := ImageRecord{Agent: agent, Version: version, Tag: imageTag(agent, version), Base: baseImage, Recipe: recipeRevision}
	if dry {
		return rec, nil
	}
	if err := a.Runner.Stream(ctx, nil, os.Stdout, os.Stderr, "podman", "pull", baseImage); err != nil {
		return rec, err
	}
	b, err := a.Runner.Output(ctx, "podman", "image", "inspect", baseImage, "--format", "{{.Digest}}")
	if err != nil {
		return rec, err
	}
	rec.BaseDigest = strings.TrimSpace(string(b))
	if !strings.HasPrefix(rec.BaseDigest, "sha256:") {
		return rec, fmt.Errorf("base image digest unavailable")
	}
	dir, err := os.MkdirTemp("", "agentbox-image-*")
	if err != nil {
		return rec, err
	}
	defer os.RemoveAll(dir)
	dockerfile := fmt.Sprintf("FROM docker.io/library/node@%s\nRUN apt-get update && apt-get install -y --no-install-recommends git tmux ca-certificates && rm -rf /var/lib/apt/lists/*\nRUN npm install -g '%s@%s' && npm cache clean --force\nRUN %s --version\nRUN groupadd sandbox && useradd -g sandbox -m -s /bin/sh sandbox && mkdir -p /workspace && chown sandbox:sandbox /workspace\nWORKDIR /workspace\nUSER sandbox\n", rec.BaseDigest, pkg, version, agentCommand(agent))
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0600); err != nil {
		return rec, err
	}
	args := []string{"build", "-t", rec.Tag, "--label", "agentbox=true", "--label", "agentbox.agent=" + agent, "--label", "agentbox.version=" + version, "--label", "agentbox.recipe=" + recipeRevision, "--label", "agentbox.base-digest=" + rec.BaseDigest, dir}
	if err := a.Runner.Stream(ctx, nil, os.Stdout, os.Stderr, "podman", args...); err != nil {
		return rec, err
	}
	b, err = a.Runner.Output(ctx, "podman", "image", "inspect", rec.Tag, "--format", "{{.Id}} {{.Digest}}")
	if err != nil {
		return rec, err
	}
	fields := strings.Fields(string(b))
	if len(fields) > 0 {
		rec.ImageID = fields[0]
	}
	if len(fields) > 1 && strings.HasPrefix(fields[1], "sha256:") {
		rec.Digest = fields[1]
	}
	if rec.ImageID == "" {
		return rec, fmt.Errorf("built image ID unavailable")
	}
	return rec, nil
}

func agentCommand(agent string) string {
	if agent == "claude" {
		return "claude"
	}
	if agent == "opencode" {
		return "opencode"
	}
	return "codex"
}

func (a *App) ImageList(ctx context.Context) ([]byte, error) {
	raw, err := a.Runner.Output(ctx, "podman", "images", "--format", "json")
	if err != nil {
		return nil, err
	}
	var records []json.RawMessage
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}
	filtered := make([]json.RawMessage, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		var meta struct {
			ID     string            `json:"Id"`
			Labels map[string]string `json:"Labels"`
		}
		if err := json.Unmarshal(record, &meta); err != nil {
			return nil, err
		}
		if meta.Labels["agentbox"] == "true" || meta.Labels["ai-sandbox.spike"] == "true" {
			// Podman emits one JSON row per repository tag, but each row for a
			// multi-tagged image repeats the full Names list and image ID.
			if meta.ID != "" {
				if _, ok := seen[meta.ID]; ok {
					continue
				}
				seen[meta.ID] = struct{}{}
			}
			filtered = append(filtered, record)
		}
	}
	return json.Marshal(filtered)
}

func (a *App) ImagePush(ctx context.Context, localTag, remote string, dry bool) error {
	if localTag == "" || remote == "" {
		return fmt.Errorf("local and remote image references required")
	}
	if dry {
		fmt.Printf("podman tag %s %s\npodman push %s\n", localTag, remote, remote)
		return nil
	}
	if _, err := a.Runner.Output(ctx, "podman", "image", "inspect", localTag); err != nil {
		return err
	}
	if _, err := a.Runner.Output(ctx, "podman", "tag", localTag, remote); err != nil {
		return err
	}
	return a.Runner.Stream(ctx, nil, os.Stdout, os.Stderr, "podman", "push", remote)
}

func (a *App) ImagePull(ctx context.Context, ref string, dry bool) error {
	if ref == "" {
		return fmt.Errorf("remote image reference required")
	}
	if dry {
		fmt.Println("podman pull", ref)
		return nil
	}
	return a.Runner.Stream(ctx, nil, os.Stdout, os.Stderr, "podman", "pull", ref)
}
