import { useEffect, useState } from 'react';
import { api } from '../api';

/** Read-only tail of a run's agent log, used for runs that cannot attach. */
export function LogSnapshot({ runId, label }: { runId: string; label: string }) {
  const [content, setContent] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    setContent('');
    setError('');
    void api
      .logs(runId, 400)
      .then(({ logs }) => {
        if (active) setContent(logs);
      })
      .catch((err: unknown) => {
        if (active) setError((err as Error).message);
      });
    return () => {
      active = false;
    };
  }, [runId]);

  return (
    <div className="min-h-0 flex-1 overflow-auto bg-black p-2 agentbox-scrollbar" aria-label={label}>
      <div className="mb-1 font-mono text-xs text-zinc-500">Run is not attachable · showing the last output</div>
      <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-zinc-300">
        {error || content || 'No output recorded.'}
      </pre>
    </div>
  );
}
