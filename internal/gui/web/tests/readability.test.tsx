import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { LogDrawer } from '../src/components/LogDrawer';
import { ExpandableValue } from '../src/components/ui/copy-value';
import { Disclosure } from '../src/components/ui/disclosure';
import { AdvancedPanel } from '../src/pages/setup/AdvancedPanel';
import { normalizeTerminalText } from '../src/lib/text';

function response(value: unknown, status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => value };
}

afterEach(() => {
  vi.unstubAllGlobals();
  cleanup();
});

describe('terminal output normalization', () => {
  it('drops SGR, OSC, and cursor controls while keeping the text', () => {
    const raw = '\u001b]0;window title\u0007\u001b[32mready\u001b[0m\u001b[?2004h\nline two\u001b[K';
    expect(normalizeTerminalText(raw)).toBe('ready\nline two');
  });

  it('resolves carriage-return updates and backspaces', () => {
    expect(normalizeTerminalText('progress 10%\rprogress 100%\n')).toBe('progress 100%\n');
    expect(normalizeTerminalText('abc\bX\n')).toBe('abX\n');
  });
});

describe('log drawer states', () => {
  it('renders ANSI-rich output as readable text', () => {
    render(
      <LogDrawer
        state={{ open: true, title: 'Logs · codex', content: '\u001b[31mred\u001b[0m output\n' }}
        onClose={() => undefined}
      />
    );
    expect(screen.getByText(/red output/)).toBeInTheDocument();
    expect(document.body.textContent).not.toContain('\u001b');
  });

  it('distinguishes loading, empty, and error states', () => {
    const { rerender } = render(
      <LogDrawer state={{ open: true, title: 'Build image', content: '', loading: true }} onClose={() => undefined} />
    );
    expect(screen.getByText('Loading output…')).toBeInTheDocument();

    rerender(<LogDrawer state={{ open: true, title: 'Build image', content: '', loading: false }} onClose={() => undefined} />);
    expect(screen.getByText('No output recorded.')).toBeInTheDocument();

    rerender(
      <LogDrawer
        state={{ open: true, title: 'Logs · codex', content: '', loading: false, error: 'log read failed' }}
        onClose={() => undefined}
      />
    );
    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('Unable to load output');
    expect(alert).toHaveTextContent('log read failed');
    expect(screen.queryByText('No output recorded.')).toBeNull();
  });
});

describe('detail reveal', () => {
  it('shortens long values but exposes the full text and a copy action', () => {
    const value = 'localhost/openshell:codex-1.0.0-r3-with-a-very-long-suffix';
    render(<ExpandableValue value={value} label="image reference" max={20} />);
    expect(screen.getByText('Show full')).toBeInTheDocument();
    expect(screen.getByText(value)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy image reference' })).toBeInTheDocument();
    expect(screen.getByText(/…/)).toBeInTheDocument();
  });

  it('keeps secondary content behind a native disclosure', async () => {
    const user = userEvent.setup();
    render(
      <Disclosure summary="Resolved configuration">
        <p>Image defaults</p>
      </Disclosure>
    );
    const details = screen.getByText('Resolved configuration').closest('details');
    expect(details).not.toHaveAttribute('open');
    await user.click(screen.getByText('Resolved configuration'));
    expect(details).toHaveAttribute('open');
    expect(screen.getByText('Image defaults')).toBeInTheDocument();
  });
});

describe('doctor readability', () => {
  it('summarizes failing checks and exposes complete output with an action', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = input.toString();
      if (url === '/api/settings') return response({ version: 1, worktrees: { root: '/trees', branch_prefix: 'agentbox/' } });
      if (url === '/api/doctor') {
        return response([
          { name: 'git', ok: true, detail: '/usr/bin/git' },
          { name: 'gateway', ok: false, detail: '{"status":"down","endpoint":"https://localhost:17670","path":"/very/long/path/to/the/gateway/configuration/file.yaml"}' }
        ]);
      }
      throw new Error(`unmocked request ${url}`);
    }));

    render(<AdvancedPanel />);
    expect(await screen.findByText('1 of 2 checks failed')).toBeInTheDocument();
    const gateway = screen.getByText('gateway').closest('details');
    if (!gateway) throw new Error('gateway check missing');
    expect(within(gateway).getByText('Full check output')).toBeInTheDocument();
    expect(within(gateway).getByText(/"status": "down"/)).toBeInTheDocument();
    expect(within(gateway).getByText('Action: Restart OpenShell with `brew services restart openshell`.')).toBeInTheDocument();
    expect(within(gateway).getByRole('button', { name: 'Copy gateway output' })).toBeInTheDocument();
    expect(screen.getByText('OK')).toBeInTheDocument();
    expect(screen.getByText('Fail')).toBeInTheDocument();
  });
});
