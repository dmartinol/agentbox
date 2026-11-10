import type { HTMLAttributes } from 'react';
import { cn } from '../../lib/utils';

export type BadgeTone = 'neutral' | 'success' | 'warning' | 'danger' | 'info' | 'accent';

const tones: Record<BadgeTone, string> = {
  neutral: 'bg-zinc-800 text-zinc-300 border-zinc-700',
  success: 'bg-emerald-500/12 text-emerald-300 border-emerald-500/25',
  warning: 'bg-amber-500/12 text-amber-300 border-amber-500/25',
  danger: 'bg-red-500/12 text-red-300 border-red-500/25',
  info: 'bg-sky-500/12 text-sky-300 border-sky-500/25',
  accent: 'bg-indigo-500/12 text-indigo-300 border-indigo-500/25'
};

export function Badge({ className, tone = 'neutral', ...props }: HTMLAttributes<HTMLSpanElement> & { tone?: BadgeTone }) {
  return (
    <span
      className={cn(
        'inline-flex h-5 shrink-0 items-center gap-1 rounded border px-1.5 text-[11px] font-medium leading-none',
        tones[tone],
        className
      )}
      {...props}
    />
  );
}
