import { useEffect, useState } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import type { Run } from '../types';
import { Modal } from './ui/dialog';

export function AgentTerminal({ run, onClose }: { run: Run; onClose: () => void }) {
  const [terminalElement, setTerminalElement] = useState<HTMLDivElement | null>(null);

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
    // The fit addon sizes the terminal to the container, and the container
    // is sized by the modal layout, so every rendered row stays visible.
    // The server sizes the remote pty from the reported dimensions.
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(terminalElement);
    fit.fit();
    terminal.focus();

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(
      `${protocol}//${window.location.host}/api/runs/${encodeURIComponent(run.id)}/attach?cols=${terminal.cols}&rows=${terminal.rows}`
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
    socket.onmessage = (event) => terminal.write(event.data);
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
  }, [run.id, terminalElement]);

  return (
    <Modal
      open
      onOpenChange={(open) => !open && onClose()}
      // Escape belongs to the attached program (exiting a picker, closing a
      // dialog inside the agent session), so it must reach the PTY instead
      // of closing this dialog. Detach with the close button or Ctrl-b d.
      onEscapeKeyDown={(event) => event.preventDefault()}
      title={`Attach · ${run.name || run.id}`}
      description={`${run.agent} agent in ${run.project || 'unassigned project'}`}
      className="w-[min(96vw,72rem)]"
      bodyClassName="max-h-none overflow-y-hidden p-0"
      footer={
        <p className="text-sm text-muted">
          Detach with the close button (Esc is sent to the agent session). Ctrl-b d also detaches without finishing the run.
        </p>
      }
    >
      <div
        ref={setTerminalElement}
        className="h-[64vh] overflow-hidden rounded-lg border border-border bg-black p-2"
        aria-label={`Terminal for ${run.name || run.id}`}
      />
    </Modal>
  );
}
