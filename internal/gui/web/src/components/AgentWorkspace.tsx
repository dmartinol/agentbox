import { Suspense, lazy, useCallback, useEffect, useMemo, useState } from 'react';
import { ArrowLeft, Plus, X } from 'lucide-react';
import { api } from '../api';
import type { Run } from '../types';
import { cn } from '../lib/utils';
import { Button } from './ui/button';
import { canAttachRun, statusLabel } from './StatusBadge';
import { LogSnapshot } from './LogSnapshot';

const AgentTerminal = lazy(() => import('./AgentTerminal').then((module) => ({ default: module.AgentTerminal })));

type ShellTab = { runId: string; session: string };

const dotTones: Record<string, string> = {
  running: 'bg-info',
  created: 'bg-info',
  prepared: 'bg-info',
  idle: 'bg-warning',
  'policy-blocked': 'bg-warning',
  unreachable: 'bg-warning',
  finished: 'bg-success',
  error: 'bg-danger',
  'create-failed': 'bg-danger'
};

const terminalFallback = <div className="grid min-h-0 flex-1 place-items-center bg-black text-sm text-muted">Loading terminal…</div>;

/**
 * Project workspace: one tab per agent of the project, plus detached shell
 * tabs created on demand. Only the visible tab holds a pty; the others are
 * represented by their status and a cached log tail.
 */
