import type { ReactNode } from 'react';
import { Dialog } from 'radix-ui';
import { X } from 'lucide-react';
import { cn } from '../../lib/utils';
import { Button } from './button';

export function Modal({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  className
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  className?: string;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/65 backdrop-blur-[1px]" />
        <Dialog.Content
          className={cn(
            'fixed left-1/2 top-1/2 z-50 max-h-[88vh] w-[min(92vw,44rem)] -translate-x-1/2 -translate-y-1/2',
            'overflow-hidden rounded-xl border border-border bg-background shadow-2xl',
            className
          )}
        >
          <div className="flex items-start justify-between gap-3 border-b border-border px-4 py-3">
            <div className="min-w-0">
              <Dialog.Title className="truncate text-sm font-semibold">{title}</Dialog.Title>
              {description ? <Dialog.Description className="mt-1 text-sm text-muted">{description}</Dialog.Description> : null}
            </div>
            <Dialog.Close asChild>
              <Button variant="ghost" size="icon" aria-label="Close"><X className="size-4" /></Button>
            </Dialog.Close>
          </div>
          <div className="max-h-[64vh] overflow-y-auto px-4 py-3 agentbox-scrollbar">{children}</div>
          {footer ? <div className="flex justify-end gap-2 border-t border-border px-4 py-3">{footer}</div> : null}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
