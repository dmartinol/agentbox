import type { ReactNode } from 'react';
import { Tabs } from 'radix-ui';
import { cn } from '../../lib/utils';

export function TabList({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <Tabs.List className={cn('flex h-9 shrink-0 items-center gap-1 overflow-x-auto border-b border-border px-2', className)}>
      {children}
    </Tabs.List>
  );
}

export function TabTrigger({ value, children }: { value: string; children: ReactNode }) {
  return (
    <Tabs.Trigger
      value={value}
      className={cn(
        'h-7 shrink-0 rounded px-2 text-sm font-medium text-muted transition-colors',
        'hover:bg-surface-2 hover:text-foreground data-[state=active]:bg-accent data-[state=active]:text-accent-foreground'
      )}
    >
      {children}
    </Tabs.Trigger>
  );
}

export function TabContent({ value, children, className }: { value: string; children: ReactNode; className?: string }) {
  return <Tabs.Content value={value} className={cn('min-h-0 flex-1 overflow-y-auto p-3 agentbox-scrollbar', className)}>{children}</Tabs.Content>;
}
