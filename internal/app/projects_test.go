package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type checkoutGitAnswers struct {
	upstream    string
	upstreamErr error
	counts      string
	countsErr   error
	branch      string
	branchErr   error
	headErr     error
	upstreamRef string
	refErr      error
}

func (a checkoutGitAnswers) runner() *fakeRunner {
	return &fakeRunner{output: func(call recordedCommand) ([]byte, error) {
		switch {
		case gitArgsContain(call, "rev-parse", "--abbrev-ref"):
			if a.upstreamErr != nil {
				return nil, a.upstreamErr
			}
			return []byte(a.upstream + "\n"), nil
		case gitArgsContain(call, "rev-list", "--left-right"):
			if a.countsErr != nil {
				return nil, a.countsErr
			}
			return []byte(a.counts), nil
		case gitArgsContain(call, "symbolic-ref"):
			if a.branchErr != nil {
				return nil, a.branchErr
			}
			return []byte(a.branch + "\n"), nil
		case gitArgsContain(call, "rev-parse", "--verify"):
			if a.headErr != nil {
				return nil, a.headErr
			}
			return []byte("abc123\n"), nil
		case gitArgsContain(call, "for-each-ref", "--format=%(upstream)", "refs/heads/"+a.branch):
			if a.refErr != nil {
				return nil, a.refErr
			}
			return []byte(a.upstreamRef), nil
		}
		return nil, errors.New("unexpected git call")
	}}
}

