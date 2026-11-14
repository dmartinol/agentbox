import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App } from '../src/App';
import { ProvidersPanel } from '../src/pages/setup/ProvidersPanel';
import { ProfilesPanel } from '../src/pages/setup/ProfilesPanel';
import { McpPanel } from '../src/pages/setup/McpPanel';
import { SkillsPanel } from '../src/pages/setup/SkillsPanel';
import { mcps, overview, providers, runs, skillEntries } from './fixtures';

const xtermHooks = vi.hoisted(() => ({
  resize: undefined as undefined | ((size: { cols: number; rows: number }) => void)
}));

vi.mock('@xterm/xterm', () => ({
  Terminal: class MockTerminal {
    cols = 120;
    rows = 36;
    open() {}
    focus() {}
    write() {}
    dispose() {}
    onData() {}
    loadAddon() {}
    onResize(callback: (size: { cols: number; rows: number }) => void) {
      xtermHooks.resize = callback;
    }
  }
}));

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class MockFitAddon {
    fit() {}
    proposeDimensions() {
      return undefined;
    }
    dispose() {}
  }
}));


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
    'GET /api/mcps': mcps,
    'GET /api/skills': skillEntries,
    'GET /api/doctor': overview.doctor,
    'GET /api/runs': runs,
    'GET /api/profiles': overview.profiles,
    'GET /api/providers': providers,
    'GET /api/providers?gateway=openshell': providers,
    'GET /api/providers/model-gateway/models?gateway=openshell': ['gpt-6-luna', 'model-x'],
    'GET /api/gateways': overview.gateways,
    'GET /api/images': overview.images,
    'GET /api/registry': { reference: 'quay.io/team/openshell' },
    'GET /api/runs/run-active/logs': { logs: 'agent log output' },
    'GET /api/runs/run-active/logs?tail=200': { logs: 'agent log output' },
    'GET /api/runs/run-finished/logs?tail=400': { logs: 'finished output' },
    'GET /api/runs/run-active/shells': [],
    'POST /api/runs/run-active/shells': { name: 'shell-1' },
    'POST /api/runs/run-active/shells/shell-1/delete': { deleted: true },
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

  it('refreshes a starting agent without a page reload', async () => {
    const starting = { ...runs[0], id: 'run-creating', name: 'fresh-run', status: 'creating' as const, status_detail: 'sandbox is warming up' };
    const ready = { ...starting, status: 'ready' as const, status_detail: '' };
    mockRoutes({ ...defaultRoutes(), 'GET /api/overview': { ...overview, runs: [starting] }, 'GET /api/runs': [ready] });
    render(<App />);
    expect((await screen.findAllByText('Creating')).length).toBeGreaterThan(0);
    await waitFor(() => expect(screen.getAllByText('Ready').length).toBeGreaterThan(0), { timeout: 4000 });
  });

  it('counts agents waiting for input as Needs You', async () => {
    const waitingRuns = runs.map((run) => run.id === 'run-idle' ? { ...run, status: 'waiting' as const, status_detail: 'permission requested' } : run);
    mockRoutes({ ...defaultRoutes(), 'GET /api/overview': { ...overview, runs: waitingRuns }, 'GET /api/runs': waitingRuns });
    render(<App />);
    expect(await screen.findByText(/Needs You 1/)).toBeInTheDocument();
    expect((await screen.findAllByLabelText('Waiting for input')).length).toBeGreaterThan(0);
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

  it('refreshes new-agent profile options after creating a profile', async () => {
    const newProfile = { ...overview.profiles[0], name: 'new-codex' };
    let profileCreated = false;
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input.toString();
      const method = init?.method ?? 'GET';
      if (url === '/api/overview' && method === 'GET') {
        return jsonResponse({ ...overview, profiles: profileCreated ? [newProfile] : [] });
      }
      if (url === '/api/mcps' && method === 'GET') return jsonResponse(mcps);
      if (url === '/api/skills' && method === 'GET') return jsonResponse(skillEntries);
      if (url === '/api/doctor' && method === 'GET') return jsonResponse(overview.doctor);
      if (url === '/api/profiles' && method === 'GET') {
        return jsonResponse(profileCreated ? [newProfile] : []);
      }
      if (url === '/api/profiles' && method === 'POST') {
        profileCreated = true;
        return jsonResponse(newProfile);
      }
      if (url === '/api/providers' && method === 'GET') return jsonResponse(providers);
      if (url === '/api/gateways' && method === 'GET') return jsonResponse(overview.gateways);
      if (url === '/api/images' && method === 'GET') return jsonResponse(overview.images);
      throw new Error(`unmocked profile refresh request ${method} ${url}`);
    }));

    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /setup/i }))[0]);
    await user.click(await screen.findByRole('tab', { name: 'Profiles' }));
    await user.click(await screen.findByRole('button', { name: 'Create profile' }));
    await user.type(await screen.findByLabelText(/^Name/), 'new-codex');
    await user.type(screen.getByLabelText('Model'), 'new-model');
    await user.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Profile saved')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Close' }));
    await user.click(screen.getByRole('button', { name: /new agent/i }));
    const profileSelect = await screen.findByLabelText(/^Profile/);
    expect(within(profileSelect).getByRole('option', { name: 'new-codex' })).toBeInTheDocument();
  });

  it('shows start errors inside the new-agent dialog', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input.toString();
      const method = init?.method ?? 'GET';
      if (url === '/api/overview' && method === 'GET') return jsonResponse(overview);
      if (url === '/api/mcps' && method === 'GET') return jsonResponse(mcps);
      if (url === '/api/skills' && method === 'GET') return jsonResponse(skillEntries);
      if (url === '/api/doctor' && method === 'GET') return jsonResponse(overview.doctor);
      if (url === '/api/runs' && method === 'POST') {
        return jsonResponse({ error: 'profile "codex-enmaas" references missing policy file ~/.agentbox/policies/codex-enmaas.yaml; edit the profile or restore the policy' }, 400);
      }
      throw new Error(`unmocked start request ${method} ${url}`);
    }));

    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole('button', { name: /new agent/i }));
    await user.click(await screen.findByRole('button', { name: 'Start' }));
    expect(await screen.findByText('Unable to start agent')).toBeInTheDocument();
    expect(screen.getByText(/references missing policy file/)).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('opens agent logs in the bottom drawer', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /logs/i }))[0]);
    expect(await screen.findByText(/Logs · active-run/i)).toBeInTheDocument();
    expect(await screen.findByText('agent log output')).toBeInTheDocument();
  });

  it('opens an interactive agent terminal from the fleet', async () => {
    const sockets: Array<{ url: string; close: () => void }> = [];
    class MockWebSocket {
      url: string;
      onmessage: ((event: { data: string }) => void) | null = null;
      onerror: (() => void) | null = null;
      onclose: ((event: { code: number; reason: string }) => void) | null = null;

      constructor(url: string) {
        this.url = url;
        sockets.push(this);
      }

      send() {}
      close() {}
    }
    vi.stubGlobal('WebSocket', MockWebSocket);
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /attach/i }))[0]);
    expect(await screen.findByRole('tablist', { name: 'demo agents' })).toBeInTheDocument();
    expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual([
      'active-run',
      'idle-run',
      'finished-run'
    ]);
    expect(await screen.findByLabelText('Terminal for active-run')).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).toBeNull();
    await waitFor(() => expect(sockets).toHaveLength(1));
    expect(sockets[0]?.url).toBe('ws://localhost:3000/api/runs/run-active/attach?cols=120&rows=36');
  });

  it('reports terminal resizes over the attach socket', async () => {
    const sent: unknown[] = [];
    class MockWebSocket {
      static readonly OPEN = 1;
      readyState = 1;
      onmessage: ((event: { data: string }) => void) | null = null;
      onerror: (() => void) | null = null;
      onclose: ((event: { code: number; reason: string }) => void) | null = null;
      send(data: unknown) {
        sent.push(data);
      }
      close() {}
    }
    vi.stubGlobal('WebSocket', MockWebSocket);
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /attach/i }))[0]);
    expect(await screen.findByLabelText('Terminal for active-run')).toBeInTheDocument();
    expect(xtermHooks.resize).toBeTypeOf('function');
    xtermHooks.resize?.({ cols: 100, rows: 30 });
    expect(sent).toContain('{"type":"resize","cols":100,"rows":30}');
  });

  it('keeps the workspace terminal mounted when Escape is pressed', async () => {
    class MockWebSocket {
      onmessage: ((event: { data: string }) => void) | null = null;
      onerror: (() => void) | null = null;
      onclose: ((event: { code: number; reason: string }) => void) | null = null;
      send() {}
      close() {}
    }
    vi.stubGlobal('WebSocket', MockWebSocket);
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /attach/i }))[0]);
    expect(await screen.findByLabelText('Terminal for active-run')).toBeInTheDocument();
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.getByLabelText('Terminal for active-run')).toBeInTheDocument();
  });

  it('shows a read-only snapshot for a run that cannot attach', async () => {
    class MockWebSocket {
      onmessage: ((event: { data: string }) => void) | null = null;
      onerror: (() => void) | null = null;
      onclose: ((event: { code: number; reason: string }) => void) | null = null;
      send() {}
      close() {}
    }
    vi.stubGlobal('WebSocket', MockWebSocket);
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /attach/i }))[0]);
    await user.click(await screen.findByRole('tab', { name: /finished-run/ }));
    expect(await screen.findByLabelText('Output for finished-run')).toBeInTheDocument();
    expect(await screen.findByText('finished output')).toBeInTheDocument();
    expect(screen.queryByLabelText('Terminal for finished-run')).toBeNull();
  });

  it('creates and closes a shell tab for the active agent', async () => {
    const sockets: Array<{ url: string }> = [];
    class MockWebSocket {
      url: string;
      onmessage: ((event: { data: string }) => void) | null = null;
      onerror: (() => void) | null = null;
      onclose: ((event: { code: number; reason: string }) => void) | null = null;

      constructor(url: string) {
        this.url = url;
        sockets.push(this);
      }

      send() {}
      close() {}
    }
    vi.stubGlobal('WebSocket', MockWebSocket);
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /attach/i }))[0]);
    await user.click(await screen.findByRole('button', { name: /shell/i }));
    expect(await screen.findByRole('tab', { name: /active-run · shell-1/i })).toBeInTheDocument();
    await waitFor(() => expect(sockets.at(-1)?.url).toContain('session=shell-1'));
    await user.click(screen.getByRole('button', { name: 'Close shell-1' }));
    await waitFor(() => expect(screen.queryByRole('tab', { name: /shell-1/i })).toBeNull());
    expect(await screen.findByLabelText('Terminal for active-run')).toBeInTheDocument();
  });

  it('still closes other dialogs with Escape', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByText('active-run'));
    await user.click(screen.getByRole('button', { name: 'Delete agent' }));
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
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
    for (const label of ['Providers', 'Profiles', 'Projects', 'Images', 'Gateways', 'Policies', 'MCP Tools', 'Skills', 'Advanced']) {
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

  it('marks built-in providers as non-editable and non-deletable', async () => {
    render(<ProvidersPanel />);
    const row = (await screen.findByText('OPENAI_API_KEY')).closest('tr');
    if (!row) throw new Error('provider row missing');
    expect(within(row).getByText('Built-in')).toBeInTheDocument();
    expect(within(row).getByRole('button', { name: 'Edit' })).toBeDisabled();
    expect(within(row).getByRole('button', { name: 'Delete' })).toBeDisabled();
  });
});

