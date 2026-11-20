import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api';

afterEach(() => vi.unstubAllGlobals());

function mockFetch(response: Response | { status: number; body: unknown }) {
  vi.stubGlobal('fetch', vi.fn(async () => {
    if (response instanceof Response) return response;
    return { ok: response.status >= 200 && response.status < 300, status: response.status, json: async () => response.body };
  }));
}

describe('API client', () => {
  it('posts JSON and unwraps successful responses', async () => {
    mockFetch({ status: 201, body: { id: 'run-new' } });
    await expect(api.startRun({ project: 'demo', profile: 'codex', gateway: 'openshell' })).resolves.toEqual({ id: 'run-new' });
    const calls = (fetch as unknown as { mock: { calls: unknown[][] } }).mock.calls;
    const init = calls[0][1] as RequestInit;
    expect(init.method).toBe('POST');
    expect(init.headers).toMatchObject({ 'Content-Type': 'application/json' });
  });

  it('surfaces API error messages', async () => {
    mockFetch({ status: 400, body: { error: 'project not found' } });
    await expect(api.sleepRun('missing')).rejects.toThrow('project not found');
  });

  it('posts lifecycle actions to their own routes', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL) => ({ ok: true, status: 200, json: async () => ({ id: 'run-1' }) }));
    vi.stubGlobal('fetch', fetchMock);
    await api.sleepRun('run-1');
    await api.resumeRun('run-1');
    await api.finishRun('run-1');
    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      '/api/runs/run-1/sleep',
      '/api/runs/run-1/resume',
      '/api/runs/run-1/finish'
    ]);
  });
});

  it('issues guarded resource lifecycle requests', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => ({ ok: true, status: 200, json: async () => ({ deleted: true }) }));
    vi.stubGlobal('fetch', fetchMock);
    await api.deleteProject('demo');
    await api.deleteProfile('codex');
    await api.deleteGateway('openshell');
    await api.clearRegistry();
    const paths = fetchMock.mock.calls.map((call) => [call[0], (call[1] as RequestInit).method]);
    expect(paths).toEqual([
      ['/api/projects/demo/delete', 'POST'],
      ['/api/profiles/codex/delete', 'POST'],
      ['/api/gateways/openshell/delete', 'POST'],
      ['/api/registry/clear', 'POST']
    ]);
  });
