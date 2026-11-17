import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { AvailableSkills, SkillStatus, SkillsEntry } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { EmptyState } from '../../components/ui/empty';
import { Field } from '../../components/ui/field';
import { Input, Select } from '../../components/ui/input';
import { Modal } from '../../components/ui/dialog';
import { ConfirmDialog } from '../../components/ui/confirm';
import { ExpandableValue } from '../../components/ui/copy-value';
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/table';

const agents = ['claude', 'codex', 'opencode'];
const emptyAvailable: AvailableSkills = { agent: 'claude', skills: [], plugins: [] };

export function SkillsPanel() {
  const [entries, setEntries] = useState<SkillsEntry[]>([]);
  const [status, setStatus] = useState<SkillStatus[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState({ name: '', agent: 'claude' });
  const [available, setAvailable] = useState<AvailableSkills>(emptyAvailable);
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]);
  const [selectedPlugins, setSelectedPlugins] = useState<string[]>([]);
  const [busy, setBusy] = useState<string>();
  const [editing, setEditing] = useState(false);
  const [formError, setFormError] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<SkillsEntry | null>(null);
  const [deleteError, setDeleteError] = useState('');
  const [deleteBusy, setDeleteBusy] = useState(false);

  const load = async () => {
    try {
      const [nextEntries, nextStatus] = await Promise.all([api.skills(), api.skillsStatus()]);
      setEntries(nextEntries ?? []);
      setStatus(nextStatus ?? []);
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  const loadAvailable = async (agent: string) => {
    try {
      setAvailable((await api.availableSkills(agent)) ?? { ...emptyAvailable, agent });
      setError(null);
    } catch (err) {
      setAvailable({ ...emptyAvailable, agent });
      setError(err as Error);
    }
  };

  const openCreate = async () => {
    setEditing(false);
    setCreateForm({ name: '', agent: 'claude' });
    setSelectedSkills([]);
    setSelectedPlugins([]);
    setCreateOpen(true);
    await loadAvailable('claude');
  };

  const openEdit = async (entry: SkillsEntry) => {
    setEditing(true);
    setCreateForm({ name: entry.name, agent: entry.agent });
    setSelectedSkills(entry.skills ?? []);
    setSelectedPlugins(entry.plugins ?? []);
    setAvailable({ ...emptyAvailable, agent: entry.agent });
    setFormError('');
    setCreateOpen(true);
    await loadAvailable(entry.agent);
  };

  const changeAgent = async (agent: string) => {
    setCreateForm({ ...createForm, agent });
    setSelectedSkills([]);
    setSelectedPlugins([]);
    await loadAvailable(agent);
  };

  const toggle = (value: string, selected: string[], setSelected: (next: string[]) => void) => {
    setSelected(selected.includes(value) ? selected.filter((item) => item !== value) : [...selected, value]);
  };

  const create = async () => {
    setBusy('create');
    try {
      await api.createSkills({
        version: 1,
        name: createForm.name.trim(),
        agent: createForm.agent,
        skills: selectedSkills,
        plugins: selectedPlugins
      });
      setCreateOpen(false);
      setNotice(`Skills entry ${editing ? 'saved' : 'created'}`);
      await load();
    } catch (err) {
      setFormError((err as Error).message);
    } finally {
      setBusy(undefined);
    }
  };

  const remove = async () => {
    if (!deleteTarget) return;
    setDeleteBusy(true);
    setDeleteError('');
    try {
      await api.deleteSkills(deleteTarget.name);
      setDeleteTarget(null);
      setNotice(`Skills entry ${deleteTarget.name} deleted`);
      await load();
    } catch (err) {
      setDeleteError((err as Error).message);
    } finally {
      setDeleteBusy(false);
    }
  };

  const roles = status.filter((entry) => entry.kind === 'role');

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap justify-end"><Button size="sm" onClick={() => void openCreate()}>Create skills entry</Button></div>
      {notice ? <Alert variant="success" title={notice} /> : null}
      {error ? <Alert variant="danger" title="Unable to load skills">{error.message}</Alert> : null}
      {loading ? <EmptyState title="Loading skills" /> : entries.length === 0 ? (
        <EmptyState title="No skills entries" description="Create an entry that selects host skills or plugin bundles for one agent, then pick it when starting a run." />
      ) : (
        <Table>
          <THead><TR><TH className="w-[22%]">Name</TH><TH className="w-[14%]">Agent</TH><TH>Skills</TH><TH>Plugins</TH><TH className="w-[14%]">Actions</TH></TR></THead>
          <TBody>
            {entries.map((entry) => (
              <TR key={entry.name}>
                <TD className="truncate font-medium"><span title={entry.name}>{entry.name}</span></TD>
                <TD label="Agent" className="truncate">{entry.agent}</TD>
                <TD label="Skills">
                  <ExpandableValue value={entry.skills.join(', ') || '—'} label={`skills in ${entry.name}`} max={44} mono={false} />
                </TD>
                <TD label="Plugins">
                  <ExpandableValue value={entry.plugins.join(', ') || '—'} label={`plugins in ${entry.name}`} max={44} mono={false} />
                </TD>
                <TD label="Actions">
                  <div className="flex flex-wrap gap-1">
                    <Button size="xs" variant="ghost" onClick={() => void openEdit(entry)}>Edit</Button>
                    <Button size="xs" variant="ghost" className="text-danger" loading={busy === entry.name} onClick={() => { setDeleteTarget(entry); setDeleteError(''); }}>Delete</Button>
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}
      <div className="space-y-2 rounded-lg border border-border bg-surface/40 p-2.5">
        <div>
          <div className="text-sm font-medium text-foreground">Agent roles</div>
          <div className="text-xs text-muted">Installed globally by <code>agentbox skills install</code> for the orchestrator stages. A harness scans these directories at session start, so restart the session after installing.</div>
        </div>
        {roles.length === 0 ? (
          <EmptyState title="No agent roles reported" description="Run agentbox skills install to write the orchestrator controller plus the triage, planner, implementer, fixer, verifier, reviewer, and retro stage roles." />
        ) : (
          <Table>
            <THead><TR><TH className="w-[28%]">Role</TH><TH className="w-[14%]">Harness</TH><TH className="w-[14%]">State</TH><TH>Path</TH></TR></THead>
            <TBody>
              {roles.flatMap((role) => role.installations.map((installation) => (
                <TR key={`${role.name}-${installation.target}`}>
                  <TD className="truncate font-medium"><span title={role.name}>{role.name}</span></TD>
                  <TD label="Harness" className="truncate">{installation.target}</TD>
                  <TD label="State">
                    <div className="min-w-0">
                      <span>{installation.state}</span>
                      {installation.detail ? (
                        <span className="mt-0.5 block text-xs text-muted">
                          <ExpandableValue
                            value={installation.detail}
                            label={`state detail for ${role.name} on ${installation.target}`}
                            max={44}
                            mono={false}
                          />
                        </span>
                      ) : null}
                    </div>
                  </TD>
                  <TD label="Path"><ExpandableValue value={installation.path} label={`path for ${role.name} on ${installation.target}`} max={40} /></TD>
                </TR>
              )))}
            </TBody>
          </Table>
        )}
      </div>
      <div className="rounded-lg border border-border bg-surface/40 p-2.5 text-sm text-muted">
        <div className="mb-1 font-medium text-foreground">How skills work</div>
        <div>Entries select host skills and whole plugin skill bundles for one agent. At start, selected SKILL.md folders are copied into the agent's user skills directory inside the sandbox. Plugins contribute only their skills: plugin scripts, apps, hooks, commands, and MCP servers are not delivered. Missing skills or plugins warn and the run still starts.</div>
      </div>

      <Modal open={createOpen} onOpenChange={setCreateOpen} title={editing ? `Edit ${createForm.name}` : 'Create skills entry'} footer={<><Button variant="ghost" onClick={() => setCreateOpen(false)}>Cancel</Button><Button loading={busy === 'create'} disabled={!createForm.name.trim() || (selectedSkills.length === 0 && selectedPlugins.length === 0)} onClick={() => void create()}>{editing ? 'Save' : 'Create'}</Button></>}>
        <div className="grid gap-3">
          {formError ? <div className="sm:col-span-2"><Alert variant="danger" title="Unable to save skills entry">{formError}</Alert></div> : null}
          <Field label="Name" htmlFor="skills-name" required><Input id="skills-name" readOnly={editing} value={createForm.name} onChange={(event) => setCreateForm({ ...createForm, name: event.target.value })} /></Field>
          <Field label="Agent" htmlFor="skills-agent" required helper="Host skills are discovered per agent.">
            <Select id="skills-agent" value={createForm.agent} onChange={(event) => void changeAgent(event.target.value)}>
              {agents.map((agent) => <option key={agent} value={agent}>{agent}</option>)}
            </Select>
          </Field>
          <Field label="Skills" htmlFor="skills-list" helper={available.skills.length === 0 ? `No standalone ${createForm.agent} skills found on this host.` : 'Standalone skill folders are copied into the run.'}>
            <div id="skills-list" className="max-h-44 space-y-1 overflow-y-auto rounded-lg border border-border p-2">
              {selectedSkills.filter((skill) => !available.skills.some((item) => item.name === skill)).length ? (
                <div className="text-xs text-warning">Unavailable saved skills are shown below and can be removed from the entry.</div>
              ) : null}
              {selectedSkills.filter((skill) => !available.skills.some((item) => item.name === skill)).map((skill) => (
                <label key={`saved-${skill}`} className="flex items-start gap-2 text-sm">
                  <input type="checkbox" checked onChange={() => toggle(skill, selectedSkills, setSelectedSkills)} className="mt-0.5 size-3.5 accent-accent" />
                  {skill} <span className="text-xs text-muted">(unavailable)</span>
                </label>
              ))}
              {available.skills.map((skill) => (
                <label key={skill.name} className="flex items-start gap-2 text-sm">
                  <input type="checkbox" aria-label={skill.name} checked={selectedSkills.includes(skill.name)} onChange={() => toggle(skill.name, selectedSkills, setSelectedSkills)} className="mt-0.5 size-3.5 accent-accent" />
                  <span>
                    <span className="font-medium">{skill.name}</span>
                    {skill.description ? <span className="block text-xs text-muted">{skill.description}</span> : null}
                  </span>
                </label>
              ))}
            </div>
          </Field>
          <Field label="Plugins" htmlFor="skills-plugins" helper={available.plugins.length === 0 ? `No ${createForm.agent} plugin skill bundles found on this host.` : 'A plugin is included or excluded as a whole.'}>
            <div id="skills-plugins" className="max-h-44 space-y-1 overflow-y-auto rounded-lg border border-border p-2">
              {selectedPlugins.filter((plugin) => !available.plugins.some((item) => item.id === plugin)).length ? (
                <div className="text-xs text-warning">Unavailable saved plugins are shown below and can be removed from the entry.</div>
              ) : null}
              {selectedPlugins.filter((plugin) => !available.plugins.some((item) => item.id === plugin)).map((plugin) => (
                <label key={`saved-${plugin}`} className="flex items-center gap-2 text-sm">
                  <input type="checkbox" checked onChange={() => toggle(plugin, selectedPlugins, setSelectedPlugins)} className="size-3.5 accent-accent" />
                  {plugin} <span className="text-xs text-muted">(unavailable)</span>
                </label>
              ))}
              {available.plugins.map((plugin) => (
                <label key={plugin.id} className="flex items-center gap-2 text-sm">
                  <input type="checkbox" aria-label={plugin.id} checked={selectedPlugins.includes(plugin.id)} onChange={() => toggle(plugin.id, selectedPlugins, setSelectedPlugins)} className="size-3.5 accent-accent" />
                  <span className="font-medium">{plugin.id}</span>
                  <span className="text-xs text-muted">{plugin.skills} bundled skill{plugin.skills === 1 ? '' : 's'}</span>
                </label>
              ))}
            </div>
          </Field>
        </div>
      </Modal>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
            setDeleteError('');
          }
        }}
        title={deleteTarget ? `Delete ${deleteTarget.name}` : 'Delete skills entry'}
        confirmLabel="Delete"
        busyLabel="Deleting"
        busy={deleteBusy}
        onConfirm={() => void remove()}
      >
        {deleteError ? <Alert variant="danger" title="Skills deletion failed" className="mb-3">{deleteError}</Alert> : null}
        <p className="text-sm text-muted">Remove only this AgentBox selection. Host skills, plugins, and project files are kept.</p>
      </ConfirmDialog>
    </div>
  );
}
