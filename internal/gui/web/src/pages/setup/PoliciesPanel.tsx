import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { PolicyConfiguration, Run } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { ActionButton } from '../../components/ui/action';
import { ConfirmDialog } from '../../components/ui/confirm';
import { ExpandableValue } from '../../components/ui/copy-value';
import { EmptyState } from '../../components/ui/empty';
import { Field } from '../../components/ui/field';
import { Input, Select, Textarea } from '../../components/ui/input';
import { Modal } from '../../components/ui/dialog';
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/table';

export function PoliciesPanel() {
  const [policies, setPolicies] = useState<PolicyConfiguration[]>([]);
  const [runs, setRuns] = useState<Run[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState('');
  const [hostOpen, setHostOpen] = useState(false);
  const [hostForm, setHostForm] = useState({ name: '', agent: 'codex', host: '' });
  const [applyOpen, setApplyOpen] = useState(false);
  const [applyForm, setApplyForm] = useState({ name: '', runId: '' });
  const [editorOpen, setEditorOpen] = useState(false);
  const [editorName, setEditorName] = useState('');
  const [editorContent, setEditorContent] = useState('');
  const [busy, setBusy] = useState<string>();
  const [runPolicyView, setRunPolicyView] = useState<{ title: string; path: string; content: string } | null>(null);
  const [runPolicyBusy, setRunPolicyBusy] = useState('');
  const [deletePolicy, setDeletePolicy] = useState<PolicyConfiguration | null>(null);
  const [deleteError, setDeleteError] = useState('');
  const [deleteBusy, setDeleteBusy] = useState(false);

  const load = async () => {
    try {
      const [nextPolicies, nextRuns] = await Promise.all([api.policies(), api.runs()]);
      setPolicies(nextPolicies ?? []);
      setRuns(nextRuns ?? []);
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

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

  const viewRunPolicy = async (run: Run) => {
    setRunPolicyBusy(run.id);
    try {
      const result = await api.runPolicyContent(run.id);
      setRunPolicyView({ title: run.name || run.id, path: result.path, content: result.content });
    } catch (err) {
      setError(err as Error);
    } finally {
      setRunPolicyBusy('');
    }
  };

  const remove = async () => {
    if (!deletePolicy) return;
    setDeleteBusy(true);
    setDeleteError('');
    try {
      await api.deletePolicy(deletePolicy.name);
      setDeletePolicy(null);
      setNotice(`Policy ${deletePolicy.name} deleted`);
      await load();
    } catch (err) {
      setDeleteError((err as Error).message);
    } finally {
      setDeleteBusy(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex justify-end"><Button size="sm" onClick={() => setCreateOpen(true)}>Create policy</Button></div>
      {notice ? <Alert variant="success" title={notice} /> : null}
      {error ? <Alert variant="danger" title="Unable to load policies">{error.message}</Alert> : null}
      {loading ? <EmptyState title="Loading policies" /> : policies.length === 0 ? (
        <EmptyState title="No reusable policies" description="Create a policy with safe defaults." />
      ) : (
        <Table>
          <THead><TR><TH className="w-[16%]">Name</TH><TH className="w-[24%]">Used by</TH><TH>Path</TH><TH className="w-[24%]">Actions</TH></TR></THead>
          <TBody>
            {policies.map((policy) => (
              <TR key={policy.name}>
                <TD className="truncate font-medium"><span title={policy.name}>{policy.name}</span></TD>
                <TD label="Used by" className="text-muted">
                  <ExpandableValue value={policy.used_by?.join(', ') || '—'} label={`profiles using ${policy.name}`} max={40} mono={false} />
                </TD>
                <TD label="Path"><ExpandableValue value={policy.path} label={`path for ${policy.name}`} max={40} /></TD>
                <TD label="Actions">
                  <div className="flex flex-wrap gap-1">
                    <Button size="xs" variant="ghost" onClick={() => { setHostForm({ name: policy.name, agent: 'codex', host: '' }); setHostOpen(true); }}>Add host</Button>
                    <Button size="xs" variant="ghost" onClick={() => { setApplyForm({ name: policy.name, runId: runs[0]?.id ?? '' }); setApplyOpen(true); }}>Apply</Button>
                    <Button size="xs" variant="ghost" onClick={async () => { const content = await api.policyContent(policy.name); setEditorName(policy.name); setEditorContent(content.content); setEditorOpen(true); }}>YAML</Button>
                    <ActionButton
                      size="xs"
                      variant="ghost"
                      disabledReason={
                        policy.used_by?.length
                          ? `Used by profiles: ${policy.used_by.join(', ')}`
                          : policy.default_for
                            ? `Generated default for profile ${policy.default_for}`
                            : undefined
                      }
                      onClick={() => { setDeletePolicy(policy); setDeleteError(''); }}
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

      {loading ? null : (
      <div className="space-y-2">
        <div className="flex flex-wrap items-baseline justify-between gap-1">
          <h3 className="text-sm font-semibold">Agent run policies</h3>
          <span className="text-xs text-muted">Each run copies a reusable policy at start; later edits stay on that run.</span>
        </div>
        {runs.length === 0 ? (
          <EmptyState title="No agent runs" description="A run's policy copy appears here after the agent starts." />
        ) : (
          <Table>
            <THead><TR><TH className="w-[28%]">Run</TH><TH className="w-[12%]">Agent</TH><TH>Policy</TH><TH className="w-[14%]">Actions</TH></TR></THead>
            <TBody>
              {runs.map((run) => (
                <TR key={run.id}>
                  <TD className="truncate font-medium"><span title={run.name || run.id}>{run.name || run.id}</span></TD>
                  <TD label="Agent" className="text-muted">{run.agent}</TD>
                  <TD label="Policy">
                    <div className="min-w-0 space-y-1">
                      <ExpandableValue value={run.policy} label={`policy for ${run.name || run.id}`} max={40} />
                      {run.pending_policy ? <div className="break-all text-xs text-warning">Pending for next sandbox: {run.pending_policy}</div> : null}
                    </div>
                  </TD>
                  <TD label="Actions">
                    <Button size="xs" variant="ghost" loading={runPolicyBusy === run.id} onClick={() => void viewRunPolicy(run)}>View</Button>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        )}
      </div>
      )}

      <Modal open={createOpen} onOpenChange={setCreateOpen} title="Create policy" footer={<><Button variant="ghost" onClick={() => setCreateOpen(false)}>Cancel</Button><Button loading={busy === 'create'} onClick={() => void action('create', async () => { await api.createPolicy(createName); setCreateOpen(false); }, 'Policy created')}>Create</Button></>}>
        <Field label="Name" htmlFor="policy-name" required><Input id="policy-name" value={createName} onChange={(event) => setCreateName(event.target.value)} /></Field>
      </Modal>

      <Modal open={hostOpen} onOpenChange={setHostOpen} title="Add network host" footer={<><Button variant="ghost" onClick={() => setHostOpen(false)}>Cancel</Button><Button loading={busy === 'host'} onClick={() => void action('host', async () => { await api.addPolicyHost(hostForm.name, hostForm.agent, hostForm.host); setHostOpen(false); }, 'Network rule added')}>Allow host</Button></>}>
        <div className="grid gap-3">
          <Field label="Policy" htmlFor="host-policy" required><Input id="host-policy" value={hostForm.name} onChange={(event) => setHostForm({ ...hostForm, name: event.target.value })} /></Field>
          <Field label="Agent" htmlFor="host-agent" required>
            <Select id="host-agent" value={hostForm.agent} onChange={(event) => setHostForm({ ...hostForm, agent: event.target.value })}>
              {['codex', 'claude', 'opencode'].map((agent) => <option key={agent} value={agent}>{agent}</option>)}
            </Select>
          </Field>
          <Field label="Host" htmlFor="host-host" required><Input id="host-host" value={hostForm.host} onChange={(event) => setHostForm({ ...hostForm, host: event.target.value })} /></Field>
        </div>
      </Modal>

      <Modal open={applyOpen} onOpenChange={setApplyOpen} title="Apply policy" footer={<><Button variant="ghost" onClick={() => setApplyOpen(false)}>Cancel</Button><Button loading={busy === 'apply'} onClick={() => void action('apply', async () => { await api.applyPolicy(applyForm.name, applyForm.runId); setApplyOpen(false); }, 'Policy apply requested')}>{busy === 'apply' ? 'Applying' : 'Apply'}</Button></>}>
        <div className="grid gap-3">
          <Field label="Policy" htmlFor="apply-policy" required><Input id="apply-policy" value={applyForm.name} onChange={(event) => setApplyForm({ ...applyForm, name: event.target.value })} /></Field>
          <Field label="Run" htmlFor="apply-run" required>
            <Select id="apply-run" value={applyForm.runId} onChange={(event) => setApplyForm({ ...applyForm, runId: event.target.value })}>
              {runs.map((run) => <option key={run.id} value={run.id}>{run.name || run.id}</option>)}
            </Select>
          </Field>
        </div>
      </Modal>

      <Modal open={editorOpen} onOpenChange={setEditorOpen} title={`Edit ${editorName}`} className="w-[min(94vw,56rem)]" footer={<><Button variant="ghost" onClick={() => setEditorOpen(false)}>Cancel</Button><Button loading={busy === 'save'} onClick={() => void action('save', async () => { await api.savePolicyContent(editorName, editorContent); setEditorOpen(false); }, 'Policy saved')}>Save</Button></>}>
        <Textarea aria-label="Policy YAML" rows={20} value={editorContent} onChange={(event) => setEditorContent(event.target.value)} />
      </Modal>

      <Modal
        open={Boolean(runPolicyView)}
        onOpenChange={(open) => { if (!open) setRunPolicyView(null); }}
        title={runPolicyView ? `Run policy · ${runPolicyView.title}` : 'Run policy'}
        className="w-[min(94vw,56rem)]"
        footer={<Button onClick={() => setRunPolicyView(null)}>Close</Button>}
      >
        {runPolicyView ? (
          <div className="space-y-2">
            <div className="break-all font-mono text-xs text-muted">{runPolicyView.path}</div>
            <pre className="agentbox-output max-h-[60vh] overflow-auto rounded-lg border border-border bg-surface/50 p-2 font-mono text-xs leading-4">{runPolicyView.content}</pre>
          </div>
        ) : null}
      </Modal>

      <ConfirmDialog
        open={Boolean(deletePolicy)}
        onOpenChange={(open) => {
          if (!open) {
            setDeletePolicy(null);
            setDeleteError('');
          }
        }}
        title={deletePolicy ? `Delete ${deletePolicy.name}` : 'Delete policy'}
        confirmLabel="Delete policy"
        busyLabel="Deleting…"
        busy={deleteBusy}
        onConfirm={() => void remove()}
      >
        {deleteError ? <Alert variant="danger" title="Policy deletion failed" className="mb-3">{deleteError}</Alert> : null}
        <p className="text-sm text-muted">Remove only the policy YAML. Profiles that reference another policy are kept.</p>
      </ConfirmDialog>
    </div>
  );
}
