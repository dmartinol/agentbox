import { useMemo } from 'react';
import { Dialog } from 'radix-ui';
import { Loader2, Square, X } from 'lucide-react';
import { Button } from './ui/button';
import { CopyButton } from './ui/copy-value';
import { StatusBadge } from './StatusBadge';
import { normalizeTerminalText } from '../lib/text';

export interface LogDrawerState {
  open: boolean;
  title: string;
  content: string;
  loading?: boolean;
  error?: string;
  taskId?: string;
  taskStatus?: string;
}

export function LogDrawer({
  state,
  onClose,
  onStop,
  stopping = false
}: {
  state: LogDrawerState;
  onClose: () => void;
  onStop?: (taskId: string) => void;
  stopping?: boolean;
}) {
  // Render terminal-shaped text, not raw escape sequences.
  const content = useMemo(() => normalizeTerminalText(state.content), [state.content]);
  const empty = !content.trim();

  return (
    <Dialog.Root open={state.open} onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-30 bg-black/35" />
        <Dialog.Content
          className="fixed bottom-0 left-0 right-0 z-40 flex h-[42vh] max-h-[70vh] flex-col border-t border-border bg-background shadow-2xl"
          aria-describedby={undefined}
        >
          <div className="flex min-h-10 shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border px-3 py-1.5">
            <div className="flex min-w-0 flex-1 items-center gap-2">
              <Dialog.Title className="min-w-0 truncate text-sm font-semibold" title={state.title}>{state.title}</Dialog.Title>
              {state.taskStatus ? <StatusBadge status={state.taskStatus} /> : null}
              {state.loading ? (
                <span className="inline-flex items-center gap-1 text-xs text-muted" role="status">
                  <Loader2 className="size-3.5 animate-spin" aria-hidden /> Loading output
                </span>
              ) : null}
            </div>
            <div className="flex items-center gap-1">
              {!empty ? <CopyButton value={content} label="output" /> : null}
              {state.taskId && state.taskStatus === 'running' ? (
                <Button size="xs" variant="danger" loading={stopping} onClick={() => onStop?.(state.taskId!)}>
                  <Square className="size-3" /> {stopping ? 'Stopping' : 'Stop'}
                </Button>
              ) : null}
              <Dialog.Close asChild>
                <Button size="icon" variant="ghost" aria-label="Close log drawer"><X className="size-4" /></Button>
              </Dialog.Close>
            </div>
          </div>
          <div className="agentbox-scrollbar min-h-0 flex-1 overflow-auto bg-surface-2 p-3">
            {state.error ? (
              <div role="alert" className="space-y-1 text-sm text-danger">
                <div className="font-semibold">{state.taskStatus === 'failed' ? 'Task failed' : 'Unable to load output'}</div>
                <div className="break-words">{state.error}</div>
              </div>
            ) : null}
            {empty ? (
              state.error ? null : (
                <div className="text-sm text-muted" role={state.loading ? 'status' : undefined}>
                  {state.loading ? 'Loading output…' : 'No output recorded.'}
                </div>
              )
            ) : (
              <pre className={state.error ? 'agentbox-output mt-2 font-mono text-xs leading-5 text-foreground' : 'agentbox-output font-mono text-xs leading-5 text-foreground'}>{content}</pre>
            )}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
