import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App } from '../src/App';
import { ProvidersPanel } from '../src/pages/setup/ProvidersPanel';
import { ProfilesPanel } from '../src/pages/setup/ProfilesPanel';
import { overview, providers, runs } from './fixtures';

function jsonResponse(value: unknown, status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => value };
}

function mockRoutes(routes: Record<string, unknown>) {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = input.toString();
    const method = init?.method ?? 'GET';
    const key = `${method} ${url}`;
    if (!(key in routes)) throw new Error(`unmocked request ${key}`);
    const route = routes[key];
    if (route && typeof route === 'object' && 'mockStatus' in route) {
      const response = route as { mockStatus: number; body: unknown };
      return jsonResponse(response.body, response.mockStatus);
    }
    return jsonResponse(route);
  }));
}

beforeEach(() => {
  mockRoutes({
    'GET /api/overview': overview,
    'GET /api/doctor': overview.doctor,
    'GET /api/runs': runs,
    'GET /api/profiles': overview.profiles,
    'GET /api/providers': providers,
    'GET /api/providers?gateway=openshell': providers,
    'GET /api/gateways': overview.gateways,
    'GET /api/images': overview.images,
    'GET /api/registry': { reference: 'quay.io/team/openshell' },
    'GET /api/runs/run-active/logs': { logs: 'agent log output' },
    'GET /api/runs/run-active/diff': { diff: 'diff --git a/file b/file' },
    'GET /api/runs/run-active/policy': { effective: { version: 1 } },
    'POST /api/providers/model-gateway/update': { ok: true },
    'POST /api/providers/unused-gateway/delete?gateway=openshell': { deleted: true }
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  cleanup();
});

describe('fleet shell', () => {
  it('shows the compact project-grouped fleet and doctor state', async () => {
    render(<App />);
    expect(await screen.findByText('Doctor OK')).toBeInTheDocument();
    expect((await screen.findAllByText('demo')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('active-run').length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: /new agent/i })).toBeInTheDocument();
  });

  it('opens agent logs in the bottom drawer', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /logs/i }))[0]);
    expect(await screen.findByText(/Logs · active-run/i)).toBeInTheDocument();
    expect(await screen.findByText('agent log output')).toBeInTheDocument();
  });

  it('shows selected agent metadata', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByText('active-run'));
    expect(await screen.findByText('Agent type')).toBeInTheDocument();
    const metadata = screen.getByText('Agent type').closest('dl');
    expect(metadata).not.toBeNull();
    expect(within(metadata as HTMLElement).getByText('agentbox/demo/active-run')).toBeInTheDocument();
  });
});

describe('setup sheet', () => {
  it('offers all setup surfaces in one compact sheet', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /setup/i }))[0]);
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    for (const label of ['Providers', 'Profiles', 'Projects', 'Images', 'Gateways', 'Policies', 'Advanced']) {
      expect(screen.getByRole('tab', { name: label })).toBeInTheDocument();
    }
    expect(await screen.findByText('model-gateway')).toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Profiles' }));
    expect(await screen.findByText('codex-default')).toBeInTheDocument();
  });
});

describe('provider management', () => {
  it('shows profile usage and blocks deleting a used provider', async () => {
    render(<ProvidersPanel />);
    const usedRow = (await screen.findByText('model-gateway')).closest('tr');
    if (!usedRow) throw new Error('provider row missing');
    expect(within(usedRow).getByText('codex-default')).toBeInTheDocument();
    expect(within(usedRow).getByRole('button', { name: 'Delete' })).toBeDisabled();
  });

  it('edits a single-credential provider', async () => {
    const user = userEvent.setup();
    render(<ProvidersPanel />);
    const row = (await screen.findByText('model-gateway')).closest('tr');
    if (!row) throw new Error('provider row missing');
    await user.click(within(row).getByRole('button', { name: 'Edit' }));
    const credential = await screen.findByRole('textbox', { name: /Credential env/i });
    expect(credential).toHaveValue('MY_GATEWAY_KEY');
    await user.clear(credential);
    await user.type(credential, 'NEW_MODEL_KEY');
    await user.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Provider model-gateway updated')).toBeInTheDocument();
  });

  it('deletes an unused provider after confirmation', async () => {
    const user = userEvent.setup();
    render(<ProvidersPanel />);
    const row = (await screen.findByText('unused-gateway')).closest('tr');
    if (!row) throw new Error('provider row missing');
    await user.click(within(row).getByRole('button', { name: 'Delete' }));
    await user.click(await screen.findByRole('button', { name: 'Delete' }));
    expect(await screen.findByText('Provider unused-gateway deleted')).toBeInTheDocument();
  });
});

describe('profiles', () => {
  it('creates a profile using configured providers', async () => {
    const user = userEvent.setup();
    render(<ProfilesPanel />);
    await user.click(await screen.findByRole('button', { name: 'Create profile' }));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument();
  });
});

describe('image builds', () => {
  it('streams build output and exposes stop while the task runs', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input.toString();
      const method = init?.method ?? 'GET';
      if (url === '/api/gateways' && method === 'GET') return jsonResponse(overview.gateways);
      if (url === '/api/providers?gateway=openshell' && method === 'GET') return jsonResponse(providers);
      if (url === '/api/images' && method === 'GET') return jsonResponse(overview.images);
      if (url === '/api/registry' && method === 'GET') return jsonResponse({ reference: 'quay.io/team/openshell' });
      if (url === '/api/images/build' && method === 'POST') {
        return jsonResponse({ id: 'build-task', status: 'running', output: 'build line one\n' });
      }
      if (url === '/api/tasks/build-task' && method === 'GET') {
        return jsonResponse({ id: 'build-task', status: 'running', output: 'build line one\nbuild line two\n' });
      }
      if (url === '/api/tasks/build-task/stop' && method === 'POST') {
        return jsonResponse({ id: 'build-task', status: 'stopped', output: 'build line one\nbuild line two\nbuild stopped\n' });
      }
      throw new Error(`unmocked image build request ${method} ${url}`);
    }));

    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /setup/i }))[0]);
    await user.click(await screen.findByRole('tab', { name: 'Images' }));
    await user.click(await screen.findByRole('button', { name: 'Build image' }));
    await user.click(await screen.findByRole('button', { name: /^Build$/ }));
    expect(await screen.findByText(/Build image · codex/i)).toBeInTheDocument();
    expect(await screen.findByText(/build line one/i)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText(/build line two/i)).toBeInTheDocument(), { timeout: 1800 });
    await user.click(screen.getByRole('button', { name: /stop/i }));
    await waitFor(() => expect(screen.getByText(/build stopped/i)).toBeInTheDocument());
  });
});

describe('provider setup', () => {
  it('prefills direct OpenAI defaults', async () => {
    const user = userEvent.setup();
    render(<ProvidersPanel />);
    await user.click(await screen.findByRole('button', { name: 'Set up provider' }));
    const dialog = await screen.findByRole('dialog');
    await user.selectOptions(within(dialog).getByLabelText(/^Type/), 'openai');
    expect(within(dialog).getByLabelText(/^Host or base URL/)).toHaveValue('api.openai.com');
    expect(within(dialog).getByLabelText(/^Credential env/)).toHaveValue('OPENAI_API_KEY');
    expect(within(dialog).getByLabelText(/^Host or base URL/)).toBeDisabled();
    expect(within(dialog).getByLabelText(/^Credential env/)).toBeDisabled();
  });
});
