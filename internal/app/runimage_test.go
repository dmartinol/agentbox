package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const runRepoManifest = `schema: 1

image:
  packages:
    - make
  toolchains:
    - name: go
      url: https://go.dev/dl/go1.27.4.linux-amd64.tar.gz
      sha256: 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840
      bin: /usr/local/go/bin/go

verify:
  - go version

hosts:
  - host: proxy.golang.org
    port: 443
    binaries: [/usr/local/go/bin/go]
`

type runImageFixture struct {
	root     string
	store    Store
	manifest string
	recipe   int
	baseRef  string
}

func newRunImageFixture(t *testing.T, manifest string) *runImageFixture {
	t.Helper()
	recipe, err := strconv.Atoi(CurrentImageRecipe())
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Base: t.TempDir()}
	root := filepath.Join(store.Base, "checkout")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if manifest != "" {
		if err := writeRepoManifest(t, root, manifest); err != nil {
			t.Fatal(err)
		}
	}
	return &runImageFixture{
		root:     root,
		store:    store,
		manifest: manifest,
		recipe:   recipe,
		baseRef:  fmt.Sprintf("localhost/openshell:codex-0.169.0-r%d", recipe),
	}
}

func writeRepoManifest(t *testing.T, root, manifest string) error {
	t.Helper()
	dir := filepath.Join(root, ".agentbox")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(manifest), 0o644)
}

func (f *runImageFixture) digest() string {
	return repoManifestDigest([]byte(f.manifest))
}

func (f *runImageFixture) baseRecord(version string) map[string]any {
	return map[string]any{
		"Id":    "base-" + version,
		"Names": []string{fmt.Sprintf("localhost/openshell:codex-%s-r%d", version, f.recipe)},
		"Labels": map[string]string{
			"agentbox":         "true",
			"agentbox.recipe":  strconv.Itoa(f.recipe),
			"agentbox.agent":   "codex",
			"agentbox.version": version,
		},
	}
}

func (f *runImageFixture) repoTag(t *testing.T, baseRef string) string {
	t.Helper()
	cfg, err := ParseRepoConfig([]byte(f.manifest))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := repoImageHash(baseRef, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return repoImageTag("github.com/owner/repo", "codex", hash)
}

func (f *runImageFixture) repoRecord(t *testing.T, baseRef, version, manifestDigest string) map[string]any {
	t.Helper()
	return map[string]any{
		"Id":    "repo-" + version,
		"Names": []string{f.repoTag(t, baseRef)},
		"Labels": map[string]string{
			"agentbox":                 "true",
			"agentbox.recipe":          strconv.Itoa(f.recipe),
			"agentbox.agent":           "codex",
			"agentbox.version":         version,
			"agentbox.worker":          "true",
			"agentbox.repo":            "github.com/owner/repo",
			"agentbox.manifest-digest": manifestDigest,
		},
	}
}

func (f *runImageFixture) fragmentPath() string {
	return filepath.Join(f.store.PolicyDir(), "repo-github-com-owner-repo.yaml")
}

func (f *runImageFixture) writeFragment(t *testing.T) {
	t.Helper()
	cfg, err := ParseRepoConfig([]byte(f.manifest))
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := RepoPolicyFragment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(f.store.PolicyDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.fragmentPath(), fragment, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *runImageFixture) profile() Profile {
	return Profile{
		Version:  1,
		Name:     "mycodex",
		Agent:    "codex",
		Image:    f.baseRef,
		Mode:     "standard",
		Provider: "provider",
		Policy:   filepath.Join(f.store.PolicyDir(), "profile.yaml"),
		Approval: "prompted",
	}
}

// app scripts the commands a run start needs: repository identity, the local
// image list, project root discovery, gateway info, and image inspection.
func (f *runImageFixture) app(t *testing.T, records ...map[string]any) (*App, *fakeRunner) {
	t.Helper()
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		switch {
		case call.name == "git" && commandHasArg(call, "rev-parse"):
			return []byte(f.root), nil
		case call.name == "git" && commandHasArg(call, "remote"):
			return []byte("git@github.com:owner/repo.git"), nil
		case call.name == "podman" && commandHasArg(call, "images"):
			return data, nil
		case call.name == "podman" && commandHasArg(call, "inspect"):
			return []byte("[]"), nil
		case call.name == "openshell" && commandHasArg(call, "gateway"):
			return []byte(`{"driver":"podman"}`), nil
		}
		return nil, nil
	}}
	return &App{Store: f.store, Runner: runner}, runner
}

