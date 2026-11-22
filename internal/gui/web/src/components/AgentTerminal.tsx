import { useEffect, useRef, useState } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { api } from '../api';
import type { Run } from '../types';

/**
 * A single live terminal attached to one tmux session inside a run's
 * sandbox. Inactive workspace tabs unmount it, so only the visible tab
 * holds a pty; switching back shows the cached log tail while the
 * session reattaches.
 */
export function AgentTerminal({ run, session = 'agent' }: { run: Run; session?: string }) {
  const [terminalElement, setTerminalElement] = useState<HTMLDivElement | null>(null);
  const [connected, setConnected] = useState(false);
  const [phase, setPhase] = useState<'attaching' | 'connected' | 'reconnecting' | 'closed'>('attaching');
  const [preview, setPreview] = useState('');
  const connectedRef = useRef(false);

  useEffect(() => {
    connectedRef.current = false;
    setConnected(false);
    setPhase('attaching');
    setPreview('');
    if (session !== 'agent') return;
    let active = true;
    void api
      .logs(run.id, 200)
      .then(({ logs }) => {
        if (active) setPreview(logs);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [run.id, session]);

  useEffect(() => {
    if (!terminalElement) return;

    const terminal = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      theme: {
        background: '#09090b',
        foreground: '#f4f4f5',
        cursor: '#f4f4f5'
      }
    });
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(terminalElement);
    fit.fit();
    terminal.focus();

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const query = new URLSearchParams({ cols: String(terminal.cols), rows: String(terminal.rows) });
    if (session !== 'agent') query.set('session', session);
    const url = `${protocol}//${window.location.host}/api/runs/${encodeURIComponent(run.id)}/attach?${query.toString()}`;

    // The GUI can restart while a terminal pane is open. A dropped socket
    // closes with 1006 and no reason, so keep retrying instead of leaving a
    // dead pane that only a page reload can recover.
    let socket: WebSocket | undefined;
    let retryTimer: number | undefined;
    let attempt = 0;
    let disposed = false;

    const connect = () => {
      if (disposed) return;
      const next = new WebSocket(url);
      // Terminal output arrives as binary frames because a pty chunk can split
      // a multibyte character. Reading them as text makes the browser drop the
      // socket with a reasonless 1006 close.
      next.binaryType = 'arraybuffer';
      socket = next;
      next.onmessage = (event) => {
        attempt = 0;
        if (!connectedRef.current) {
          connectedRef.current = true;
          setConnected(true);
        }
        setPhase('connected');
        terminal.write(typeof event.data === 'string' ? event.data : new Uint8Array(event.data as ArrayBuffer));
      };
      next.onerror = () => {
        if (!disposed) terminal.write('\r\n[AgentBox] terminal connection error\r\n');
      };
      next.onclose = (event) => {
        if (disposed) return;
        connectedRef.current = false;
        setConnected(false);
        if (event.code === 1000) {
          setPhase('closed');
          return;
        }
        // A close frame with a reason is a deliberate server refusal, such as
        // an unreachable sandbox. Retrying cannot fix it and would spawn a
        // stream for every attempt, so show the reason and stop.
        if (event.reason) {
          setPhase('closed');
          terminal.write(`\r\n[AgentBox] terminal closed: ${event.reason}\r\n`);
          return;
        }
        if (attempt >= 6) {
          setPhase('closed');
          terminal.write(`\r\n[AgentBox] terminal closed: ${event.code}; gave up after 6 attempts\r\n`);
          return;
        }
        attempt += 1;
        const delay = Math.min(500 * 2 ** (attempt - 1), 8000);
        setPhase('reconnecting');
        terminal.write(`\r\n[AgentBox] terminal closed: ${event.reason || event.code}; reconnecting in ${delay / 1000}s\r\n`);
        retryTimer = window.setTimeout(connect, delay);
      };
    };

    // Terminal input is sent as binary frames; text frames are reserved
    // for control messages such as resize.
    const encoder = new TextEncoder();
    terminal.onData((data) => {
      if (socket?.readyState === WebSocket.OPEN) socket.send(encoder.encode(data));
    });
    terminal.onResize(({ cols, rows }) => {
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: 'resize', cols, rows }));
      }
    });
    connect();

    const refit = () => fit.fit();
    window.addEventListener('resize', refit);
    let observer: ResizeObserver | undefined;
    if (typeof ResizeObserver !== 'undefined') {
      observer = new ResizeObserver(refit);
      observer.observe(terminalElement);
    }

    return () => {
      disposed = true;
      window.clearTimeout(retryTimer);
      window.removeEventListener('resize', refit);
      observer?.disconnect();
      socket?.close();
      terminal.dispose();
    };
  }, [run.id, session, terminalElement]);

  const label = session === 'agent' ? `Terminal for ${run.name || run.id}` : `Shell ${session} for ${run.name || run.id}`;
  return (
    <div className="relative flex min-h-0 flex-1 flex-col bg-black">
      <div ref={setTerminalElement} className="min-h-0 flex-1 overflow-hidden p-1" aria-label={label} />
      {!connected ? (
        phase === 'reconnecting' ? (
          <div className="pointer-events-none absolute right-2 top-2 rounded bg-amber-500/15 px-2 py-1 font-mono text-xs text-amber-200">
            Connection lost · reconnecting…
          </div>
        ) : phase === 'closed' ? (
          <div className="pointer-events-none absolute right-2 top-2 rounded bg-zinc-500/15 px-2 py-1 font-mono text-xs text-zinc-300">
            Terminal disconnected
          </div>
        ) : (
          <div className="pointer-events-none absolute inset-0 overflow-hidden bg-black p-2 font-mono text-xs leading-relaxed text-zinc-400">
            <div className="mb-1 text-zinc-500">{preview ? 'Last output · reattaching…' : 'Attaching…'}</div>
            <pre className="whitespace-pre-wrap break-words">{preview}</pre>
          </div>
        )
      ) : null}
    </div>
  );
}
