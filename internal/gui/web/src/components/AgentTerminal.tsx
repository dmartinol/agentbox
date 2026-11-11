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
  const [preview, setPreview] = useState('');
  const connectedRef = useRef(false);

  useEffect(() => {
    connectedRef.current = false;
    setConnected(false);
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
    const socket = new WebSocket(
      `${protocol}//${window.location.host}/api/runs/${encodeURIComponent(run.id)}/attach?${query.toString()}`
    );

    // Terminal input is sent as binary frames; text frames are reserved
    // for control messages such as resize.
    const encoder = new TextEncoder();
    terminal.onData((data) => socket.send(encoder.encode(data)));
    terminal.onResize(({ cols, rows }) => {
      if (socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: 'resize', cols, rows }));
      }
    });
    socket.onmessage = (event) => {
      if (!connectedRef.current) {
        connectedRef.current = true;
        setConnected(true);
      }
      terminal.write(event.data);
    };
    socket.onerror = () => terminal.write('\r\n[AgentBox] terminal connection error\r\n');
    socket.onclose = (event) => {
      if (event.code !== 1000) {
        terminal.write(`\r\n[AgentBox] terminal closed: ${event.reason || event.code}\r\n`);
      }
    };

    const refit = () => fit.fit();
    window.addEventListener('resize', refit);
    let observer: ResizeObserver | undefined;
    if (typeof ResizeObserver !== 'undefined') {
      observer = new ResizeObserver(refit);
      observer.observe(terminalElement);
    }

    return () => {
      window.removeEventListener('resize', refit);
      observer?.disconnect();
      socket.close();
      terminal.dispose();
    };
  }, [run.id, session, terminalElement]);

  const label = session === 'agent' ? `Terminal for ${run.name || run.id}` : `Shell ${session} for ${run.name || run.id}`;
  return (
    <div className="relative flex min-h-0 flex-1 flex-col bg-black">
      <div ref={setTerminalElement} className="min-h-0 flex-1 overflow-hidden p-1" aria-label={label} />
      {!connected ? (
        <div className="pointer-events-none absolute inset-0 overflow-hidden bg-black p-2 font-mono text-xs leading-relaxed text-zinc-400">
          <div className="mb-1 text-zinc-500">{preview ? 'Last output · reattaching…' : 'Attaching…'}</div>
          <pre className="whitespace-pre-wrap break-words">{preview}</pre>
        </div>
      ) : null}
    </div>
  );
}
