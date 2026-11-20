package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Fragment outcomes reported by RepoSetup.
const (
	RepoFragmentCreated   = "created"
	RepoFragmentUpdated   = "updated"
	RepoFragmentUnchanged = "unchanged"
	RepoFragmentPreview   = "preview"
)

// RepoSetupOptions configures `agentbox repo setup`.
type RepoSetupOptions struct {
	// Root is a directory inside the repository to set up. Empty means the
	// working directory.
	Root string
	// Agents limits the image build to specific agents. Empty defaults to
	// the agents present in local profiles.
	Agents []string
	// Dry validates the manifest and previews the plan without running
	// podman or writing files.
	Dry bool
}

// RepoImageRecord describes one repository image built or confirmed by
// setup.
type RepoImageRecord struct {
	Agent          string `json:"agent"`
	Base           string `json:"base"`
	Tag            string `json:"tag"`
	ManifestDigest string `json:"manifest_digest"`
	// Built is false when the content-addressed tag already existed and
	// the build was skipped.
	Built   bool   `json:"built"`
	ImageID string `json:"image_id,omitempty"`
	Digest  string `json:"digest,omitempty"`
}

// RepoSetupResult reports everything setup did or, in a dry run, would do.
// It lists references only: setup never creates or edits profiles, and runs
// resolve the repository image and fragment on their own.
type RepoSetupResult struct {
	Repo           string            `json:"repo"`
	Root           string            `json:"root"`
	ManifestPath   string            `json:"manifest_path"`
	ManifestDigest string            `json:"manifest_digest"`
	Agents         []string          `json:"agents"`
	FragmentPath   string            `json:"fragment_path"`
	FragmentStatus string            `json:"fragment_status"`
	Images         []RepoImageRecord `json:"images"`
	Dry            bool              `json:"dry,omitempty"`
}

// Summary renders the human-readable reference report: the repository
// identity, what was built (or would be) on which base, and where the
// fragment lives. Setup never touches profiles, so the report says so.
func (r RepoSetupResult) Summary() string {
	var b strings.Builder
	if r.Dry {
		b.WriteString("dry run: manifest validated; nothing was built or written\n")
	}
	fmt.Fprintf(&b, "repo:      %s\n", r.Repo)
	fmt.Fprintf(&b, "manifest:  %s (%s)\n", r.ManifestPath, r.ManifestDigest)
	for _, image := range r.Images {
		state := "built"
		if !image.Built {
			state = "already present"
		}
		fmt.Fprintf(&b, "%s image: %s (%s)\n  base:   %s\n", image.Agent, image.Tag, state, image.Base)
	}
	if len(r.Images) == 0 {
		for _, agent := range r.Agents {
			fmt.Fprintf(&b, "%s image: tag resolves at build time from the newest local %s image (recipe r%s)\n", agent, agent, CurrentImageRecipe())
		}
	}
	fmt.Fprintf(&b, "fragment:  %s (%s)\n", r.FragmentPath, r.FragmentStatus)
	b.WriteString("profiles need no change: repository runs resolve the repository image and fragment automatically\n")
	return b.String()
}

func (a *App) RepoSetup(ctx context.Context, opt RepoSetupOptions) (RepoSetupResult, error) {
	return a.RepoSetupWithOutput(ctx, opt, os.Stdout, os.Stderr)
}

func (a *App) RepoSetupWithOutput(ctx context.Context, opt RepoSetupOptions, stdout, stderr io.Writer) (RepoSetupResult, error) {
	result := RepoSetupResult{Images: []RepoImageRecord{}}
	root := strings.TrimSpace(opt.Root)
	if root == "" {
		root = "."
	}
	repoRoot, err := a.gitOutput(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return result, fmt.Errorf("%s is not a Git repository: %w", root, err)
	}
	result.Root = repoRoot

	// Validate before any side effect: a broken manifest must not build a
	// partial image or touch the fragment.
	cfg, err := LoadRepoConfig(repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			checkout := a.ProjectCheckout(ctx, repoRoot)
			return result, fmt.Errorf("no repository manifest at %s; checkout is %s (derived from local remote-tracking refs; no fetch). If the manifest is already committed upstream, update this checkout first; otherwise author .agentbox/config.yaml first (see the repository manifest section of docs/openshell-architecture.md) and commit it: %w", RepoConfigPath(repoRoot), checkout.Summary(), err)
		}
		return result, err
	}
	result.ManifestPath = RepoConfigPath(repoRoot)
	manifestBytes, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		return result, err
	}
	result.ManifestDigest = repoManifestDigest(manifestBytes)

	remote, err := a.gitOutput(ctx, repoRoot, "remote", "get-url", "origin")
	if err != nil {
		return result, fmt.Errorf("repository %s has no readable origin remote; AgentBox identifies repositories by their origin remote: %w", repoRoot, err)
	}
	result.Repo = normalizeGitRemote(remote)
	if result.Repo == "" {
		return result, fmt.Errorf("origin remote %q could not be normalized to host/owner/repo", remote)
	}

	agents, err := a.repoSetupAgents(opt.Agents)
	if err != nil {
		return result, err
	}
	result.Agents = agents
	result.Dry = opt.Dry

	// The fragment is keyed by the repository identity, not by a registered
	// project name, so every checkout and worktree of the same repository
	// shares one fragment.
	result.FragmentPath = filepath.Join(a.Store.PolicyDir(), "repo-"+repoKey(result.Repo)+".yaml")
	fragment, err := RepoPolicyFragment(cfg)
	if err != nil {
		return result, err
	}
	result.FragmentStatus, err = applyRepoFragment(result.FragmentPath, fragment, opt.Dry)
	if err != nil {
		return result, err
	}
	if opt.Dry {
		// Image tags are content-addressed against the resolved base
		// reference, which needs podman; a dry run stays podman-free.
		return result, nil
	}
	result.Images, err = a.repoSetupImages(ctx, cfg, agents, result.Repo, result.ManifestDigest, stdout, stderr)
	if err != nil {
		return result, err
	}
	return result, nil
}

