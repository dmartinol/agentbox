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
    await expect(api.stopRun('missing')).rejects.toThrow('project not found');
  });
});
