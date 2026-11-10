import { forwardRef } from 'react';
import type { InputHTMLAttributes, SelectHTMLAttributes, TextareaHTMLAttributes } from 'react';
import { cn } from '../../lib/utils';

const controlClass = cn(
  'h-9 w-full min-w-0 rounded-md border border-border bg-surface px-2 text-foreground',
  'placeholder:text-muted focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent/30',
  'disabled:cursor-not-allowed disabled:opacity-60 read-only:text-muted'
);

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input({ className, ...props }, ref) {
  return <input ref={ref} className={cn(controlClass, className)} {...props} />;
});

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(function Select({ className, ...props }, ref) {
  return <select ref={ref} className={cn(controlClass, 'pr-7', className)} {...props} />;
});

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(function Textarea({ className, ...props }, ref) {
  return <textarea ref={ref} className={cn(controlClass, 'h-auto min-h-20 py-1.5 font-mono text-sm leading-5', className)} {...props} />;
});
