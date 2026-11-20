import { useEffect, useMemo, useRef, useState } from 'react';
import { Bot, CircleHelp, CircleSlash, Loader2, Monitor, RefreshCw, Search, Terminal, X } from 'lucide-react';
import { api } from '../api';
import { approvalForProfile, approvalLabel, approvalOptions, defaultApproval } from '../approvals';
import type { MCPTools, Overview, Profile, Run, SkillsEntry, StartRunRequest } from '../types';
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
import { compareVersions, formatSyncTimestamp, middleEllipsis } from '../lib/text';
import { StatusBadge, SyncStateBadge, canAttachRun, isActiveRun, canFinishRun, canResumeRun, canSleepRun, isWaitingRun, statusLabel, syncStateExplanation, syncStateLabel } from '../components/StatusBadge';
import type { LogDrawerState } from '../components/LogDrawer';

const allProjects = 'All projects';

const autoImage = 'auto';
const profileImage = 'profile';

function imageSourceLabel(source?: string): string {
  switch (source) {
    case 'repo':
      return 'Repository image';
    case 'override':
      return 'Run override';
    case 'profile':
      return 'Profile image';
    default:
      return 'Not recorded';
  }
}

function policyProvenanceLines(run: Run): string[] {
  const provenance = run.policy_provenance;
  const lines = provenance ? [
    `Policy provenance: ${provenance.source}`,
    `Gateway: ${provenance.gateway}`,
    `Provider: ${provenance.provider}`,
    `Mode: ${provenance.mode}`,
    `Provider host: ${provenance.host}:${provenance.port}`,
    `Provider rule: ${provenance.rule}`,
    `Agent binary: ${provenance.agent_binary}`
  ] : ['Policy provenance: Not recorded (pre-#79 run)'];
  lines.push(
    `Repository manifest: ${run.manifest_digest || 'No repository manifest'}`,
    `Runtime-added hosts: ${run.runtime_hosts?.length ?? 0}`
  );
  if (provenance?.legacy_path) lines.push(`Legacy base (network rules only): ${provenance.legacy_path}`);
  lines.push(
    run.policy_fragment
      ? `Repository fragment: ${run.policy_fragment}${run.policy_fragment_digest ? ` (${run.policy_fragment_digest})` : ''}`
      : 'Repository fragment: none'
  );
  return lines;
}

function AgentConfiguration({ run }: { run: Run }) {
  const identifiers = [
    ['ID', run.id],
    ['Harness', run.agent],
    ['Branch', run.branch],
    ['Sandbox', run.sandbox],
    ['Worktree', run.worktree],
    ['Mode', run.mode],
    ['Image source', imageSourceLabel(run.image_source)],
    ...(run.base_image && run.base_image !== run.image ? [['Base image', run.base_image]] : []),
    ...(run.image_repo ? [['Repository', run.image_repo]] : []),
    ...(run.manifest_digest ? [['Manifest digest', run.manifest_digest]] : []),
    ...(run.policy_fragment ? [['Policy fragment', `${run.policy_fragment}${run.policy_fragment_digest ? ` (${run.policy_fragment_digest})` : ''}`]] : [])
  ] as [string, string][];
  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Project" htmlFor="agent-project">
          <Input id="agent-project" readOnly value={run.project || 'Unassigned'} />
        </Field>
        <Field label="Profile" htmlFor="agent-profile">
          <Input id="agent-profile" readOnly value={run.profile || '—'} />
        </Field>
        <Field label="Gateway" htmlFor="agent-gateway">
          <Input id="agent-gateway" readOnly value={run.gateway} />
        </Field>
        <Field label="Agent name" htmlFor="agent-name">
          <Input id="agent-name" readOnly value={run.name || '—'} />
        </Field>
        <Field label="MCP tools" htmlFor="agent-mcp">
          <Input id="agent-mcp" readOnly value={run.mcp || 'None'} />
        </Field>
        <Field label="Skills" htmlFor="agent-skills">
          <Input id="agent-skills" readOnly value={run.skills || 'None'} />
        </Field>
        <Field label="Image" htmlFor="agent-image" className="sm:col-span-2">
          <Input id="agent-image" readOnly value={run.image} />
        </Field>
        <Field label="Model" htmlFor="agent-model">
          <Input id="agent-model" readOnly value={run.model || '—'} />
        </Field>
        <Field label="Approval" htmlFor="agent-approval">
          <Input id="agent-approval" readOnly value={approvalLabel(run.approval)} />
        </Field>
      </div>

      <Disclosure summary="Identifiers and repository">
        <dl className="grid gap-1.5 text-sm sm:grid-cols-2">
          {identifiers.map(([label, value]) => (
            <div key={label} className="grid min-w-0 grid-cols-[7rem_minmax(0,1fr)] items-start gap-2">
              <dt className="text-muted">{label}</dt>
              <dd className="min-w-0 text-foreground">
                <ExpandableValue value={value} label={`${label} for ${run.name || run.id}`} max={34} />
              </dd>
            </div>
          ))}
        </dl>
      </Disclosure>

      {run.runtime_hosts?.length ? (
        <Disclosure summary={`Runtime host rules (${run.runtime_hosts.length})`}>
          <ul className="space-y-2 text-sm">
            {run.runtime_hosts.map((host) => (
              <li key={`${host.host}:${host.port}:${host.binary}:${host.rule}`} className="space-y-0.5">
                <div className="break-all font-mono text-xs">{host.host}:{host.port}</div>
                <div className="break-all font-mono text-xs text-muted">{host.binary}</div>
                <div className="text-xs text-muted">
                  Rule {host.rule} · {host.source === 'run+manifest' ? 'persisted in the manifest' : 'this run only'}
                </div>
              </li>
            ))}
          </ul>
        </Disclosure>
      ) : null}
    </div>
  );
}

