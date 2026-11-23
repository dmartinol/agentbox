import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { api } from '../api';
import { logSourceNote, normalizeTerminalText } from '../lib/text';

const TAIL_LINES = 200;

/** Read-only tail of a run's agent log, used for runs that cannot attach. */
export function LogSnapshot({ runId, label }: { runId: string; label: string }) {
  const [content, setContent] = useState('');
  const [source, setSource] = useState<string>();
  const [fullContent, setFullContent] = useState<string | null>(null);
  const [showAll, setShowAll] = useState(false);
  const [error, setError] = useState('');
  const [loadingMore, setLoadingMore] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let active = true;
    setContent('');
    setSource(undefined);
    setFullContent(null);
    setShowAll(false);
    setError('');
    void api
      .logs(runId, TAIL_LINES)
      .then(({ logs, logSource }) => {
        if (!active) return;
        setContent(logs);
        setSource(logSource);
      })
      .catch((err: unknown) => {
        if (active) setError((err as Error).message);
      });
    return () => {
      active = false;
    };
  }, [runId]);

  const shown = fullContent ?? content;
  const note = logSourceNote(source);
  useLayoutEffect(() => {
    const element = scrollRef.current;
    if (element) element.scrollTop = element.scrollHeight;
  }, [shown]);

  const showEarlier = async () => {
    setLoadingMore(true);
    setError('');
    try {
      const { logs, logSource } = await api.logs(runId);
      setFullContent(logs);
      setSource(logSource);
      setShowAll(true);
      window.requestAnimationFrame(() => {
        if (scrollRef.current) scrollRef.current.scrollTop = 0;
      });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoadingMore(false);
    }
  };

  const lineCount = content ? content.split('\n').length : 0;
  const mayHaveEarlier = fullContent === null && lineCount >= TAIL_LINES;

  return (
    <div ref={scrollRef} className="min-h-0 flex-1 overflow-auto bg-black p-2 agentbox-scrollbar" aria-label={label}>
      <div className="mb-1 font-mono text-xs text-zinc-500">Run is not attachable · showing the last output</div>
      {mayHaveEarlier || showAll ? (
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2 rounded border border-zinc-700 bg-zinc-900 px-2 py-1">
          <span className="font-mono text-xs text-zinc-400">
            {showAll ? 'Showing the full output' : `Showing the last ${TAIL_LINES} lines`}
          </span>
          {mayHaveEarlier ? (
            <button
              type="button"
              className="rounded border border-zinc-600 px-2 py-0.5 font-mono text-xs text-zinc-200 hover:bg-zinc-800"
              disabled={loadingMore}
              onClick={() => void showEarlier()}
            >
              {loadingMore ? 'Loading earlier output…' : 'Show earlier output'}
            </button>
          ) : null}
        </div>
      ) : null}
      {note ? <div className="mb-1 font-mono text-xs text-zinc-500">{note}</div> : null}
      <pre className="agentbox-output font-mono text-xs leading-relaxed text-zinc-300">
        {error || normalizeTerminalText(shown) || 'No output recorded.'}
      </pre>
    </div>
  );
}
