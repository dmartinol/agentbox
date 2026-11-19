package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

func (f *runImageFixture) fragmentDigest(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(f.fragmentPath())
	if err != nil {
		t.Fatal(err)
	}
	return contentDigest(b)
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
		Provider: "openai",
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

	image := app.ResolveRunImage(context.Background(), f.profile(), f.root)
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

	image := app.ResolveRunImage(context.Background(), f.profile(), f.root)
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

	image := app.ResolveRunImage(context.Background(), f.profile(), f.root)
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

	image := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if image.Source != ImageSourceProfile {
		t.Fatalf("source = %q, want profile", image.Source)
	}
	if len(image.Warnings) != 1 || !strings.Contains(image.Warnings[0], "policy fragment") {
		t.Fatalf("warnings = %v, want a fragment hint", image.Warnings)
	}
}

func TestResolveRunImageWarnsWhenFragmentIsStale(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app, _ := f.app(t, f.baseRecord("0.169.0"), f.repoRecord(t, f.baseRef, "0.169.0", f.digest()))

	changed := strings.Replace(runRepoManifest, "proxy.golang.org", "proxy.golang.dev", 1)
	if err := writeRepoManifest(t, f.root, changed); err != nil {
		t.Fatal(err)
	}
	image := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if image.Source != ImageSourceRepo || image.Image != f.repoTag(t, f.baseRef) {
		t.Fatalf("image = %+v, want the repository image", image)
	}
	if len(image.Warnings) != 1 || !strings.Contains(image.Warnings[0], "out of date") {
		t.Fatalf("warnings = %v, want a stale fragment warning", image.Warnings)
	}
}

