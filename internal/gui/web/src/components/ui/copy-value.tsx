import { useEffect, useRef, useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { cn } from '../../lib/utils';
import { middleEllipsis } from '../../lib/text';
import { Button } from './button';

async function writeClipboard(value: string) {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return;
    }
  } catch {
    // Fall through to the selection fallback.
  }
  const area = document.createElement('textarea');
  area.value = value;
  area.setAttribute('readonly', '');
  area.style.position = 'fixed';
  area.style.opacity = '0';
  document.body.appendChild(area);
  area.select();
  try {
    document.execCommand?.('copy');
  } finally {
    area.remove();
  }
}

export function CopyButton({ value, label, className }: { value: string; label: string; className?: string }) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number>();

  useEffect(() => () => window.clearTimeout(timer.current), []);

  return (
    <Button
      type="button"
      size="icon"
      variant="ghost"
      className={cn('size-6 shrink-0 text-muted', className)}
      aria-label={`Copy ${label}`}
      title={`Copy ${label}`}
      onClick={(event) => {
        event.stopPropagation();
        void writeClipboard(value).then(() => {
          setCopied(true);
          window.clearTimeout(timer.current);
          timer.current = window.setTimeout(() => setCopied(false), 1500);
        });
      }}
    >
      {copied ? <Check className="size-3 text-success" aria-hidden /> : <Copy className="size-3" aria-hidden />}
    </Button>
  );
}

/**
 * A value that stays compact in a scan view but never hides its full text.
 * Long values shorten in the middle and expand in place; every value has a
 * copy control, and the text remains selectable.
 */
export function ExpandableValue({
  value,
  label,
  max = 44,
  mono = true,
  className,
  keepCopy = true
}: {
  value?: string;
  label: string;
  max?: number;
  mono?: boolean;
  className?: string;
  keepCopy?: boolean;
}) {
  if (!value) return <span className="text-muted">—</span>;

  const shortened = value.length > max;
  return (
    <span className={cn('inline-flex min-w-0 max-w-full items-start gap-1', className)}>
      {shortened ? (
        <details className="group min-w-0">
          <summary
            className="flex min-w-0 cursor-pointer list-none items-center gap-1 rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            aria-label={`${label}: ${value}. Show full value`}
            title={value}
          >
            <span className={cn('min-w-0 [overflow-wrap:anywhere]', mono && 'font-mono text-xs')}>{middleEllipsis(value, max)}</span>
            <span className="shrink-0 text-xs text-accent underline decoration-dotted group-open:hidden">Show full</span>
            <span className="hidden shrink-0 text-xs text-accent underline decoration-dotted group-open:inline">Hide</span>
          </summary>
          <span className={cn('mt-1 block whitespace-pre-wrap [overflow-wrap:anywhere] text-foreground', mono && 'font-mono text-xs')}>{value}</span>
        </details>
      ) : (
        <span className={cn('min-w-0 [overflow-wrap:anywhere]', mono && 'font-mono text-xs')}>{value}</span>
      )}
      {keepCopy ? <CopyButton value={value} label={label} /> : null}
    </span>
  );
}
