import type { ReactNode } from 'react';
import { Inbox } from 'lucide-react';

export function EmptyState({ title, description, action }: { title: string; description?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 px-4 py-10 text-center">
      <Inbox className="size-7 text-muted" />
      <div className="text-sm font-medium text-foreground">{title}</div>
      {description ? <div className="max-w-sm text-sm leading-4 text-muted">{description}</div> : null}
      {action}
    </div>
  );
}
