import { Badge, type BadgeTone } from './ui/badge';

const meta: Record<string, { tone: BadgeTone; label: string }> = {
  running: { tone: 'info', label: 'Running' },
  created: { tone: 'accent', label: 'Created' },
  prepared: { tone: 'accent', label: 'Prepared' },
  preview: { tone: 'accent', label: 'Preview' },
  finished: { tone: 'success', label: 'Finished' },
  stopped: { tone: 'neutral', label: 'Stopped' },
  error: { tone: 'danger', label: 'Error' },
  'create-failed': { tone: 'danger', label: 'Create failed' },
  'policy-blocked': { tone: 'warning', label: 'Policy blocked' },
  unreadable: { tone: 'warning', label: 'Unreadable' },
  unreachable: { tone: 'warning', label: 'Unreachable' },
  succeeded: { tone: 'success', label: 'Succeeded' },
  failed: { tone: 'danger', label: 'Failed' },
  canceled: { tone: 'neutral', label: 'Canceled' }
};

export const activeStatuses = new Set(['running', 'created', 'prepared']);

export function isActiveRun(status: string): boolean {
  return activeStatuses.has(status.toLowerCase());
}

export function statusLabel(status: string): string {
  return meta[status.toLowerCase()]?.label ?? status;
}

export function StatusBadge({ status }: { status: string }) {
  const item = meta[status.toLowerCase()] ?? { tone: 'neutral' as BadgeTone, label: status };
  return <Badge tone={item.tone}>{item.label}</Badge>;
}
