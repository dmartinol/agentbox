import { forwardRef } from 'react';
import type { ButtonHTMLAttributes } from 'react';
import { cn } from '../../lib/utils';

export type ButtonVariant = 'default' | 'secondary' | 'outline' | 'ghost' | 'danger';
export type ButtonSize = 'default' | 'sm' | 'xs' | 'icon';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
}

const variants: Record<ButtonVariant, string> = {
  default: 'bg-accent text-accent-foreground hover:bg-accent disabled:bg-accent/50',
  secondary: 'bg-surface-2 text-foreground hover:bg-surface-3',
  outline: 'border border-border bg-transparent text-foreground hover:bg-surface-2',
  ghost: 'bg-transparent text-foreground hover:bg-surface-2 hover:text-foreground',
  danger: 'bg-danger text-accent-foreground hover:bg-danger disabled:bg-danger/50'
};

const sizes: Record<ButtonSize, string> = {
  default: 'h-9 px-3',
  sm: 'h-8 px-3',
  xs: 'h-7 px-2 text-sm',
  icon: 'h-8 w-8'
};

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { className, variant = 'default', size = 'default', loading, children, disabled, ...props },
  ref
) {
  return (
    <button
      ref={ref}
      className={cn(
        'inline-flex items-center justify-center gap-1.5 rounded-md font-medium transition-colors',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent',
        'disabled:cursor-not-allowed disabled:opacity-65',
        variants[variant],
        sizes[size],
        className
      )}
      disabled={disabled || loading}
      {...props}
    >
      {loading ? <span className="size-3 animate-spin rounded-full border-2 border-current border-t-transparent" aria-hidden /> : null}
      {children}
    </button>
  );
});
