import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { Project, ProjectCheckout } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { ActionButton } from '../../components/ui/action';
import { ConfirmDialog } from '../../components/ui/confirm';
import { ExpandableValue } from '../../components/ui/copy-value';
import { EmptyState } from '../../components/ui/empty';
import { Field } from '../../components/ui/field';
import { Input } from '../../components/ui/input';
import { Modal } from '../../components/ui/dialog';
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/table';
import { Disclosure } from '../../components/ui/disclosure';

const emptyProject: Project = { version: 1, name: '', root: '', worktree: true };

function checkoutText(checkout?: ProjectCheckout): string {
  if (!checkout) return 'unknown';
  switch (checkout.state) {
    case 'up-to-date':
      return 'up to date';
    case 'behind':
      return `${checkout.behind} behind`;
    case 'ahead':
      return `${checkout.ahead} ahead`;
    case 'diverged':
      return `${checkout.ahead} ahead, ${checkout.behind} behind`;
    case 'no-upstream':
      return 'no upstream';
    default:
      return 'unknown';
  }
}

function ProjectCheckoutRow({ checkout }: { checkout?: ProjectCheckout }) {
  return (
    <>
      <dt className="text-muted">Checkout</dt>
      <dd className="min-w-0 break-words">
        {checkoutText(checkout)}
        {checkout?.reason ? <span className="block text-xs text-muted">{checkout.reason}</span> : null}
        <span className="block text-xs text-muted">Derived from local remote-tracking refs; no fetch.</span>
      </dd>
    </>
  );
}

function ProjectRepositoryDetails({ project }: { project: Project }) {
  const status = project.repo_status;
  return (
    <div className="space-y-2 text-sm">
      {status ? <p className="break-words text-xs text-muted">Read-only · edit .agentbox/config.yaml or run agentbox repo setup</p> : null}
      <dl className="grid grid-cols-[7rem_minmax(0,1fr)] gap-x-2 gap-y-1">
        <ProjectCheckoutRow checkout={project.checkout} />
        {status ? (
          <>
            <dt className="text-muted">Manifest</dt>
            <dd className="min-w-0">{status.manifest_present ? 'present' : 'absent'} · <span className="break-all">{status.manifest_path}</span>{status.manifest_digest ? ` · ${status.manifest_digest}` : ''}</dd>
            <dt className="text-muted">Fragment</dt>
            <dd className="min-w-0">{status.fragment_status}{status.fragment_path ? ' · ' : ''}{status.fragment_path ? <span className="break-all">{status.fragment_path}</span> : null}{status.fragment_digest ? ` · ${status.fragment_digest}` : ''}</dd>
          </>
        ) : null}
      </dl>
      {status ? (
        <>
          <div>
            <div className="text-xs font-semibold uppercase tracking-wide text-muted">Manifest host rules</div>
            {status.host_rules?.length ? (
              <ul className="mt-1 space-y-1">
                {status.host_rules.map((rule) => (
                  <li key={`${rule.host}:${rule.port}`} className="break-all font-mono text-xs">
                    {rule.host}:{rule.port} — {rule.binaries.join(', ')}
                  </li>
                ))}
              </ul>
            ) : <p className="mt-1 text-xs text-muted">None</p>}
          </div>
          <div>
            <div className="text-xs font-semibold uppercase tracking-wide text-muted">Worker images</div>
            <ul className="mt-1 space-y-1">
              {status.worker_images.map((image) => (
                <li key={image.agent} className="break-all text-xs">
                  {image.agent}: {image.status}{image.reference ? ` · ${image.reference}` : ''}
                </li>
              ))}
            </ul>
          </div>
          {status.warnings?.length ? (
            <div>
              <div className="text-xs font-semibold uppercase tracking-wide text-muted">Warnings</div>
              <ul className="mt-1 list-disc space-y-1 pl-4 text-xs">
                {status.warnings.map((warning) => <li key={warning}>{warning}</li>)}
              </ul>
            </div>
          ) : null}
        </>
      ) : (
        <div>
          <p className="text-sm text-muted">Repository status unavailable.</p>
        </div>
      )}
    </div>
  );
}

