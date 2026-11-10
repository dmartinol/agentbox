import type { ReactNode } from 'react';
import { Dialog } from 'radix-ui';
import { X } from 'lucide-react';
import { cn } from '../../lib/utils';
import { Button } from './button';

export function Sheet({
  open,
  onOpenChange,
  title,
  description,
  children,
  width = '34rem'
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  width?: string;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-30 bg-black/45" />
        <Dialog.Content
          style={{ width }}
          className="fixed right-0 top-0 z-40 flex h-full max-w-[94vw] flex-col border-l border-zinc-800 bg-zinc-950 shadow-2xl"
        >
          <div className="flex h-12 items-center justify-between gap-3 border-b border-zinc-800 px-4">
            <div className="min-w-0">
              <Dialog.Title className="truncate text-sm font-semibold">{title}</Dialog.Title>
              {description ? <Dialog.Description className="truncate text-xs text-zinc-500">{description}</Dialog.Description> : null}
            </div>
            <Dialog.Close asChild>
              <Button variant="ghost" size="icon" aria-label="Close"><X className="size-4" /></Button>
            </Dialog.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto agentbox-scrollbar">{children}</div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
