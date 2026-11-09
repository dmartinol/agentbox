package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type ImageArtifact struct {
	Reference string `json:"reference"`
	Agent     string `json:"agent"`
	Version   string `json:"version"`
	Recipe    int    `json:"recipe"`
	OS        string `json:"os,omitempty"`
	Arch      string `json:"arch,omitempty"`
	Digest    string `json:"digest,omitempty"`
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
	var records []struct {
		Names  []string          `json:"Names"`
		Digest string            `json:"Digest"`
		Arch   string            `json:"Arch"`
		OS     string            `json:"Os"`
		Labels map[string]string `json:"Labels"`
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	var artifacts []ImageArtifact
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
		for _, name := range record.Names {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			artifact := ImageArtifact{
				Reference: name,
				Agent:     label("agent"),
				Version:   label("version"),
				Recipe:    recipe,
				OS:        record.OS,
				Arch:      record.Arch,
				Digest:    record.Digest,
			}
			if artifact.Agent == "" || artifact.Version == "" {
				continue
			}
			artifacts = append(artifacts, artifact)
		}
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Reference < artifacts[j].Reference })
	return artifacts, nil
}

func (a *App) LocalImage(ctx context.Context, reference string) (ImageArtifact, error) {
	images, err := a.LocalImages(ctx)
	if err != nil {
		return ImageArtifact{}, err
	}
	for _, image := range images {
		if image.Reference == reference {
			return image, nil
		}
	}
	return ImageArtifact{}, fmt.Errorf("local AgentBox image %q not found", reference)
}
