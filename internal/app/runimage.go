package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"
)

// ImageSource records where a run's effective image came from.
type ImageSource string

const (
	// ImageSourceProfile is the profile's base image, used when the project
	// has no manifest or no repository image matches it.
	ImageSourceProfile ImageSource = "profile"
	// ImageSourceRepo is a repository image layered on a current base image.
	ImageSourceRepo ImageSource = "repo"
	// ImageSourceOverride is an explicit tag passed for one run.
	ImageSourceOverride ImageSource = "override"
)

// RunImage is the image layer one run starts from, plus the provenance that
// run details surface. The zero value is the profile image.
type RunImage struct {
	Image          string
	BaseImage      string
	Source         ImageSource
	Repo           string
	ManifestDigest string
	FragmentPath   string
	Warnings       []string

	fragment []byte
}

// ComposePolicy returns the policy bytes a run materializes. A repository
// image unions the repository fragment into the base policy; every other
// source keeps the base policy unchanged.
func (i RunImage) ComposePolicy(base []byte) ([]byte, error) {
	if i.Source != ImageSourceRepo {
		return base, nil
	}
	return composeRepoPolicy(base, i.fragment)
}

// ResolveRunImage picks the image a run starts from and explains the choice.
// A project without a manifest keeps the profile image. A project with a
// manifest uses the repository image whose content matches the manifest on a
// current-recipe base, preferring the newest base, and reports drift instead
// of failing. Nothing here builds an image.
func (a *App) ResolveRunImage(ctx context.Context, p Profile, root string) (RunImage, error) {
	image := RunImage{Image: p.Image, BaseImage: p.Image, Source: ImageSourceProfile}
	cfg, err := LoadRepoConfig(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return image, nil
		}
		return RunImage{}, err
	}
	manifestBytes, err := os.ReadFile(RepoConfigPath(root))
	if err != nil {
		return RunImage{}, err
	}
	digest := repoManifestDigest(manifestBytes)
	image.ManifestDigest = digest

	remote, err := a.gitOutput(ctx, root, "remote", "get-url", "origin")
	if err != nil {
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository identity unavailable (%v); the run uses the profile image", err))
		return image, nil
	}
	repo := normalizeGitRemote(remote)
	if repo == "" {
		image.Warnings = append(image.Warnings, "repository origin remote could not be normalized; the run uses the profile image")
		return image, nil
	}
	image.Repo = repo
	image.FragmentPath = filepath.Join(a.Store.PolicyDir(), "repo-"+repoKey(repo)+".yaml")

	images, err := a.LocalImages(ctx)
	if err != nil {
		return RunImage{}, err
	}
	found, base, newestBase, ok := repoImageForAgent(images, repo, p.Agent, cfg)
	fragment, fragmentErr := os.ReadFile(image.FragmentPath)
	switch {
	case !ok:
		image.Warnings = append(image.Warnings, fmt.Sprintf("no current repository image for %s in %s; run `agentbox repo setup --agent %s` — this run uses the profile image", p.Agent, repo, p.Agent))
		return image, nil
	case fragmentErr != nil:
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository policy fragment %s is unavailable; run `agentbox repo setup --agent %s` — this run uses the profile image", image.FragmentPath, p.Agent))
		return image, nil
	}
	if _, err := ParsePolicy(fragment); err != nil {
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository policy fragment %s is invalid (%v); run `agentbox repo setup` — this run uses the profile image", image.FragmentPath, err))
		return image, nil
	}
	image.Image = found.Reference
	image.BaseImage = base.Reference
	image.Source = ImageSourceRepo
	image.fragment = fragment
	if found.ManifestDigest != "" && found.ManifestDigest != digest {
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository manifest changed since image %s was built; run `agentbox repo setup` to refresh the policy fragment", found.Reference))
	}
	if newestBase.Reference != "" && newestBase.Reference != base.Reference {
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository image %s is built on %s while the newest local base is %s; run `agentbox repo setup` to rebuild", found.Reference, base.Reference, newestBase.Reference))
	}
	return image, nil
}

