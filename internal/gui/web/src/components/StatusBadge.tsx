import type { ReactNode } from 'react';
import {
  Circle,
  CircleCheck,
  CircleDashed,
  CircleHelp,
  CircleStop,
  CircleX,
  Loader2,
  TriangleAlert
} from 'lucide-react';
import { Badge, type BadgeTone } from './ui/badge';
import { cn } from '../lib/utils';
import type { RunStatus } from '../types';

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
  // In-flight UI states, never persisted on a run.
  stopping: { tone: 'warning', label: 'Stopping', glyph: <Loader2 className={cn(glyphClass, 'animate-spin')} /> },
  deleting: { tone: 'danger', label: 'Deleting', glyph: <Loader2 className={cn(glyphClass, 'animate-spin')} /> },
  succeeded: { tone: 'success', label: 'Succeeded', glyph: <CircleCheck className={glyphClass} /> },
  canceled: { tone: 'neutral', label: 'Canceled', glyph: <CircleStop className={cn(glyphClass, 'text-muted')} /> }
};

export const activeStatuses = new Set<RunStatus>(['creating', 'ready', 'working', 'waiting']);
export const attachableStatuses = new Set<RunStatus>(['ready', 'working', 'waiting']);
export const stoppableStatuses = new Set<RunStatus>(['ready', 'working', 'waiting']);
export const waitingStatuses = new Set<RunStatus>(['waiting']);

export function isActiveRun(status: string): boolean {
  return activeStatuses.has(status.toLowerCase() as RunStatus);
}

export function canAttachRun(status: string): boolean {
  return attachableStatuses.has(status.toLowerCase() as RunStatus);
}

export function canStopRun(status: string): boolean {
  return stoppableStatuses.has(status.toLowerCase() as RunStatus);
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
