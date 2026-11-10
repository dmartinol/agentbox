import type { ReactNode } from 'react';
import { cn } from '../../lib/utils';

export function Field({
  label,
  htmlFor,
  required,
  helper,
  error,
  children,
  className
}: {
  label: ReactNode;
  htmlFor?: string;
  required?: boolean;
  helper?: ReactNode;
  error?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn('min-w-0 space-y-1', className)}>
      <label htmlFor={htmlFor} className="flex items-center gap-1 text-xs font-medium text-zinc-400">
        {label}
        {required ? <span aria-hidden className="text-red-400">*</span> : null}
      </label>
      {children}
      {helper ? <p className="text-[11px] leading-4 text-zinc-500">{helper}</p> : null}
      {error ? <p className="text-[11px] leading-4 text-red-400">{error}</p> : null}
    </div>
  );
}