// repoImageForAgent finds the repository image whose content matches cfg on
// some current-recipe base for the agent. The tag hash covers the base
// reference, so recomputing it per candidate base also identifies the base an
// existing image was built on, without new image labels.
func repoImageForAgent(images []ImageArtifact, repo, agent string, cfg RepoConfig) (found, base, newest ImageArtifact, ok bool) {
	recipe, err := strconv.Atoi(CurrentImageRecipe())
	if err != nil {
		return ImageArtifact{}, ImageArtifact{}, ImageArtifact{}, false
	}
	var bases []ImageArtifact
	for _, image := range images {
		if image.Worker || image.Agent != agent || image.Recipe != recipe {
			continue
		}
		bases = append(bases, image)
		if newest.Reference == "" || compareImageVersions(image.Version, newest.Version) > 0 {
			newest = image
		}
	}
	for _, image := range images {
		if !image.Worker || image.Repo != repo || image.Agent != agent {
			continue
		}
		for _, candidate := range bases {
			hash, err := repoImageHash(candidate.Reference, cfg)
			if err != nil {
				continue
			}
			if !imageHasReference(image, repoImageTag(repo, agent, hash)) {
				continue
			}
			if base.Reference == "" || compareImageVersions(candidate.Version, base.Version) > 0 {
				found, base, ok = image, candidate, true
			}
		}
	}
	return found, base, newest, ok
}

func imageHasReference(image ImageArtifact, reference string) bool {
	if image.Reference == reference {
		return true
	}
	for _, candidate := range image.References {
		if candidate == reference {
			return true
		}
	}
	return false
}

// composeRepoPolicy merges the repository fragment's network rules into the
// base policy. The base owns the static sections, the process user, and the
// Landlock hard requirement. Identical rules collapse to one, and two rules
// that claim the same named scope with different content are a conflict: the
// fragment and the profile must agree instead of one silently winning.
// #79 replaces the base policy input without touching this composition.
func composeRepoPolicy(base, fragment []byte) ([]byte, error) {
	basePolicy, err := ParsePolicy(base)
	if err != nil {
		return nil, err
	}
	fragmentPolicy, err := ParsePolicy(fragment)
	if err != nil {
		return nil, err
	}
	if basePolicy.Network == nil {
		basePolicy.Network = map[string]NetworkRule{}
	}
	for name, rule := range fragmentPolicy.Network {
		rule.Name = name
		if existing, ok := basePolicy.Network[name]; ok {
			existing.Name = name
			if !reflect.DeepEqual(canonicalRule(existing), canonicalRule(rule)) {
				return nil, fmt.Errorf("network rule %q is defined differently by the profile policy and the repository fragment; align them before running", name)
			}
			continue
		}
		basePolicy.Network[name] = rule
	}
	return yaml.Marshal(basePolicy)
}

// canonicalRule strips the rule name and orders endpoints and binaries so
// equal scopes compare equal regardless of how they were written.
func canonicalRule(rule NetworkRule) NetworkRule {
	rule.Name = ""
	endpoints := append([]Endpoint(nil), rule.Endpoints...)
	for i := range endpoints {
		if endpoints[i].Protocol == "" {
			endpoints[i].Protocol = "tcp"
		}
	}
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Host != endpoints[j].Host {
			return endpoints[i].Host < endpoints[j].Host
		}
		if endpoints[i].Port != endpoints[j].Port {
			return endpoints[i].Port < endpoints[j].Port
		}
		return endpoints[i].Protocol < endpoints[j].Protocol
	})
	binaries := append([]Binary(nil), rule.Binaries...)
	sort.Slice(binaries, func(i, j int) bool { return binaries[i].Path < binaries[j].Path })
	rule.Endpoints = endpoints
	rule.Binaries = binaries
	return rule
}
