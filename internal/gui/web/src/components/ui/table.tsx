import type { HTMLAttributes, TableHTMLAttributes, TdHTMLAttributes, ThHTMLAttributes } from 'react';
import { cn } from '../../lib/utils';

export function Table({ className, ...props }: TableHTMLAttributes<HTMLTableElement>) {
  return (
    <div className={cn('agentbox-table-frame', className)}>
      <table className="agentbox-table w-full table-fixed border-collapse text-sm" {...props} />
    </div>
  );
}

export function THead({ className, ...props }: HTMLAttributes<HTMLTableSectionElement>) {
  return <thead className={cn('bg-surface/70 text-left text-xs uppercase tracking-wide text-muted', className)} {...props} />;
}

export function TBody({ className, ...props }: HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody className={cn('divide-y divide-border/80', className)} {...props} />;
}

export function TR({ className, ...props }: HTMLAttributes<HTMLTableRowElement>) {
  return <tr className={cn('transition-colors hover:bg-surface/50', className)} {...props} />;
}

export function TH({ className, ...props }: ThHTMLAttributes<HTMLTableCellElement>) {
  return <th className={cn('h-9 px-2 font-medium', className)} {...props} />;
}

/**
 * `label` names the field for assistive technology when the table collapses
 * into compact cards on narrow screens. Card mode hides visible captions and
 * relies on the headerless scan layout; table view keeps the column header.
 */
export function TD({ label, className, children, ...props }: TdHTMLAttributes<HTMLTableCellElement> & { label?: string }) {
  return (
    <td data-label={label} className={cn('h-10 px-2 align-middle', className)} {...props}>
      {label ? <span className="agentbox-card-label">{label}: </span> : null}
      {children}
    </td>
  );
}
