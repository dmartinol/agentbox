import { useEffect, useState } from 'react';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import type { Run } from '../types';
import { Modal } from './ui/dialog';

export function AgentTerminal({ run, onClose }: { run: Run; onClose: () => void }) {
  const [terminalElement, setTerminalElement] = useState<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!terminalElement) return;

    const terminal = new Terminal({
      cols: 120,
      rows: 36,
      cursorBlink: true,
      fontSize: 13,
      theme: {
        background: '#09090b',
        foreground: '#f4f4f5',
        cursor: '#f4f4f5'
      }
    });
    terminal.open(terminalElement);
    terminal.focus();

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(`${protocol}//${window.location.host}/api/runs/${encodeURIComponent(run.id)}/attach`);

    terminal.onData((data) => socket.send(data));
    socket.onmessage = (event) => terminal.write(event.data);
    socket.onerror = () => terminal.write('\r\n[AgentBox] terminal connection error\r\n');
    socket.onclose = (event) => {
      if (event.code !== 1000) {
        terminal.write(`\r\n[AgentBox] terminal closed: ${event.reason || event.code}\r\n`);
      }
    };

    return () => {
      socket.close();
      terminal.dispose();
    };
  }, [run.id, terminalElement]);

  return (
    <Modal
      open
      onOpenChange={(open) => !open && onClose()}
      title={`Attach · ${run.name || run.id}`}
      description={`${run.agent} agent in ${run.project || 'unassigned project'}`}
      className="w-[min(96vw,72rem)]"
      footer={
        <p className="text-sm text-muted">
          Detach with the close button. In the agent session, Ctrl-b d also detaches without finishing the run.
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