func gitArgsContain(call recordedCommand, want ...string) bool {
	for i := 0; i+len(want) <= len(call.args); i++ {
		matches := true
		for j := range want {
			if call.args[i+j] != want[j] {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func TestProjectCheckoutStates(t *testing.T) {
	noUpstream := errors.New("no upstream configured")
	notGitRepo := errors.New("fatal: not a git repository (or any of the parent directories): .git")
	cases := []struct {
		name           string
		answers        checkoutGitAnswers
		state          string
		ahead          int
		behind         int
		reason         string
		reasonContains string
	}{
		{
			name:    "up to date",
			answers: checkoutGitAnswers{upstream: "origin/main", counts: "0\t0\n"},
			state:   checkoutUpToDate,
		},
		{
			name:    "behind",
			answers: checkoutGitAnswers{upstream: "origin/main", counts: "0\t22\n"},
			state:   checkoutBehind,
			behind:  22,
		},
		{
			name:    "ahead",
			answers: checkoutGitAnswers{upstream: "origin/main", counts: "3\t0\n"},
			state:   checkoutAhead,
			ahead:   3,
		},
		{
			name:    "diverged",
			answers: checkoutGitAnswers{upstream: "origin/main", counts: "2\t5\n"},
			state:   checkoutDiverged,
			ahead:   2,
			behind:  5,
		},
		{
			name:    "no upstream",
			answers: checkoutGitAnswers{upstreamErr: noUpstream, branch: "main", upstreamRef: ""},
			state:   checkoutNoUpstream,
		},
		{
			name:    "detached head",
			answers: checkoutGitAnswers{upstreamErr: noUpstream, branchErr: errors.New("detached")},
			state:   checkoutNoUpstream,
			reason:  "detached HEAD",
		},
		{
			name:           "unborn branch",
			answers:        checkoutGitAnswers{upstreamErr: noUpstream, branch: "feature", headErr: errors.New("bad revision")},
			state:          checkoutUnknown,
			reasonContains: "branch feature has no commits",
		},
		{
			name:           "rev-list failure",
			answers:        checkoutGitAnswers{upstream: "origin/main", countsErr: errors.New("rev-list failed")},
			state:          checkoutUnknown,
			reasonContains: "rev-list failed",
		},
		{
			name:           "not a git repository",
			answers:        checkoutGitAnswers{upstreamErr: notGitRepo, branchErr: notGitRepo, headErr: notGitRepo},
			state:          checkoutUnknown,
			reasonContains: "not a git repository",
		},
		{
			name:    "upstream ref missing",
			answers: checkoutGitAnswers{upstreamErr: noUpstream, branch: "main", upstreamRef: "refs/remotes/origin/main\n"},
			state:   checkoutUnknown,
			reason:  "upstream origin/main is not available locally; fetch it",
		},
		{
			name:           "for-each-ref failure",
			answers:        checkoutGitAnswers{upstreamErr: noUpstream, branch: "main", refErr: errors.New("for-each-ref failed")},
			state:          checkoutUnknown,
			reasonContains: "for-each-ref failed",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			runner := test.answers.runner()
			got := (&App{Runner: runner}).ProjectCheckout(context.Background(), "/repo")
			if got.State != test.state || got.Ahead != test.ahead || got.Behind != test.behind {
				t.Fatalf("checkout = %+v; want state %q ahead %d behind %d", got, test.state, test.ahead, test.behind)
			}
			if test.reason != "" && got.Reason != test.reason {
				t.Fatalf("reason = %q; want %q", got.Reason, test.reason)
			}
			if test.reasonContains != "" && !strings.Contains(got.Reason, test.reasonContains) {
				t.Fatalf("reason = %q; want it to contain %q", got.Reason, test.reasonContains)
			}
		})
	}
}

func TestProjectCheckoutSummary(t *testing.T) {
	cases := []struct {
		checkout ProjectCheckout
		want     string
	}{
		{checkout: ProjectCheckout{State: checkoutUpToDate}, want: "up to date"},
		{checkout: ProjectCheckout{State: checkoutBehind, Behind: 22}, want: "22 behind"},
		{checkout: ProjectCheckout{State: checkoutAhead, Ahead: 3}, want: "3 ahead"},
		{checkout: ProjectCheckout{State: checkoutDiverged, Ahead: 2, Behind: 5}, want: "2 ahead, 5 behind"},
		{checkout: ProjectCheckout{State: checkoutNoUpstream}, want: "no upstream"},
		{checkout: ProjectCheckout{State: checkoutUnknown}, want: "unknown"},
		{checkout: ProjectCheckout{State: checkoutUnknown, Reason: "not a git repository"}, want: "unknown (not a git repository)"},
	}
	for _, test := range cases {
		if got := test.checkout.Summary(); got != test.want {
			t.Fatalf("Summary(%+v) = %q; want %q", test.checkout, got, test.want)
		}
	}
}

func TestProjectCheckoutDoesNotFetch(t *testing.T) {
	runner := (checkoutGitAnswers{upstream: "origin/main", counts: "0\t3\n"}).runner()
	app := App{Runner: runner}
	if got := app.ProjectCheckout(context.Background(), "/repo"); got.State != checkoutBehind || got.Behind != 3 {
		t.Fatalf("checkout = %+v", got)
	}
	if len(runner.outputs) != 2 {
		t.Fatalf("normal checkout used %d git commands; want 2", len(runner.outputs))
	}
	for _, call := range runner.outputs {
		for _, arg := range call.args {
			switch arg {
			case "fetch", "pull", "ls-remote":
				t.Fatalf("checkout ran a network command: %#v", call)
			}
		}
		if gitArgsContain(call, "remote", "update") {
			t.Fatalf("checkout ran a remote update: %#v", call)
		}
	}
}

func TestProjectRegistrationAndDirectRun(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	source := filepath.Join(base, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "init", "-q")
	cmd(t, source, "git", "config", "user.name", "Tester")
	cmd(t, source, "git", "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "add", ".")
	cmd(t, source, "git", "commit", "-qm", "base")

	app := App{Store: Store{Base: filepath.Join(base, "state")}, Runner: OSRunner{}}
	project, err := app.RegisterProject(ctx, "dev", source, false)
	if err != nil {
		t.Fatal(err)
	}
	if project.UseWorktree() {
		t.Fatal("project should be direct mode")
	}
	loaded, err := app.Store.LoadProject("dev")
	if err != nil || loaded.Root != project.Root {
		t.Fatalf("project did not round trip: %+v, %v", loaded, err)
	}

	run, err := app.CreateDirectRun(ctx, project.Root, "run-direct123")
	if err != nil {
		t.Fatal(err)
	}
	if run.Worktree != project.Root || run.Source != project.Root || run.Isolated() {
		t.Fatalf("direct run used unexpected workspace: %+v", run)
	}
	if run.Branch == "" || run.Baseline == "" {
		t.Fatalf("direct run missing branch/baseline: %+v", run)
	}

	run.Status = StatusWorking
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	app.Runner = statusRunnerFor("running", "session=1\ndead=0\ncommand=codex\nactivity={\"state\":\"working\"}\n")
	if err := app.requireDirectRunAvailable(ctx, project.Root); err == nil || !strings.Contains(err.Error(), "active run") {
		t.Fatalf("active direct run was not guarded: %v", err)
	}
	app.Runner = statusRunnerFor("running", "session=0\ndead=1\nfinished=1\n")
	if err := app.requireDirectRunAvailable(ctx, project.Root); err != nil {
		t.Fatalf("completed direct run still blocked: %v", err)
	}
	app.Runner = &fakeRunner{output: func(recordedCommand) ([]byte, error) { return nil, errors.New("gateway down") }}
	if err := app.requireDirectRunAvailable(ctx, project.Root); err == nil || !strings.Contains(err.Error(), "cannot verify") {
		t.Fatalf("unverifiable direct run was not guarded: %v", err)
	}
}

func TestDirectRunRequiresCleanCheckout(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	source := filepath.Join(base, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "init", "-q")
	cmd(t, source, "git", "config", "user.name", "Tester")
	cmd(t, source, "git", "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "add", ".")
	cmd(t, source, "git", "commit", "-qm", "base")
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("dirty\n"), 0600); err != nil {
		t.Fatal(err)
	}
	app := App{Store: Store{Base: filepath.Join(base, "state")}, Runner: OSRunner{}}
	if _, err := app.CreateDirectRun(ctx, source, "run-dirty123"); err == nil || !strings.Contains(err.Error(), "clean checkout") {
		t.Fatalf("dirty direct run was accepted: %v", err)
	}
}

func TestDeleteDirectRunKeepsProjectCheckout(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	source := filepath.Join(base, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "init", "-q")
	cmd(t, source, "git", "config", "user.name", "Tester")
	cmd(t, source, "git", "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd(t, source, "git", "add", ".")
	cmd(t, source, "git", "commit", "-qm", "base")
	runner := &fakeRunner{}
	app := App{Store: Store{Base: filepath.Join(base, "state")}, Runner: runner}
	run, err := app.CreateDirectRun(ctx, source, "run-directdelete")
	if err != nil {
		t.Fatal(err)
	}
	run.Status = StatusCreating
	run.Synced = true
	run.Policy = filepath.Join(app.Store.PolicyDir(), run.ID+".yaml")
	if err := app.Store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(ctx, run.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("direct project checkout was removed: %v", err)
	}
	for _, call := range runner.outputs {
		if call.name == "openshell" && containsCommand(call.args, "sandbox", "delete") {
			t.Fatalf("prepared record attempted a sandbox delete: %#v", call)
		}
	}
}

func TestDeleteProjectRemovesRegistrationAndKeepsRoot(t *testing.T) {
	root := t.TempDir()
	store := Store{Base: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProject(Project{Version: 1, Name: "unused", Root: root}); err != nil {
		t.Fatal(err)
	}
	app := App{Store: store, Runner: &fakeRunner{}}
	if err := app.DeleteProject("unused"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.ProjectPath("unused")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("project file still exists: %v", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("project root not retained: %v", err)
	}
	if err := app.DeleteProject("unused"); err == nil {
		t.Fatal("missing project was accepted")
	}
}

func TestDeleteProjectRefusedWhenAnyRunReferences(t *testing.T) {
	for _, status := range []RunStatus{StatusWorking, StatusCompleted} {
		t.Run(string(status), func(t *testing.T) {
			store := Store{Base: t.TempDir()}
			if err := store.Ensure(); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveProject(Project{Version: 1, Name: "held", Root: "/repo"}); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveRun(Run{ID: "run-held123", Name: "held-run", Project: "held", Status: status}); err != nil {
				t.Fatal(err)
			}
			app := App{Store: store, Runner: &fakeRunner{}}
			err := app.DeleteProject("held")
			var conflict *ConflictError
			if err == nil || !errors.As(err, &conflict) {
				t.Fatalf("error = %v; want conflict", err)
			}
			if !slices.Equal(conflict.Dependents, []string{"held-run"}) {
				t.Fatalf("dependents = %#v", conflict.Dependents)
			}
			if _, err := os.Stat(store.ProjectPath("held")); err != nil {
				t.Fatalf("project was removed: %v", err)
			}
		})
	}
}
