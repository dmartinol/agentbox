package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type runArtifactKind string

const (
	runArtifactSourceBundle  runArtifactKind = "-source.bundle"
	runArtifactConfigTar     runArtifactKind = "-config.tar"
	runArtifactInitialPatch  runArtifactKind = "-initial.patch"
	runArtifactMirrorPatch   runArtifactKind = "-mirror.patch"
	runArtifactMirrorTar     runArtifactKind = "-mirror.tar"
	runArtifactConfigPayload runArtifactKind = "-config"
)

var runArtifactSuffixes = []runArtifactKind{
	runArtifactSourceBundle,
	runArtifactConfigTar,
	runArtifactInitialPatch,
	runArtifactMirrorPatch,
	runArtifactMirrorTar,
	runArtifactConfigPayload,
}

func runArtifactPath(store Store, id string, kind runArtifactKind) string {
	return filepath.Join(store.RunsDir(), id+string(kind))
}

func runArtifactPaths(store Store, id string) []string {
	paths := make([]string, 0, len(runArtifactSuffixes))
	for _, kind := range runArtifactSuffixes {
		paths = append(paths, runArtifactPath(store, id, kind))
	}
	return paths
}

type PrunedRunArtifact struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

type RunArtifactPruneResult struct {
	DryRun     bool                `json:"dry_run"`
	Artifacts  []PrunedRunArtifact `json:"artifacts"`
	FreedBytes int64               `json:"freed_bytes"`
}

func (a *App) PruneRunArtifacts(dryRun bool) (RunArtifactPruneResult, error) {
	result := RunArtifactPruneResult{DryRun: dryRun, Artifacts: []PrunedRunArtifact{}}
	entries, err := os.ReadDir(a.Store.RunsDir())
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}

	paths := make([]string, 0)
	for _, entry := range entries {
		id, ok := runArtifactID(entry.Name())
		if !ok {
			continue
		}
		if _, err := os.Stat(a.Store.RunPath(id)); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		paths = append(paths, filepath.Join(a.Store.RunsDir(), entry.Name()))
	}
	sort.Strings(paths)

	for _, path := range paths {
		size, err := runArtifactSize(path)
		if err != nil {
			return result, err
		}
		if dryRun {
			result.Artifacts = append(result.Artifacts, PrunedRunArtifact{Path: path, Bytes: size})
			result.FreedBytes += size
			continue
		}
		id, _ := runArtifactID(filepath.Base(path))
		if _, err := os.Stat(a.Store.RunPath(id)); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		if err := os.RemoveAll(path); err != nil {
			return result, fmt.Errorf("failed to remove orphaned run artifact %q: %w", path, err)
		}
		result.Artifacts = append(result.Artifacts, PrunedRunArtifact{Path: path, Bytes: size})
		result.FreedBytes += size
	}
	return result, nil
}

func runArtifactID(name string) (string, bool) {
	for _, suffix := range runArtifactSuffixes {
		if !strings.HasSuffix(name, string(suffix)) {
			continue
		}
		id := strings.TrimSuffix(name, string(suffix))
		if strings.HasPrefix(id, "run-") && safeName.MatchString(id) {
			return id, true
		}
	}
	return "", false
}

func runArtifactSize(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}