export function AgentWorkspace({
  project,
  runs,
  initialRunId,
  unread,
  onClose,
  onFocusRun
}: {
  project: string;
  runs: Run[];
  initialRunId: string;
  unread: Set<string>;
  onClose: () => void;
  onFocusRun: (runId: string) => void;
}) {
  const [shellTabs, setShellTabs] = useState<ShellTab[]>([]);
  const [activeKey, setActiveKey] = useState(`agent:${initialRunId}`);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState('');

  const runsById = useMemo(() => new Map(runs.map((run) => [run.id, run])), [runs]);

  const shells = useMemo(
    () => shellTabs.filter((tab) => runsById.has(tab.runId)),
    [shellTabs, runsById]
  );

  const active = useMemo(() => {
    if (activeKey.startsWith('agent:')) {
      const run = runsById.get(activeKey.slice('agent:'.length));
      if (run) return { kind: 'agent' as const, run };
    } else {
      const [kind, runId, session] = activeKey.split(':');
      const run = runsById.get(runId);
      if (kind === 'shell' && run && shells.some((tab) => tab.runId === runId && tab.session === session)) {
        return { kind: 'shell' as const, run, session };
      }
    }
    const fallback = runs[0];
    return fallback ? { kind: 'agent' as const, run: fallback } : undefined;
  }, [activeKey, runs, runsById, shells]);

  const activeRunId = active?.run.id;

  useEffect(() => {
    if (active && active.kind === 'agent' && activeKey !== `agent:${active.run.id}`) {
      setActiveKey(`agent:${active.run.id}`);
    }
  }, [active, activeKey]);

  useEffect(() => {
    if (activeRunId) onFocusRun(activeRunId);
  }, [activeRunId, onFocusRun]);

  useEffect(() => {
    if (!activeRunId) return;
    let alive = true;
    void api
      .shells(activeRunId)
      .then((names) => {
        if (!alive) return;
        setShellTabs((current) => {
          const next = [...current];
          for (const session of names) {
            if (!next.some((tab) => tab.runId === activeRunId && tab.session === session)) {
              next.push({ runId: activeRunId, session });
            }
          }
          return next;
        });
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [activeRunId]);

  const createShell = useCallback(async () => {
    if (!activeRunId) return;
    setCreating(true);
    setError('');
    try {
      const { name } = await api.createShell(activeRunId);
      setShellTabs((current) => [...current, { runId: activeRunId, session: name }]);
      setActiveKey(`shell:${activeRunId}:${name}`);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setCreating(false);
    }
  }, [activeRunId]);

  const closeShell = async (tab: ShellTab) => {
    setShellTabs((current) => current.filter((item) => !(item.runId === tab.runId && item.session === tab.session)));
    setActiveKey(`agent:${tab.runId}`);
    try {
      await api.deleteShell(tab.runId, tab.session);
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const needingAttention = runs.filter((run) => run.status === 'idle' || run.status === 'error').length;

  const tabClass = (selected: boolean) =>
    cn(
      'flex h-7 shrink-0 items-center gap-1.5 rounded px-2 text-sm font-medium text-muted transition-colors',
      'hover:bg-surface-2 hover:text-foreground',
      selected && 'bg-accent text-accent-foreground hover:bg-accent hover:text-accent-foreground'
    );

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border bg-background/90 px-2">
        <Button variant="ghost" size="xs" onClick={onClose}>
          <ArrowLeft className="size-3.5" /> Fleet
        </Button>
        <span className="h-4 w-px bg-border" />
        <div className="min-w-0">
          <span className="truncate text-sm font-semibold">{project}</span>
          <span className="ml-2 text-xs text-muted">
            {runs.length} agent{runs.length === 1 ? '' : 's'}
            {needingAttention > 0 ? ` · ${needingAttention} need${needingAttention === 1 ? 's' : ''} you` : ''}
          </span>
        </div>
        <div className="ml-auto flex items-center gap-1">
          <Button size="xs" variant="outline" loading={creating} disabled={!activeRunId} onClick={() => void createShell()}>
            <Plus className="size-3" /> Shell
          </Button>
        </div>
      </div>

      <div role="tablist" aria-label={`${project} agents`} className="flex h-9 shrink-0 items-center gap-1 overflow-x-auto border-b border-border px-2">
        {runs.map((run) => {
          const key = `agent:${run.id}`;
          return (
            <button
              key={key}
              role="tab"
              aria-selected={activeKey === key}
              title={`${statusLabel(run.status)} · ${run.branch}`}
              className={tabClass(activeKey === key)}
              onClick={() => setActiveKey(key)}
            >
              <span className={cn('size-1.5 shrink-0 rounded-full', dotTones[run.status.toLowerCase()] ?? 'bg-muted')} />
              <span className="max-w-40 truncate">{run.name || run.id}</span>
              {unread.has(run.id) ? <span className="size-1.5 shrink-0 rounded-full bg-accent" aria-label="Unread activity" /> : null}
            </button>
          );
        })}
        {shells.map((tab) => {
          const key = `shell:${tab.runId}:${tab.session}`;
          const run = runsById.get(tab.runId);
          return (
            <span key={key} className={cn('flex h-7 shrink-0 items-center rounded', activeKey === key && 'bg-accent text-accent-foreground')}>
              <button
                role="tab"
                aria-selected={activeKey === key}
                title={`${tab.session} · ${run?.name || tab.runId}`}
                className={cn(tabClass(activeKey === key), 'pr-1')}
                onClick={() => setActiveKey(key)}
              >
                <span className="size-1.5 shrink-0 rounded-full bg-info" />
                <span className="max-w-40 truncate">{run?.name || tab.runId} · {tab.session}</span>
              </button>
              <button
                aria-label={`Close ${tab.session}`}
                className="mr-1 grid size-4 place-items-center rounded text-muted hover:bg-danger/20 hover:text-danger"
                onClick={() => void closeShell(tab)}
              >
                <X className="size-3" />
              </button>
            </span>
          );
        })}
      </div>

      {active?.kind === 'agent' && canAttachRun(active.run.status) ? (
        <Suspense fallback={terminalFallback}>
          <AgentTerminal run={active.run} />
        </Suspense>
      ) : active?.kind === 'agent' ? (
        <LogSnapshot runId={active.run.id} label={`Output for ${active.run.name || active.run.id}`} />
      ) : active?.kind === 'shell' ? (
        <Suspense fallback={terminalFallback}>
          <AgentTerminal run={active.run} session={active.session} />
        </Suspense>
      ) : (
        <div className="grid min-h-0 flex-1 place-items-center text-sm text-muted">This project has no agents yet.</div>
      )}

      <div className="flex h-7 shrink-0 items-center gap-3 border-t border-border bg-background/80 px-3 text-xs text-muted">
        {active ? (
          <>
            <span className="truncate">{active.run.branch}</span>
            <span className="truncate font-mono">{active.run.sandbox}</span>
            <span className="ml-auto shrink-0">Detach: Ctrl-b d</span>
          </>
        ) : null}
        {error ? <span className="truncate text-danger">{error}</span> : null}
      </div>
    </div>
  );
}