describe('profiles', () => {
  it('uses provider defaults and an agent-filtered image selector', async () => {
    const user = userEvent.setup();
    render(<ProfilesPanel />);
    await user.click(await screen.findByRole('button', { name: 'Create profile' }));
    const dialog = await screen.findByRole('dialog');

    const image = within(dialog).getByLabelText(/^Image/);
    expect(image).toHaveValue('localhost/openshell:codex-1.0.0-r3');
    const imageOptions = within(image).getAllByRole('option');
    expect(imageOptions.map((option) => (option as HTMLOptionElement).value)).toEqual([
      'localhost/openshell:codex-1.0.0-r3',
      'localhost/openshell:codex-0.9.0-r3'
    ]);

    await user.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Gateway mode requires an exact model ID.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Discover' }));
    const model = await screen.findByLabelText('Model');
    await waitFor(() => expect(model).toBeEnabled());
    expect(model).toHaveValue('');
    await user.selectOptions(model, 'gpt-6-luna');
    expect(model).toHaveValue('gpt-6-luna');
    const baseURL = within(dialog).getByLabelText(/^Base URL/);
    const apiKeyEnv = within(dialog).getByLabelText(/^API key env/);
    expect(baseURL).toHaveValue('https://models.example.test/v1');
    expect(apiKeyEnv).toHaveValue('MY_GATEWAY_KEY');
    expect(baseURL).toBeDisabled();
    expect(apiKeyEnv).toBeDisabled();

    await user.selectOptions(within(dialog).getByLabelText(/^Provider/), 'openai');
    expect(within(dialog).getByLabelText(/^Mode\s*\*?$/)).toHaveValue('standard');
    await user.selectOptions(within(dialog).getByLabelText(/^Mode\s*\*?$/), 'gateway');
    await user.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Direct OpenAI and Anthropic providers require standard mode.')).toBeInTheDocument();
    await user.selectOptions(within(dialog).getByLabelText(/^Provider/), 'unused-gateway');
    expect(within(dialog).getByLabelText(/^Mode\s*\*?$/)).toHaveValue('gateway');
    expect(baseURL).toHaveValue('https://unused.example.test/v1');
    expect(apiKeyEnv).toHaveValue('UNUSED_GATEWAY_KEY');

    await user.click(within(dialog).getByLabelText('Use provider base URL and credential environment'));
    expect(baseURL).toBeEnabled();
    expect(apiKeyEnv).toBeEnabled();
    await user.clear(baseURL);
    await user.type(baseURL, 'https://override.example.test/v1');
    expect(baseURL).toHaveValue('https://override.example.test/v1');

    await user.selectOptions(within(dialog).getByLabelText(/^Agent/), 'opencode');
    expect(image).toHaveValue('localhost/openshell:opencode-2.0.0-r3');
    expect(within(image).getAllByRole('option').map((option) => (option as HTMLOptionElement).value)).toEqual([
      'localhost/openshell:opencode-2.0.0-r3'
    ]);
  });
});

