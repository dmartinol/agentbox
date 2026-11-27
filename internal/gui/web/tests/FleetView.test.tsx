import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/react';
import { FleetView } from '../src/pages/FleetView';
import { mcps, overview, runs, skillEntries } from './fixtures';
import type { Run } from '../src/types';

afterEach(() => {
  vi.unstubAllGlobals();
  cleanup();
});

describe('FleetView', () => {
  it('counts active agents using displayed status', async () => {
    const activeRuns: Run[] = [
      { ...runs[0], id: 'run-working', name: 'working-run', status: 'working' },
      { ...runs[0], id: 'run-waiting', name: 'waiting-run', status: 'waiting' },
      { ...runs[0], id: 'run-ready', name: 'ready-run', status: 'ready', exited: false },
      { ...runs[0], id: 'run-exited', name: 'exited-run', status: 'ready', exited: true },
      { ...runs[0], id: 'run-stopped', name: 'stopped-run', status: 'stopped', exited: true }
    ];
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const key = `${init?.method ?? 'GET'} ${input.toString()}`;
      const responses: Record<string, unknown> = {
        'GET /api/overview': { ...overview, runs: activeRuns },
        'GET /api/mcps': mcps,
        'GET /api/skills': skillEntries
      };
      if (!(key in responses)) throw new Error(`unmocked request ${key}`);
      const body = responses[key];
      return { ok: true, status: 200, json: async () => body };
    }));

    render(<FleetView openLog={vi.fn()} updateLog={vi.fn()} />);

    expect(await screen.findByText('3 active')).toBeInTheDocument();
    for (const [name, label] of [
      ['working-run', 'Working'],
      ['waiting-run', 'Waiting'],
      ['ready-run', 'Ready'],
      ['exited-run', 'Exited'],
      ['stopped-run', 'Stopped']
    ] as const) {
      const row = screen.getByText(name).closest('tr');
      expect(row).not.toBeNull();
      expect(within(row as HTMLElement).getByText(label)).toBeInTheDocument();
    }
  });
});
