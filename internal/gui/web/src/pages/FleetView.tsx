import { useEffect, useMemo, useRef, useState } from 'react';
import { Bot, ChevronRight, CircleHelp, CircleSlash, Loader2, Monitor, RefreshCw, Search, Terminal, X } from 'lucide-react';
import { api } from '../api';
import { approvalForProfile, approvalLabel, approvalOptions, defaultApproval } from '../approvals';
import type { MCPTools, Overview, PolicyHostError, PolicyHostResponse, Profile, Run, SkillsEntry, StartRunRequest } from '../types';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { Card, CardBody, CardHeader, CardTitle } from '../components/ui/card';
import { Alert } from '../components/ui/alert';
import { EmptyState } from '../components/ui/empty';
import { Field } from '../components/ui/field';
import { Input, Select, Textarea } from '../components/ui/input';
import { Modal } from '../components/ui/dialog';
import { ConfirmDialog } from '../components/ui/confirm';
import { Table, TBody, TD, TH, THead, TR } from '../components/ui/table';
import { AgentWorkspace } from '../components/AgentWorkspace';
import { Disclosure } from '../components/ui/disclosure';
import { ExpandableValue } from '../components/ui/copy-value';
import { cn } from '../lib/utils';
import { formatSyncTimestamp } from '../lib/text';
import { StatusBadge, SyncStateBadge, canAttachRun, isActiveRun, canStopRun, isWaitingRun, statusLabel, syncStateExplanation, syncStateLabel } from '../components/StatusBadge';
import type { LogDrawerState } from '../components/LogDrawer';

const allProjects = 'All projects';

// Fleet stays live on its own: a run that is still starting must not sit on
// Creating until someone reloads the page. Poll fast while something is
// changing or while a terminal is on screen, a little slower while an idle
// agent could start a turn, and stay cheap once every run is terminal.
function fleetPollDelay(runs: Run[], watching: boolean): number {
  if (runs.some((run) => run.status === 'creating' || run.status === 'working' || run.status === 'waiting')) return 1000;
  if (runs.some((run) => run.status === 'ready')) return watching ? 1500 : 2500;
  return 15000;
}

type AgentAction = 'sync' | 'stop' | 'delete';