// repoSetupAgents resolves the requested agent list, or defaults to the
// agents present in local profiles.
func (a *App) repoSetupAgents(requested []string) ([]string, error) {
	if len(requested) > 0 {
		return dedupeAgents(requested)
	}
	profiles, err := a.Store.ListProfiles()
	if err != nil {
		return nil, err
	}
	agents := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Agent != "" {
			agents = append(agents, profile.Agent)
		}
	}
	if len(agents) == 0 {
		return nil, fmt.Errorf("no --agent given and no local profile declares an agent; create a profile first (`agentbox profile create`) or pass --agent claude,codex")
	}
	return dedupeAgents(agents)
}

func dedupeAgents(agents []string) ([]string, error) {
	seen := make(map[string]bool, len(agents))
	out := make([]string, 0, len(agents))
	for _, agent := range agents {
		if _, err := agentPackage(agent); err != nil {
			return nil, err
		}
		if !seen[agent] {
			seen[agent] = true
			out = append(out, agent)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (a *App) gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	b, err := a.Runner.Output(ctx, "git", append([]string{"-C", dir}, args...)...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// repoSetupImages builds one repository image per agent on top of the
// current local base image, skipping builds whose content-addressed tag
// already exists.
func (a *App) repoSetupImages(ctx context.Context, cfg RepoConfig, agents []string, repo, manifestDigest string, stdout, stderr io.Writer) ([]RepoImageRecord, error) {
	images, err := a.LocalImages(ctx)
	if err != nil {
		return nil, err
	}
	bases, err := repoBaseImages(agents, images)
	if err != nil {
		return nil, err
	}
	records := make([]RepoImageRecord, 0, len(agents))
	for _, agent := range agents {
		base := bases[agent]
		hash, err := repoImageHash(base.Reference, cfg)
		if err != nil {
			return records, err
		}
		tag := repoImageTag(repo, agent, hash)
		record := RepoImageRecord{Agent: agent, Base: base.Reference, Tag: tag, ManifestDigest: manifestDigest}
		if !repoImageExists(tag, images) {
			dir, err := os.MkdirTemp("", "agentbox-repo-*")
			if err != nil {
				return records, err
			}
			dockerfile := repoImageDockerfile(base.Reference, cfg)
			writeErr := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0600)
			if writeErr == nil {
				args := []string{"build", "-t", tag,
					"--label", "agentbox.worker=true",
					"--label", "agentbox.repo=" + repo,
					"--label", "agentbox.manifest-digest=" + manifestDigest,
					"--label", "agentbox.base=" + base.Reference,
					"--label", "agentbox.agent=" + agent,
					"--label", "agentbox.version=" + base.Version,
					"--label", "agentbox.recipe=" + CurrentImageRecipe(),
					dir}
				writeErr = a.Runner.Stream(ctx, nil, stdout, stderr, "podman", args...)
			}
			os.RemoveAll(dir)
			if writeErr != nil {
				return records, fmt.Errorf("build %s: %w", tag, writeErr)
			}
			record.Built = true
		}
		b, err := a.Runner.Output(ctx, "podman", "image", "inspect", tag, "--format", "{{.Id}} {{.Digest}}")
		if err != nil {
			return records, err
		}
		fields := strings.Fields(string(b))
		if len(fields) > 0 {
			record.ImageID = fields[0]
		}
		if len(fields) > 1 && strings.HasPrefix(fields[1], "sha256:") {
			record.Digest = fields[1]
		}
		if record.ImageID == "" {
			return records, fmt.Errorf("built image ID unavailable for %s", tag)
		}
		records = append(records, record)
	}
	return records, nil
}

// repoBaseImages picks, per agent, the newest local base image at the
// current recipe revision. Repository images (agentbox.worker) are never
// used as bases.
func repoBaseImages(agents []string, images []ImageArtifact) (map[string]ImageArtifact, error) {
	recipe, err := strconv.Atoi(CurrentImageRecipe())
	if err != nil {
		return nil, fmt.Errorf("invalid recipe revision %q", CurrentImageRecipe())
	}
	bases := make(map[string]ImageArtifact, len(agents))
	for _, agent := range agents {
		var best ImageArtifact
		for _, image := range images {
			if image.Worker || image.Agent != agent || image.Recipe != recipe {
				continue
			}
			if best.Reference == "" || compareImageVersions(image.Version, best.Version) > 0 {
				best = image
			}
		}
		if best.Reference == "" {
			return nil, fmt.Errorf("no local AgentBox image for %s at recipe r%s; run `agentbox image build %s` first", agent, CurrentImageRecipe(), agent)
		}
		bases[agent] = best
	}
	return bases, nil
}

func repoImageExists(tag string, images []ImageArtifact) bool {
	for _, image := range images {
		if image.Reference == tag {
			return true
		}
		for _, reference := range image.References {
			if reference == tag {
				return true
			}
		}
	}
	return false
}

// compareImageVersions orders release versions by dot-separated numeric
// segments, falling back to a lexicographic tiebreak.
func compareImageVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		av, bv := -1, -1
		if i < len(as) {
			av = leadingDigits(as[i])
		}
		if i < len(bs) {
			bv = leadingDigits(bs[i])
		}
		if av != bv {
			return av - bv
		}
	}
	return strings.Compare(a, b)
}

func leadingDigits(segment string) int {
	end := 0
	for end < len(segment) && segment[end] >= '0' && segment[end] <= '9' {
		end++
	}
	if end == 0 {
		return -1
	}
	n, err := strconv.Atoi(segment[:end])
	if err != nil {
		return -1
	}
	return n
}

// repoImageTag derives the content-addressed repository image tag. The hash
// covers the base reference and the image-affecting manifest sections only,
// so a hosts-only manifest change keeps the previous tag.
func repoImageTag(repo, agent, hash string) string {
	return fmt.Sprintf("localhost/openshell:repo-%s-%s-%s", repoKey(repo), agent, hash)
}

func repoImageHash(base string, cfg RepoConfig) (string, error) {
	return repoImageHashWithRecipe(repoImageRecipe, base, cfg)
}

func repoImageHashWithRecipe(recipe, base string, cfg RepoConfig) (string, error) {
	input := struct {
		Recipe string           `yaml:"recipe"`
		Base   string           `yaml:"base"`
		Image  RepoImageSection `yaml:"image"`
		Verify []string         `yaml:"verify"`
	}{recipe, base, cfg.Image, cfg.Verify}
	b, err := yaml.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:4]), nil
}

