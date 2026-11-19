package skills

import (
	"strings"
	"testing"
)

// cliCollapsed returns the rendered agentbox-cli skill with whitespace
// collapsed so assertions do not depend on line wrapping.
func cliCollapsed(t *testing.T) string {
	t.Helper()
	content, err := Embedded("agentbox-cli", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(string(content)), " ")
}

// TestCLISkillRepositoryEnvironmentAuthoring pins the one-time manifest
// authoring procedure: evidence-derived entries for toolchains, packages,
// verify commands, and hosts; the exact-host and credential-free rules; and
// dry-run validation before setup.
func TestCLISkillRepositoryEnvironmentAuthoring(t *testing.T) {
	body := cliCollapsed(t)
	wantAll(t, body,
		"set up a repository for AgentBox agents",
		"Set up AgentBox for this repository.",
		"`.agentbox/config.yaml`",
		"no CLI command infers tools or hosts",
		"image.toolchains",
		"image.packages",
		"verify",
		"proxy.golang.org",
		"sum.golang.org",
		"registry.npmjs.org",
		"/usr/bin/git",
		"git remote get-url origin",
		"Exact hostnames only",
		"no credentials or tokens anywhere in the file",
		"agentbox repo setup --dry",
		"agentbox repo setup",
	)
}

// TestCLISkillRepositoryEnvironmentOwnership pins profile ownership and the
// stale-binary guard: setup reports references, profiles stay untouched, and
// an unknown repo command means the binary predates repository manifests.
func TestCLISkillRepositoryEnvironmentOwnership(t *testing.T) {
	body := cliCollapsed(t)
	wantAll(t, body,
		"do not create or edit profiles",
		"runs for the project use the repository image automatically",
		"the installed binary predates repository manifests",
	)
}

// TestCLISkillBootstrapAndCLIOnly pins the ground rules: a missing or stale
// binary is installed from main, and environment operations never drive
// openshell or podman directly.
func TestCLISkillBootstrapAndCLIOnly(t *testing.T) {
	body := cliCollapsed(t)
	wantAll(t, body,
		"go install github.com/dmartinol/agentbox/cmd/agentbox@main",
		"Never drive `openshell` or `podman` directly",
	)
}

// TestCLISkillStopAndSyncSequence pins the supported close sequence: stop
// syncs the changes out before the sandbox stops, keeps the run, branch, and
// worktree, and work resumes from the synced branch.
func TestCLISkillStopAndSyncSequence(t *testing.T) {
	body := cliCollapsed(t)
	wantAll(t, body,
		"syncs the sandbox changes into the host worktree before the sandbox stops",
		"work resumes from the synced branch",
		"refuses to overwrite a worktree that changed since the last sync",
	)
}

// TestCLISkillHostBlockPersistsToManifest pins the recovery rule: an allowed
// host is also appended to the repository manifest, and policy files are
// never hand-edited to persist a host.
func TestCLISkillHostBlockPersistsToManifest(t *testing.T) {
	body := cliCollapsed(t)
	wantAll(t, body,
		"append the host to its `hosts` section",
		"Never hand-edit policy files to persist a host",
	)
}

// TestOrchestratorSkillRepositoryEnvironment pins the orchestrator's one-time
// repository worker environment procedure: the triage check, the approval and
// preparation standing, the agentbox-cli procedure with dry-run validation,
// the stale-binary guard, and profile ownership.
func TestOrchestratorSkillRepositoryEnvironment(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"prepare repository environments",
		"Repository worker environment (one-time)",
		"Check for it during triage",
		"the same standing as worktree creation, not stage work",
		"agentbox repo setup --dry",
		"agentbox repo setup",
		"the installed binary predates repository manifests",
		"Never create or edit a profile for this",
		"runs started for the project use the repository image automatically",
	)
}

// TestOrchestratorSkillAgentBoxWorkerConventions pins the AgentBox worker
// loop conventions: run naming and the generated branch format, the stop and
// sync close sequence for interrupted workers, and host-denial recovery.
func TestOrchestratorSkillAgentBoxWorkerConventions(t *testing.T) {
	body := collapsed(t)
	wantAll(t, body,
		"issue-<number>-<slug>",
		"<branch_prefix>/<project>/issue-<number>-<slug>",
		"agentbox agent stop RUN_ID",
		"syncs the changes into the host worktree before the sandbox stops",
		"agentbox policy edit --allow-host HOST RUN_ID",
		"append the host to `.agentbox/config.yaml`",
	)
}

// TestRepoSetupReferencesAreConsistentAcrossSkills asserts both skills
// reference the repository setup commands and the bootstrap install with
// identical spelling, so the instructions cannot drift apart.
func TestRepoSetupReferencesAreConsistentAcrossSkills(t *testing.T) {
	for _, name := range []string{"agentbox-cli", "agentbox-orchestrator"} {
		content, err := Embedded(name, "9.9.9")
		if err != nil {
			t.Fatal(err)
		}
		body := strings.Join(strings.Fields(string(content)), " ")
		wantAll(t, body,
			"agentbox repo setup --dry",
			"agentbox repo setup",
			"go install github.com/dmartinol/agentbox/cmd/agentbox@main",
			"`.agentbox/config.yaml`",
		)
	}
}