describe('image builds', () => {
  it('streams build output and exposes stop while the task runs', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input.toString();
      const method = init?.method ?? 'GET';
      if (url === '/api/gateways' && method === 'GET') return jsonResponse(overview.gateways);
      if (url === '/api/mcps' && method === 'GET') return jsonResponse(mcps);
      if (url === '/api/skills' && method === 'GET') return jsonResponse(skillEntries);
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
    await waitFor(() => expect(within(row).getByText('Working')).toBeInTheDocument());
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
    expect(within(dialog).getByLabelText(/^MCP tools/)).toHaveValue('codex-work');
    expect(within(dialog).getByLabelText(/^Skills/)).toHaveValue('codex-skills');
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

describe('image lifecycle', () => {
  async function openImages(tab = 'Images') {
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /setup/i }))[0]);
    await user.click(await screen.findByRole('tab', { name: tab }));
    return user;
  }

  it('disables delete for an image held by a profile and names the holder', async () => {
    await openImages();
    const row = (await screen.findByText('localhost/openshell:codex-1.0.0-r3')).closest('tr');
    if (!row) throw new Error('image row missing');
    const button = within(row).getByRole('button', { name: 'Delete' });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('title', 'In use by profile codex-default');
    expect(within(row).getByText('profile codex-default')).toBeInTheDocument();
  });

  it('prunes unused images after confirmation', async () => {
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/images/prune': { removed: ['localhost/openshell:opencode-2.0.0-r3'] }
    });
    const user = await openImages();
    await user.click(await screen.findByRole('button', { name: 'Prune unused' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Prune' }));
    expect(await screen.findByText('Removed 1 unused image(s)')).toBeInTheDocument();
  });

  it('shows delete failures inside the delete dialog', async () => {
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/images/delete': { mockStatus: 409, body: { error: 'image "localhost/openshell:codex-0.9.0-r3" is in use by run late-holder (running)' } }
    });
    const user = await openImages();
    const row = (await screen.findByText('localhost/openshell:codex-0.9.0-r3')).closest('tr');
    if (!row) throw new Error('image row missing');
    await user.click(within(row).getByRole('button', { name: 'Delete' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }));
    expect(within(dialog).getByRole('alert')).toHaveTextContent('Unable to delete image');
    expect(within(dialog).getByText(/run late-holder \(running\)/i)).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Delete' })).toBeEnabled();
  });

  it('shows prune failures inside the prune dialog', async () => {
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/images/prune': { mockStatus: 500, body: { error: 'remove "localhost/openshell:codex-0.9.0-r3": container engine unavailable; start Podman and retry' } }
    });
    const user = await openImages();
    await user.click(await screen.findByRole('button', { name: 'Prune unused' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Prune' }));
    expect(within(dialog).getByRole('alert')).toHaveTextContent('Unable to prune images');
    expect(within(dialog).getByText(/container engine unavailable; start Podman and retry/i)).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Prune' })).toBeEnabled();
  });

  it('streams refresh-all progress in the log drawer', async () => {
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/images/refresh': { id: 'refresh-task', status: 'running', output: 'Refreshing codex to 1.0.0\n' },
      'GET /api/tasks/refresh-task': { id: 'refresh-task', status: 'running', output: 'Refreshing codex to 1.0.0\nBuilt image: localhost/openshell:codex-1.0.0-r4\n' }
    });
    const user = await openImages();
    await user.click(await screen.findByRole('button', { name: 'Refresh all agents' }));
    expect(await screen.findAllByText('Refresh all agents')).toHaveLength(2);
    expect(await screen.findAllByText(/running/i).then((elements) => elements.length)).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: 'Stop' })).toBeInTheDocument();
    expect(await screen.findByText(/Refreshing codex to 1.0.0/i)).toBeInTheDocument();
    expect(await screen.findByText(/Built image: localhost\/openshell:codex-1.0.0-r4/i)).toBeInTheDocument();
  });
});

