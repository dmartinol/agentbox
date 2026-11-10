import type { ReactNode } from 'react';
import { Inbox } from 'lucide-react';

export function EmptyState({ title, description, action }: { title: string; description?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 px-4 py-10 text-center">
      <Inbox className="size-7 text-zinc-600" />
      <div className="text-sm font-medium text-zinc-300">{title}</div>
      {description ? <div className="max-w-sm text-xs leading-4 text-zinc-500">{description}</div> : null}
      {action}
    </div>
  );
}
