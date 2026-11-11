import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { PolicyConfiguration, Run } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
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
                <TD className="truncate font-medium">{policy.name}</TD>
                <TD className="truncate text-muted">{policy.used_by?.join(', ') || '—'}</TD>
                <TD className="truncate font-mono text-xs text-muted">{policy.path}</TD>
                <TD>
                  <div className="flex flex-wrap gap-1">
                    <Button size="xs" variant="ghost" onClick={() => { setHostForm({ name: policy.name, agent: 'codex', host: '' }); setHostOpen(true); }}>Add host</Button>
                    <Button size="xs" variant="ghost" onClick={() => { setApplyForm({ name: policy.name, runId: runs[0]?.id ?? '' }); setApplyOpen(true); }}>Apply</Button>
                    <Button size="xs" variant="ghost" onClick={async () => { const content = await api.policyContent(policy.name); setEditorName(policy.name); setEditorContent(content.content); setEditorOpen(true); }}>YAML</Button>
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
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
    </div>
  );
}