describe('provider setup', () => {
  it('only offers gateway provider types because direct providers are built in', async () => {
    const user = userEvent.setup();
    render(<ProvidersPanel />);
    await user.click(await screen.findByRole('button', { name: 'Set up provider' }));
    const dialog = await screen.findByRole('dialog');
    const type = within(dialog).getByLabelText(/^Type/);
    expect(within(type).queryByRole('option', { name: 'openai' })).not.toBeInTheDocument();
    expect(within(type).queryByRole('option', { name: 'anthropic' })).not.toBeInTheDocument();
  });
});

describe('mcp tools', () => {
  it('lists entries and detected local MCP files', async () => {
    mockRoutes({
      'GET /api/mcps': mcps,
    'GET /api/skills': skillEntries,
      'GET /api/mcps/local': [{ project: 'demo', root: '/repo', files: ['.mcp.json'] }]
    });
    render(<McpPanel />);
    expect(await screen.findByText('codex-work')).toBeInTheDocument();
    expect(screen.getByText('github, sentry')).toBeInTheDocument();
    expect(screen.getByText('How MCP tools work')).toBeInTheDocument();
    expect(screen.getByText('.mcp.json')).toBeInTheDocument();
  });

  it('creates an entry from discovered global servers', async () => {
    let created: Record<string, unknown> | undefined;
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input.toString();
      const method = init?.method ?? 'GET';
      if (url === '/api/mcps' && method === 'GET') return jsonResponse(created ? [created] : []);
      if (url === '/api/mcps/local' && method === 'GET') return jsonResponse([]);
      if (url === '/api/mcps/servers?agent=claude' && method === 'GET') return jsonResponse(['notion', 'sentry']);
      if (url === '/api/mcps' && method === 'POST') {
        created = JSON.parse(String(init?.body)) as Record<string, unknown>;
        return jsonResponse(created);
      }
      throw new Error(`unmocked request ${method} ${url}`);
    }));
    const user = userEvent.setup();
    render(<McpPanel />);
    await user.click(await screen.findByRole('button', { name: 'Create MCP tools' }));
    await user.type(await screen.findByLabelText(/^Name/), 'claude-work');
    await user.click(await screen.findByLabelText('notion'));
    await user.click(screen.getByRole('button', { name: 'Create' }));
    expect(await screen.findByText('MCP tools entry created')).toBeInTheDocument();
    expect(created).toEqual({ version: 1, name: 'claude-work', agent: 'claude', servers: ['notion'] });
    expect(await screen.findByText('claude-work')).toBeInTheDocument();
  });

  it('deletes an entry', async () => {
    mockRoutes({
      'GET /api/mcps': mcps,
    'GET /api/skills': skillEntries,
      'GET /api/mcps/local': [],
      'POST /api/mcps/codex-work/delete': { deleted: true }
    });
    const user = userEvent.setup();
    render(<McpPanel />);
    const row = (await screen.findByText('codex-work')).closest('tr');
    if (!row) throw new Error('entry row missing');
    await user.click(within(row).getByRole('button', { name: 'Delete' }));
    expect(await screen.findByText('MCP tools entry codex-work deleted')).toBeInTheDocument();
  });
});

