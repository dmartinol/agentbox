import type { ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, Info, XCircle } from 'lucide-react';
import { cn } from '../../lib/utils';

const variants = {
  info: { className: 'border-sky-500/30 bg-sky-500/10 text-sky-200', Icon: Info },
  success: { className: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200', Icon: CheckCircle2 },
  warning: { className: 'border-amber-500/30 bg-amber-500/10 text-amber-200', Icon: AlertTriangle },
  danger: { className: 'border-red-500/30 bg-red-500/10 text-red-200', Icon: XCircle }
};

export function Alert({
  variant = 'info',
  title,
  children,
  className
}: {
  variant?: keyof typeof variants;
  title: ReactNode;
  children?: ReactNode;
  className?: string;
}) {
  const { className: tone, Icon } = variants[variant];
  return (
    <div className={cn('flex gap-2 rounded-lg border p-2.5', tone, className)} role={variant === 'danger' ? 'alert' : 'status'}>
      <Icon className="mt-0.5 size-4 shrink-0" />
      <div className="min-w-0 space-y-1">
        <div className="text-xs font-semibold">{title}</div>
        {children ? <div className="text-xs leading-4 opacity-90">{children}</div> : null}
      </div>
    </div>
  );
}
