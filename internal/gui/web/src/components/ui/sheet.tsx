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
          className="fixed right-0 top-0 z-40 flex h-full max-w-full flex-col border-l border-border bg-background shadow-2xl sm:max-w-[94vw]"
        >
          <div className="flex min-h-12 shrink-0 items-center justify-between gap-3 border-b border-border px-4 py-2">
            <div className="min-w-0">
              <Dialog.Title className="truncate text-sm font-semibold">{title}</Dialog.Title>
              {description ? <Dialog.Description className="truncate text-sm text-muted" title={typeof description === 'string' ? description : undefined}>{description}</Dialog.Description> : null}
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