func (f *runImageFixture) saveProfile(t *testing.T, app *App) Profile {
	t.Helper()
	if err := os.MkdirAll(f.store.PolicyDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	profile := f.profile()
	if err := os.WriteFile(profile.Policy, DefaultPolicy(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	return profile
}

func TestResolveRunImageUsesRepositoryImage(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app, _ := f.app(t, f.baseRecord("0.169.0"), f.repoRecord(t, f.baseRef, "0.169.0", f.digest()))

	image, err := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if err != nil {
		t.Fatal(err)
	}
	if image.Source != ImageSourceRepo {
		t.Fatalf("source = %q, want repo", image.Source)
	}
	if image.Image != f.repoTag(t, f.baseRef) {
		t.Fatalf("image = %q, want repository tag", image.Image)
	}
	if image.BaseImage != f.baseRef {
		t.Fatalf("base = %q, want %q", image.BaseImage, f.baseRef)
	}
	if image.Repo != "github.com/owner/repo" {
		t.Fatalf("repo = %q", image.Repo)
	}
	if image.ManifestDigest != f.digest() {
		t.Fatalf("digest = %q", image.ManifestDigest)
	}
	if image.FragmentPath != f.fragmentPath() {
		t.Fatalf("fragment = %q", image.FragmentPath)
	}
	if len(image.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", image.Warnings)
	}
}

func TestResolveRunImageKeepsProfileWithoutManifest(t *testing.T) {
	f := newRunImageFixture(t, "")
	app, runner := f.app(t, f.baseRecord("0.169.0"))

	image, err := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if err != nil {
		t.Fatal(err)
	}
	if image.Source != ImageSourceProfile || image.Image != f.baseRef || image.BaseImage != f.baseRef {
		t.Fatalf("image = %+v, want the profile image", image)
	}
	if len(image.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", image.Warnings)
	}
	for _, call := range runner.outputs {
		if call.name == "podman" {
			t.Fatalf("no manifest must not list images: %v", call)
		}
	}
}

func TestResolveRunImageFallsBackWhenImageMissing(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app, _ := f.app(t, f.baseRecord("0.169.0"))

	image, err := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if err != nil {
		t.Fatal(err)
	}
	if image.Source != ImageSourceProfile || image.Image != f.baseRef {
		t.Fatalf("image = %+v, want the profile image", image)
	}
	if len(image.Warnings) != 1 || !strings.Contains(image.Warnings[0], "agentbox repo setup --agent codex") {
		t.Fatalf("warnings = %v, want a setup hint", image.Warnings)
	}
}

func TestResolveRunImageFallsBackWhenFragmentMissing(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	app, _ := f.app(t, f.baseRecord("0.169.0"), f.repoRecord(t, f.baseRef, "0.169.0", f.digest()))

	image, err := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if err != nil {
		t.Fatal(err)
	}
	if image.Source != ImageSourceProfile {
		t.Fatalf("source = %q, want profile", image.Source)
	}
	if len(image.Warnings) != 1 || !strings.Contains(image.Warnings[0], "policy fragment") {
		t.Fatalf("warnings = %v, want a fragment hint", image.Warnings)
	}
}

func TestResolveRunImageWarnsOnManifestDrift(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app, _ := f.app(t, f.baseRecord("0.169.0"), f.repoRecord(t, f.baseRef, "0.169.0", f.digest()))

	changed := strings.Replace(runRepoManifest, "proxy.golang.org", "proxy.golang.dev", 1)
	if err := writeRepoManifest(t, f.root, changed); err != nil {
		t.Fatal(err)
	}
	image, err := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if err != nil {
		t.Fatal(err)
	}
	if image.Source != ImageSourceRepo || image.Image != f.repoTag(t, f.baseRef) {
		t.Fatalf("image = %+v, want the repository image", image)
	}
	if len(image.Warnings) != 1 || !strings.Contains(image.Warnings[0], "manifest changed") {
		t.Fatalf("warnings = %v, want manifest drift", image.Warnings)
	}
}

func TestResolveRunImageWarnsOnStaleBase(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app, _ := f.app(t,
		f.baseRecord("0.169.0"),
		f.baseRecord("0.170.0"),
		f.repoRecord(t, f.baseRef, "0.169.0", f.digest()),
	)

	image, err := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if err != nil {
		t.Fatal(err)
	}
	if image.Source != ImageSourceRepo || image.Image != f.repoTag(t, f.baseRef) {
		t.Fatalf("image = %+v, want the older-base repository image", image)
	}
	if image.BaseImage != f.baseRef {
		t.Fatalf("base = %q, want %q", image.BaseImage, f.baseRef)
	}
	if len(image.Warnings) != 1 || !strings.Contains(image.Warnings[0], "0.170.0") {
		t.Fatalf("warnings = %v, want stale base drift", image.Warnings)
	}
}

func testPolicyBytes(t *testing.T, rules map[string]NetworkRule) []byte {
	t.Helper()
	policy, err := ParsePolicy(DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	policy.Network = rules
	out, err := yaml.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestComposeRepoPolicyUnionsRules(t *testing.T) {
	base := testPolicyBytes(t, map[string]NetworkRule{
		"agent": {
			Name:      "agent",
			Endpoints: []Endpoint{{Host: "api.openai.com", Port: 443}},
			Binaries:  []Binary{{Path: "/usr/local/bin/codex"}},
		},
	})
	fragment := testPolicyBytes(t, map[string]NetworkRule{
		"go-module": {
			Name:      "go-module",
			Endpoints: []Endpoint{{Host: "proxy.golang.org", Port: 443}},
			Binaries:  []Binary{{Path: "/usr/local/go/bin/go"}},
		},
	})

	composed, err := composeRepoPolicy(base, fragment)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := ParsePolicy(composed)
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.Network) != 2 {
		t.Fatalf("rules = %v, want both rules", policy.Network)
	}
	if policy.Process.User != "sandbox" || policy.Landlock.Compatibility != "hard_requirement" {
		t.Fatalf("base policy sections changed: %+v", policy)
	}
	if got := policy.Network["go-module"]; len(got.Binaries) != 1 || got.Binaries[0].Path != "/usr/local/go/bin/go" {
		t.Fatalf("binary scoping lost: %+v", got)
	}
}

func TestComposeRepoPolicyDedupesIdenticalRules(t *testing.T) {
	rule := NetworkRule{
		Name:      "registry",
		Endpoints: []Endpoint{{Host: "registry.npmjs.org", Port: 443, Protocol: "tcp"}},
		Binaries:  []Binary{{Path: "/usr/local/bin/npm"}},
	}
	base := testPolicyBytes(t, map[string]NetworkRule{"registry": rule})
	// The fragment writes the same scope without a protocol, so it must
	// still compare equal rather than duplicate or conflict.
	fragmentRule := rule
	fragmentRule.Endpoints = []Endpoint{{Host: "registry.npmjs.org", Port: 443}}
	fragment := testPolicyBytes(t, map[string]NetworkRule{"registry": fragmentRule})

	composed, err := composeRepoPolicy(base, fragment)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := ParsePolicy(composed)
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.Network) != 1 {
		t.Fatalf("rules = %v, want the identical rule collapsed", policy.Network)
	}
}

func TestComposeRepoPolicyRejectsConflictingScope(t *testing.T) {
	base := testPolicyBytes(t, map[string]NetworkRule{
		"go-module": {
			Name:      "go-module",
			Endpoints: []Endpoint{{Host: "proxy.golang.org", Port: 443}},
			Binaries:  []Binary{{Path: "/usr/local/go/bin/go"}},
		},
	})
	fragment := testPolicyBytes(t, map[string]NetworkRule{
		"go-module": {
			Name:      "go-module",
			Endpoints: []Endpoint{{Host: "sum.golang.org", Port: 443}},
			Binaries:  []Binary{{Path: "/usr/local/go/bin/go"}},
		},
	})

	_, err := composeRepoPolicy(base, fragment)
	if err == nil || !strings.Contains(err.Error(), `network rule "go-module"`) {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestStartRecordsRepositoryImage(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app, _ := f.app(t, f.baseRecord("0.169.0"), f.repoRecord(t, f.baseRef, "0.169.0", f.digest()))
	profile := f.saveProfile(t, app)

	run, err := app.Start(context.Background(), StartOptions{
		Profile: profile.Name,
		Source:  f.root,
		Gateway: "openshell",
		Dry:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.Image != f.repoTag(t, f.baseRef) || run.ImageSource != string(ImageSourceRepo) {
		t.Fatalf("image = %q (%s), want the repository image", run.Image, run.ImageSource)
	}
	if run.BaseImage != f.baseRef || run.ImageRepo != "github.com/owner/repo" {
		t.Fatalf("provenance = base %q repo %q", run.BaseImage, run.ImageRepo)
	}
	if run.ManifestDigest != f.digest() || run.PolicyFragment != f.fragmentPath() {
		t.Fatalf("policy provenance = digest %q fragment %q", run.ManifestDigest, run.PolicyFragment)
	}
	if len(run.ImageWarnings) != 0 {
		t.Fatalf("warnings = %v", run.ImageWarnings)
	}

	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	loaded, err := app.Store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Image != run.Image || loaded.BaseImage != run.BaseImage || loaded.ImageSource != run.ImageSource || loaded.ImageRepo != run.ImageRepo || loaded.ManifestDigest != run.ManifestDigest || loaded.PolicyFragment != run.PolicyFragment {
		t.Fatalf("run record lost image provenance: %+v", loaded)
	}
}

func TestStartImageOverrideSkipsRepositoryResolution(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app, runner := f.app(t, f.baseRecord("0.169.0"), f.repoRecord(t, f.baseRef, "0.169.0", f.digest()))
	profile := f.saveProfile(t, app)

	run, err := app.Start(context.Background(), StartOptions{
		Profile: profile.Name,
		Source:  f.root,
		Gateway: "openshell",
		Image:   "localhost/custom:1",
		Dry:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.Image != "localhost/custom:1" || run.ImageSource != string(ImageSourceOverride) {
		t.Fatalf("image = %q (%s), want the override", run.Image, run.ImageSource)
	}
	if run.BaseImage != "" || run.PolicyFragment != "" || len(run.ImageWarnings) != 0 {
		t.Fatalf("override kept repository provenance: %+v", run)
	}
	for _, call := range runner.outputs {
		if call.name == "podman" && commandHasArg(call, "images") {
			t.Fatal("override must not resolve repository images")
		}
	}
}

func TestStartProfileImageForcesProfileBase(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app, _ := f.app(t, f.baseRecord("0.169.0"), f.repoRecord(t, f.baseRef, "0.169.0", f.digest()))
	profile := f.saveProfile(t, app)

	run, err := app.Start(context.Background(), StartOptions{
		Profile: profile.Name,
		Source:  f.root,
		Gateway: "openshell",
		Image:   "profile",
		Dry:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.Image != f.baseRef || run.ImageSource != string(ImageSourceProfile) {
		t.Fatalf("image = %q (%s), want the profile image", run.Image, run.ImageSource)
	}
	if len(run.ImageWarnings) != 0 || run.PolicyFragment != "" {
		t.Fatalf("forced profile image reported repository provenance: %+v", run)
	}
}
