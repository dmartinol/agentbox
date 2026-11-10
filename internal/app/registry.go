package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func (a *App) Registry() (Registry, error) {
	return a.Store.LoadRegistry()
}

func (a *App) SetRegistry(reference string) error {
	return a.Store.SaveRegistry(Registry{Reference: reference})
}

func (a *App) ClearRegistry() error {
	return a.Store.ClearRegistry()
}

// RegistryDiscover lists AgentBox image tags for the configured repository.
// It uses Podman's existing login state and does not handle credentials.
func (a *App) RegistryDiscover(ctx context.Context) ([]ImageArtifact, error) {
	registry, err := a.Store.LoadRegistry()
	if err != nil {
		return nil, err
	}
	if registry.Reference == "" {
		return nil, fmt.Errorf("no external registry configured")
	}
	reference := registry.NormalizedReference()
	data, err := a.Runner.Output(ctx, "podman", "search", "--list-tags", "--format", "json", reference)
	if err != nil {
		return nil, err
	}
	images, err := parseRegistrySearch(data, reference)
	if err != nil {
		return nil, err
	}
	return images, nil
}

func parseRegistrySearch(data []byte, fallbackReference string) ([]ImageArtifact, error) {
	tags, err := registryTags(data, fallbackReference)
	if err != nil {
		return nil, err
	}
	images := make([]ImageArtifact, 0, len(tags))
	for _, tag := range tags {
		artifact, ok := ParseRemoteTag(tag)
		if !ok {
			continue
		}
		artifact.Reference = firstTagReference(data, tag, fallbackReference)
		images = append(images, artifact)
	}
	sort.Slice(images, func(i, j int) bool {
		if images[i].Agent != images[j].Agent {
			return images[i].Agent < images[j].Agent
		}
		if images[i].Version != images[j].Version {
			return images[i].Version < images[j].Version
		}
		return images[i].Reference < images[j].Reference
	})
	return images, nil
}

func registryTags(data []byte, fallbackReference string) ([]string, error) {
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err == nil {
		var tags []string
		for _, row := range rows {
			tag := firstString(row, "Tag", "tag")
			if tag == "" {
				name := firstString(row, "Name", "name", "Repository", "repository")
				if name != "" {
					if idx := strings.LastIndex(name, ":"); idx > strings.LastIndex(name, "/") {
						tag = name[idx+1:]
					}
				}
			}
			if tag != "" {
				tags = append(tags, tag)
			}
		}
		return tags, nil
	}
	var stringsOnly []string
	if err := json.Unmarshal(data, &stringsOnly); err != nil {
		return nil, fmt.Errorf("unrecognized registry discovery output")
	}
	for i, image := range stringsOnly {
		if idx := strings.LastIndex(image, ":"); idx > strings.LastIndex(image, "/") {
			stringsOnly[i] = image[idx+1:]
		}
	}
	return stringsOnly, nil
}

func firstTagReference(data []byte, tag, fallbackReference string) string {
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err == nil {
		for _, row := range rows {
			name := firstString(row, "Name", "name", "Repository", "repository")
			rowTag := firstString(row, "Tag", "tag")
			if rowTag == tag || (rowTag == "" && strings.HasSuffix(name, ":"+tag)) {
				if name != "" {
					if strings.HasSuffix(name, ":"+tag) {
						return name
					}
					return strings.TrimSuffix(name, "/") + ":" + tag
				}
			}
		}
	}
	return strings.TrimSuffix(fallbackReference, "/") + ":" + tag
}

func firstString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := row[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