export function FleetView({
  refreshKey = 0,
  openLog,
  updateLog
}: {
  refreshKey?: number;
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
  const [workspace, setWorkspace] = useState<{ project: string; runId: string }>();
  const [focusedRunId, setFocusedRunId] = useState<string>();
  const [unread, setUnread] = useState<Set<string>>(new Set());
  const previousStatuses = useRef<Map<string, string>>(new Map());
  const [startOpen, setStartOpen] = useState(false);
  const [startError, setStartError] = useState('');
  const [startSubmitting, setStartSubmitting] = useState(false);
  const [startForm, setStartForm] = useState<StartRunRequest>({ project: '', profile: '', gateway: '', name: '', prompt: '', mcp: '', skills: '' });
  const [useOverrides, setUseOverrides] = useState(false);
  const [overrides, setOverrides] = useState({ image: '', model: '', approval: defaultApproval });
  const approvalExplicit = useRef(false);
  const [mcpEntries, setMcpEntries] = useState<MCPTools[]>([]);
  const [skillsEntries, setSkillsEntries] = useState<SkillsEntry[]>([]);
  const [startWarnings, setStartWarnings] = useState<string[]>([]);
  const [imageWarnings, setImageWarnings] = useState<string[]>([]);
  const [skillWarnings, setSkillWarnings] = useState<string[]>([]);
  const [pending, setPending] = useState<Record<string, AgentAction>>({});
  const [deleteTarget, setDeleteTarget] = useState<Run>();
  const [copySource, setCopySource] = useState<Run>();
  const [hostTarget, setHostTarget] = useState<Run>();
  const [hostValue, setHostValue] = useState('');
  const [hostError, setHostError] = useState('');
  const [hostResult, setHostResult] = useState<PolicyHostResponse>();
  const [hostRolledBack, setHostRolledBack] = useState<boolean>();
  const [hostSubmitting, setHostSubmitting] = useState(false);

  const load = async () => {
    try {
      const [data, mcps, skills] = await Promise.all([api.overview(), api.mcps(), api.skills()]);
      setOverview(data);
      setMcpEntries(mcps ?? []);
      setSkillsEntries(skills ?? []);
      setSelected((current) => current ? data.runs.find((agent) => agent.id === current.id) : undefined);
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, [refreshKey]);

  useEffect(() => {
    if (!overview) return;
    setStartForm((form) => ({
      ...form,
      project: overview.projects.some((project) => project.name === form.project) ? form.project : overview.projects[0]?.name || '',
      profile: overview.profiles.some((profile) => profile.name === form.profile) ? form.profile : overview.profiles[0]?.name || '',
      gateway: overview.gateways.some((gateway) => gateway.name === form.gateway) ? form.gateway : overview.gateways[0]?.name || 'openshell'
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

  const needsYouCount = useMemo(() => agents.filter((agent) => isWaitingRun(agent.status)).length, [agents]);

  const grouped = useMemo(() => {
    const groups = new Map<string, Run[]>();
    for (const agent of filtered) {
      const project = agent.project || 'Unassigned';
      groups.set(project, [...groups.get(project) ?? [], agent]);
    }
    return Array.from(groups.entries()).sort(([left], [right]) => left.localeCompare(right));
  }, [filtered]);

  const selectedProfile: Profile | undefined = overview?.profiles.find((profile) => profile.name === startForm.profile);
  const canStart = Boolean(overview && startForm.project && startForm.gateway);

  useEffect(() => {
    if (!startOpen || approvalExplicit.current) return;
    setOverrides((current) => ({ ...current, approval: approvalForProfile(selectedProfile) }));
  }, [startOpen, selectedProfile?.approval]);

  const openWorkspace = (agent: Run) => {
    setFocusedRunId(agent.id);
    setUnread((current) => {
      const next = new Set(current);
      next.delete(agent.id);
      return next;
    });
    setWorkspace({ project: agent.project || 'Unassigned', runId: agent.id });
    if (typeof Notification !== 'undefined' && Notification.permission === 'default') {
      void Notification.requestPermission();
    }
  };

  const closeWorkspace = () => {
    setWorkspace(undefined);
    setFocusedRunId(undefined);
  };

  useEffect(() => {
    const previous = previousStatuses.current;
    const next = new Map<string, string>();
    for (const agent of agents) {
      const before = previous.get(agent.id);
      next.set(agent.id, agent.status);
      if (!before || before === agent.status) continue;
      const needsYou = agent.status === 'waiting' || agent.status === 'completed';
      if (!needsYou || agent.id === focusedRunId) continue;
      setUnread((current) => new Set(current).add(agent.id));
      if (typeof Notification !== 'undefined' && Notification.permission === 'granted') {
        new Notification(`${agent.name || agent.id} ${statusLabel(agent.status).toLowerCase()}`, {
          body: `${agent.project || 'Unassigned'} · ${agent.agent}`
        });
      }
    }
    previousStatuses.current = next;
  }, [agents, focusedRunId]);

  const pollDelay = fleetPollDelay(agents, Boolean(workspace));

  useEffect(() => {
    let cancelled = false;
    let timer = 0;
    let inFlight = false;

    const refresh = async () => {
      if (cancelled || inFlight || document.visibilityState === 'hidden') return;
      inFlight = true;
      try {
        const next = await api.runs();
        if (cancelled) return;
        setOverview((current) => (current ? { ...current, runs: next } : current));
        setSelected((current) => (current ? next.find((run) => run.id === current.id) : current));
      } catch {
        // Keep the last known fleet; the next tick retries.
      } finally {
        inFlight = false;
      }
    };

    const tick = async () => {
      await refresh();
      if (!cancelled) schedule();
    };

    const schedule = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => void tick(), pollDelay);
    };

    // Returning to the tab refreshes immediately instead of waiting out a
    // long terminal-state delay.
    const onVisibilityChange = () => {
      if (cancelled || document.visibilityState === 'hidden') return;
      window.clearTimeout(timer);
      void tick();
    };

    schedule();
    document.addEventListener('visibilitychange', onVisibilityChange);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVisibilityChange);
    };
  }, [pollDelay]);

  const inFlightStatus = (agent: Run): string => {
    const kind = pending[agent.id];
    if (kind === 'stop') return 'stopping';
    if (kind === 'delete') return 'deleting';
    return agent.status;
  };

  const runAgentAction = async (agent: Run, kind: AgentAction, callback: () => Promise<unknown>, message: string) => {
    setActionError('');
    setNotice('');
    setPending((current) => ({ ...current, [agent.id]: kind }));
    try {
      await callback();
      setNotice(message);
      await load();
    } catch (err) {
      setActionError((err as Error).message);
    } finally {
      setPending((current) => {
        const next = { ...current };
        delete next[agent.id];
        return next;
      });
    }
  };

  const openStart = () => {
    const nextProfile = startForm.profile || overview?.profiles[0]?.name || '';
    setStartError('');
    setStartWarnings([]);
    setImageWarnings([]);
    setSkillWarnings([]);
    setCopySource(undefined);
    setStartForm((form) => ({
      ...form,
      project: form.project || overview?.projects[0]?.name || '',
      profile: form.profile || overview?.profiles[0]?.name || '',
      gateway: form.gateway || overview?.gateways[0]?.name || 'openshell',
      name: '',
      prompt: '',
      mcp: '',
      skills: ''
    }));
    setUseOverrides(false);
    setOverrides({ image: '', model: '', approval: approvalForProfile(overview?.profiles.find((profile) => profile.name === nextProfile)) });
    approvalExplicit.current = false;
    // Open first so the click always responds. The catalog refresh can settle
    // behind the dialog and fill in project/profile/MCP/skills options.
    setStartOpen(true);
    void load();
  };

  const copyAgent = (run: Run) => {
    const profiles = overview?.profiles ?? [];
    const profile = profiles.find((item) => item.image === run.image) ?? profiles.find((item) => item.agent === run.agent);
    setStartError('');
    setCopySource(run);
    setStartForm({
      project: run.project || overview?.projects[0]?.name || '',
      profile: profile?.name || profiles[0]?.name || '',
      gateway: run.gateway || overview?.gateways[0]?.name || 'openshell',
      name: '',
      prompt: '',
      mcp: run.mcp || '',
      skills: run.skills || ''
    });
    setStartWarnings([]);
    setImageWarnings([]);
    setSkillWarnings([]);
    setUseOverrides(true);
    setOverrides({ image: run.image, model: run.model, approval: run.approval || approvalForProfile(profile) });
    approvalExplicit.current = true;
    setStartOpen(true);
  };

  const closeStart = () => {
    setStartOpen(false);
    setCopySource(undefined);
  };

  const startAgent = async () => {
    setStartError('');
    setNotice('');
    setStartSubmitting(true);
    try {
      const created = await api.startRun({
        ...startForm,
        ...(useOverrides ? overrides : {}),
        name: startForm.name?.trim() || undefined,
        prompt: startForm.prompt?.trim() || undefined,
        mcp: startForm.mcp?.trim() || undefined,
        skills: startForm.skills?.trim() || undefined
      });
      closeStart();
      setSelected(created);
      setImageWarnings(created.image_warnings ?? []);
      setStartWarnings(created.mcp_warnings ?? []);
      setSkillWarnings(created.skills_warnings ?? []);
      setNotice(`Agent ${created.name || created.id} started`);
      await load();
    } catch (err) {
      setStartError((err as Error).message);
    } finally {
      setStartSubmitting(false);
    }
  };

  const confirmDelete = () => {
    const target = deleteTarget;
    if (!target) return;
    setDeleteTarget(undefined);
    void runAgentAction(target, 'delete', async () => {
      await api.deleteRun(target.id);
      setSelected((current) => (current?.id === target.id ? undefined : current));
    }, 'Agent deleted');
  };


  const openDetail = async (agent: Run, kind: 'logs' | 'diff' | 'policy') => {
    // Below md the details sidebar becomes a bottom sheet. Opening the output
    // drawer on top of it stacks two overlays, so the drawer takes over.
    const narrow = typeof window.matchMedia === 'function' && window.matchMedia('(max-width: 767.98px)').matches;
    setSelected(narrow ? undefined : agent);
    if (kind === 'logs') {
      openLog({ title: `Logs · ${agent.name || agent.id}`, content: '', loading: true });
      try {
        updateLog({ content: (await api.logs(agent.id)).logs, loading: false, error: undefined });
      } catch (err) {
        updateLog({ content: '', loading: false, error: (err as Error).message });
      }
      return;
    }
    if (kind === 'diff') {
      openLog({ title: `Diff · ${agent.name || agent.id}`, content: '', loading: true });
      try {
        updateLog({ content: (await api.diff(agent.id)).diff, loading: false, error: undefined });
      } catch (err) {
        updateLog({ content: '', loading: false, error: (err as Error).message });
      }
      return;
    }
    openLog({ title: `Policy · ${agent.name || agent.id}`, content: '', loading: true });
    try {
      updateLog({ content: JSON.stringify(await api.policy(agent.id), null, 2), loading: false, error: undefined });
    } catch (err) {
      updateLog({ content: '', loading: false, error: (err as Error).message });
    }
  };

  const resetHostDialog = () => {
    setHostValue('');
    setHostError('');
    setHostResult(undefined);
    setHostRolledBack(undefined);
  };

  const closeHostDialog = () => {
    setHostTarget(undefined);
    resetHostDialog();
  };

  const openHostDialog = async (agent: Run) => {
    setSelected(agent);
    setHostTarget(agent);
    resetHostDialog();
    try {
      setHostTarget(await api.run(agent.id));
    } catch (err) {
      setHostError((err as Error).message);
    }
  };

  const submitHost = async () => {
    const target = hostTarget;
    const host = hostValue.trim();
    if (!target || !host) return;
    setHostSubmitting(true);
    setHostError('');
    setHostResult(undefined);
    setHostRolledBack(undefined);
    try {
      setHostResult(await api.allowHost(target.id, host));
      void load();
    } catch (err) {
      const failure = err as PolicyHostError;
      setHostError(failure.message);
      if (failure.outcome === 'openshell-rejected') setHostRolledBack(Boolean(failure.rolledBack));
    } finally {
      setHostSubmitting(false);
    }
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      {workspace ? (
        <AgentWorkspace
          project={workspace.project}
          runs={agents.filter((agent) => (agent.project || 'Unassigned') === workspace.project)}
          initialRunId={workspace.runId}
          unread={unread}
          onFocusRun={(id) => {
            setFocusedRunId(id);
            setUnread((current) => {
              if (!current.has(id)) return current;
              const next = new Set(current);
              next.delete(id);
              return next;
            });
          }}
          onClose={closeWorkspace}
        />
      ) : (
      <>
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border bg-background/90 px-3 py-2">
        <div className="relative min-w-0 flex-1 basis-full sm:basis-56">
          <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
          <Input
            aria-label="Search agents"
            className="pl-7"
            placeholder="Search agents, projects, branches, or sandboxes"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </div>
        <Select aria-label="Filter by project" className="min-w-0 flex-1 sm:w-44 sm:flex-none" value={projectFilter} onChange={(event) => setProjectFilter(event.target.value)}>
          {projects.map((project) => <option key={project} value={project}>{project}</option>)}
        </Select>
        {needsYouCount > 0 ? (
          <Badge tone="warning" aria-label={`${needsYouCount} agents waiting for input`}>
            <CircleHelp className="size-3" /> Needs You {needsYouCount}
          </Badge>
        ) : null}
        <div className="ml-auto flex items-center gap-2">
          <Button variant="ghost" size="icon" aria-label="Refresh agents" onClick={() => void load()}>
            {loading ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
          </Button>
          <Button onClick={openStart}><Bot className="size-3.5" /> New agent</Button>
        </div>
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
              action={<Button className="mt-2" onClick={openStart}>New agent</Button>}
            />
          ) : filtered.length === 0 ? (
            <EmptyState title="No matching agents" description="Adjust search or project filters." />
          ) : (
            grouped.map(([project, projectAgents]) => (
              <Card key={project} className="mx-3 my-3 overflow-hidden">
                <CardHeader className="h-auto flex-wrap py-2">
                  <CardTitle className="min-w-0 [overflow-wrap:anywhere]" title={project}>{project}</CardTitle>
                  <div className="flex flex-wrap items-center gap-1.5">
                    <Badge tone="neutral">{projectAgents.length}</Badge>
                    <Badge tone={projectAgents.some((agent) => isActiveRun(agent.status)) ? 'success' : 'neutral'}>
                      {projectAgents.filter((agent) => isActiveRun(agent.status)).length} active
                    </Badge>
                    <Button size="xs" variant="ghost" onClick={() => openWorkspace(projectAgents[0])}>
                      <Monitor className="size-3" /> Workspace
                    </Button>
                  </div>
                </CardHeader>
                <Table aria-label={`Agents in ${project}`} className="agentbox-fleet-table">
                  <THead>
                    <TR>
                      <TH className="w-[20%]">Agent</TH>
                      <TH className="w-[7%]">Type</TH>
                      <TH className="w-[11%]">Status</TH>
                      <TH className="w-[13%]">Sync</TH>
                      <TH className="w-[28%]">Branch</TH>
                      <TH className="w-[21%]">Actions</TH>
                    </TR>
                  </THead>
                  <TBody>
                    {projectAgents.map((agent) => {
                      const busy = pending[agent.id];
                      const attachable = canAttachRun(agent.status);
                      const label = agent.name || agent.id;
                      const status = inFlightStatus(agent);
                      const syncBusy = busy === 'sync';
                      const syncLabel = syncBusy ? 'Syncing…' : syncStateLabel(agent.sync_state);
                      return (
                        <TR
                          key={agent.id}
                          className={selected?.id === agent.id ? 'bg-accent/10' : undefined}
                          onClick={() => setSelected(agent)}
                        >
                          <TD className="truncate font-medium" onClick={(event) => { event.stopPropagation(); setSelected(agent); }}>
                            <span title={label}>{label}</span>
                          </TD>
                          <TD label="Type" aria-label={`Type: ${agent.agent}`}>{agent.agent}</TD>
                          <TD label="Status" aria-label={`Status: ${statusLabel(status)}`}><StatusBadge status={status} /></TD>
                          <TD
                            label="Sync"
                            aria-label={`Sync: ${syncLabel}`}
                          >
                            {syncBusy ? (
                              <Badge tone="info"><Loader2 className="size-3 animate-spin" />Syncing…</Badge>
                            ) : (
                              <SyncStateBadge state={agent.sync_state} />
                            )}
                          </TD>
                          <TD label="Branch">
                            <ExpandableValue value={agent.branch} label={`branch for ${label}`} max={28} />
                          </TD>
                          <TD label="Actions">
                            <div className="flex flex-wrap items-center gap-1">
                              <Button
                                size="xs"
                                variant="secondary"
                                disabled={!attachable}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  if (attachable) openWorkspace(agent);
                                  else void openDetail(agent, 'logs');
                                }}
                              >
                                {attachable ? <><Monitor className="size-3" /> Attach</> : <><Terminal className="size-3" /> Logs</>}
                              </Button>
                              {attachable ? (
                                <Button size="xs" variant="ghost" onClick={(event) => { event.stopPropagation(); void openDetail(agent, 'logs'); }}>
                                  <Terminal className="size-3" /> Logs
                                </Button>
                              ) : null}
                              <Button
                                size="xs"
                                variant="ghost"
                                className="ml-auto sm:ml-0"
                                aria-label={`Details for ${label}`}
                                onClick={(event) => { event.stopPropagation(); setSelected(agent); }}
                              >
                                Details <ChevronRight className="size-3" aria-hidden />
                              </Button>
                            </div>
                          </TD>
                        </TR>
                      );
                    })}
                  </TBody>
                </Table>
              </Card>
            ))
          )}
        </div>

        <aside
          aria-label="Agent details"
          className={cn(
            'border-border bg-background/95',
            selected
              ? 'fixed inset-x-0 bottom-0 z-30 max-h-[72vh] overflow-y-auto rounded-t-xl border-t shadow-2xl agentbox-scrollbar md:static md:z-auto md:max-h-none md:w-96 md:shrink-0 md:rounded-none md:border-l md:border-t-0 md:bg-background/70 md:shadow-none'
              : 'hidden md:block md:w-96 md:shrink-0 md:border-l'
          )}
        >
          {selected ? (
            <div className="space-y-3 p-3">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <div className="text-sm font-semibold [overflow-wrap:anywhere]">{selected.name || selected.id}</div>
                  <div className="text-sm text-muted">{selected.project || 'Unassigned'} · {selected.agent}</div>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  <StatusBadge status={inFlightStatus(selected)} />
                  <Button size="icon" variant="ghost" className="md:hidden" aria-label="Close agent details" onClick={() => setSelected(undefined)}>
                    <X className="size-4" />
                  </Button>
                </div>
              </div>

              <dl className="space-y-1.5 rounded-lg border border-border bg-surface/50 p-2.5 text-sm">
                {([
                  ['ID', selected.id, 18],
                  ['Agent type', selected.agent, 18],
                  ['Branch', selected.branch, 18],
                  ['Sandbox', selected.sandbox, 18],
                  ['Image', selected.image, 18],
                  ['Mode', selected.mode, 18],
                  ['Approval', approvalLabel(selected.approval), 18],
                  ['Worktree', selected.worktree, 18],
                  ...(selected.status_detail ? [['Detail', selected.status_detail, 40] as const] : [])
                ] as const).map(([label, value, max]) => (
                  <div key={label} className="grid grid-cols-[5.5rem_minmax(0,1fr)] items-start gap-2">
                    <dt className="text-muted">{label}</dt>
                    <dd className="min-w-0 text-foreground">
                      <ExpandableValue value={value} label={`${label} for ${selected.name || selected.id}`} max={max} />
                    </dd>
                  </div>
                ))}
              </dl>

              <section aria-label="Sync" className="space-y-1.5 rounded-lg border border-border p-2">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted">Sync</h3>
                <dl className="grid grid-cols-[5.5rem_minmax(0,1fr)] items-center gap-x-2 gap-y-1 text-sm">
                  <dt className="text-muted">Sync state</dt>
                  <dd><SyncStateBadge state={selected.sync_state} /></dd>
                  <dt className="text-muted">Last sync</dt>
                  <dd className="min-w-0 text-foreground">{formatSyncTimestamp(selected.last_successful_sync_at)}</dd>
                </dl>
                {syncStateExplanation(selected.sync_state) ? (
                  <p className="text-xs text-muted">{syncStateExplanation(selected.sync_state)}</p>
                ) : null}
              </section>

              <section aria-label="Open agent" className="space-y-1.5 rounded-lg border border-border p-2">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted">Open</h3>
                <div className="grid grid-cols-2 gap-1.5">
                  <Button
                    variant="outline"
                    disabled={!canAttachRun(selected.status)}
                    onClick={() => openWorkspace(selected)}
                  >
                    Attach
                  </Button>
                  <Button variant="outline" onClick={() => void openDetail(selected, 'logs')}>View logs</Button>
                  <Button variant="outline" onClick={() => void openDetail(selected, 'diff')}>View diff</Button>
                  <Button variant="outline" onClick={() => void openDetail(selected, 'policy')}>View policy</Button>
                </div>
              </section>

              <section aria-label="Manage agent" className="space-y-1.5 rounded-lg border border-border p-2">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted">Manage</h3>
                <div className="grid gap-1.5">
                  <div className="grid grid-cols-2 gap-1.5">
                    <Button
                      variant="outline"
                      loading={pending[selected.id] === 'sync'}
                      disabled={Boolean(pending[selected.id])}
                      onClick={() => void runAgentAction(selected, 'sync', () => api.syncRun(selected.id), `Sync requested for ${selected.name || selected.id}`)}
                    >
                      {pending[selected.id] === 'sync' ? 'Syncing…' : 'Sync'}
                    </Button>
                    <Button
                      variant="outline"
                      loading={pending[selected.id] === 'stop'}
                      disabled={Boolean(pending[selected.id]) || !canStopRun(selected.status)}
                      onClick={() => void runAgentAction(selected, 'stop', () => api.stopRun(selected.id), `Stop requested for ${selected.name || selected.id}`)}
                    >
                      {pending[selected.id] === 'stop' ? 'Stopping' : 'Stop'}
                    </Button>
                  </div>
                  <Button
                    variant="outline"
                    loading={hostSubmitting && hostTarget?.id === selected.id}
                    disabled={!canAttachRun(selected.status) || Boolean(pending[selected.id])}
                    title={!canAttachRun(selected.status) ? 'Allow host is available only for Ready, Working, and Waiting agents.' : undefined}
                    onClick={() => void openHostDialog(selected)}
                  >
                    {hostSubmitting && hostTarget?.id === selected.id ? 'Allowing' : 'Allow host'}
                  </Button>
                  <Button variant="outline" onClick={() => copyAgent(selected)}>Copy agent</Button>
                </div>
              </section>

              <div className="border-t border-border pt-2">
                <Button
                  variant="danger"
                  className="w-full"
                  loading={pending[selected.id] === 'delete'}
                  disabled={Boolean(pending[selected.id])}
                  onClick={() => setDeleteTarget(selected)}
                >
                  {pending[selected.id] === 'delete' ? 'Deleting agent' : 'Delete agent'}
                </Button>
              </div>
            </div>
          ) : (
            <EmptyState title="No agent selected" description="Select an agent to inspect metadata, logs, diff, and policy." />
          )}
        </aside>
      </div>
      </>
      )}

      {(notice || actionError) ? (
        <div className={`h-8 shrink-0 border-t px-3 text-sm leading-8 ${actionError ? 'border-danger/30 bg-danger/10 text-danger' : 'border-success/30 bg-success/10 text-success'}`}>
          {actionError || notice}
        </div>
      ) : null}

      {imageWarnings.length > 0 ? (
        <Alert variant="warning" title="Image warnings" className="m-3">
          <ul className="list-disc space-y-1 pl-4">
            {imageWarnings.map((warning) => <li key={warning}>{warning}</li>)}
          </ul>
        </Alert>
      ) : null}
      {startWarnings.length > 0 ? (
        <Alert variant="warning" title="MCP tools warnings" className="m-3">
          <ul className="list-disc space-y-1 pl-4">
            {startWarnings.map((warning) => <li key={warning}>{warning}</li>)}
          </ul>
        </Alert>
      ) : null}
      {skillWarnings.length > 0 ? (
        <Alert variant="warning" title="Skills warnings" className="m-3">
          <ul className="list-disc space-y-1 pl-4">
            {skillWarnings.map((warning) => <li key={warning}>{warning}</li>)}
          </ul>
        </Alert>
      ) : null}

      <Modal
        open={startOpen}
        onOpenChange={(open) => {
          setStartOpen(open);
          if (!open) setCopySource(undefined);
        }}
        title={copySource ? `Copy ${copySource.name || copySource.id}` : 'New agent'}
        description={copySource
          ? `Start a new agent with the configuration of ${copySource.name || copySource.id}.`
          : 'Start an OpenShell-backed agent from a registered project and profile.'}
        footer={
          <>
            <Button variant="ghost" onClick={closeStart}>Cancel</Button>
            <Button loading={startSubmitting} disabled={!canStart} onClick={() => void startAgent()}>{startSubmitting ? 'Starting' : 'Start'}</Button>
          </>
        }
      >
        {startError ? <Alert variant="danger" title="Unable to start agent" className="mb-3">{startError}</Alert> : null}
        {!overview ? <Alert variant="info" title="Loading configuration" className="mb-3">Fetching projects, profiles, and gateways.</Alert> : null}
        {overview && (!overview.projects.length || !overview.gateways.length) ? (
          <Alert variant="warning" title="No startable configuration" className="mb-3">Create a project and gateway in Setup, then try again.</Alert>
        ) : null}
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Project" htmlFor="new-agent-project" required>
            <Select id="new-agent-project" value={startForm.project} onChange={(event) => setStartForm({ ...startForm, project: event.target.value })}>
              {(overview?.projects ?? []).map((project) => <option key={project.name} value={project.name}>{project.name}</option>)}
            </Select>
          </Field>
          <Field label="Profile" htmlFor="new-agent-profile" required>
            <Select id="new-agent-profile" value={startForm.profile} onChange={(event) => {
              const profile = event.target.value;
              const agent = overview?.profiles.find((item) => item.name === profile)?.agent;
              setStartForm((form) => ({
                ...form,
                profile,
                mcp: mcpEntries.find((entry) => entry.name === form.mcp)?.agent === agent ? form.mcp : '',
                skills: skillsEntries.find((entry) => entry.name === form.skills)?.agent === agent ? form.skills : ''
              }));
            }}>
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
          <Field label="MCP tools" htmlFor="new-agent-mcp" helper="Optional; global MCP servers delivered into the run. Project-local MCP files always ride along.">
            <Select id="new-agent-mcp" value={startForm.mcp ?? ''} onChange={(event) => setStartForm({ ...startForm, mcp: event.target.value })}>
              <option value="">None</option>
              {mcpEntries.filter((entry) => !selectedProfile || entry.agent === selectedProfile.agent).map((entry) => (
                <option key={entry.name} value={entry.name}>{entry.name} ({entry.servers.length} server{entry.servers.length === 1 ? '' : 's'})</option>
              ))}
            </Select>
          </Field>
          <Field label="Skills" htmlFor="new-agent-skills" helper="Optional; host skills and whole plugin skill bundles delivered into the run.">
            <Select id="new-agent-skills" value={startForm.skills ?? ''} onChange={(event) => setStartForm({ ...startForm, skills: event.target.value })}>
              <option value="">None</option>
              {skillsEntries.filter((entry) => !selectedProfile || entry.agent === selectedProfile.agent).map((entry) => (
                <option key={entry.name} value={entry.name}>{entry.name} ({entry.skills.length + entry.plugins.length} selection{entry.skills.length + entry.plugins.length === 1 ? '' : 's'})</option>
              ))}
            </Select>
          </Field>
          <Field label="Prompt" htmlFor="new-agent-prompt" className="sm:col-span-2">
            <Textarea id="new-agent-prompt" rows={4} value={startForm.prompt} onChange={(event) => setStartForm({ ...startForm, prompt: event.target.value })} />
          </Field>
          <div className="sm:col-span-2">
            <Disclosure
              summary={(
                <span className="flex min-w-0 flex-wrap items-baseline gap-x-2">
                  <span>Resolved configuration</span>
                  <span className="min-w-0 text-xs font-normal text-muted">
                    {selectedProfile ? `${selectedProfile.name} · ${selectedProfile.agent}` : 'Select a profile to see defaults'}
                  </span>
                </span>
              )}
            >
              {selectedProfile ? (
                <dl className="grid gap-1.5 text-sm sm:grid-cols-2">
                  {([
                    ['Image', selectedProfile.image, `image for ${selectedProfile.name}`],
                    ['Model', selectedProfile.model || '—', `model for ${selectedProfile.name}`],
                    ['Provider', selectedProfile.provider, `provider for ${selectedProfile.name}`],
                    ['Approval', approvalLabel(selectedProfile.approval), `approval for ${selectedProfile.name}`]
                  ] as const).map(([label, value, copyLabel]) => (
                    <div key={label} className="grid min-w-0 grid-cols-[5rem_minmax(0,1fr)] items-start gap-2">
                      <dt className="text-muted">{label}</dt>
                      <dd className="min-w-0 text-foreground"><ExpandableValue value={value} label={copyLabel} max={38} /></dd>
                    </div>
                  ))}
                </dl>
              ) : (
                <p className="text-sm text-muted">Select a profile to see its image, model, provider, and approval defaults.</p>
              )}
            </Disclosure>
          </div>
          <label className="flex items-center gap-2 text-sm text-muted sm:col-span-2">
            <input type="checkbox" checked={useOverrides} onChange={(event) => setUseOverrides(event.target.checked)} className="size-3.5 accent-accent" />
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
                <Select id="new-agent-approval" value={overrides.approval} onChange={(event) => {
                  approvalExplicit.current = true;
                  setOverrides({ ...overrides, approval: event.target.value });
                }}>
                  {approvalOptions.map(({ value, label }) => <option key={value} value={value}>{label}</option>)}
                </Select>
              </Field>
            </>
          ) : null}
        </div>
      </Modal>

      <Modal
        open={Boolean(hostTarget)}
        onOpenChange={(open) => { if (!open) closeHostDialog(); }}
        title={hostResult ? 'Host allowed' : 'Allow host'}
        footer={hostResult ? (
          <>
            <Button variant="outline" onClick={resetHostDialog}>Allow another host</Button>
            <Button onClick={closeHostDialog}>Close</Button>
          </>
        ) : (
          <>
            <Button variant="outline" onClick={closeHostDialog} disabled={hostSubmitting}>Cancel</Button>
            <Button loading={hostSubmitting} disabled={!hostValue.trim()} onClick={() => void submitHost()}>
              {hostSubmitting ? 'Allowing' : 'Allow host'}
            </Button>
          </>
        )}
      >
        <div className="space-y-3">
          <Field label="Run" htmlFor="allow-host-run">
            <Input id="allow-host-run" readOnly value={hostTarget ? hostTarget.name || hostTarget.id : ''} />
          </Field>
          <Field label="Agent binary" htmlFor="allow-host-binary">
            <Input id="allow-host-binary" readOnly value={hostTarget?.agent_binary || 'Loading…'} />
          </Field>
          <Field label="Host" htmlFor="allow-host-host" required>
            <Input
              id="allow-host-host"
              value={hostValue}
              disabled={Boolean(hostResult)}
              onChange={(event) => setHostValue(event.target.value)}
              placeholder="api.example.com"
            />
          </Field>
          {hostResult ? (
            <Alert variant="success" title={hostResult.outcome === 'live-applied' ? 'Network rule applied live' : 'Static change saved for a future sandbox'}>
              <div className="space-y-2">
                <div>
                  Saved in this agent's run policy.
                  {hostResult.policyPath ? <div className="break-all font-mono text-xs">{hostResult.policyPath}</div> : null}
                  <div className="text-xs text-muted">
                    Setup → Policies lists reusable templates that runs copy at start; run policies are separate.
                  </div>
                </div>
                {hostResult.diff ? <pre className="max-h-56 overflow-auto whitespace-pre-wrap font-mono text-xs leading-4">{hostResult.diff}</pre> : null}
                <Button
                  size="xs"
                  variant="outline"
                  onClick={() => {
                    const target = hostTarget;
                    closeHostDialog();
                    if (target) void openDetail(target, 'policy');
                  }}
                >
                  View policy
                </Button>
              </div>
            </Alert>
          ) : null}
          {hostError ? (
            <Alert variant="danger" title="Unable to allow host">
              <div>{hostError}</div>
              {hostRolledBack !== undefined ? <div>Rolled back: {hostRolledBack ? 'yes' : 'no'}</div> : null}
            </Alert>
          ) : null}
        </div>
      </Modal>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => !open && setDeleteTarget(undefined)}
        title={deleteTarget ? `Delete ${deleteTarget.name || deleteTarget.id}` : 'Delete agent'}
        confirmLabel="Delete"
        onConfirm={confirmDelete}
      >
        <p className="text-sm text-muted">
          This stops the agent and permanently removes its sandbox, worktree, and branch. Unsynced work is discarded.
        </p>
      </ConfirmDialog>
    </div>
  );
}