func repoManifestDigest(b []byte) string {
	return contentDigest(b)
}

// contentDigest is the sha256 reference used for manifest and fragment
// provenance.
func contentDigest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// repoKey reduces a normalized repository remote to a lowercase,
// hyphen-separated identifier that is safe in image tags and policy file
// names. Overlong keys are truncated with a hash suffix so distinct
// repositories never collide.
func repoKey(repo string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(repo) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	key := strings.Trim(b.String(), "-")
	if key == "" {
		return "repo"
	}
	if len(key) > 48 {
		sum := sha256.Sum256([]byte(repo))
		key = key[:41] + "-" + hex.EncodeToString(sum[:3])
	}
	return key
}

// repoImageRecipe invalidates repository image tags when the generated
// layer changes without a manifest change.
const repoImageRecipe = "1"

// repoImageDockerfile renders the repository layer: packages and pinned
// toolchains installed as root, then every verify command run as the
// sandbox user so an unusable toolchain fails the build, not the run.
func repoImageDockerfile(base string, cfg RepoConfig) string {
	var b strings.Builder
	fmt.Fprintf(&b, "FROM %s\n", base)
	b.WriteString("USER root\n")
	if len(cfg.Image.Packages) > 0 {
		fmt.Fprintf(&b, "RUN apt-get update && apt-get install -y --no-install-recommends %s && rm -rf /var/lib/apt/lists/*\n", strings.Join(cfg.Image.Packages, " "))
	}
	for _, tc := range cfg.Image.Toolchains {
		fmt.Fprintf(&b, "ENV PATH=%s:$PATH\n", filepath.Dir(tc.Bin))
	}
	for _, tc := range cfg.Image.Toolchains {
		archive := "/tmp/agentbox-toolchain-" + tc.Name
		fmt.Fprintf(&b, "RUN curl -fsSL %s -o %s && echo '%s  %s' | sha256sum -c -", tc.URL, archive, tc.SHA256, archive)
		if repoToolchainIsArchive(tc.URL) {
			b.WriteString(" && tar -C /usr/local -xf " + archive + " && rm -f " + archive)
		} else {
			b.WriteString(" && install -Dm755 " + archive + " " + tc.Bin)
		}
		b.WriteString("\n")
	}
	// OpenShell replaces the process PATH at runtime, so the Dockerfile ENV
	// does not survive into agent shells. Link every toolchain binary into
	// /usr/local/bin, which is on PATH for login and non-login shells alike,
	// and keep a profile.d entry for toolchains whose directory holds helper
	// binaries. OpenShell resolves the link target for binary-scoped policy,
	// so the manifest's declared binary path still matches.
	var toolchainDirs []string
	for _, tc := range cfg.Image.Toolchains {
		dir := filepath.Dir(tc.Bin)
		if dir == "/usr/local/bin" {
			continue
		}
		fmt.Fprintf(&b, "RUN ln -sf %s /usr/local/bin/%s\n", tc.Bin, filepath.Base(tc.Bin))
		toolchainDirs = append(toolchainDirs, dir)
	}
	if len(toolchainDirs) > 0 {
		fmt.Fprintf(&b, "RUN printf 'export PATH=\"%s:$PATH\"\\n' > /etc/profile.d/agentbox-toolchains.sh\n", strings.Join(toolchainDirs, ":"))
	}
	b.WriteString("USER sandbox\n")
	for _, cmd := range cfg.Verify {
		fmt.Fprintf(&b, "RUN %s\n", cmd)
	}
	b.WriteString("WORKDIR /workspace\n")
	return b.String()
}

