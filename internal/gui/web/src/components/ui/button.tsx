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
  default: 'bg-indigo-600 text-white hover:bg-indigo-500 disabled:bg-indigo-600/50',
  secondary: 'bg-zinc-800 text-zinc-100 hover:bg-zinc-700',
  outline: 'border border-zinc-700 bg-transparent text-zinc-100 hover:bg-zinc-800',
  ghost: 'bg-transparent text-zinc-300 hover:bg-zinc-800 hover:text-zinc-50',
  danger: 'bg-red-600 text-white hover:bg-red-500 disabled:bg-red-600/50'
};

const sizes: Record<ButtonSize, string> = {
  default: 'h-8 px-3',
  sm: 'h-7 px-2.5',
  xs: 'h-6 px-2 text-xs',
  icon: 'h-7 w-7'
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
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500',
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
