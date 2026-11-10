import { useEffect, useMemo, useState } from 'react';
import { Bot, CircleSlash, Loader2, RefreshCw, Search, Settings, Terminal } from 'lucide-react';
import { api } from '../api';
import type { Overview, Profile, Run, StartRunRequest } from '../types';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { Card, CardBody, CardHeader, CardTitle } from '../components/ui/card';
import { EmptyState } from '../components/ui/empty';
import { Field } from '../components/ui/field';
import { Input, Select, Textarea } from '../components/ui/input';
import { Modal } from '../components/ui/dialog';
import { Table, TBody, TD, TH, THead, TR } from '../components/ui/table';
import { StatusBadge, isActiveRun } from '../components/StatusBadge';
import type { LogDrawerState } from '../components/LogDrawer';

const allProjects = 'All projects';

export function FleetView({
  onOpenSetup,
  openLog,
  updateLog
}: {
  onOpenSetup: () => void;
  openLog: (state: Omit<LogDrawerState, 'open'>) => void;
  updateLog: (state: Partial<LogDrawerState>) => void;
}) {
  const [overview, setOverview] = useState<Overview>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [actionError, setActionError] = useState('');
  const [notice, setNotice] = useState('');
  const [search, setSearch] = useState('');
  const [projectFilter, setProjectFilter] = useState(allProjects);
  const [selected, setSelected] = useState<Run>();
  const [startOpen, setStartOpen] = useState(false);
  const [startForm, setStartForm] = useState<StartRunRequest>({ project: '', profile: '', gateway: '', name: '', prompt: '' });
  const [useOverrides, setUseOverrides] = useState(false);
  const [overrides, setOverrides] = useState({ image: '', model: '', approval: 'prompted' });

  const load = async () => {
    try {
      const data = await api.overview();
      setOverview(data);
      setSelected((current) => current ? data.runs.find((agent) => agent.id === current.id) : undefined);
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  useEffect(() => {
    if (!overview) return;
    setStartForm((form) => ({
      ...form,
      project: form.project || overview.projects[0]?.name || '',
      profile: form.profile || overview.profiles[0]?.name || '',
      gateway: form.gateway || overview.gateways[0]?.name || 'openshell'
    }));
  }, [overview]);

  const agents = overview?.runs ?? [];
  const projects = useMemo(() => {
    const names = new Set([...agents.map((agent) => agent.project || 'Unassigned'), ...(overview?.projects ?? []).map((project) => project.name)]);
    return [allProjects, ...Array.from(names).sort()];
  }, [agents, overview?.projects]);

  const filtered = useMemo(() => agents.filter((agent) => {
    const needle = search.trim().toLowerCase();
    const matchesSearch = !needle || [agent.id, agent.name, agent.project, agent.agent, agent.branch, agent.sandbox].some((value) => value?.toLowerCase().includes(needle));
    const project = agent.project || 'Unassigned';
    return matchesSearch && (projectFilter === allProjects || project === projectFilter);
  }), [agents, search, projectFilter]);

  const grouped = useMemo(() => {
    const groups = new Map<string, Run[]>();
    for (const agent of filtered) {
      const project = agent.project || 'Unassigned';
      groups.set(project, [...groups.get(project) ?? [], agent]);
    }
    return Array.from(groups.entries()).sort(([left], [right]) => left.localeCompare(right));
  }, [filtered]);

  const selectedProfile: Profile | undefined = overview?.profiles.find((profile) => profile.name === startForm.profile);

  const action = async (callback: () => Promise<unknown>, message: string) => {
    setActionError('');
    setNotice('');
    try {
      await callback();
      setNotice(message);
      await load();
    } catch (err) {
      setActionError((err as Error).message);
    }
  };

  const openDetail = async (agent: Run, kind: 'logs' | 'diff' | 'policy') => {
    setSelected(agent);
    if (kind === 'logs') {
      openLog({ title: `Logs · ${agent.name || agent.id}`, content: '', loading: true });
      try {
        updateLog({ content: (await api.logs(agent.id)).logs, loading: false });
      } catch (err) {
        updateLog({ content: (err as Error).message, loading: false });
      }
      return;
    }
    if (kind === 'diff') {
      openLog({ title: `Diff · ${agent.name || agent.id}`, content: '', loading: true });
      try {
        updateLog({ content: (await api.diff(agent.id)).diff, loading: false });
      } catch (err) {
        updateLog({ content: (err as Error).message, loading: false });
      }
      return;
    }
    openLog({ title: `Policy · ${agent.name || agent.id}`, content: '', loading: true });
    try {
      updateLog({ content: JSON.stringify(await api.policy(agent.id), null, 2), loading: false });
    } catch (err) {
      updateLog({ content: (err as Error).message, loading: false });
    }
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-12 shrink-0 items-center gap-2 border-b border-zinc-800 bg-zinc-950/90 px-3">
        <div className="relative min-w-56 flex-1">
          <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-zinc-500" />
          <Input
            aria-label="Search agents"
            className="pl-7"
            placeholder="Search agents, projects, branches, or sandboxes"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </div>
        <Select aria-label="Filter by project" className="w-44" value={projectFilter} onChange={(event) => setProjectFilter(event.target.value)}>
          {projects.map((project) => <option key={project} value={project}>{project}</option>)}
        </Select>
        <Button variant="ghost" size="icon" aria-label="Refresh agents" onClick={() => void load()}>
          {loading ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
        </Button>
        <Button variant="outline" onClick={onOpenSetup}><Settings className="size-3.5" /> Setup</Button>
        <Button onClick={() => setStartOpen(true)}><Bot className="size-3.5" /> New agent</Button>
      </div>

      <div className="flex min-h-0 flex-1">
        <div className="agentbox-scrollbar min-w-0 flex-1 overflow-y-auto">
          {error ? (
            <EmptyState title="Unable to load fleet" description={error.message} />
          ) : loading && !overview ? (
            <EmptyState title="Loading fleet" description="Fetching projects, agents, and gateway status." />
          ) : agents.length === 0 ? (
            <EmptyState
              title="No agents yet"
              description="Create a project and profile in Setup, then start your first agent."
              action={<Button className="mt-2" onClick={() => setStartOpen(true)}>New agent</Button>}
            />
          ) : filtered.length === 0 ? (
            <EmptyState title="No matching agents" description="Adjust search or project filters." />
          ) : (
            grouped.map(([project, projectAgents]) => (
              <Card key={project} className="mx-3 my-3 overflow-hidden">
                <CardHeader>
                  <CardTitle className="truncate">{project}</CardTitle>
                  <div className="flex items-center gap-1.5">
                    <Badge tone="neutral">{projectAgents.length}</Badge>
                    <Badge tone={projectAgents.some((agent) => isActiveRun(agent.status)) ? 'success' : 'neutral'}>
                      {projectAgents.filter((agent) => isActiveRun(agent.status)).length} active
                    </Badge>
                  </div>
                </CardHeader>
                <Table aria-label={`Agents in ${project}`}>
                  <THead>
                    <TR>
                      <TH className="w-[22%]">Agent</TH>
                      <TH className="w-[11%]">Type</TH>
                      <TH className="w-[10%]">Status</TH>
                      <TH className="w-[10%]">Sync</TH>
                      <TH className="w-[22%]">Branch</TH>
                      <TH>Actions</TH>
                    </TR>
                  </THead>
                  <TBody>
                    {projectAgents.map((agent) => (
                      <TR
                        key={agent.id}
                        className={selected?.id === agent.id ? 'bg-indigo-500/10' : undefined}
                        onClick={() => setSelected(agent)}
                      >
                        <TD className="truncate font-medium" onClick={(event) => { event.stopPropagation(); setSelected(agent); }}>{agent.name || agent.id}</TD>
                        <TD>{agent.agent}</TD>
                        <TD><StatusBadge status={agent.status} /></TD>
                        <TD className={agent.synced ? 'text-emerald-300' : 'text-amber-300'}>{agent.synced ? 'Synced' : 'Unsynced'}</TD>
                        <TD className="truncate font-mono text-[11px] text-zinc-400">{agent.branch}</TD>
                        <TD>
                          <div className="flex flex-wrap gap-1">
                            <Button size="xs" variant="ghost" onClick={(event) => { event.stopPropagation(); void openDetail(agent, 'logs'); }}><Terminal className="size-3" /> Logs</Button>
                            <Button size="xs" variant="ghost" onClick={(event) => { event.stopPropagation(); void openDetail(agent, 'diff'); }}>Diff</Button>
                            <Button size="xs" variant="ghost" onClick={(event) => { event.stopPropagation(); void action(() => api.syncRun(agent.id), `Sync requested for ${agent.name || agent.id}`); }}>Sync</Button>
                            <Button size="xs" variant="ghost" disabled={!isActiveRun(agent.status)} onClick={(event) => { event.stopPropagation(); void action(() => api.stopRun(agent.id), `Stop requested for ${agent.name || agent.id}`); }}>Stop</Button>
                          </div>
                        </TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              </Card>
            ))
          )}
        </div>

        <aside className="hidden w-80 shrink-0 border-l border-zinc-800 bg-zinc-950/70 md:block">
          {selected ? (
            <div className="space-y-3 p-3">
              <div className="flex items-center justify-between gap-2">
                <div className="min-w-0">
                  <div className="truncate text-sm font-semibold">{selected.name || selected.id}</div>
                  <div className="text-xs text-zinc-500">{selected.project || 'Unassigned'} · {selected.agent}</div>
                </div>
                <StatusBadge status={selected.status} />
              </div>
              <dl className="space-y-1.5 rounded-lg border border-zinc-800 bg-zinc-900/50 p-2.5 text-xs">
                {[
                  ['ID', selected.id],
                  ['Agent type', selected.agent],
                  ['Branch', selected.branch],
                  ['Sandbox', selected.sandbox],
                  ['Image', selected.image],
                  ['Mode', selected.mode],
                  ['Approval', selected.approval],
                  ['Worktree', selected.worktree]
                ].map(([label, value]) => (
                  <div key={label} className="grid grid-cols-[5.5rem_1fr] gap-2">
                    <dt className="text-zinc-500">{label}</dt>
                    <dd className="min-w-0 truncate text-zinc-300">{value}</dd>
                  </div>
                ))}
              </dl>
              <div className="grid gap-1.5">
                <Button variant="outline" onClick={() => void openDetail(selected, 'logs')}>View logs</Button>
                <Button variant="outline" onClick={() => void openDetail(selected, 'diff')}>View diff</Button>
                <Button variant="outline" onClick={() => void openDetail(selected, 'policy')}>View policy</Button>
                <Button variant="danger" onClick={() => void action(async () => { await api.deleteRun(selected.id); setSelected(undefined); }, 'Agent deleted')}>Delete agent</Button>
              </div>
            </div>
          ) : (
            <EmptyState title="No agent selected" description="Select an agent to inspect metadata, logs, diff, and policy." />
          )}
        </aside>
      </div>

      {(notice || actionError) ? (
        <div className={`h-8 shrink-0 border-t px-3 text-xs leading-8 ${actionError ? 'border-red-500/30 bg-red-500/10 text-red-200' : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200'}`}>
          {actionError || notice}
        </div>
      ) : null}

      <Modal
        open={startOpen}
        onOpenChange={setStartOpen}
        title="New agent"
        description="Start an OpenShell-backed agent from a registered project and profile."
        footer={
          <>
            <Button variant="ghost" onClick={() => setStartOpen(false)}>Cancel</Button>
            <Button onClick={() => void action(async () => {
              await api.startRun({
                ...startForm,
                ...(useOverrides ? overrides : {}),
                name: startForm.name?.trim() || undefined,
                prompt: startForm.prompt?.trim() || undefined
              });
              setStartOpen(false);
            }, 'Agent started')}>Start</Button>
          </>
        }
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Project" htmlFor="new-agent-project" required>
            <Select id="new-agent-project" value={startForm.project} onChange={(event) => setStartForm({ ...startForm, project: event.target.value })}>
              {(overview?.projects ?? []).map((project) => <option key={project.name} value={project.name}>{project.name}</option>)}
            </Select>
          </Field>
          <Field label="Profile" htmlFor="new-agent-profile" required>
            <Select id="new-agent-profile" value={startForm.profile} onChange={(event) => setStartForm({ ...startForm, profile: event.target.value })}>
              {(overview?.profiles ?? []).map((profile) => <option key={profile.name} value={profile.name}>{profile.name}</option>)}
            </Select>
          </Field>
          <Field label="Gateway" htmlFor="new-agent-gateway" required>
            <Select id="new-agent-gateway" value={startForm.gateway} onChange={(event) => setStartForm({ ...startForm, gateway: event.target.value })}>
              {(overview?.gateways ?? []).map((gateway) => <option key={gateway.name} value={gateway.name}>{gateway.name}</option>)}
            </Select>
          </Field>
          <Field label="Agent name" htmlFor="new-agent-name" helper="Optional; unique within project.">
            <Input id="new-agent-name" value={startForm.name} onChange={(event) => setStartForm({ ...startForm, name: event.target.value })} />
          </Field>
          <Field label="Prompt" htmlFor="new-agent-prompt" className="sm:col-span-2">
            <Textarea id="new-agent-prompt" rows={4} value={startForm.prompt} onChange={(event) => setStartForm({ ...startForm, prompt: event.target.value })} />
          </Field>
          <div className="sm:col-span-2 rounded-lg border border-zinc-800 bg-zinc-900/40 p-2.5 text-xs text-zinc-400">
            {selectedProfile ? (
              <div className="space-y-1">
                <div>Image: <span className="text-zinc-200">{selectedProfile.image}</span></div>
                <div>Model: <span className="text-zinc-200">{selectedProfile.model || '-'}</span></div>
                <div>Provider: <span className="text-zinc-200">{selectedProfile.provider}</span></div>
                <div>Approval: <span className="text-zinc-200">{selectedProfile.approval}</span></div>
              </div>
            ) : 'Select a profile to see defaults.'}
          </div>
          <label className="flex items-center gap-2 text-xs text-zinc-400 sm:col-span-2">
            <input type="checkbox" checked={useOverrides} onChange={(event) => setUseOverrides(event.target.checked)} className="size-3.5 accent-indigo-500" />
            Override image, model, and approval
          </label>
          {useOverrides ? (
            <>
              <Field label="Image" htmlFor="new-agent-image">
                <Input id="new-agent-image" list="new-agent-images" value={overrides.image} onChange={(event) => setOverrides({ ...overrides, image: event.target.value })} />
              </Field>
              <datalist id="new-agent-images">{(overview?.images ?? []).map((image) => <option key={image.reference} value={image.reference} />)}</datalist>
              <Field label="Model" htmlFor="new-agent-model">
                <Input id="new-agent-model" value={overrides.model} onChange={(event) => setOverrides({ ...overrides, model: event.target.value })} />
              </Field>
              <Field label="Approval" htmlFor="new-agent-approval">
                <Select id="new-agent-approval" value={overrides.approval} onChange={(event) => setOverrides({ ...overrides, approval: event.target.value })}>
                  <option value="prompted">prompted</option>
                  <option value="unattended">unattended</option>
                </Select>
              </Field>
            </>
          ) : null}
        </div>
      </Modal>
    </div>
  );
}