func TestResolveRunImageClearsWarningAfterFragmentRefresh(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app, _ := f.app(t, f.baseRecord("0.169.0"), f.repoRecord(t, f.baseRef, "0.169.0", f.digest()))

	// A hosts-only edit keeps the content-addressed image tag; rerunning repo
	// setup refreshes the fragment without rebuilding the image.
	changed := strings.Replace(runRepoManifest, "proxy.golang.org", "proxy.golang.dev", 1)
	if err := writeRepoManifest(t, f.root, changed); err != nil {
		t.Fatal(err)
	}
	f.manifest = changed
	f.writeFragment(t)

	image := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if image.Source != ImageSourceRepo {
		t.Fatalf("image = %+v, want the repository image", image)
	}
	if len(image.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none after repo setup refreshed the fragment", image.Warnings)
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

	image := app.ResolveRunImage(context.Background(), f.profile(), f.root)
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

func TestCanonicalRuleIgnoresNamesAndNilLists(t *testing.T) {
	withNil := NetworkRule{
		Name:      "first",
		Endpoints: []Endpoint{{Host: "proxy.golang.org", Port: 443}},
	}
	withEmpty := NetworkRule{
		Name:      "second",
		Endpoints: []Endpoint{{Host: "proxy.golang.org", Port: 443, Protocol: "tcp"}},
		Binaries:  []Binary{},
	}
	if !reflect.DeepEqual(canonicalRule(withNil), canonicalRule(withEmpty)) {
		t.Fatalf("equivalent scopes differ: %+v vs %+v", canonicalRule(withNil), canonicalRule(withEmpty))
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
	if run.ManifestDigest != f.digest() || run.PolicyFragment != f.fragmentPath() || run.PolicyFragmentDigest != f.fragmentDigest(t) {
		t.Fatalf("policy provenance = digest %q fragment %q (%q)", run.ManifestDigest, run.PolicyFragment, run.PolicyFragmentDigest)
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
	if loaded.Image != run.Image || loaded.BaseImage != run.BaseImage || loaded.ImageSource != run.ImageSource || loaded.ImageRepo != run.ImageRepo || loaded.ManifestDigest != run.ManifestDigest || loaded.PolicyFragment != run.PolicyFragment || loaded.PolicyFragmentDigest != run.PolicyFragmentDigest {
		t.Fatalf("run record lost image provenance: %+v", loaded)
	}
	if !reflect.DeepEqual(loaded.PolicyProvenance, run.PolicyProvenance) {
		t.Fatalf("run record lost policy provenance: %+v", loaded.PolicyProvenance)
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

func TestResolveRunImageFallsBackWhenManifestInvalid(t *testing.T) {
	f := newRunImageFixture(t, "schema: 1\nunknown_field: true\n")
	app, runner := f.app(t, f.baseRecord("0.169.0"))

	image := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if image.Source != ImageSourceProfile || image.Image != f.baseRef {
		t.Fatalf("image = %+v, want the profile image", image)
	}
	if image.ManifestDigest != "" {
		t.Fatalf("digest = %q, want empty for an unusable manifest", image.ManifestDigest)
	}
	if len(image.Warnings) != 1 || !strings.Contains(image.Warnings[0], "unusable") {
		t.Fatalf("warnings = %v, want an unusable-manifest warning", image.Warnings)
	}
	for _, call := range runner.outputs {
		if call.name == "podman" {
			t.Fatalf("an unusable manifest must not inspect images: %v", call)
		}
	}
}

func TestResolveRunImageFallsBackWhenManifestUnreadable(t *testing.T) {
	f := newRunImageFixture(t, "")
	if err := os.MkdirAll(RepoConfigPath(f.root), 0o755); err != nil {
		t.Fatal(err)
	}
	app, _ := f.app(t, f.baseRecord("0.169.0"))

	image := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if image.Source != ImageSourceProfile {
		t.Fatalf("source = %q, want profile", image.Source)
	}
	if len(image.Warnings) != 1 || !strings.Contains(image.Warnings[0], "unreadable") {
		t.Fatalf("warnings = %v, want an unreadable-manifest warning", image.Warnings)
	}
}

func TestResolveRunImageFallsBackWhenInventoryFails(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app := &App{Store: f.store, Runner: &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		switch {
		case call.name == "git" && commandHasArg(call, "remote"):
			return []byte("git@github.com:owner/repo.git"), nil
		case call.name == "podman" && commandHasArg(call, "images"):
			return nil, errTestInventory
		}
		return nil, nil
	}}}

	image := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if image.Source != ImageSourceProfile || image.Image != f.baseRef {
		t.Fatalf("image = %+v, want the profile image", image)
	}
	if len(image.Warnings) != 1 || !strings.Contains(image.Warnings[0], "local image inventory unavailable") {
		t.Fatalf("warnings = %v, want an inventory warning", image.Warnings)
	}
}

func TestResolveRunImagePrefersNewestMatchingBase(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	newBase := fmt.Sprintf("localhost/openshell:codex-0.170.0-r%d", f.recipe)
	app, _ := f.app(t,
		f.baseRecord("0.169.0"),
		f.baseRecord("0.170.0"),
		f.repoRecord(t, f.baseRef, "0.169.0", f.digest()),
		f.repoRecord(t, newBase, "0.170.0", f.digest()),
	)

	image := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if image.Image != f.repoTag(t, newBase) || image.BaseImage != newBase {
		t.Fatalf("image = %q on base %q, want the newest base %q", image.Image, image.BaseImage, newBase)
	}
	if len(image.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", image.Warnings)
	}
}

func TestStartWithRepositoryImageComposesRunPolicy(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	app, _ := f.app(t, f.baseRecord("0.169.0"), f.repoRecord(t, f.baseRef, "0.169.0", f.digest()))
	if err := os.MkdirAll(f.store.PolicyDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	profile := f.profile()
	basePolicy := testPolicyBytes(t, map[string]NetworkRule{
		"profile-only": {
			Name:      "profile-only",
			Endpoints: []Endpoint{{Host: "api.example.com", Port: 443}},
			Binaries:  []Binary{{Path: "/usr/local/bin/codex"}},
		},
	})
	if err := os.WriteFile(profile.Policy, basePolicy, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	useWorktree := false
	if err := app.Store.SaveProject(Project{Version: 1, Name: "checkout", Root: f.root, Worktree: &useWorktree}); err != nil {
		t.Fatal(err)
	}
	runner := app.Runner.(*fakeRunner)
	base := runner.output
	runner.output = func(call recordedCommand) ([]byte, error) {
		if call.name == "git" {
			switch {
			case commandHasArg(call, "bundle"):
				return nil, errTestStopStart
			case commandHasArg(call, "--show-toplevel"):
				return []byte(f.root), nil
			case commandHasArg(call, "symbolic-ref"):
				return []byte("main"), nil
			case commandHasArg(call, "rev-parse"):
				return []byte("abc123"), nil
			case commandHasArg(call, "status"), commandHasArg(call, "ls-files"), commandHasArg(call, "diff"):
				return nil, nil
			}
		}
		return base(call)
	}

	run, err := app.Start(context.Background(), StartOptions{Profile: profile.Name, Project: "checkout", Gateway: "openshell"})
	if !errors.Is(err, errTestStopStart) {
		t.Fatalf("start error = %v, want the deliberate bundle failure", err)
	}
	if run.Image != f.repoTag(t, f.baseRef) || run.ImageSource != string(ImageSourceRepo) {
		t.Fatalf("image = %q (%s), want the repository image", run.Image, run.ImageSource)
	}
	if run.PolicyFragmentDigest != f.fragmentDigest(t) || run.PolicyFragment != f.fragmentPath() {
		t.Fatalf("fragment provenance = %q %q", run.PolicyFragment, run.PolicyFragmentDigest)
	}
	composed, err := os.ReadFile(run.Policy)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := ParsePolicy(composed)
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.Network) != 3 {
		t.Fatalf("rules = %v, want the derived provider rule, the profile rule, and the repository rule", policy.Network)
	}
	for _, name := range []string{"provider-api-openai-com-443", "profile-only", "host-proxy-golang-org-443"} {
		if _, ok := policy.Network[name]; !ok {
			t.Fatalf("run policy lost rule %q: %v", name, policy.Network)
		}
	}
	if policy.Landlock.Compatibility != "hard_requirement" {
		t.Fatalf("hard requirement = %q", policy.Landlock.Compatibility)
	}
}

var (
	errTestInventory = errors.New("podman socket unavailable")
	errTestStopStart = errors.New("stop before sandbox")
)

func TestResolveRunImageIgnoresPreviousRecipeImages(t *testing.T) {
	f := newRunImageFixture(t, runRepoManifest)
	f.writeFragment(t)
	oldRecipe := f.recipe - 1
	oldBaseRef := fmt.Sprintf("localhost/openshell:codex-0.169.0-r%d", oldRecipe)
	labels := map[string]string{
		"agentbox":         "true",
		"agentbox.recipe":  strconv.Itoa(oldRecipe),
		"agentbox.agent":   "codex",
		"agentbox.version": "0.169.0",
	}
	oldRepoLabels := map[string]string{
		"agentbox.worker": "true",
		"agentbox.repo":   "github.com/owner/repo",
	}
	for key, value := range labels {
		oldRepoLabels[key] = value
	}
	app, _ := f.app(t,
		f.baseRecord("0.169.0"),
		map[string]any{"Id": "old-base", "Names": []string{oldBaseRef}, "Labels": labels},
		map[string]any{"Id": "old-repo", "Names": []string{f.repoTag(t, oldBaseRef)}, "Labels": oldRepoLabels},
	)

	image := app.ResolveRunImage(context.Background(), f.profile(), f.root)
	if image.Source != ImageSourceProfile || image.Image != f.baseRef {
		t.Fatalf("image = %+v, want the profile image", image)
	}
	if len(image.Warnings) != 1 || !strings.Contains(image.Warnings[0], "no current repository image") {
		t.Fatalf("warnings = %v, want a missing-image hint", image.Warnings)
	}
}
