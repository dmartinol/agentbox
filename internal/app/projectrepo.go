package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

const (
	ProjectRepoFragmentAbsent  = "absent"
	ProjectRepoFragmentCurrent = "current"
	ProjectRepoFragmentStale   = "stale"
	ProjectRepoFragmentInvalid = "invalid"
)

type ProjectWorkerImageStatus struct {
	Agent          string `json:"agent"`
	Status         string `json:"status"`
	Reference      string `json:"reference,omitempty"`
	ManifestDigest string `json:"manifest_digest,omitempty"`
}

type ProjectRepoStatus struct {
	ManifestPresent bool                       `json:"manifest_present"`
	ManifestPath    string                     `json:"manifest_path"`
	ManifestDigest  string                     `json:"manifest_digest,omitempty"`
	FragmentPath    string                     `json:"fragment_path,omitempty"`
	FragmentDigest  string                     `json:"fragment_digest,omitempty"`
	FragmentStatus  string                     `json:"fragment_status"`
	HostRules       []RepoHostRule             `json:"host_rules,omitempty"`
	WorkerImages    []ProjectWorkerImageStatus `json:"worker_images"`
	Warnings        []string                   `json:"warnings,omitempty"`
}

func (a *App) ProjectRepoStatus(ctx context.Context, root string) ProjectRepoStatus {
	status := ProjectRepoStatus{
		ManifestPath:   RepoConfigPath(root),
		FragmentStatus: ProjectRepoFragmentAbsent,
		WorkerImages:   []ProjectWorkerImageStatus{},
		HostRules:      []RepoHostRule{},
	}
	manifest, err := os.ReadFile(status.ManifestPath)
	status.ManifestPresent = !errors.Is(err, os.ErrNotExist)
	if err != nil && status.ManifestPresent {
		status.Warnings = append(status.Warnings, fmt.Sprintf("repository manifest %s is unreadable: %v", status.ManifestPath, err))
	}
	var config RepoConfig
	manifestValid := false
	if status.ManifestPresent && err == nil {
		config, err = ParseRepoConfig(manifest)
		if err != nil {
			status.Warnings = append(status.Warnings, fmt.Sprintf("repository manifest %s is invalid: %v", status.ManifestPath, err))
		} else {
			manifestValid = true
			status.ManifestDigest = repoManifestDigest(manifest)
			if status.ManifestDigest == "" {
				status.Warnings = append(status.Warnings, fmt.Sprintf("repository manifest %s has no content digest; run `agentbox repo setup` to refresh repository images", status.ManifestPath))
			}
			status.HostRules = append(status.HostRules, config.Hosts...)
		}
	}

	repo, identityErr := a.RepoIdentity(ctx, root)
	if identityErr != nil {
		status.Warnings = append(status.Warnings, fmt.Sprintf("repository identity unavailable: %v", identityErr))
	} else if repo == "" {
		status.Warnings = append(status.Warnings, "repository identity unavailable: origin remote could not be normalized")
	}
	if repo != "" {
		status.FragmentPath = a.storeRepoFragmentPath(repo)
	}

	var images []ImageArtifact
	imageInventoryTried := false
	imageInventoryAvailable := false
	manifestUsable := manifestValid && status.ManifestDigest != ""
	if manifestUsable && identityErr == nil && repo != "" {
		var imageErr error
		imageInventoryTried = true
		images, imageErr = a.LocalImages(ctx)
		if imageErr != nil {
			status.Warnings = append(status.Warnings, fmt.Sprintf("local image inventory unavailable: %v", imageErr))
		} else {
			imageInventoryAvailable = true
		}
	}

	if status.FragmentPath != "" {
		fragment, fragmentErr := os.ReadFile(status.FragmentPath)
		switch {
		case errors.Is(fragmentErr, os.ErrNotExist):
			status.FragmentStatus = ProjectRepoFragmentAbsent
		case fragmentErr != nil:
			status.FragmentStatus = ProjectRepoFragmentInvalid
			status.Warnings = append(status.Warnings, fmt.Sprintf("repository policy fragment %s is unreadable: %v", status.FragmentPath, fragmentErr))
		default:
			status.FragmentDigest = contentDigest(fragment)
			if _, err := ParsePolicy(fragment); err != nil {
				status.FragmentStatus = ProjectRepoFragmentInvalid
				status.Warnings = append(status.Warnings, fmt.Sprintf("repository policy fragment %s is invalid: %v", status.FragmentPath, err))
			} else if manifestValid {
				expected, err := RepoPolicyFragment(config)
				if err != nil {
					status.FragmentStatus = ProjectRepoFragmentInvalid
					status.Warnings = append(status.Warnings, fmt.Sprintf("repository policy fragment for %s cannot be generated: %v", status.ManifestPath, err))
				} else if string(fragment) == string(expected) {
					status.FragmentStatus = ProjectRepoFragmentCurrent
				} else {
					status.FragmentStatus = ProjectRepoFragmentStale
				}
			} else {
				status.FragmentStatus = ProjectRepoFragmentInvalid
			}
		}
	}

	for _, agent := range []string{"claude", "codex", "opencode"} {
		worker := ProjectWorkerImageStatus{Agent: agent, Status: "missing"}
		if !manifestUsable {
			worker.Status = "missing"
		} else if identityErr != nil || repo == "" || (imageInventoryTried && !imageInventoryAvailable) {
			worker.Status = "unknown"
		} else if found, ok := projectWorkerImage(images, repo, agent, status.ManifestDigest, config); ok {
			worker.Status = "current"
			worker.Reference = found.Reference
			worker.ManifestDigest = found.ManifestDigest
		} else if stale, ok := projectAnyWorkerImage(images, repo, agent); ok {
			worker.Status = "stale"
			worker.Reference = stale.Reference
			worker.ManifestDigest = stale.ManifestDigest
		}
		status.WorkerImages = append(status.WorkerImages, worker)
	}
	sort.Slice(status.WorkerImages, func(left, right int) bool {
		return status.WorkerImages[left].Agent < status.WorkerImages[right].Agent
	})
	sort.Strings(status.Warnings)
	return status
}

