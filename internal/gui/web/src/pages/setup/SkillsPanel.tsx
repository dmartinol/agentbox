import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { AvailableSkills, SkillsEntry } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { EmptyState } from '../../components/ui/empty';
import { Field } from '../../components/ui/field';
import { Input, Select } from '../../components/ui/input';
import { Modal } from '../../components/ui/dialog';
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/table';

const agents = ['claude', 'codex', 'opencode'];
const emptyAvailable: AvailableSkills = { agent: 'claude', skills: [], plugins: [] };

export function SkillsPanel() {
  const [entries, setEntries] = useState<SkillsEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState({ name: '', agent: 'claude' });
  const [available, setAvailable] = useState<AvailableSkills>(emptyAvailable);
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]);
  const [selectedPlugins, setSelectedPlugins] = useState<string[]>([]);
  const [busy, setBusy] = useState<string>();

  const load = async () => {
    try {
      setEntries((await api.skills()) ?? []);
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
    setCreateForm({ name: '', agent: 'claude' });
    setSelectedSkills([]);
    setSelectedPlugins([]);
    setCreateOpen(true);
    await loadAvailable('claude');
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
      setNotice('Skills entry created');
      await load();
    } catch (err) {
      setError(err as Error);
    } finally {
      setBusy(undefined);
    }
  };

  const remove = async (name: string) => {
    setBusy(name);
    try {
      await api.deleteSkills(name);
      setNotice(`Skills entry ${name} deleted`);
      await load();
    } catch (err) {
      setError(err as Error);
    } finally {
      setBusy(undefined);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex justify-end"><Button size="sm" onClick={() => void openCreate()}>Create skills entry</Button></div>
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
                <TD className="truncate font-medium">{entry.name}</TD>
                <TD className="truncate">{entry.agent}</TD>
                <TD className="truncate text-muted">{entry.skills.join(', ') || '—'}</TD>
                <TD className="truncate text-muted">{entry.plugins.join(', ') || '—'}</TD>
                <TD>
                  <Button size="xs" variant="ghost" loading={busy === entry.name} onClick={() => void remove(entry.name)}>Delete</Button>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}
      <div className="rounded-lg border border-border bg-surface/40 p-2.5 text-sm text-muted">
        <div className="mb-1 font-medium text-foreground">How skills work</div>
        <div>Entries select host skills and whole plugin skill bundles for one agent. At start, selected SKILL.md folders are copied into the agent's user skills directory inside the sandbox. Plugins contribute only their skills: plugin scripts, apps, hooks, commands, and MCP servers are not delivered. Missing skills or plugins warn and the run still starts.</div>
      </div>

      <Modal open={createOpen} onOpenChange={setCreateOpen} title="Create skills entry" footer={<><Button variant="ghost" onClick={() => setCreateOpen(false)}>Cancel</Button><Button loading={busy === 'create'} disabled={!createForm.name.trim() || (selectedSkills.length === 0 && selectedPlugins.length === 0)} onClick={() => void create()}>Create</Button></>}>
        <div className="grid gap-3">
          <Field label="Name" htmlFor="skills-name" required><Input id="skills-name" value={createForm.name} onChange={(event) => setCreateForm({ ...createForm, name: event.target.value })} /></Field>
          <Field label="Agent" htmlFor="skills-agent" required helper="Host skills are discovered per agent.">
            <Select id="skills-agent" value={createForm.agent} onChange={(event) => void changeAgent(event.target.value)}>
              {agents.map((agent) => <option key={agent} value={agent}>{agent}</option>)}
            </Select>
          </Field>
          <Field label="Skills" htmlFor="skills-list" helper={available.skills.length === 0 ? `No standalone ${createForm.agent} skills found on this host.` : 'Standalone skill folders are copied into the run.'}>
            <div id="skills-list" className="max-h-44 space-y-1 overflow-y-auto rounded-lg border border-border p-2">
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
    </div>
  );
}