describe('mcp start selection', () => {
  it('offers only profile-compatible entries and surfaces drift warnings', async () => {
    let startBody: Record<string, unknown> | undefined;
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input.toString();
      const method = init?.method ?? 'GET';
      if (url === '/api/overview' && method === 'GET') return jsonResponse(overview);
      if (url === '/api/mcps' && method === 'GET') return jsonResponse(mcps);
      if (url === '/api/skills' && method === 'GET') return jsonResponse(skillEntries);
      if (url === '/api/doctor' && method === 'GET') return jsonResponse(overview.doctor);
      if (url === '/api/runs' && method === 'POST') {
        startBody = JSON.parse(String(init?.body)) as Record<string, unknown>;
        return jsonResponse({ ...runs[0], mcp: 'codex-work', mcp_warnings: ['MCP server "gone" from entry "codex-work" is not configured for codex on this host; starting without it'] });
      }
      throw new Error(`unmocked request ${method} ${url}`);
    }));
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole('button', { name: 'New agent' }));
    const select = await screen.findByLabelText(/^MCP tools/);
    expect(within(select).queryByRole('option', { name: /claude-work/ })).toBeNull();
    expect(within(select).getByRole('option', { name: /codex-work/ })).toBeInTheDocument();
    await user.selectOptions(select, 'codex-work');
    await user.click(screen.getByRole('button', { name: 'Start' }));
    expect(await screen.findByText('MCP tools warnings')).toBeInTheDocument();
    expect(await screen.findByText(/starting without it/)).toBeInTheDocument();
    expect(startBody?.mcp).toBe('codex-work');
  });
});