func (a *App) storeRepoFragmentPath(repo string) string {
	return filepath.Join(a.Store.PolicyDir(), "repo-"+repoKey(repo)+".yaml")
}

// projectWorkerImage finds the repository image whose content-addressed tag
// matches the current manifest on a current-recipe base. Matching only the
// manifest digest is not enough: a previous build of the same manifest can
// still be present under a different tag, and reporting that one as current
// sends the GUI to an image the run resolver will not choose.
func projectWorkerImage(images []ImageArtifact, repo, agent, manifestDigest string, config RepoConfig) (ImageArtifact, bool) {
	if manifestDigest == "" {
		return ImageArtifact{}, false
	}
	recipe, err := strconv.Atoi(CurrentImageRecipe())
	if err != nil {
		return ImageArtifact{}, false
	}
	var bases []ImageArtifact
	for _, image := range images {
		if image.Worker || image.Agent != agent || image.Recipe != recipe {
			continue
		}
		bases = append(bases, image)
	}
	var found ImageArtifact
	var foundBase ImageArtifact
	for _, image := range images {
		if !image.Worker || image.Repo != repo || image.Agent != agent || image.ManifestDigest != manifestDigest {
			continue
		}
		for _, base := range bases {
			hash, err := repoImageHash(base.Reference, config)
			if err != nil {
				continue
			}
			if !imageHasReference(image, repoImageTag(repo, agent, hash)) {
				continue
			}
			if found.Reference == "" || compareImageVersions(base.Version, foundBase.Version) > 0 {
				found, foundBase = image, base
			}
		}
	}
	if found.Reference == "" {
		return ImageArtifact{}, false
	}
	return found, true
}

func projectAnyWorkerImage(images []ImageArtifact, repo, agent string) (ImageArtifact, bool) {
	for _, image := range images {
		if image.Worker && image.Repo == repo && image.Agent == agent {
			return image, true
		}
	}
	return ImageArtifact{}, false
}
