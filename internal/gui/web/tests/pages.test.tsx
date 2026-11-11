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
    const value = typeof route === 'function' ? await (route as () => unknown)() : route;
    if (value && typeof value === 'object' && 'mockStatus' in value) {
      const response = value as { mockStatus: number; body: unknown };
      return jsonResponse(response.body, response.mockStatus);
    }
    return jsonResponse(value);
  }));
}

function defaultRoutes(): Record<string, unknown> {
  return {
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
  };
}

beforeEach(() => {
  mockRoutes(defaultRoutes());
});

afterEach(() => {
  vi.unstubAllGlobals();
  window.localStorage.clear();
  cleanup();
});

describe('fleet shell', () => {
  it('shows the compact project-grouped fleet and doctor state', async () => {
    render(<App />);
    expect(await screen.findByText('Doctor OK')).toBeInTheDocument();
    expect((await screen.findAllByText('demo')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('active-run').length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: /new agent/i })).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: /setup/i })).toHaveLength(1);
  });

  it('defaults to light mode and remembers the selected theme', async () => {
    const user = userEvent.setup();
    render(<App />);
    expect(document.documentElement).toHaveClass('theme-light');
    await user.click(screen.getByRole('button', { name: 'Switch to dark theme' }));
    expect(document.documentElement).not.toHaveClass('theme-light');
    expect(window.localStorage.getItem('agentbox-theme')).toBe('dark');
    expect(screen.getByRole('button', { name: 'Switch to light theme' })).toBeInTheDocument();
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
    expect(screen.getByRole('tab', { name: 'Providers', selected: true })).toHaveClass(
      'data-[state=active]:bg-accent',
      'data-[state=active]:text-accent-foreground'
    );
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

describe('fleet actions', () => {
  it('marks a sync in flight until it settles', async () => {
    let resolveSync!: (value: unknown) => void;
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/runs/run-active/sync': () => new Promise((resolve) => { resolveSync = resolve; })
    });
    const user = userEvent.setup();
    render(<App />);
    const row = (await screen.findByText('active-run')).closest('tr');
    if (!row) throw new Error('agent row missing');
    await user.click(within(row).getByRole('button', { name: 'Sync' }));
    expect(within(row).getByRole('button', { name: 'Syncing' })).toBeDisabled();
    expect(within(row).getByText('Syncing', { selector: 'td' })).toBeInTheDocument();
    resolveSync(runs[0]);
    await waitFor(() => expect(within(row).getByRole('button', { name: 'Sync' })).toBeEnabled());
    expect(await screen.findByText('Sync requested for active-run')).toBeInTheDocument();
  });

  it('marks a stop in flight until it settles', async () => {
    let resolveStop!: (value: unknown) => void;
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/runs/run-active/stop': () => new Promise((resolve) => { resolveStop = resolve; })
    });
    const user = userEvent.setup();
    render(<App />);
    const row = (await screen.findByText('active-run')).closest('tr');
    if (!row) throw new Error('agent row missing');
    await user.click(within(row).getByRole('button', { name: 'Stop' }));
    expect(within(row).getByRole('button', { name: 'Stopping' })).toBeDisabled();
    expect(within(row).getByText('Stopping', { selector: 'span' })).toBeInTheDocument();
    resolveStop(runs[0]);
    await waitFor(() => expect(within(row).getByText('Running')).toBeInTheDocument());
  });

  it('marks a new agent start in flight', async () => {
    let resolveStart!: (value: unknown) => void;
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/runs': () => new Promise((resolve) => { resolveStart = resolve; })
    });
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'New agent' }));
    await user.click(await screen.findByRole('button', { name: 'Start' }));
    expect(screen.getByRole('button', { name: 'Starting' })).toBeDisabled();
    resolveStart(runs[0]);
    expect(await screen.findByText('Agent active-run started')).toBeInTheDocument();
    expect(await screen.findByText('Agent type')).toBeInTheDocument();
  });

  it('selects a started agent so its details stay visible', async () => {
    let resolveStart!: (value: unknown) => void;
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/runs': () => new Promise((resolve) => { resolveStart = resolve; })
    });
    const user = userEvent.setup();
    render(<App />);
    expect(screen.queryByText('Agent type')).toBeNull();
    await user.click(screen.getByRole('button', { name: 'New agent' }));
    await user.click(await screen.findByRole('button', { name: 'Start' }));
    resolveStart(runs[0]);
    const sidebar = await screen.findByText('Agent type');
    const metadata = sidebar.closest('dl');
    if (!metadata) throw new Error('agent metadata missing');
    await waitFor(() => expect(within(metadata).getByText('agentbox/demo/active-run')).toBeInTheDocument());
  });

  it('prefills a new agent from a copied run', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByText('active-run'));
    await user.click(screen.getByRole('button', { name: 'Copy agent' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Copy active-run')).toBeInTheDocument();
    expect(within(dialog).getByLabelText(/^Project/)).toHaveValue('demo');
    expect(within(dialog).getByLabelText(/^Profile/)).toHaveValue('codex-default');
    expect(within(dialog).getByLabelText(/^Gateway/)).toHaveValue('openshell');
    expect(within(dialog).getByLabelText('Override image, model, and approval')).toBeChecked();
    expect(within(dialog).getByLabelText(/^Image/)).toHaveValue('localhost/openshell:codex-1.0.0-r3');
    expect(within(dialog).getByLabelText(/^Model/)).toHaveValue('model-x');
    expect(within(dialog).getByLabelText(/^Approval/)).toHaveValue('unattended');
  });

  it('opens a fresh form after copying', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByText('active-run'));
    await user.click(screen.getByRole('button', { name: 'Copy agent' }));
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    await user.click(screen.getByRole('button', { name: 'New agent' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('New agent')).toBeInTheDocument();
    expect(within(dialog).queryByLabelText(/^Image/)).toBeNull();
    expect(within(dialog).getByLabelText('Override image, model, and approval')).not.toBeChecked();
  });

  it('requires confirmation before deleting an agent', async () => {
    let deleteRequested = false;
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/runs/run-active/delete': () => {
        deleteRequested = true;
        return { deleted: true };
      }
    });
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByText('active-run'));
    await user.click(screen.getByRole('button', { name: 'Delete agent' }));
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('Delete active-run')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(deleteRequested).toBe(false);
  });

  it('marks an agent Deleting while deletion is in flight', async () => {
    let resolveDelete!: (value: unknown) => void;
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/runs/run-active/delete': () => new Promise((resolve) => { resolveDelete = resolve; })
    });
    const user = userEvent.setup();
    render(<App />);
    const row = (await screen.findByText('active-run')).closest('tr');
    if (!row) throw new Error('agent row missing');
    await user.click(await screen.findByText('active-run'));
    await user.click(screen.getByRole('button', { name: 'Delete agent' }));
    await user.click(screen.getByRole('button', { name: 'Delete' }));
    expect(within(row).getByText('Deleting', { selector: 'span' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Deleting agent' })).toBeDisabled();
    resolveDelete({ deleted: true });
    expect(await screen.findByText('Agent deleted')).toBeInTheDocument();
  });
});

describe('image pulls', () => {
  it('marks an image pull in flight', async () => {
    let resolvePull!: (value: unknown) => void;
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/images/pull': () => new Promise((resolve) => { resolvePull = resolve; })
    });
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /setup/i }))[0]);
    await user.click(await screen.findByRole('tab', { name: 'Images' }));
    await user.click(await screen.findByRole('button', { name: 'Pull image' }));
    await user.click(await screen.findByRole('button', { name: 'Pull' }));
    expect(screen.getByRole('button', { name: 'Pulling' })).toBeDisabled();
    resolvePull({ ok: true });
    expect(await screen.findByText('Image pull requested')).toBeInTheDocument();
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
