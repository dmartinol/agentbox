package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

type ImageArtifact struct {
	Reference  string   `json:"reference"`
	Agent      string   `json:"agent"`
	Version    string   `json:"version"`
	Recipe     int      `json:"recipe"`
	OS         string   `json:"os,omitempty"`
	Arch       string   `json:"arch,omitempty"`
	Digest     string   `json:"digest,omitempty"`
	References []string `json:"-"`
}

var remoteImageTag = regexp.MustCompile(`^(claude|codex|opencode)-(.+)-r([0-9]+)(?:-([a-z0-9]+)-([a-z0-9]+))?$`)

func (img ImageArtifact) RemoteTag() string {
	tag := fmt.Sprintf("%s-%s-r%d", img.Agent, img.Version, img.Recipe)
	if img.OS != "" && img.Arch != "" {
		tag += "-" + img.OS + "-" + img.Arch
	}
	return tag
}

func RemoteReference(base string, img ImageArtifact) string {
	return strings.TrimSuffix(strings.TrimSpace(base), "/") + ":" + img.RemoteTag()
}

func ParseRemoteTag(tag string) (ImageArtifact, bool) {
	matches := remoteImageTag.FindStringSubmatch(tag)
	if matches == nil {
		return ImageArtifact{}, false
	}
	recipe, err := strconv.Atoi(matches[3])
	if err != nil {
		return ImageArtifact{}, false
	}
	return ImageArtifact{
		Agent:   matches[1],
		Version: matches[2],
		Recipe:  recipe,
		OS:      matches[4],
		Arch:    matches[5],
	}, true
}

// LocalImages returns AgentBox images known to the local Podman store.
func (a *App) LocalImages(ctx context.Context) ([]ImageArtifact, error) {
	data, err := a.ImageList(ctx)
	if err != nil {
		return nil, err
	}
	return parseLocalImages(data)
}

func parseLocalImages(data []byte) ([]ImageArtifact, error) {
	var records []struct {
		ID     string            `json:"Id"`
		Names  []string          `json:"Names"`
		Digest string            `json:"Digest"`
		Arch   string            `json:"Arch"`
		OS     string            `json:"Os"`
		Labels map[string]string `json:"Labels"`
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	artifacts := make([]ImageArtifact, 0, len(records))
	seen := map[string]struct{}{}
	for _, record := range records {
		label := func(name string) string {
			if value := record.Labels["agentbox."+name]; value != "" {
				return value
			}
			return record.Labels["ai-sandbox."+name]
		}
		recipe, err := strconv.Atoi(label("recipe"))
		if err != nil {
			continue
		}
		artifact := ImageArtifact{
			Reference:  preferredImageReference(record.Names, label("agent"), label("version"), recipe),
			References: record.Names,
			Agent:      label("agent"),
			Version:    label("version"),
			Recipe:     recipe,
			OS:         record.OS,
			Arch:       record.Arch,
			Digest:     record.Digest,
		}
		if artifact.Agent == "" || artifact.Version == "" || artifact.Reference == "" {
			continue
		}
		key := record.ID
		if key == "" {
			key = record.Digest
		}
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		artifacts = append(artifacts, artifact)
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Reference < artifacts[j].Reference })
	return artifacts, nil
}

func preferredImageReference(names []string, agent, version string, recipe int) string {
	if len(names) == 0 {
		return ""
	}
	localTag := fmt.Sprintf("localhost/openshell:%s-%s-r%d", agent, version, recipe)
	for _, name := range names {
		if name == localTag {
			return name
		}
	}
	for _, name := range names {
		if strings.HasPrefix(name, "localhost/") {
			return name
		}
	}
	return names[0]
}