export function ProjectsPanel({ onChanged }: { onChanged?: () => void }) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<Project>(emptyProject);
  const [deleteProject, setDeleteProject] = useState<Project | null>(null);
  const [deleteError, setDeleteError] = useState('');
  const [deleteBusy, setDeleteBusy] = useState(false);

  const load = async () => {
    try {
      setProjects((await api.projects()) ?? []);
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  const save = async () => {
    try {
      await api.saveProject(form);
      setOpen(false);
      setNotice('Project saved');
      await load();
      onChanged?.();
    } catch (err) {
      setError(err as Error);
    }
  };

  const remove = async () => {
    if (!deleteProject) return;
    setDeleteBusy(true);
    setDeleteError('');
    try {
      await api.deleteProject(deleteProject.name);
      setDeleteProject(null);
      setNotice(`Project ${deleteProject.name} deleted`);
      await load();
      onChanged?.();
    } catch (err) {
      setDeleteError((err as Error).message);
    } finally {
      setDeleteBusy(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex justify-end"><Button size="sm" onClick={() => { setForm(emptyProject); setOpen(true); }}>Create project</Button></div>
      {notice ? <Alert variant="success" title={notice} /> : null}
      {error ? <Alert variant="danger" title="Unable to load projects">{error.message}</Alert> : null}
      {loading ? <EmptyState title="Loading projects" /> : projects.length === 0 ? (
        <EmptyState title="No projects" description="Register a Git repository before starting agents." />
      ) : (
        <Table>
          <THead><TR><TH className="w-[18%]">Name</TH><TH>Root</TH><TH className="w-[12%]">Checkout</TH><TH className="w-[12%]">Isolation</TH><TH className="w-[8%]">Action</TH></TR></THead>
          <TBody>
            {projects.map((project) => (
              <TR key={project.name}>
                <TD className="truncate font-medium"><span title={project.name}>{project.name}</span></TD>
                <TD label="Git root">
                  <div className="space-y-1">
                    <ExpandableValue value={project.root} label={`Git root for ${project.name}`} max={40} />
                    <Disclosure summary="Repository details">
                      <ProjectRepositoryDetails project={project} />
                    </Disclosure>
                  </div>
                </TD>
                <TD label="Checkout" className="break-words">{checkoutText(project.checkout)}</TD>
                <TD label="Isolation">{project.worktree === false ? 'direct' : 'worktree'}</TD>
                <TD label="Actions">
                  <div className="flex flex-wrap gap-1">
                    <Button size="xs" variant="ghost" onClick={() => { setForm(project); setOpen(true); }}>Edit</Button>
                    <ActionButton
                      size="xs"
                      variant="ghost"
                      className="text-danger"
                      disabledReason={project.used_by?.length ? `Used by runs: ${project.used_by.join(', ')}` : undefined}
                      onClick={() => { setDeleteProject(project); setDeleteError(''); }}
                    >
                      Delete
                    </ActionButton>
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}

      <Modal
        open={open}
        onOpenChange={setOpen}
        title={projects.some((project) => project.name === form.name) ? `Edit ${form.name}` : 'Create project'}
        footer={<><Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button><Button onClick={() => void save()}>Save</Button></>}
      >
        <div className="grid gap-3">
          <Field label="Name" htmlFor="project-name" required><Input id="project-name" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} /></Field>
          <Field label="Git root" htmlFor="project-root" required><Input id="project-root" value={form.root} onChange={(event) => setForm({ ...form, root: event.target.value })} /></Field>
          <label className="flex items-center gap-2 text-sm text-muted">
            <input type="checkbox" className="size-3.5 accent-accent" checked={form.worktree !== false} onChange={(event) => setForm({ ...form, worktree: event.target.checked })} />
            Use isolated Git worktrees
          </label>
        </div>
      </Modal>

      <ConfirmDialog
        open={Boolean(deleteProject)}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteProject(null);
            setDeleteError('');
          }
        }}
        title={deleteProject ? `Delete ${deleteProject.name}` : 'Delete project'}
        confirmLabel="Delete registration"
        busyLabel="Deleting…"
        busy={deleteBusy}
        onConfirm={() => void remove()}
      >
        {deleteError ? <Alert variant="danger" title="Project deletion failed" className="mb-3">{deleteError}</Alert> : null}
        <p className="text-sm text-muted">
          Remove only the AgentBox registration for {deleteProject?.name}. The Git checkout at {deleteProject?.root} and existing run records are kept.
        </p>
      </ConfirmDialog>
    </div>
  );
}
