package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	FragmentDigest string
	Warnings       []string

	fragment []byte
}

// ResolveRunImage picks the image a run starts from and explains the choice.
// A project without a manifest keeps the profile image. A project with a
// manifest uses the repository image whose content matches the manifest on a
// current-recipe base, preferring the newest base, and warns about a stale
// policy fragment or base instead of failing. Unusable repository artifacts
// never block a run: they warn and leave it on the profile image. Nothing
// here builds an image.
func (a *App) ResolveRunImage(ctx context.Context, p Profile, root string) RunImage {
	image := RunImage{Image: p.Image, BaseImage: p.Image, Source: ImageSourceProfile}
	manifestPath := RepoConfigPath(root)
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return image
		}
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository manifest %s is unreadable (%v); run `agentbox repo setup` — this run uses the profile image", manifestPath, err))
		return image
	}
	cfg, err := ParseRepoConfig(manifestBytes)
	if err != nil {
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository manifest %s is unusable (%v); run `agentbox repo setup` — this run uses the profile image", manifestPath, err))
		return image
	}
	expectedFragment, err := RepoPolicyFragment(cfg)
	if err != nil {
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository policy fragment cannot be generated from %s (%v); run `agentbox repo setup` — this run uses the profile image", manifestPath, err))
		return image
	}
	digest := repoManifestDigest(manifestBytes)
	image.ManifestDigest = digest

	repo, err := a.RepoIdentity(ctx, root)
	if err != nil {
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository identity unavailable (%v); the run uses the profile image", err))
		return image
	}
	if repo == "" {
		image.Warnings = append(image.Warnings, "repository origin remote could not be normalized; the run uses the profile image")
		return image
	}
	image.Repo = repo
	image.FragmentPath = filepath.Join(a.Store.PolicyDir(), "repo-"+repoKey(repo)+".yaml")

	images, err := a.LocalImages(ctx)
	if err != nil {
		image.Warnings = append(image.Warnings, fmt.Sprintf("local image inventory unavailable (%v); this run uses the profile image", err))
		return image
	}
	found, base, newestBase, ok := repoImageForAgent(images, repo, p.Agent, cfg)
	fragment, fragmentErr := os.ReadFile(image.FragmentPath)
	switch {
	case !ok:
		image.Warnings = append(image.Warnings, fmt.Sprintf("no current repository image for %s in %s; run `agentbox repo setup --agent %s` — this run uses the profile image", p.Agent, repo, p.Agent))
		return image
	case fragmentErr != nil:
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository policy fragment %s is unavailable; run `agentbox repo setup --agent %s` — this run uses the profile image", image.FragmentPath, p.Agent))
		return image
	}
	if _, err := ParsePolicy(fragment); err != nil {
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository policy fragment %s is invalid (%v); run `agentbox repo setup` — this run uses the profile image", image.FragmentPath, err))
		return image
	}
	image.Image = found.Reference
	image.BaseImage = base.Reference
	image.Source = ImageSourceRepo
	image.fragment = fragment
	image.FragmentDigest = contentDigest(fragment)
	if !bytes.Equal(fragment, expectedFragment) {
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository policy fragment %s is out of date with %s; run `agentbox repo setup` to refresh it", image.FragmentPath, manifestPath))
	}
	if newestBase.Reference != "" && newestBase.Reference != base.Reference {
		image.Warnings = append(image.Warnings, fmt.Sprintf("repository image %s is built on %s while the newest local base is %s; run `agentbox repo setup` to rebuild", found.Reference, base.Reference, newestBase.Reference))
	}
	return image
}

// RepoIdentity normalizes a checkout's origin remote. It is the key that
// matches a project to its repository images and policy fragment.
func (a *App) RepoIdentity(ctx context.Context, root string) (string, error) {
	remote, err := a.gitOutput(ctx, root, "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return normalizeGitRemote(remote), nil
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

// canonicalRule strips the rule name and orders endpoints and binaries so
// equal scopes compare equal regardless of how they were written.
func canonicalRule(rule NetworkRule) NetworkRule {
	rule.Name = ""
	endpoints := append([]Endpoint{}, rule.Endpoints...)
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
	binaries := append([]Binary{}, rule.Binaries...)
	sort.Slice(binaries, func(i, j int) bool { return binaries[i].Path < binaries[j].Path })
	rule.Endpoints = endpoints
	rule.Binaries = binaries
	return rule
}
