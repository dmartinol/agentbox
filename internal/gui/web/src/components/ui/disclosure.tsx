import { useState, type ReactNode } from 'react';
import { ChevronRight } from 'lucide-react';
import { cn } from '../../lib/utils';

/** Native, keyboard-operable progressive disclosure for secondary details. */
export function Disclosure({
  summary,
  children,
  className,
  defaultOpen = false
}: {
  summary: ReactNode;
  children: ReactNode;
  className?: string;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <details
      open={open}
      onToggle={(event) => setOpen((event.currentTarget as HTMLDetailsElement).open)}
      className={cn('group rounded-lg border border-border bg-surface/40', className)}
    >
      <summary className="flex cursor-pointer list-none items-center gap-2 rounded-lg px-2.5 py-2 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent">
        <ChevronRight className="size-3.5 shrink-0 text-muted transition-transform group-open:rotate-90" aria-hidden />
        <span className="min-w-0 flex-1">{summary}</span>
      </summary>
      <div className="border-t border-border px-2.5 py-2">{children}</div>
    </details>
  );
}