describe('skills', () => {
  it('lists entries and explains plugin delivery limits', async () => {
    mockRoutes({
      'GET /api/skills': skillEntries
    });
    render(<SkillsPanel />);
    expect(await screen.findByText('codex-skills')).toBeInTheDocument();
    expect(screen.getByText('orca-cli')).toBeInTheDocument();
    expect(screen.getByText('browser@openai-bundled')).toBeInTheDocument();
    expect(screen.getByText('How skills work')).toBeInTheDocument();
    expect(screen.getByText(/plugin scripts, apps, hooks, commands, and MCP servers are not delivered/i)).toBeInTheDocument();
  });

  it('creates an entry from discovered skills and plugin bundles', async () => {
    let created: Record<string, unknown> | undefined;
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input.toString();
      const method = init?.method ?? 'GET';
      if (url === '/api/skills' && method === 'GET') return jsonResponse(created ? [created] : []);
      if (url === '/api/skills/available?agent=claude' && method === 'GET') {
        return jsonResponse({
          agent: 'claude',
          skills: [{ name: 'orca-cli', description: 'Orca command line' }],
          plugins: [{ id: 'browser@openai-bundled', plugin: 'browser', marketplace: 'openai-bundled', skills: 8 }]
        });
      }
      if (url === '/api/skills' && method === 'POST') {
        created = JSON.parse(String(init?.body)) as Record<string, unknown>;
        return jsonResponse(created);
      }
      throw new Error(`unmocked request ${method} ${url}`);
    }));
    const user = userEvent.setup();
    render(<SkillsPanel />);
    await user.click(await screen.findByRole('button', { name: 'Create skills entry' }));
    await user.type(await screen.findByLabelText(/^Name/), 'claude-work');
    expect(await screen.findByText('Orca command line')).toBeInTheDocument();
    expect(screen.getByText('8 bundled skills')).toBeInTheDocument();
    await user.click(screen.getByLabelText('orca-cli'));
    await user.click(screen.getByLabelText('browser@openai-bundled'));
    await user.click(screen.getByRole('button', { name: 'Create' }));
    expect(await screen.findByText('Skills entry created')).toBeInTheDocument();
    expect(created).toEqual({ version: 1, name: 'claude-work', agent: 'claude', skills: ['orca-cli'], plugins: ['browser@openai-bundled'] });
    expect(await screen.findByText('claude-work')).toBeInTheDocument();
  });

  it('deletes an entry', async () => {
    mockRoutes({
      'GET /api/skills': skillEntries,
      'POST /api/skills/codex-skills/delete': { deleted: true }
    });
    const user = userEvent.setup();
    render(<SkillsPanel />);
    const row = (await screen.findByText('codex-skills')).closest('tr');
    if (!row) throw new Error('entry row missing');
    await user.click(within(row).getByRole('button', { name: 'Delete' }));
    expect(await screen.findByText('Skills entry codex-skills deleted')).toBeInTheDocument();
  });
});

