import type { ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, Info, XCircle } from 'lucide-react';
import { cn } from '../../lib/utils';

const variants = {
  info: { className: 'border-info/30 bg-info/10 text-info', Icon: Info },
  success: { className: 'border-success/30 bg-success/10 text-success', Icon: CheckCircle2 },
  warning: { className: 'border-warning/30 bg-warning/10 text-warning', Icon: AlertTriangle },
  danger: { className: 'border-danger/30 bg-danger/10 text-danger', Icon: XCircle }
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
        <div className="text-sm font-semibold">{title}</div>
        {children ? <div className="text-sm leading-4 opacity-90">{children}</div> : null}
      </div>
    </div>
  );
}
