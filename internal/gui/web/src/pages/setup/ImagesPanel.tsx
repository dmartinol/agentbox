import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { GuiTask, ImageArtifact, Profile, Project, Registry, RepoSetupResult } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { ActionButton } from '../../components/ui/action';
import { EmptyState } from '../../components/ui/empty';
import { Field } from '../../components/ui/field';
import { Input } from '../../components/ui/input';
import { Modal } from '../../components/ui/dialog';
import { ConfirmDialog } from '../../components/ui/confirm';
import { ExpandableValue } from '../../components/ui/copy-value';
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/table';
import { StatusBadge } from '../../components/StatusBadge';
import type { LogDrawerState } from '../../components/LogDrawer';

export function ImagesPanel({
  openLog,
  updateLog
}: {
  openLog: (state: Omit<LogDrawerState, 'open'>) => void;
  updateLog: (state: Partial<LogDrawerState>) => void;
}) {
  const [images, setImages] = useState<ImageArtifact[]>([]);
  const [registryImages, setRegistryImages] = useState<ImageArtifact[]>([]);
  const [registry, setRegistry] = useState<Registry>({ reference: '' });
  const [projects, setProjects] = useState<Project[]>([]);
  const [repoProject, setRepoProject] = useState('');
  const [repoAgents, setRepoAgents] = useState<Record<string, boolean>>({});
  const [repoAgentsTouched, setRepoAgentsTouched] = useState(false);
  const [validation, setValidation] = useState<RepoSetupResult>();
  const [repoError, setRepoError] = useState<Error | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [actionError, setActionError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [buildOpen, setBuildOpen] = useState(false);
  const [buildForm, setBuildForm] = useState({ agent: 'codex', version: '' });
  const [task, setTask] = useState<GuiTask>();
  const [pullOpen, setPullOpen] = useState(false);
  const [pullRef, setPullRef] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<ImageArtifact>();
  const [pruneOpen, setPruneOpen] = useState(false);
  const [clearOpen, setClearOpen] = useState(false);
  const [clearError, setClearError] = useState('');
  const [clearBusy, setClearBusy] = useState(false);
  const [busy, setBusy] = useState<string>();

  const load = async () => {
    try {
      const [nextImages, nextRegistry, nextProjects, nextProfiles] = await Promise.all([api.images(), api.registry(), api.projects(), api.profiles()]);
      setImages(nextImages ?? []);
      setRegistry(nextRegistry);
      setProjects(nextProjects ?? []);
      setRepoProject((current) => (current || nextProjects?.[0]?.name) || '');
      if (!repoAgentsTouched) {
        // Default the checkboxes to the agents present in local profiles,
        // matching the CLI default for `agentbox repo setup`.
        const next: Record<string, boolean> = {};
        for (const agent of ['codex', 'claude', 'opencode']) {
          next[agent] = (nextProfiles ?? []).some((profile) => profile.agent === agent);
        }
        setRepoAgents(next);
      }
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  useEffect(() => {
    if (!task || task.status !== 'running') return;
    let active = true;
    const timer = window.setInterval(async () => {
      try {
        const next = await api.task(task.id);
        if (!active) return;
        setTask(next);
        updateLog({ content: next.output, taskStatus: next.status, loading: false });
        if (next.status === 'succeeded') await load();
      } catch {
        // Keep last output on transient polling failure.
      }
    }, 1000);
    return () => { active = false; window.clearInterval(timer); };
  }, [task, updateLog]);

  const action = async (key: string, callback: () => Promise<unknown>, message: string) => {
    setBusy(key);
    try {
      await callback();
      setNotice(message);
      await load();
    } catch (err) {
      setError(err as Error);
    } finally {
      setBusy(undefined);
    }
  };

  const build = async () => {
    const next = await api.buildImage(buildForm.agent, buildForm.version);
    setTask(next);
    setBuildOpen(false);
    openLog({ title: `Build image · ${buildForm.agent}`, content: next.output, loading: true, taskId: next.id, taskStatus: next.status });
  };

  const refresh = async () => {
    setBusy('refresh');
    try {
      const next = await api.refreshImages();
      setTask(next);
      openLog({ title: 'Refresh all agents', content: next.output, loading: true, taskId: next.id, taskStatus: next.status });
    } catch (err) {
      setError(err as Error);
    } finally {
      setBusy(undefined);
    }
  };

  const selectedRepoAgents = () => ['codex', 'claude', 'opencode'].filter((agent) => repoAgents[agent]);

  const selectedProject = projects.find((project) => project.name === repoProject);

  const validateRepo = async () => {
    setBusy('repo-validate');
    setRepoError(null);
    try {
      const result = await api.validateRepoSetup({ project: repoProject, agents: selectedRepoAgents() });
      setValidation(result);
    } catch (err) {
      setRepoError(err as Error);
      setValidation(undefined);
    } finally {
      setBusy(undefined);
    }
  };

  const setupRepo = async () => {
    setBusy('repo-setup');
    setRepoError(null);
    try {
      const next = await api.setupRepo({ project: repoProject, agents: selectedRepoAgents() });
      setTask(next);
      setValidation(undefined);
      openLog({ title: `Set up repository · ${repoProject}`, content: next.output, loading: true, taskId: next.id, taskStatus: next.status });
    } catch (err) {
      setRepoError(err as Error);
    } finally {
      setBusy(undefined);
    }
  };

  const remove = async () => {
    if (!deleteTarget) return;
    setBusy('delete');
    try {
      await api.deleteImage(deleteTarget.reference);
      setNotice(`Image ${deleteTarget.reference} deleted`);
      setActionError(null);
      setDeleteTarget(undefined);
      await load();
    } catch (err) {
      setActionError(err as Error);
    } finally {
      setBusy(undefined);
    }
  };

  const prune = async () => {
    setBusy('prune');
    try {
      const result = await api.pruneImages();
      setNotice(result.removed.length ? `Removed ${result.removed.length} unused image(s)` : 'No unused images to remove');
      setActionError(null);
      setPruneOpen(false);
      await load();
    } catch (err) {
      setActionError(err as Error);
    } finally {
      setBusy(undefined);
    }
  };

  const clearRegistry = async () => {
    setClearBusy(true);
    setClearError('');
    try {
      await api.clearRegistry();
      setRegistry({ reference: '' });
      setRegistryImages([]);
      setClearOpen(false);
      setNotice('Saved registry reference removed');
    } catch (err) {
      setClearError((err as Error).message);
    } finally {
      setClearBusy(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap justify-end gap-2">
        <Button size="sm" variant="outline" loading={busy === 'refresh'} onClick={() => void refresh()}>{busy === 'refresh' ? 'Refreshing' : 'Refresh all agents'}</Button>
        <Button size="sm" variant="outline" onClick={() => { setActionError(null); setPruneOpen(true); }}>Prune unused</Button>
        <Button size="sm" onClick={() => setBuildOpen(true)}>Build image</Button>
        <Button size="sm" variant="outline" onClick={() => setPullOpen(true)}>Pull image</Button>
      </div>
      {notice ? <Alert variant="success" title={notice} /> : null}
      {error ? <Alert variant="danger" title="Unable to load images">{error.message}</Alert> : null}
      {loading ? <EmptyState title="Loading images" /> : images.length === 0 ? (
        <EmptyState title="No local images" description="Build or pull an AgentBox agent image." />
      ) : (
        <Table>
          <THead><TR><TH className="w-[34%]">Reference</TH><TH className="w-[10%]">Agent</TH><TH className="w-[10%]">Version</TH><TH className="w-[8%]">Recipe</TH><TH>Platform</TH><TH>Actions</TH></TR></THead>
          <TBody>
            {images.map((image) => {
              const holders = image.used_by ?? [];
              return (
                <TR key={image.reference}>
                  <TD className="truncate"><ExpandableValue value={image.reference} label="image reference" max={44} /></TD>
                  <TD label="Agent">{image.agent}</TD>
                  <TD label="Version">{image.version}</TD>
                  <TD label="Recipe">r{image.recipe}</TD>
                  <TD label="Platform">{image.os || 'linux'}/{image.arch || 'arm64'}</TD>
                  <TD label="Actions">
                    <div className="flex items-center gap-2">
                      <ActionButton
                        size="sm"
                        variant="ghost"
                        className="text-danger"
                        disabledReason={holders.length ? `In use by ${holders.join(', ')}` : undefined}
                        onClick={() => { setActionError(null); setDeleteTarget(image); }}
                      >
                        Delete
                      </ActionButton>
                    </div>
                  </TD>
                </TR>
              );
            })}
          </TBody>
        </Table>
      )}

      <div className="rounded-lg border border-border p-3">
        <p className="text-sm font-medium">Repository images</p>
        <p className="mt-1 text-sm text-muted">Build the repository image and policy fragment from the project's committed .agentbox/config.yaml. Profiles need no change.</p>
        <div className="mt-3 flex flex-wrap items-end gap-2">
          <Field label="Project" htmlFor="repo-setup-project" className="min-w-0 flex-1 basis-full sm:basis-48">
            <select
              id="repo-setup-project"
              className="h-8 w-full rounded-md border border-border bg-surface px-2"
              value={repoProject}
              onChange={(event) => { setRepoProject(event.target.value); setValidation(undefined); setRepoError(null); }}
            >
              {projects.length === 0 ? <option value="">No registered projects</option> : null}
              {projects.map((project) => (
                <option key={project.name} value={project.name}>{project.manifest ? project.name : `${project.name} — no manifest`}</option>
              ))}
            </select>
          </Field>
          <div className="flex flex-wrap items-end gap-3">
            <span className="text-sm text-muted" id="repo-setup-agents-label">Agents</span>
            <div className="flex flex-wrap gap-3" role="group" aria-labelledby="repo-setup-agents-label">
              {['codex', 'claude', 'opencode'].map((agent) => (
                <label key={agent} className="flex items-center gap-1 text-sm">
                  <input
                    type="checkbox"
                    checked={Boolean(repoAgents[agent])}
                    onChange={(event) => { setRepoAgentsTouched(true); setRepoAgents({ ...repoAgents, [agent]: event.target.checked }); }}
                  />
                  {agent}
                </label>
              ))}
            </div>
          </div>
          <Button
            size="sm"
            variant="outline"
            loading={busy === 'repo-validate'}
            disabled={!repoProject}
            onClick={() => void validateRepo()}
          >
            {busy === 'repo-validate' ? 'Validating' : 'Validate'}
          </Button>
          <ActionButton
            size="sm"
            disabledReason={!repoProject ? 'Register a project first' : selectedRepoAgents().length === 0 ? 'Select at least one agent' : undefined}
            onClick={() => void setupRepo()}
          >
            {busy === 'repo-setup' ? 'Setting up' : 'Set up repository'}
          </ActionButton>
        </div>
        {selectedProject && !selectedProject.manifest ? (
          <p className="mt-2 text-sm text-muted">
            No .agentbox/config.yaml committed at {selectedProject.root}. Runs for this project keep using the profile image until a manifest is committed.
          </p>
        ) : null}
        {repoError ? <Alert variant="danger" title="Repository setup failed">{repoError.message}</Alert> : null}
        {validation ? (
          <div className="mt-3 space-y-1 text-sm">
            <div><span className="text-muted">Repository:</span> {validation.repo}</div>
            <div className="flex min-w-0 items-baseline gap-1"><span className="text-muted">Manifest digest:</span> <ExpandableValue value={validation.manifest_digest} label="manifest digest" max={40} /></div>
            <div><span className="text-muted">Agents:</span> {validation.agents.join(', ')}</div>
            <div className="flex min-w-0 items-baseline gap-1"><span className="text-muted">Fragment:</span> <ExpandableValue value={validation.fragment_path} label="fragment path" max={44} /> ({validation.fragment_status})</div>
            <p className="text-muted">Dry run: nothing was built or written. Image tags resolve at build time from the newest local base images.</p>
          </div>
        ) : null}
      </div>

      <div className="rounded-lg border border-border p-3">
        <div className="flex flex-wrap items-end gap-2">
          <Field label="Registry repository" htmlFor="registry-reference" className="min-w-0 flex-1 basis-full sm:basis-48">
            <Input id="registry-reference" value={registry.reference} onChange={(event) => setRegistry({ reference: event.target.value })} />
          </Field>
          <Button
            size="sm"
            loading={busy === 'discover'}
            onClick={() => void action('discover', async () => {
              await api.saveRegistry(registry.reference);
              setRegistryImages((await api.discoverRegistry()) ?? []);
            }, 'Registry discovery complete')}
          >
            {busy === 'discover' ? 'Discovering' : 'Discover'}
          </Button>
          <ActionButton
            size="sm"
            variant="outline"
            disabledReason={registry.reference.trim() ? undefined : 'No saved registry reference'}
            onClick={() => { setClearError(''); setClearOpen(true); }}
          >
            Clear
          </ActionButton>
        </div>
        {registryImages.length > 0 ? (
          <Table className="mt-3">
            <THead><TR><TH className="w-[12%]">Agent</TH><TH className="w-[12%]">Version</TH><TH>Reference</TH></TR></THead>
            <TBody>
              {registryImages.map((image) => (
                <TR key={image.reference}>
                  <TD label="Agent">{image.agent}</TD>
                  <TD label="Version">{image.version}</TD>
                  <TD label="Reference"><ExpandableValue value={image.reference} label="registry image reference" max={44} /></TD>
                </TR>
              ))}
            </TBody>
          </Table>
        ) : null}
      </div>

      <Modal
        open={buildOpen}
        onOpenChange={setBuildOpen}
        title="Build image"
        description="Build a local AgentBox agent image."
        footer={<><Button variant="ghost" onClick={() => setBuildOpen(false)}>Cancel</Button><Button loading={busy === 'build'} onClick={() => void action('build', build, 'Image build started')}>{busy === 'build' ? 'Building' : 'Build'}</Button></>}
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Agent" htmlFor="image-agent" required>
            <select id="image-agent" className="h-8 rounded-md border border-border bg-surface px-2" value={buildForm.agent} onChange={(event) => setBuildForm({ ...buildForm, agent: event.target.value })}>
              {['codex', 'claude', 'opencode'].map((agent) => <option key={agent} value={agent}>{agent}</option>)}
            </select>
          </Field>
          <Field label="Exact version" htmlFor="image-version" helper="Blank resolves latest."><Input id="image-version" value={buildForm.version} onChange={(event) => setBuildForm({ ...buildForm, version: event.target.value })} /></Field>
          <Button
            variant="outline"
            loading={busy === 'version'}
            onClick={() => void action('version', async () => setBuildForm({ ...buildForm, version: (await api.imageVersion(buildForm.agent)).version }), 'Version discovered')}
          >
            {busy === 'version' ? 'Discovering' : 'Discover latest'}
          </Button>
        </div>
        {task ? <div className="mt-3 flex items-center gap-2 text-sm text-muted"><StatusBadge status={task.status} /> Build output opens in the log drawer.</div> : null}
      </Modal>

      <Modal
        open={pullOpen}
        onOpenChange={setPullOpen}
        title="Pull image"
        footer={<><Button variant="ghost" onClick={() => setPullOpen(false)}>Cancel</Button><Button loading={busy === 'pull'} onClick={() => void action('pull', async () => { await api.pullImage(pullRef); setPullOpen(false); }, 'Image pull requested')}>{busy === 'pull' ? 'Pulling' : 'Pull'}</Button></>}
      >
        <Field label="Remote reference" htmlFor="pull-reference" required><Input id="pull-reference" value={pullRef} onChange={(event) => setPullRef(event.target.value)} /></Field>
      </Modal>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => !open && setDeleteTarget(undefined)}
        title={deleteTarget ? `Delete ${deleteTarget.reference}` : 'Delete image'}
        confirmLabel="Delete"
        busyLabel="Deleting"
        busy={busy === 'delete'}
        disabled={Boolean(deleteTarget?.used_by?.length)}
        onConfirm={() => void remove()}
      >
        {actionError ? <Alert variant="danger" title="Unable to delete image">{actionError.message}</Alert> : null}
        {deleteTarget?.used_by?.length ? (
          <Alert variant="warning" title="Image is in use">Remove it from these holders first: {deleteTarget.used_by.join(', ')}</Alert>
        ) : <p className="text-sm text-muted">Delete this local image and its tags. Profile and run records are kept.</p>}
      </ConfirmDialog>

      <ConfirmDialog
        open={pruneOpen}
        onOpenChange={setPruneOpen}
        title="Prune unused images"
        confirmLabel="Prune"
        busyLabel="Pruning"
        busy={busy === 'prune'}
        onConfirm={() => void prune()}
      >
        {actionError ? <Alert variant="danger" title="Unable to prune images">{actionError.message}</Alert> : null}
        <p className="text-sm text-muted">Delete every AgentBox image that no profile and no non-terminal run references.</p>
      </ConfirmDialog>

      <ConfirmDialog
        open={clearOpen}
        onOpenChange={(open) => {
          if (!open) {
            setClearOpen(false);
            setClearError('');
          }
        }}
        title="Clear saved registry"
        confirmLabel="Clear registry"
        busyLabel="Clearing"
        busy={clearBusy}
        onConfirm={() => void clearRegistry()}
      >
        {clearError ? <Alert variant="danger" title="Unable to clear registry" className="mb-3">{clearError}</Alert> : null}
        <p className="text-sm text-muted">Remove only the saved registry reference. Podman images and registry credentials are kept.</p>
      </ConfirmDialog>
    </div>
  );
}