// repoToolchainIsArchive reports whether a toolchain URL names a tar archive
// (extracted under /usr/local); anything else installs as a single binary
// at the declared bin path.
func repoToolchainIsArchive(u string) bool {
	lower := strings.ToLower(u)
	for _, suffix := range []string{".tar.gz", ".tgz", ".tar.xz", ".txz", ".tar.bz2", ".tbz2", ".tar"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// RepoPolicyFragment renders the reusable policy fragment for a manifest:
// the DefaultPolicy baseline (filesystem, process, hard Landlock
// requirement) with the manifest's host rules copied verbatim as
// binary-scoped network rules. The output is deterministic, so unchanged
// manifests produce byte-identical fragments.
func RepoPolicyFragment(cfg RepoConfig) ([]byte, error) {
	var policy Policy
	if err := yaml.Unmarshal(DefaultPolicy(), &policy); err != nil {
		return nil, err
	}
	policy.Network = make(map[string]NetworkRule, len(cfg.Hosts))
	for _, rule := range cfg.Hosts {
		name, err := repoHostRuleName(rule.Host, rule.Port)
		if err != nil {
			return nil, err
		}
		binaries := make([]Binary, 0, len(rule.Binaries))
		for _, path := range rule.Binaries {
			binaries = append(binaries, Binary{Path: path})
		}
		policy.Network[name] = NetworkRule{
			Name:      name,
			Endpoints: []Endpoint{{Host: rule.Host, Port: rule.Port, Protocol: "tcp"}},
			Binaries:  binaries,
		}
	}
	out, err := yaml.Marshal(policy)
	if err != nil {
		return nil, err
	}
	if _, err := ParsePolicy(out); err != nil {
		return nil, fmt.Errorf("generated fragment is not a valid policy: %w", err)
	}
	return out, nil
}

func repoHostRuleName(host string, port int) (string, error) {
	name := "host-" + strings.ReplaceAll(host, ".", "-") + "-" + strconv.Itoa(port)
	if len(name) > 63 {
		sum := sha256.Sum256([]byte(name))
		name = name[:56] + "-" + hex.EncodeToString(sum[:3])
	}
	if !safeName.MatchString(name) {
		return "", fmt.Errorf("host %s:%d does not map to a valid network rule name", host, port)
	}
	return name, nil
}

// applyRepoFragment writes the fragment unless the dry flag is set or the
// on-disk content already matches, and reports the outcome.
func applyRepoFragment(path string, content []byte, dry bool) (string, error) {
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if dry {
			return RepoFragmentPreview, nil
		}
		return RepoFragmentCreated, atomicWrite(path, content)
	case err != nil:
		return "", err
	case bytes.Equal(old, content):
		return RepoFragmentUnchanged, nil
	case dry:
		return RepoFragmentPreview, nil
	default:
		return RepoFragmentUpdated, atomicWrite(path, content)
	}
}
