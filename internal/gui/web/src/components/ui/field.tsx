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
      <label htmlFor={htmlFor} className="flex items-center gap-1 text-sm font-medium text-muted">
        {label}
        {required ? <span aria-hidden className="text-danger">*</span> : null}
      </label>
      {children}
      {helper ? <p className="text-xs leading-4 text-muted">{helper}</p> : null}
      {error ? <p className="text-xs leading-4 text-danger">{error}</p> : null}
    </div>
  );
}
