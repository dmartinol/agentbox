import type { ReactNode } from 'react';
import { Tabs } from 'radix-ui';
import { cn } from '../../lib/utils';

export function TabList({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <Tabs.List className={cn('flex h-9 shrink-0 items-center gap-1 overflow-x-auto border-b border-zinc-800 px-2', className)}>
      {children}
    </Tabs.List>
  );
}

export function TabTrigger({ value, children }: { value: string; children: ReactNode }) {
  return (
    <Tabs.Trigger
      value={value}
      className={cn(
        'h-7 shrink-0 rounded px-2 text-xs font-medium text-zinc-400 transition-colors',
        'hover:bg-zinc-800 hover:text-zinc-100 data-[state=active]:bg-zinc-800 data-[state=active]:text-white'
      )}
    >
      {children}
    </Tabs.Trigger>
  );
}

export function TabContent({ value, children, className }: { value: string; children: ReactNode; className?: string }) {
  return <Tabs.Content value={value} className={cn('min-h-0 flex-1 overflow-y-auto p-3 agentbox-scrollbar', className)}>{children}</Tabs.Content>;
}
