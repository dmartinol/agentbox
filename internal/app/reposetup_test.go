package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const repoSetupManifest = `schema: 1

image:
  packages:
    - make
    - gcc
  toolchains:
    - name: go
      url: https://go.dev/dl/go1.26.11.linux-arm64.tar.gz
      sha256: 6c2f29b458b5add0835f18b776f69c8881a6d88428f1484a6da55755e620757e
      bin: /usr/local/go/bin/go

verify:
  - go version
  - make --version

hosts:
  - host: proxy.golang.org
    port: 443
    binaries: [/usr/local/go/bin/go]
  - host: github.com
    port: 22
    binaries: [/usr/bin/git]
`

// repoSetupEnv is a checkout with a manifest plus scripted git and podman
// answers.
type repoSetupEnv struct {
	root       string
	toplevel   string
	store      Store
	runner     *fakeRunner
	gitFail    map[string]string
	imagesJSON []byte
}

func newRepoSetupEnv(t *testing.T, manifest string) *repoSetupEnv {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agentbox"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agentbox", "config.yaml"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	env := &repoSetupEnv{root: root, toplevel: root, store: Store{Base: t.TempDir()}, gitFail: map[string]string{}}
	env.runner = &fakeRunner{output: env.output}
	return env
}

func (e *repoSetupEnv) output(call recordedCommand) ([]byte, error) {
	if call.name == "git" {
		if len(call.args) >= 3 && call.args[0] == "-C" && call.args[2] == "rev-parse" {
			if msg, failed := e.gitFail["rev-parse"]; failed {
				return nil, errors.New(msg)
			}
			return []byte(e.toplevel + "\n"), nil
		}
		if len(call.args) >= 3 && call.args[0] == "-C" && call.args[2] == "remote" {
			if msg, failed := e.gitFail["remote"]; failed {
				return nil, errors.New(msg)
			}
			return []byte("git@github.com:owner/repo.git\n"), nil
		}
		return nil, fmt.Errorf("unexpected git call: %#v", call)
	}
	if call.name == "podman" {
		if len(call.args) > 0 && call.args[0] == "images" {
			if e.imagesJSON == nil {
				return []byte("[]"), nil
			}
			return e.imagesJSON, nil
		}
		if len(call.args) > 0 && call.args[0] == "image" && len(call.args) > 2 && call.args[1] == "inspect" {
			return []byte("sha256:repo-image-id sha256:repo-digest\n"), nil
		}
	}
	return nil, fmt.Errorf("unexpected command: %#v", call)
}

