import { useId } from 'react';
import { Button, type ButtonProps } from './button';

export interface ActionButtonProps extends ButtonProps {
  disabledReason?: string;
}

export function ActionButton({ disabledReason, onClick, children, ...props }: ActionButtonProps) {
  const reasonId = useId();
  if (!disabledReason) {
    return (
      <Button onClick={onClick} {...props}>
        {children}
      </Button>
    );
  }
  return (
    <span className="inline-flex flex-col gap-1">
      <Button
        {...props}
        aria-disabled="true"
        aria-describedby={reasonId}
        onClick={(event) => event.preventDefault()}
      >
        {children}
      </Button>
      <span id={reasonId} className="text-xs text-muted">
        {disabledReason}
      </span>
    </span>
  );
}
