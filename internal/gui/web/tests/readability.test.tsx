import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { LogDrawer } from '../src/components/LogDrawer';
import { LogSnapshot } from '../src/components/LogSnapshot';
import { ExpandableValue } from '../src/components/ui/copy-value';
import { Disclosure } from '../src/components/ui/disclosure';
import { AdvancedPanel } from '../src/pages/setup/AdvancedPanel';
import { logSourceNote, normalizeTerminalText } from '../src/lib/text';

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

  it('strips synchronized output, cursor addressing, and kitty key reports', () => {
    const raw = [
      '\u001b[?2026h\u001b[39m\u001b[49m\u001b[0m\u001b[37;3H\u001b[?25h',
      '\u001b[?2026l\u001b]0;\u28b9 hey | project\u0007',
      '\u001b[?2026h\u001b[2K\u001b[10;1H\u001b[2m>_ OpenAI Codex\u001b[22m\u001b[K',
      '\u001b[?u\u001b[>1u\u001b[<u\u001b]7501;state=done\u001b\\'
    ].join('');
    expect(normalizeTerminalText(raw)).toBe('>_ OpenAI Codex');
  });

  it('strips device-control and charset escapes left by a raw stream', () => {
    expect(normalizeTerminalText('\u001bP1$r0m\u001b\\ready\u001b(B\n')).toBe('ready\n');
  });

  it('explains fallback log sources instead of presenting them as normal output', () => {
    expect(logSourceNote('screen')).toBe('');
    expect(logSourceNote('snapshot')).toMatch(/agent session has ended/);
    expect(logSourceNote('stream')).toMatch(/may be garbled/);
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

  it('shows the latest 200 lines and reveals earlier output on request', async () => {
    const user = userEvent.setup();
    const lines = ['needle-start', ...Array.from({ length: 448 }, (_, index) => `middle ${index}`), 'needle-end'];
    render(<LogDrawer state={{ open: true, title: 'Logs · codex', content: lines.join('\n') }} onClose={() => undefined} />);

    expect(screen.queryByText(/needle-start/)).toBeNull();
    expect(screen.getByText(/needle-end/)).toBeInTheDocument();
    expect(screen.getByText(`${lines.length - 200} earlier line(s) hidden`)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Show earlier output' }));
    expect(screen.getByText(/needle-start/)).toBeInTheDocument();
    expect(screen.getByText('Showing the full output')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Collapse to last 200 lines' }));
    expect(screen.queryByText(/needle-start/)).toBeNull();
  });

  it('follows new output while pinned to the bottom', async () => {
    const first = Array.from({ length: 30 }, (_, index) => `line ${index}`);
    const { rerender } = render(
      <LogDrawer state={{ open: true, title: 'Logs · codex', content: first.join('\n') }} onClose={() => undefined} />
    );
    const scroller = screen.getByText(/line 29/).closest('div[class*="overflow-auto"]') as HTMLElement;
    if (!scroller) throw new Error('scroll container missing');
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 2000 });
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 400 });

    rerender(
      <LogDrawer
        state={{ open: true, title: 'Logs · codex', content: [...first, 'line 30', 'line 31'].join('\n') }}
        onClose={() => undefined}
      />
    );

    await waitFor(() => expect(scroller.scrollTop).toBe(2000));
    expect(screen.queryByRole('button', { name: 'Jump to latest' })).toBeNull();
  });

  it('tails a live run by refreshing its rendered screen', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response({ logs: 'rendered screen\n', logSource: 'screen' }));
    vi.stubGlobal('fetch', fetchMock);
    render(
      <LogDrawer
        state={{ open: true, title: 'Logs · codex', content: 'opened snapshot\n', runId: 'run-9', runStatus: 'working' }}
        onClose={() => undefined}
      />
    );
    expect(await screen.findByText(/rendered screen/)).toBeInTheDocument();
    expect(fetchMock.mock.calls.at(-1)?.[0]).toBe('/api/runs/run-9/logs?tail=400');
  });

  it('does not poll a completed run', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response({ logs: 'rendered screen\n' }));
    vi.stubGlobal('fetch', fetchMock);
    render(
      <LogDrawer
        state={{ open: true, title: 'Logs · codex', content: 'finished output\n', runId: 'run-9', runStatus: 'completed' }}
        onClose={() => undefined}
      />
    );
    expect(screen.getByText(/finished output/)).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('shows raw-stream fallback output with an explanation', () => {
    render(
      <LogDrawer
        state={{ open: true, title: 'Logs · codex', content: 'garbled\n', logSource: 'stream' }}
        onClose={() => undefined}
      />
    );
    expect(screen.getByText(/may be garbled/)).toBeInTheDocument();
  });

  it('offers jump-to-latest once the log is scrolled away from the bottom', async () => {
    const user = userEvent.setup();
    render(
      <LogDrawer
        state={{ open: true, title: 'Logs · codex', content: ['first', 'second', 'third'].join('\n') }}
        onClose={() => undefined}
      />
    );
    expect(screen.queryByRole('button', { name: 'Jump to latest' })).toBeNull();
    const scroller = screen.getByText(/third/).closest('div[class*="overflow-auto"]');
    if (!scroller) throw new Error('scroll container missing');
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 1000 });
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 });
    (scroller as HTMLElement).scrollTop = 0;
    fireEvent.scroll(scroller);

    const jump = await screen.findByRole('button', { name: 'Jump to latest' });
    await user.click(jump);
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Jump to latest' })).toBeNull());
  });
});

describe('log snapshot tail', () => {
  it('loads the tail and fetches the full log on request', async () => {
    const user = userEvent.setup();
    const tailTail = Array.from({ length: 200 }, (_, index) => `tail ${index}`);
    tailTail[0] = 'snapshot-start';
    tailTail[199] = 'snapshot-end';
    const full = ['full-start', ...tailTail];
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(response({ logs: tailTail.join('\n') }))
      .mockResolvedValueOnce(response({ logs: full.join('\n') }));
    vi.stubGlobal('fetch', fetchMock);

    render(<LogSnapshot runId="run-1" label="Agent log" />);
    expect(await screen.findByText(/snapshot-end/)).toBeInTheDocument();
    expect(screen.queryByText(/full-start/)).toBeNull();

    await user.click(screen.getByRole('button', { name: 'Show earlier output' }));
    expect(await screen.findByText(/full-start/)).toBeInTheDocument();
    expect(screen.getByText('Showing the full output')).toBeInTheDocument();
    expect(fetchMock.mock.calls.at(-1)?.[0]).toBe('/api/runs/run-1/logs');
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
