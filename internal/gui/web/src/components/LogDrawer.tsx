import { Dialog } from 'radix-ui';
import { Loader2, Square, X } from 'lucide-react';
import { Button } from './ui/button';
import { StatusBadge } from './StatusBadge';

export interface LogDrawerState {
  open: boolean;
  title: string;
  content: string;
  loading?: boolean;
  taskId?: string;
  taskStatus?: string;
}

export function LogDrawer({
  state,
  onClose,
  onStop
}: {
  state: LogDrawerState;
  onClose: () => void;
  onStop?: (taskId: string) => void;
}) {
  return (
    <Dialog.Root open={state.open} onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-30 bg-black/35" />
        <Dialog.Content className="fixed bottom-0 left-0 right-0 z-40 flex h-[42vh] max-h-[70vh] flex-col border-t border-zinc-800 bg-zinc-950 shadow-2xl">
          <div className="flex h-10 shrink-0 items-center justify-between gap-3 border-b border-zinc-800 px-3">
            <div className="flex min-w-0 items-center gap-2">
              <Dialog.Title className="truncate text-xs font-semibold">{state.title}</Dialog.Title>
              {state.taskStatus ? <StatusBadge status={state.taskStatus} /> : null}
              {state.loading ? <Loader2 className="size-3.5 animate-spin text-zinc-400" /> : null}
            </div>
            <div className="flex items-center gap-1">
              {state.taskId && state.taskStatus === 'running' ? (
                <Button size="xs" variant="danger" onClick={() => onStop?.(state.taskId!)}>
                  <Square className="size-3" /> Stop
                </Button>
              ) : null}
              <Dialog.Close asChild>
                <Button size="icon" variant="ghost" aria-label="Close log drawer"><X className="size-4" /></Button>
              </Dialog.Close>
            </div>
          </div>
          <pre className="agentbox-scrollbar flex-1 overflow-auto bg-black/45 p-3 font-mono text-[11px] leading-5 text-zinc-300">
            {state.content || (state.loading ? 'Loading…' : 'No output')}
          </pre>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