func (a *App) LocalImage(ctx context.Context, reference string) (ImageArtifact, error) {
	data, err := a.ImageList(ctx)
	if err != nil {
		return ImageArtifact{}, err
	}
	images, err := parseLocalImages(data)
	if err != nil {
		return ImageArtifact{}, err
	}
	for _, image := range images {
		if image.Reference == reference || slices.Contains(image.References, reference) {
			return image, nil
		}
	}
	return ImageArtifact{}, fmt.Errorf("local AgentBox image %q not found", reference)
}

// ImageUsage maps local AgentBox image references to the profiles and
// non-terminal runs that still reference them.
func (a *App) ImageUsage() (map[string][]string, error) {
	usage := map[string][]string{}
	if a.Store.Base == "" {
		return usage, nil
	}
	profiles, err := a.Store.ListProfiles()
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if profile.Image != "" {
			usage[profile.Image] = append(usage[profile.Image], "profile "+profile.Name)
		}
	}
	runs, err := a.Store.ListRuns()
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if run.Image == "" || terminalRunStatus(run.Status) {
			continue
		}
		name := run.ID
		if run.Name != "" {
			name = run.Name
		}
		usage[run.Image] = append(usage[run.Image], "run "+name+" ("+string(run.Status)+")")
	}
	for reference := range usage {
		sort.Strings(usage[reference])
	}
	return usage, nil
}

func terminalRunStatus(status RunStatus) bool {
	switch status {
	case StatusFailed, StatusStopped, StatusCompleted:
		return true
	}
	return false
}

// ImageRemove deletes one local AgentBox image. Images referenced by a profile
// or a non-terminal run are refused with the names of their holders.
func (a *App) ImageRemove(ctx context.Context, reference string) error {
	image, err := a.LocalImage(ctx, reference)
	if err != nil {
		return err
	}
	usage, err := a.ImageUsage()
	if err != nil {
		return err
	}
	var holders []string
	for _, imageReference := range image.References {
		holders = append(holders, usage[imageReference]...)
	}
	if len(holders) > 0 {
		sort.Strings(holders)
		return fmt.Errorf("image %q is in use by %s", reference, strings.Join(holders, ", "))
	}
	_, err = a.Runner.Output(ctx, "podman", "image", "rm", image.Reference)
	return err
}

// ImagePrune deletes named AgentBox images that no profile and no non-terminal
// run references, plus dangling images explicitly labeled as AgentBox images.
func (a *App) ImagePrune(ctx context.Context) ([]string, error) {
	data, err := a.ImageList(ctx)
	if err != nil {
		return nil, err
	}
	images, err := parseLocalImages(data)
	if err != nil {
		return nil, err
	}
	dangling, err := danglingAgentBoxImageIDs(data)
	if err != nil {
		return nil, err
	}
	usage, err := a.ImageUsage()
	if err != nil {
		return nil, err
	}
	removed := make([]string, 0, len(images))
	for _, image := range images {
		if imageIsUsed(image, usage) {
			continue
		}
		if _, err := a.Runner.Output(ctx, "podman", "image", "rm", image.Reference); err != nil {
			return removed, fmt.Errorf("remove %q: %w", image.Reference, err)
		}
		removed = append(removed, image.Reference)
	}
	for _, imageID := range dangling {
		if _, err := a.Runner.Output(ctx, "podman", "image", "rm", imageID); err != nil {
			return removed, fmt.Errorf("remove %q: %w", imageID, err)
		}
		removed = append(removed, imageID)
	}
	return removed, nil
}

func danglingAgentBoxImageIDs(data []byte) ([]string, error) {
	var records []struct {
		ID     string            `json:"Id"`
		Names  []string          `json:"Names"`
		Labels map[string]string `json:"Labels"`
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	for _, record := range records {
		if record.ID != "" && len(record.Names) == 0 && record.Labels["agentbox"] == "true" {
			ids = append(ids, record.ID)
		}
	}
	return ids, nil
}

func imageIsUsed(image ImageArtifact, usage map[string][]string) bool {
	for _, reference := range image.References {
		if len(usage[reference]) > 0 {
			return true
		}
	}
	return false
}