// Fleet stays live on its own: a run that is still starting must not sit on
// Creating until someone reloads the page. Poll fast while something is
// changing or while a terminal is on screen, a little slower while an idle
// agent could start a turn, and stay cheap once every run is terminal.
function fleetPollDelay(runs: Run[], watching: boolean): number {
  if (runs.some((run) => run.status === 'creating' || run.status === 'working' || run.status === 'waiting')) return 1000;
  if (runs.some((run) => run.status === 'ready')) return watching ? 1500 : 2500;
  return 15000;
}

type AgentAction = 'sync' | 'sleep' | 'resume' | 'finish' | 'delete';

// manageHint explains why the lifecycle actions are disabled, so the reason is
// visible instead of only implied by a greyed-out button.
function manageHint(agent: Run): string {
  switch (agent.status) {
    case 'working':
      return 'Sleep and Finish are unavailable while the agent works. Wait for the turn, or run `agentbox agent sleep --force`.';
    case 'completed':
      return 'Completed runs cannot be resumed. Delete removes the record and worktree.';
    case 'stopped':
      return 'Resume starts the sandbox and continues the last conversation in the same worktree.';
    case 'ready':
      return agent.exited
        ? 'The agent exited. Resume continues the last conversation; Finish ends the run and keeps the transcript.'
        : 'Sleep keeps the worktree and transcript. Resume continues the conversation later.';
    case 'waiting':
      return 'Sleep keeps the worktree and transcript. Resume continues the conversation later.';
    default:
      return `Lifecycle actions are unavailable while the run is ${statusLabel(agent.status).toLowerCase()}.`;
  }
}

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
  const [configRunId, setConfigRunId] = useState<string>();
  const configTrigger = useRef<HTMLButtonElement>(null);

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
  const configRun = configRunId ? agents.find((agent) => agent.id === configRunId) : undefined;
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
  const selectedProject = overview?.projects.find((project) => project.name === startForm.project);
  const repoImages = useMemo(() => {
    const repo = selectedProject?.repo;
    const agent = selectedProfile?.agent;
    if (!repo || !agent) return [];
    return (overview?.images ?? [])
      .filter((image) => image.worker && image.repo === repo && image.agent === agent)
      .sort((left, right) => compareVersions(right.version, left.version) || left.reference.localeCompare(right.reference));
  }, [overview?.images, selectedProject?.repo, selectedProfile?.agent]);
  const defaultImageSelection = repoImages.length ? autoImage : profileImage;
  const imageUnavailableReason = !selectedProject
    ? 'Select a project to choose an image.'
    : !selectedProject.manifest
      ? 'This project has no .agentbox/config.yaml, so runs keep the profile image.'
      : repoImages.length === 0
        ? `No repository image for ${selectedProfile?.agent || 'this agent'} in this project. Set up repository images first.`
        : undefined;
  const recordedImage = overrides.image && ![autoImage, profileImage].includes(overrides.image) && !repoImages.some((image) => image.reference === overrides.image)
    ? overrides.image
    : undefined;
  const imageSelection = overrides.image === autoImage && repoImages.length === 0 ? profileImage : overrides.image || defaultImageSelection;
  const imageHelper = recordedImage
    ? 'Keeping the image recorded on the copied run.'
    : imageUnavailableReason ?? (repoImages[0] ? `Defaults to the latest repository image: ${repoImages[0].reference}` : undefined);
  const canStart = Boolean(overview && startForm.project && startForm.gateway);

  useEffect(() => {
    if (!startOpen || approvalExplicit.current) return;
    setOverrides((current) => ({ ...current, approval: approvalForProfile(selectedProfile) }));
  }, [startOpen, selectedProfile?.approval]);

  useEffect(() => {
    if (!useOverrides) return;
    setOverrides((current) => {
      const values = new Set([autoImage, profileImage, ...repoImages.map((image) => image.reference)]);
      if (current.image && (values.has(current.image) || current.image === copySource?.image)) return current;
      return { ...current, image: defaultImageSelection };
    });
  }, [useOverrides, repoImages, defaultImageSelection, copySource?.image]);

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
    if (kind === 'sleep') return 'sleeping';
    if (kind === 'resume') return 'resuming';
    if (kind === 'finish') return 'finishing';
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
    const profile = profiles.find((item) => item.name === run.profile) ?? profiles.find((item) => item.image === run.image) ?? profiles.find((item) => item.agent === run.agent);
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

  const closeConfiguration = (restoreFocus = true) => {
    setConfigRunId(undefined);
    if (restoreFocus) window.setTimeout(() => configTrigger.current?.focus(), 0);
  };

  const startAgent = async () => {
    setStartError('');
    setNotice('');
    setStartSubmitting(true);
    try {
      const overrideImage = useOverrides && overrides.image !== autoImage ? overrides.image : undefined;
      const created = await api.startRun({
        ...startForm,
        ...(useOverrides ? { ...overrides, image: overrideImage } : {}),
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
      openLog({ title: `Logs · ${agent.name || agent.id}`, content: '', loading: true, runId: agent.id, runStatus: agent.status });
      try {
        const response = await api.logs(agent.id);
        updateLog({
          content: response.logs,
          denials: response.denials ?? [],
          denialsError: response.denialsError,
          loading: false,
          error: undefined
        });
      } catch (err) {
        updateLog({ content: '', denials: [], denialsError: undefined, loading: false, error: (err as Error).message });
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
      const policy = await api.runPolicyContent(agent.id);
      const provenance = [
        `Policy file: ${policy.path || agent.policy}`,
        ...policyProvenanceLines(agent)
      ].join('\n');
      updateLog({ content: `${provenance}\n\n${policy.content}`, loading: false, error: undefined });
    } catch (err) {
      updateLog({ content: '', loading: false, error: (err as Error).message });
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
                          className={cn(
                            'cursor-pointer focus-visible:bg-surface/50',
                            selected?.id === agent.id && 'bg-accent/10'
                          )}
                          aria-current={selected?.id === agent.id || undefined}
                          tabIndex={0}
                          onClick={() => setSelected(agent)}
                          onKeyDown={(event) => {
                            // Inner controls keep their own key handling.
                            if (event.target !== event.currentTarget) return;
                            if (event.key !== 'Enter' && event.key !== ' ') return;
                            event.preventDefault();
                            setSelected(agent);
                          }}
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
                  {selected.status_detail ? (
                    <div className="text-xs text-muted [overflow-wrap:anywhere]">{selected.status_detail}</div>
                  ) : null}
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
                  ['Harness', selected.agent, 24],
                  ['Model', selected.model || '—', 24],
                  ...(selected.profile ? [['Profile', selected.profile, 24] as const] : []),
                  ['Image', selected.image, 24]
                ] as const).map(([label, value, max]) => (
                  <div key={label} className="grid grid-cols-[5.5rem_minmax(0,1fr)] items-start gap-2">
                    <dt className="text-muted">{label}</dt>
                    <dd className="min-w-0 text-foreground">
                      <ExpandableValue value={value} label={`${label} for ${selected.name || selected.id}`} max={max} />
                      {label === 'Image' && selected.image_source ? (
                        <div className="text-xs text-muted">{imageSourceLabel(selected.image_source)}</div>
                      ) : null}
                    </dd>
                  </div>
                ))}
              </dl>

              {selected.image_warnings?.length ? (
                <Alert variant="warning" title="Image warnings">
                  <ul className="list-disc space-y-1 pl-4">
                    {selected.image_warnings.map((warning) => <li key={warning}>{warning}</li>)}
                  </ul>
                </Alert>
              ) : null}

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
                  <Button ref={configTrigger} variant="outline" className="col-span-2" onClick={() => setConfigRunId(selected.id)}>Configuration</Button>
                </div>
                {canAttachRun(selected.status) ? null : (
                  <p className="text-xs text-muted">Attach needs a running sandbox; this agent is {statusLabel(inFlightStatus(selected)).toLowerCase()}.</p>
                )}
              </section>

              <section aria-label="Manage agent" className="space-y-1.5 rounded-lg border border-border p-2">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted">Manage</h3>
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
                    loading={pending[selected.id] === 'sleep'}
                    disabled={Boolean(pending[selected.id]) || !canSleepRun(selected.status)}
                    onClick={() => void runAgentAction(selected, 'sleep', () => api.sleepRun(selected.id), `Sleep requested for ${selected.name || selected.id}`)}
                  >
                    {pending[selected.id] === 'sleep' ? 'Sleeping…' : 'Sleep'}
                  </Button>
                  <Button
                    variant="outline"
                    loading={pending[selected.id] === 'resume'}
                    disabled={Boolean(pending[selected.id]) || !canResumeRun(selected.status, selected.exited)}
                    onClick={() => void runAgentAction(selected, 'resume', async () => {
                      await api.resumeRun(selected.id);
                      openWorkspace(selected);
                    }, `Resume requested for ${selected.name || selected.id}`)}
                  >
                    {pending[selected.id] === 'resume' ? 'Resuming…' : 'Resume'}
                  </Button>
                  <Button
                    variant="outline"
                    loading={pending[selected.id] === 'finish'}
                    disabled={Boolean(pending[selected.id]) || !canFinishRun(selected.status)}
                    onClick={() => void runAgentAction(selected, 'finish', () => api.finishRun(selected.id), `Finish requested for ${selected.name || selected.id}`)}
                  >
                    {pending[selected.id] === 'finish' ? 'Finishing…' : 'Finish'}
                  </Button>
                </div>
                <p className="text-xs text-muted">{manageHint(selected)}</p>
              </section>

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
              <Field label="Image" htmlFor="new-agent-image" className="sm:col-span-2" helper={imageHelper}>
                <Select
                  id="new-agent-image"
                  value={imageSelection}
                  disabled={Boolean(imageUnavailableReason) && !recordedImage}
                  onChange={(event) => setOverrides({ ...overrides, image: event.target.value })}
                >
                  {repoImages.map((image, index) => (
                    <option key={image.reference} value={index === 0 ? autoImage : image.reference}>
                      {index === 0 ? 'Latest project image' : 'Project image'} · {image.agent} {image.version} · {middleEllipsis(image.reference, 36)}
                    </option>
                  ))}
                  {recordedImage ? <option value={recordedImage}>Recorded image · {middleEllipsis(recordedImage, 36)}</option> : null}
                  <option value={profileImage}>Use profile image{selectedProfile ? ` · ${selectedProfile.agent} profile` : ''}</option>
                </Select>
              </Field>
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
        open={Boolean(configRun)}
        onOpenChange={(open) => {
          if (!open) closeConfiguration();
        }}
        title="Configuration"
        description={configRun
          ? `Read-only configuration recorded for ${configRun.name || configRun.id}.`
          : 'Read-only configuration for the selected agent.'}
        footer={configRun ? (
          <>
            <Button
              variant="danger"
              className="mr-auto"
              loading={pending[configRun.id] === 'delete'}
              disabled={Boolean(pending[configRun.id])}
              onClick={() => {
                const target = configRun;
                closeConfiguration(false);
                setDeleteTarget(target);
              }}
            >
              {pending[configRun.id] === 'delete' ? 'Deleting agent' : 'Delete agent'}
            </Button>
            <Button variant="ghost" onClick={() => closeConfiguration()}>Done</Button>
            <Button onClick={() => {
              const target = configRun;
              closeConfiguration(false);
              copyAgent(target);
            }}>
              Copy agent
            </Button>
          </>
        ) : null}
      >
        {configRun ? <AgentConfiguration run={configRun} /> : null}
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