describe('skills start selection', () => {
  it('offers only profile-compatible entries and surfaces drift warnings', async () => {
    let startBody: Record<string, unknown> | undefined;
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input.toString();
      const method = init?.method ?? 'GET';
      if (url === '/api/overview' && method === 'GET') return jsonResponse(overview);
      if (url === '/api/mcps' && method === 'GET') return jsonResponse(mcps);
      if (url === '/api/skills' && method === 'GET') return jsonResponse(skillEntries);
      if (url === '/api/doctor' && method === 'GET') return jsonResponse(overview.doctor);
      if (url === '/api/runs' && method === 'POST') {
        startBody = JSON.parse(String(init?.body)) as Record<string, unknown>;
        return jsonResponse({ ...runs[0], skills: 'codex-skills', skills_warnings: ['skill "gone" from entry "codex-skills" is not available for codex on this host; starting without it'] });
      }
      throw new Error(`unmocked request ${method} ${url}`);
    }));
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole('button', { name: 'New agent' }));
    const select = await screen.findByLabelText(/^Skills/);
    expect(within(select).queryByRole('option', { name: /claude-skills/ })).toBeNull();
    expect(within(select).getByRole('option', { name: /codex-skills/ })).toBeInTheDocument();
    await user.selectOptions(select, 'codex-skills');
    await user.click(screen.getByRole('button', { name: 'Start' }));
    expect(await screen.findByText('Skills warnings')).toBeInTheDocument();
    expect(await screen.findByText(/starting without it/)).toBeInTheDocument();
    expect(startBody?.skills).toBe('codex-skills');
  });
});
