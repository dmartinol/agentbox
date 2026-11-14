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
