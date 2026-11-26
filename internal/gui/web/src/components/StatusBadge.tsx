import type { ReactNode } from 'react';
import {
  Circle,
  CircleCheck,
  CircleDashed,
  CircleHelp,
  CircleStop,
  CircleX,
  Loader2,
  RefreshCw,
  TriangleAlert
} from 'lucide-react';
import { Badge, type BadgeTone } from './ui/badge';
import { cn } from '../lib/utils';
import type { RunStatus, SyncState } from '../types';

type StatusMeta = { tone: BadgeTone; label: string; short?: string; glyph: ReactNode };

const glyphClass = 'size-3 shrink-0';

const meta: Record<string, StatusMeta> = {
  creating: { tone: 'neutral', label: 'Creating', glyph: <CircleDashed className={cn(glyphClass, 'text-muted')} /> },
  ready: { tone: 'neutral', label: 'Ready', glyph: <Circle className={cn(glyphClass, 'text-muted')} /> },
  working: { tone: 'info', label: 'Working', glyph: <Loader2 className={cn(glyphClass, 'animate-spin')} /> },
  waiting: { tone: 'warning', label: 'Waiting for input', short: 'Waiting', glyph: <CircleHelp className={glyphClass} /> },
  failed: { tone: 'danger', label: 'Failed', glyph: <CircleX className={glyphClass} /> },
  unreachable: { tone: 'warning', label: 'Unreachable', glyph: <TriangleAlert className={glyphClass} /> },
  stopped: { tone: 'neutral', label: 'Stopped', glyph: <CircleStop className={cn(glyphClass, 'text-muted')} /> },
  completed: { tone: 'success', label: 'Completed', glyph: <CircleCheck className={glyphClass} /> },
  // An exited agent is resumable, but it is not "Ready" to take a new turn
  // until Resume relaunches it.
  exited: { tone: 'warning', label: 'Exited', glyph: <CircleStop className={glyphClass} /> },
  // In-flight UI states, never persisted on a run.
  sleeping: { tone: 'warning', label: 'Sleeping', glyph: <Loader2 className={cn(glyphClass, 'animate-spin')} /> },
  resuming: { tone: 'info', label: 'Resuming', glyph: <Loader2 className={cn(glyphClass, 'animate-spin')} /> },
  finishing: { tone: 'warning', label: 'Finishing', glyph: <Loader2 className={cn(glyphClass, 'animate-spin')} /> },
  deleting: { tone: 'danger', label: 'Deleting', glyph: <Loader2 className={cn(glyphClass, 'animate-spin')} /> },
  succeeded: { tone: 'success', label: 'Succeeded', glyph: <CircleCheck className={glyphClass} /> },
  canceled: { tone: 'neutral', label: 'Canceled', glyph: <CircleStop className={cn(glyphClass, 'text-muted')} /> }
};

export const activeStatuses = new Set<RunStatus>(['creating', 'ready', 'working', 'waiting']);
export const attachableStatuses = new Set<RunStatus>(['ready', 'working', 'waiting']);
export const sleepableStatuses = new Set<RunStatus>(['ready', 'waiting']);
// Finish ends a Ready/Waiting run and also closes a sleeping run without
// restarting its sandbox.
export const finishableStatuses = new Set<RunStatus>(['ready', 'waiting', 'stopped']);
export const waitingStatuses = new Set<RunStatus>(['waiting']);

export function isActiveRun(status: string): boolean {
  return activeStatuses.has(status.toLowerCase() as RunStatus);
}

export function displayStatus(status: string, exited?: boolean): string {
  return status.toLowerCase() === 'ready' && exited ? 'exited' : status;
}

export function canAttachRun(status: string): boolean {
  return attachableStatuses.has(status.toLowerCase() as RunStatus);
}

export function canSleepRun(status: string): boolean {
  return sleepableStatuses.has(status.toLowerCase() as RunStatus);
}

export function canFinishRun(status: string): boolean {
  return finishableStatuses.has(status.toLowerCase() as RunStatus);
}

export function canResumeRun(status: string, exited?: boolean): boolean {
  const normalized = status.toLowerCase() as RunStatus;
  return normalized === 'stopped' || (normalized === 'ready' && Boolean(exited));
}

export function isWaitingRun(status: string): boolean {
  return waitingStatuses.has(status.toLowerCase() as RunStatus);
}

export function statusLabel(status: string): string {
  return meta[status.toLowerCase()]?.label ?? status;
}

export function StatusGlyph({ status, className }: { status: string; className?: string }) {
  const item = meta[status.toLowerCase()];
  if (!item) return <Circle className={cn(glyphClass, 'text-muted', className)} />;
  return <span className={cn('inline-flex shrink-0', className)}>{item.glyph}</span>;
}

export function StatusBadge({ status }: { status: string }) {
  const item: StatusMeta = meta[status.toLowerCase()] ?? { tone: 'neutral', label: status, glyph: <Circle className={cn(glyphClass, 'text-muted')} /> };
  return (
    <Badge tone={item.tone} aria-label={item.label} title={item.label}>
      {item.glyph}
      {item.short ?? item.label}
    </Badge>
  );
}

type SyncMeta = { tone: BadgeTone; label: string; explanation?: string; glyph: ReactNode };

const syncMeta: Record<SyncState, SyncMeta> = {
  no_changes: {
    tone: 'neutral',
    label: 'No changes',
    glyph: <CircleDashed className={cn(glyphClass, 'text-muted')} />
  },
  up_to_date: {
    tone: 'success',
    label: 'Up to date',
    glyph: <CircleCheck className={glyphClass} />
  },
  needs_sync: {
    tone: 'warning',
    label: 'Sync needed',
    explanation: 'Sandbox changes have not been transferred to the host run worktree.',
    glyph: <RefreshCw className={glyphClass} />
  },
  blocked: {
    tone: 'danger',
    label: 'Sync blocked',
    explanation: 'The host run worktree changed, so sync will refuse to overwrite it.',
    glyph: <TriangleAlert className={glyphClass} />
  },
  wrong_checkout: {
    tone: 'danger',
    label: 'Wrong checkout',
    explanation: 'Sandbox HEAD is not the run branch tip, so sync is refused. Move the intended commit onto the run branch, then retry.',
    glyph: <TriangleAlert className={glyphClass} />
  },
  unknown: {
    tone: 'neutral',
    label: 'Unable to check',
    explanation: 'AgentBox could not inspect the sandbox.',
    glyph: <CircleHelp className={glyphClass} />
  }
};

export function syncStateLabel(state: string): string {
  return syncMeta[state as SyncState]?.label ?? syncMeta.unknown.label;
}

export function syncStateExplanation(state: string): string | undefined {
  return syncMeta[state as SyncState]?.explanation;
}

export function SyncStateBadge({ state }: { state: string }) {
  const item = syncMeta[state as SyncState] ?? syncMeta.unknown;
  return (
    <Badge tone={item.tone} title={item.label}>
      {item.glyph}
      {item.label}
    </Badge>
  );
}