func podmanImagesJSON(t *testing.T, records ...map[string]any) []byte {
	t.Helper()
	out, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func imageRecord(names []string, labels map[string]string) map[string]any {
	return map[string]any{
		"Id":     "sha256:id-" + strings.Join(names, "-"),
		"Names":  names,
		"Labels": labels,
	}
}

func baseImageLabels(agent, version string) map[string]string {
	return map[string]string{
		"agentbox":         "true",
		"agentbox.agent":   agent,
		"agentbox.version": version,
		"agentbox.recipe":  CurrentImageRecipe(),
	}
}

func TestRepoSetupSummaryReportsReferences(t *testing.T) {
	env := newRepoSetupEnv(t, repoSetupManifest)
	if err := env.store.SaveProfile(profileFixture("codex-work", "codex", "/p.yaml")); err != nil {
		t.Fatal(err)
	}
	app := App{Store: env.store, Runner: env.runner}
	dry, err := app.RepoSetup(context.Background(), RepoSetupOptions{Dry: true})
	if err != nil {
		t.Fatal(err)
	}
	summary := dry.Summary()
	for _, want := range []string{
		"dry run: manifest validated; nothing was built or written",
		"repo:      github.com/owner/repo",
		"codex image: tag resolves at build time from the newest local codex image (recipe r" + CurrentImageRecipe() + ")",
		"fragment:  " + dry.FragmentPath + " (preview)",
		"profiles need no change",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("dry summary missing %q:\n%s", want, summary)
		}
	}
	applied := dry
	applied.Dry = false
	applied.Images = []RepoImageRecord{{Agent: "codex", Base: "localhost/openshell:codex-0.167.0-r4", Tag: "localhost/openshell:repo-github-com-owner-repo-codex-1a2b3c4d", Built: true}}
	summary = applied.Summary()
	for _, want := range []string{
		"codex image: localhost/openshell:repo-github-com-owner-repo-codex-1a2b3c4d (built)",
		"  base:   localhost/openshell:codex-0.167.0-r4",
		"fragment:  " + applied.FragmentPath,
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("applied summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "dry run") {
		t.Fatalf("applied summary mentions dry run:\n%s", summary)
	}
}

func TestRepoSetupDryRunValidatesWithoutSideEffects(t *testing.T) {
	env := newRepoSetupEnv(t, repoSetupManifest)
	if err := env.store.SaveProfile(profileFixture("codex-work", "codex", "/p.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := env.store.SaveProfile(profileFixture("claude-work", "claude", "/p.yaml")); err != nil {
		t.Fatal(err)
	}
	app := App{Store: env.store, Runner: env.runner}
	result, err := app.RepoSetup(context.Background(), RepoSetupOptions{Dry: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Dry {
		t.Fatal("dry run not recorded")
	}
	if result.Repo != "github.com/owner/repo" {
		t.Fatalf("repo = %q", result.Repo)
	}
	if result.Root != env.root {
		t.Fatalf("root = %q; want the Git toplevel %q", result.Root, env.root)
	}
	if !strings.HasPrefix(result.ManifestDigest, "sha256:") {
		t.Fatalf("manifest digest = %q", result.ManifestDigest)
	}
	if fmt.Sprint(result.Agents) != "[claude codex]" {
		t.Fatalf("agents = %v; want profile agents sorted", result.Agents)
	}
	if result.FragmentStatus != RepoFragmentPreview {
		t.Fatalf("fragment status = %q; want preview", result.FragmentStatus)
	}
	if result.FragmentPath != filepath.Join(env.store.PolicyDir(), "repo-github-com-owner-repo.yaml") {
		t.Fatalf("fragment path = %q", result.FragmentPath)
	}
	if len(result.Images) != 0 {
		t.Fatalf("dry run produced images: %#v", result.Images)
	}
	if _, err := os.Stat(result.FragmentPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry run wrote the fragment: %v", err)
	}
	for _, call := range append(env.runner.outputs, env.runner.streams...) {
		if call.name == "podman" {
			t.Fatalf("dry run invoked podman: %#v", call)
		}
	}
}

func TestRepoSetupBuildsContentAddressedImagePerAgent(t *testing.T) {
	env := newRepoSetupEnv(t, repoSetupManifest)
	env.imagesJSON = podmanImagesJSON(t,
		imageRecord([]string{"localhost/openshell:codex-0.166.0-r4"}, baseImageLabels("codex", "0.166.0")),
		imageRecord([]string{"localhost/openshell:codex-0.167.0-r4"}, baseImageLabels("codex", "0.167.0")),
		imageRecord([]string{"localhost/openshell:repo-github-com-owner-repo-codex-ffffffff"}, func() map[string]string {
			labels := baseImageLabels("codex", "9.9.9")
			labels["agentbox.worker"] = "true"
			labels["agentbox.repo"] = "github.com/owner/repo"
			return labels
		}()),
	)
	var dockerfiles []string
	env.runner.stream = func(call recordedCommand) error {
		if call.name != "podman" || len(call.args) == 0 || call.args[0] != "build" {
			return nil
		}
		dockerfile, err := os.ReadFile(filepath.Join(call.args[len(call.args)-1], "Dockerfile"))
		if err != nil {
			return err
		}
		dockerfiles = append(dockerfiles, string(dockerfile))
		return nil
	}
	app := App{Store: env.store, Runner: env.runner}
	result, err := app.RepoSetup(context.Background(), RepoSetupOptions{Agents: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Images) != 1 {
		t.Fatalf("images = %#v", result.Images)
	}
	image := result.Images[0]
	if image.Agent != "codex" || image.Base != "localhost/openshell:codex-0.167.0-r4" {
		t.Fatalf("unexpected image record: %+v", image)
	}
	if image.Tag != "localhost/openshell:repo-github-com-owner-repo-codex-"+repoSetupHash(t, image.Base) {
		t.Fatalf("tag = %q", image.Tag)
	}
	if !image.Built || image.ImageID != "sha256:repo-image-id" || image.Digest != "sha256:repo-digest" {
		t.Fatalf("unexpected image record: %+v", image)
	}
	if len(dockerfiles) != 1 {
		t.Fatalf("expected one build, got %d", len(dockerfiles))
	}
	dockerfile := dockerfiles[0]
	for _, want := range []string{
		"FROM localhost/openshell:codex-0.167.0-r4\n",
		"USER root\n",
		"RUN apt-get update && apt-get install -y --no-install-recommends make gcc && rm -rf /var/lib/apt/lists/*\n",
		"ENV PATH=/usr/local/go/bin:$PATH\n",
		"RUN curl -fsSL https://go.dev/dl/go1.26.11.linux-arm64.tar.gz -o /tmp/agentbox-toolchain-go && echo '6c2f29b458b5add0835f18b776f69c8881a6d88428f1484a6da55755e620757e  /tmp/agentbox-toolchain-go' | sha256sum -c - && tar -C /usr/local -xf /tmp/agentbox-toolchain-go && rm -f /tmp/agentbox-toolchain-go\n",
		"USER sandbox\n",
		"RUN go version\n",
		"RUN make --version\n",
		"WORKDIR /workspace\n",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Fatalf("Dockerfile missing %q:\n%s", want, dockerfile)
		}
	}
	build := env.runner.streams[0]
	for _, want := range []string{
		"--label", "agentbox.worker=true",
		"--label", "agentbox.repo=github.com/owner/repo",
		"--label", "agentbox.manifest-digest=" + result.ManifestDigest,
	} {
		if !commandHasArg(build, want) {
			t.Fatalf("build args missing %q: %#v", want, build.args)
		}
	}
	// The fragment must exist, parse, and carry the manifest hosts verbatim.
	fragment, err := os.ReadFile(result.FragmentPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.FragmentStatus != RepoFragmentCreated {
		t.Fatalf("fragment status = %q", result.FragmentStatus)
	}
	parsed, err := ParsePolicy(fragment)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Landlock.Compatibility != "hard_requirement" || !parsed.Filesystem.IncludeWorkdir || parsed.Process.User != "sandbox" {
		t.Fatalf("fragment lost the default baseline: %s", fragment)
	}
	if rule, ok := parsed.Network["host-proxy-golang-org-443"]; !ok || len(rule.Endpoints) != 1 || rule.Endpoints[0].Host != "proxy.golang.org" || rule.Endpoints[0].Port != 443 {
		t.Fatalf("proxy.golang.org rule missing or wrong: %#v", rule)
	} else if len(rule.Binaries) != 1 || rule.Binaries[0].Path != "/usr/local/go/bin/go" {
		t.Fatalf("proxy.golang.org binaries not verbatim: %#v", rule.Binaries)
	}
	if _, ok := parsed.Network["host-github-com-22"]; !ok {
		t.Fatalf("github.com:22 rule missing: %s", fragment)
	}
}

func repoSetupHash(t *testing.T, base string) string {
	t.Helper()
	cfg, err := ParseRepoConfig([]byte(repoSetupManifest))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := repoImageHash(base, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestRepoSetupRerunSkipsExistingImageAndKeepsFragmentStable(t *testing.T) {
	env := newRepoSetupEnv(t, repoSetupManifest)
	app := App{Store: env.store, Runner: env.runner}
	env.imagesJSON = podmanImagesJSON(t,
		imageRecord([]string{"localhost/openshell:codex-0.167.0-r4"}, baseImageLabels("codex", "0.167.0")),
	)
	first, err := app.RepoSetup(context.Background(), RepoSetupOptions{Agents: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(env.runner.streams) != 1 {
		t.Fatalf("first run should build once, got %d streams", len(env.runner.streams))
	}
	fragment, err := os.ReadFile(first.FragmentPath)
	if err != nil {
		t.Fatal(err)
	}
	// Second run: the content-addressed tag now exists locally.
	env.imagesJSON = podmanImagesJSON(t,
		imageRecord([]string{"localhost/openshell:codex-0.167.0-r4"}, baseImageLabels("codex", "0.167.0")),
		imageRecord([]string{first.Images[0].Tag}, func() map[string]string {
			labels := baseImageLabels("codex", "0.167.0")
			labels["agentbox.worker"] = "true"
			labels["agentbox.repo"] = "github.com/owner/repo"
			return labels
		}()),
	)
	second, err := app.RepoSetup(context.Background(), RepoSetupOptions{Agents: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(env.runner.streams) != 1 {
		t.Fatalf("second run rebuilt an existing tag: %#v", env.runner.streams)
	}
	if second.Images[0].Built || second.Images[0].Tag != first.Images[0].Tag {
		t.Fatalf("unexpected second-run record: %+v", second.Images[0])
	}
	if second.FragmentStatus != RepoFragmentUnchanged {
		t.Fatalf("fragment status = %q; want unchanged", second.FragmentStatus)
	}
	again, err := os.ReadFile(second.FragmentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(fragment) != string(again) {
		t.Fatal("fragment content changed between identical runs")
	}
}

func TestRepoSetupHashCoversImageSectionsOnly(t *testing.T) {
	base := "localhost/openshell:codex-0.167.0-r4"
	cfg, err := ParseRepoConfig([]byte(repoSetupManifest))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := repoImageHash(base, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// A hosts-only change keeps the tag.
	otherHosts := cfg
	otherHosts.Hosts = append(otherHosts.Hosts, RepoHostRule{Host: "sum.golang.org", Port: 443, Binaries: []string{"/usr/local/go/bin/go"}})
	hostsChanged, err := repoImageHash(base, otherHosts)
	if err != nil {
		t.Fatal(err)
	}
	if hash != hostsChanged {
		t.Fatal("hosts-only change produced a new image tag")
	}
	// Image and verify changes produce new tags.
	packageChanged := cfg
	packageChanged.Image.Packages = append([]string(nil), cfg.Image.Packages...)
	packageChanged.Image.Packages = append(packageChanged.Image.Packages, "ripgrep")
	if changed, err := repoImageHash(base, packageChanged); err != nil || changed == hash {
		t.Fatal("package change kept the image tag")
	}
	verifyChanged := cfg
	verifyChanged.Verify = append([]string(nil), cfg.Verify...)
	verifyChanged.Verify = append(verifyChanged.Verify, "gcc --version")
	if changed, err := repoImageHash(base, verifyChanged); err != nil || changed == hash {
		t.Fatal("verify change kept the image tag")
	}
	if changed, err := repoImageHash("localhost/openshell:codex-0.168.0-r4", cfg); err != nil || changed == hash {
		t.Fatal("base change kept the image tag")
	}
}

func TestRepoPolicyFragment(t *testing.T) {
	cfg, err := ParseRepoConfig([]byte(repoSetupManifest))
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := RepoPolicyFragment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	again, err := RepoPolicyFragment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(fragment) != string(again) {
		t.Fatal("fragment generation is not deterministic")
	}
	// Same host on two ports stays two rules.
	cfg.Hosts = append(cfg.Hosts, RepoHostRule{Host: "github.com", Port: 443, Binaries: []string{"/usr/local/go/bin/go"}})
	fragment, err = RepoPolicyFragment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePolicy(fragment)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed.Network["host-github-com-22"]; !ok {
		t.Fatalf("port 22 rule missing: %s", fragment)
	}
	if _, ok := parsed.Network["host-github-com-443"]; !ok {
		t.Fatalf("port 443 rule missing: %s", fragment)
	}
	// An empty hosts list yields the offline baseline.
	empty, err := RepoPolicyFragment(RepoConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParsePolicy(empty); err != nil || len(parsed.Network) != 0 {
		t.Fatalf("empty manifest fragment invalid or not empty: %s (%v)", empty, err)
	}
}

func TestRepoSetupAgentSelection(t *testing.T) {
	env := newRepoSetupEnv(t, repoSetupManifest)
	if err := env.store.SaveProfile(profileFixture("claude-work", "claude", "/p.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := env.store.SaveProfile(profileFixture("codex-work", "codex", "/p.yaml")); err != nil {
		t.Fatal(err)
	}
	app := App{Store: env.store, Runner: env.runner}
	result, err := app.RepoSetup(context.Background(), RepoSetupOptions{Dry: true})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(result.Agents) != "[claude codex]" {
		t.Fatalf("default agents = %v", result.Agents)
	}
	if _, err := app.RepoSetup(context.Background(), RepoSetupOptions{Agents: []string{"codex", "codex", "claude"}, Dry: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.RepoSetup(context.Background(), RepoSetupOptions{Agents: []string{"bogus"}, Dry: true}); err == nil || !strings.Contains(err.Error(), "agent must be") {
		t.Fatalf("invalid agent accepted: %v", err)
	}
	empty := App{Store: Store{Base: t.TempDir()}, Runner: env.runner}
	_, err = empty.RepoSetup(context.Background(), RepoSetupOptions{Dry: true})
	if err == nil || !strings.Contains(err.Error(), "no --agent given") {
		t.Fatalf("missing agent guidance: %v", err)
	}
}

func TestRepoSetupActionableErrors(t *testing.T) {
	env := newRepoSetupEnv(t, repoSetupManifest)
	app := App{Store: env.store, Runner: env.runner}
	// Missing manifest.
	env.toplevel = t.TempDir()
	_, err := app.RepoSetupWithOutput(context.Background(), RepoSetupOptions{Dry: true}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no repository manifest") || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing manifest error = %v", err)
	}
	env.toplevel = env.root
	// No origin remote.
	env.gitFail["remote"] = "exit status 128"
	_, err = app.RepoSetup(context.Background(), RepoSetupOptions{Dry: true})
	if err == nil || !strings.Contains(err.Error(), "origin remote") {
		t.Fatalf("missing origin error = %v", err)
	}
	delete(env.gitFail, "remote")
	// Invalid manifest fails before any side effect.
	if err := os.WriteFile(filepath.Join(env.root, ".agentbox", "config.yaml"), []byte("schema: 1\nimage:\n  packages: ['make=1.0']\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = app.RepoSetup(context.Background(), RepoSetupOptions{Dry: true})
	if err == nil || !strings.Contains(err.Error(), "image.packages[0]") {
		t.Fatalf("invalid manifest error = %v", err)
	}
	// Missing base image.
	if err := os.WriteFile(filepath.Join(env.root, ".agentbox", "config.yaml"), []byte(repoSetupManifest), 0600); err != nil {
		t.Fatal(err)
	}
	env.imagesJSON = podmanImagesJSON(t, imageRecord([]string{"localhost/openshell:claude-2.1.0-r4"}, baseImageLabels("claude", "2.1.0")))
	_, err = app.RepoSetup(context.Background(), RepoSetupOptions{Agents: []string{"codex"}})
	if err == nil || !strings.Contains(err.Error(), "agentbox image build codex") {
		t.Fatalf("missing base error = %v", err)
	}
}

func TestRepoSetupNotAGitRepository(t *testing.T) {
	env := newRepoSetupEnv(t, repoSetupManifest)
	env.gitFail["rev-parse"] = "fatal: not a git repository"
	app := App{Store: env.store, Runner: env.runner}
	_, err := app.RepoSetup(context.Background(), RepoSetupOptions{Dry: true})
	if err == nil || !strings.Contains(err.Error(), "not a Git repository") {
		t.Fatalf("error = %v", err)
	}
}

func TestRepoImageDockerfileInstallsBinaryToolchains(t *testing.T) {
	cfg, err := ParseRepoConfig([]byte("schema: 1\nimage:\n  toolchains:\n    - name: custom\n      url: https://example.com/tool/linux-amd64\n      sha256: 1111111111111111111111111111111111111111111111111111111111111111\n      bin: /usr/local/bin/tool\nverify:\n  - tool --version\n"))
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := repoImageDockerfile("localhost/openshell:claude-2.1.0-r4", cfg)
	for _, want := range []string{
		"FROM localhost/openshell:claude-2.1.0-r4\n",
		"USER root\n",
		"ENV PATH=/usr/local/bin:$PATH\n",
		"RUN curl -fsSL https://example.com/tool/linux-amd64 -o /tmp/agentbox-toolchain-custom && echo '1111111111111111111111111111111111111111111111111111111111111111  /tmp/agentbox-toolchain-custom' | sha256sum -c - && install -Dm755 /tmp/agentbox-toolchain-custom /usr/local/bin/tool\n",
		"USER sandbox\nRUN tool --version\n",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Fatalf("Dockerfile missing %q:\n%s", want, dockerfile)
		}
	}
	if strings.Contains(dockerfile, "apt-get install") {
		t.Fatalf("empty packages still ran apt:\n%s", dockerfile)
	}
}

func TestRepoKeySanitizesAndTruncates(t *testing.T) {
	if got := repoKey("github.com/dmartinol/agentbox"); got != "github-com-dmartinol-agentbox" {
		t.Fatalf("repoKey = %q", got)
	}
	long := strings.Repeat("a", 100) + ".example.com/owner/repo"
	if got := repoKey(long); len(got) != 48 || !safeName.MatchString("repo-"+got) {
		t.Fatalf("overlong repoKey = %q (len %d)", got, len(got))
	}
	if got := repoKey("GITHUB.COM/Owner/Repo"); got != "github-com-owner-repo" {
		t.Fatalf("repoKey = %q", got)
	}
}

func TestParseRepoConfigRejectsWhitespaceInToolchainURL(t *testing.T) {
	_, err := ParseRepoConfig([]byte("schema: 1\nimage:\n  toolchains:\n    - name: custom\n      url: https://example.com/a b\n      sha256: 1111111111111111111111111111111111111111111111111111111111111111\n      bin: /usr/local/bin/tool\n"))
	if err == nil || !strings.Contains(err.Error(), "image.toolchains[0].url") || !strings.Contains(err.Error(), "whitespace") {
		t.Fatalf("error = %v", err)
	}
}
