import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App } from '../src/App';
import { ProvidersPanel } from '../src/pages/setup/ProvidersPanel';
import { ProfilesPanel } from '../src/pages/setup/ProfilesPanel';
import { ProjectsPanel } from '../src/pages/setup/ProjectsPanel';
import { ImagesPanel } from '../src/pages/setup/ImagesPanel';
import { GatewaysPanel } from '../src/pages/setup/GatewaysPanel';
import { McpPanel } from '../src/pages/setup/McpPanel';
import { PoliciesPanel } from '../src/pages/setup/PoliciesPanel';
import { SkillsPanel } from '../src/pages/setup/SkillsPanel';
import { mcps, overview, providers, runs, skillEntries, skillStatus } from './fixtures';
import { formatSyncTimestamp } from '../src/lib/text';

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
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
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
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function defaultRoutes(): Record<string, unknown> {
  return {
    'GET /api/overview': overview,
    'GET /api/mcps': mcps,
    'GET /api/skills': skillEntries,
    'GET /api/skills/status': skillStatus,
    'GET /api/doctor': overview.doctor,
    'GET /api/runs': runs,
    'GET /api/profiles': overview.profiles,
    'GET /api/providers': providers,
    'GET /api/providers?gateway=openshell': providers,
    'GET /api/providers/model-gateway/models?gateway=openshell': ['gpt-6-luna', 'model-x'],
    'GET /api/gateways': overview.gateways,
    'GET /api/images': overview.images,
    'GET /api/projects': overview.projects,
    'GET /api/registry': { reference: 'quay.io/team/openshell' },
    'GET /api/runs/run-active/logs': { logs: 'agent log output' },
    'GET /api/runs/run-active/logs?tail=200': { logs: 'agent log output' },
    'GET /api/runs/run-finished/logs?tail=400': { logs: 'finished output' },
    'GET /api/runs/run-active/shells': [],
    'POST /api/runs/run-active/shells': { name: 'shell-1' },
    'POST /api/runs/run-active/shells/shell-1/delete': { deleted: true },
    'GET /api/runs/run-active/diff': { diff: 'diff --git a/file b/file' },
    'GET /api/runs/run-active/policy': { effective: { version: 1 } },
    'GET /api/runs/run-active/policy/content': { path: '/policies/run-active.yaml', content: 'version: 1\nnetwork_policies: {}\n' },
    'GET /api/runs/run-active': runs[0],
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

  it('keeps start disabled until the start configuration loads', async () => {
    mockRoutes({ ...defaultRoutes(), 'GET /api/overview': () => new Promise(() => {}) });
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'New agent' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Loading configuration')).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Start' })).toBeDisabled();
  });

  it('opens the new agent dialog while catalog data is still refreshing', async () => {
    let mcpsRequests = 0;
    mockRoutes({
      ...defaultRoutes(),
      'GET /api/mcps': () => {
        mcpsRequests += 1;
        return mcpsRequests === 1 ? mcps : new Promise(() => {});
      }
    });
    const user = userEvent.setup();
    render(<App />);
    expect(await screen.findByText('Doctor OK')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'New agent' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByLabelText(/^Profile/)).toHaveValue('codex-default');
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

describe('compact resource cards', () => {
  it('keeps field names for assistive technology without visible captions', async () => {
    render(<ProvidersPanel />);
    const row = (await screen.findByText('model-gateway')).closest('tr');
    if (!row) throw new Error('provider row missing');
    const labels = within(row).getAllByRole('cell').map((cell) => cell.getAttribute('data-label') ?? '');
    expect(labels).toEqual(['', 'Type', 'Credential', 'Used by', 'Actions']);
    expect(within(row).getByText('Type:', { selector: '.agentbox-card-label' })).toBeInTheDocument();
    expect(within(row).getByText('Credential:', { selector: '.agentbox-card-label' })).toBeInTheDocument();
    expect(within(row).getByText('Actions:', { selector: '.agentbox-card-label' })).toBeInTheDocument();
    expect(within(row).getByRole('button', { name: 'Edit' })).toBeInTheDocument();
    const reason = within(row).getByText('Delete dependent profiles first');
    expect(reason).toHaveClass('agentbox-action-reason');
    expect(reason.parentElement).toHaveClass('agentbox-action-with-reason');
  });
});

describe('destructive row actions', () => {
  const policies = [{ name: 'codex-default', path: '/policies/codex-default.yaml', used_by: ['codex-default'] }];

  it.each([
    ['providers', <ProvidersPanel />, 'model-gateway'],
    ['profiles', <ProfilesPanel />, 'codex-default'],
    ['projects', <ProjectsPanel />, 'demo'],
    ['images', <ImagesPanel openLog={() => undefined} updateLog={() => undefined} />, 'localhost/openshell:codex-1.0.0-r3'],
    ['gateways', <GatewaysPanel />, 'openshell'],
    ['policies', <PoliciesPanel />, '/policies/codex-default.yaml'],
    ['MCP tools', <McpPanel />, 'codex-work'],
    ['skills', <SkillsPanel />, 'codex-skills']
  ])('styles the %s destructive action consistently', async (_, panel, rowText) => {
    mockRoutes({
      ...defaultRoutes(),
      'GET /api/projects': overview.projects,
      'GET /api/policies': policies,
      'GET /api/mcps/local': []
    });
    render(panel);
    const row = (await screen.findByText(rowText)).closest('tr');
    if (!row) throw new Error(`${rowText} row missing`);
    expect(within(row).getByRole('button', { name: /^(Delete|Remove)$/ })).toHaveClass('text-danger');
  });
});

describe('policies panel', () => {
  it('lists run policy copies and shows their YAML', async () => {
    mockRoutes({
      'GET /api/policies': [{ name: 'codex-default', path: '/policies/codex-default.yaml', used_by: ['codex-default'] }],
      'GET /api/runs': runs,
      'GET /api/runs/run-active/policy/content': {
        path: '/policies/run-active.yaml',
        content: 'version: 1\nnetwork_policies:\n  host-api-example-com: {}\n'
      }
    });
    const user = userEvent.setup();
    render(<PoliciesPanel />);
    expect((await screen.findAllByText('codex-default')).length).toBeGreaterThan(0);
    expect(screen.getByText('Agent run policies')).toBeInTheDocument();
    const row = screen.getByText('/policies/run-active.yaml').closest('tr');
    if (!row) throw new Error('run policy row missing');
    await user.click(within(row).getByRole('button', { name: 'View' }));
    expect(await screen.findByText(/Run policy · active-run/)).toBeInTheDocument();
    expect(screen.getByText(/host-api-example-com/)).toBeInTheDocument();
  });
});

describe('provider management', () => {
  it('shows profile usage and blocks deleting a used provider', async () => {
    render(<ProvidersPanel />);
    const usedRow = (await screen.findByText('model-gateway')).closest('tr');
    if (!usedRow) throw new Error('provider row missing');
    expect(within(usedRow).getByText('codex-default')).toBeInTheDocument();
    const button = within(usedRow).getByRole('button', { name: 'Delete' });
    expect(button).toHaveAttribute('aria-disabled', 'true');
    expect(within(usedRow).getByText('Delete dependent profiles first')).toBeInTheDocument();
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
    expect(within(row).getByRole('button', { name: 'Edit' })).toHaveAttribute('aria-disabled', 'true');
    expect(within(row).getByRole('button', { name: 'Delete' })).toHaveAttribute('aria-disabled', 'true');
    expect(within(row).getByText('Built-in providers are managed automatically')).toBeInTheDocument();
    expect(within(row).getByText('Built-in providers cannot be deleted')).toBeInTheDocument();
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
      if (url === '/api/projects' && method === 'GET') return jsonResponse(overview.projects);
      if (url === '/api/profiles' && method === 'GET') return jsonResponse(overview.profiles);
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

describe('repository setup', () => {
  const ownedProject = { version: 1, name: 'owned', root: '/owned', worktree: true, manifest: true };
  const plan = {
    repo: 'github.com/owner/repo',
    root: '/owned',
    manifest_path: '/owned/.agentbox/config.yaml',
    manifest_digest: 'sha256:9f2c',
    agents: ['codex'],
    fragment_path: '/policies/repo-github-com-owner-repo.yaml',
    fragment_status: 'preview',
    images: [],
    dry: true
  };

  const openImagesTab = async (user: ReturnType<typeof userEvent.setup>) => {
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /setup/i }))[0]);
    await user.click(await screen.findByRole('tab', { name: 'Images' }));
  };

  it('marks projects without a manifest and defaults agents from profiles', async () => {
    mockRoutes({
      ...defaultRoutes(),
      'GET /api/projects': [
        { version: 1, name: 'demo', root: '/repo', worktree: true, manifest: false },
        ownedProject
      ]
    });
    const user = userEvent.setup();
    await openImagesTab(user);
    const projectSelect = await screen.findByRole('combobox', { name: /project/i });
    expect(within(projectSelect).getByText('demo — no manifest')).toBeInTheDocument();
    expect(within(projectSelect).getByText('owned')).toBeInTheDocument();
    expect(await screen.findByText(/No \.agentbox\/config\.yaml committed/)).toBeInTheDocument();
    // Agent checkboxes default to the agents present in local profiles.
    expect(screen.getByRole('checkbox', { name: 'codex' })).toBeChecked();
    expect(screen.getByRole('checkbox', { name: 'claude' })).not.toBeChecked();
  });

  it('validates the manifest and shows the plan without building', async () => {
    mockRoutes({
      ...defaultRoutes(),
      'GET /api/projects': [{ version: 1, name: 'demo', root: '/repo', worktree: true, manifest: false }, ownedProject],
      'POST /api/repos/setup': plan
    });
    const user = userEvent.setup();
    await openImagesTab(user);
    await user.selectOptions(await screen.findByRole('combobox', { name: /project/i }), 'owned');
    await user.click(screen.getByRole('button', { name: 'Validate' }));
    expect(await screen.findByText('github.com/owner/repo')).toBeInTheDocument();
    expect(screen.getByText('sha256:9f2c')).toBeInTheDocument();
    expect(screen.getByText(/repo-github-com-owner-repo\.yaml/)).toBeInTheDocument();
    expect(screen.getByText('(preview)')).toBeInTheDocument();
    expect(screen.getByText(/nothing was built or written/i)).toBeInTheDocument();
  });

  it('starts the setup task and streams its output', async () => {
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/repos/setup': { id: 'repo-task', status: 'running', output: 'setup line one\n' },
      'GET /api/tasks/repo-task': {
        id: 'repo-task',
        status: 'succeeded',
        output: 'setup line one\nsetup line two\ncodex image: localhost/openshell:repo-github-com-owner-repo-codex-1a2b3c4d (built)\nprofiles need no change\n'
      }
    });
    const user = userEvent.setup();
    await openImagesTab(user);
    await user.click(await screen.findByRole('button', { name: 'Set up repository' }));
    expect(await screen.findByText(/Set up repository · demo/i)).toBeInTheDocument();
    expect(await screen.findByText(/setup line one/i)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText(/profiles need no change/i)).toBeInTheDocument(), { timeout: 2500 });
  });

  it('shows actionable validation errors next to the action', async () => {
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/repos/setup': { mockStatus: 500, body: { error: 'no repository manifest at /repo/.agentbox/config.yaml; author .agentbox/config.yaml first and commit it' } }
    });
    const user = userEvent.setup();
    await openImagesTab(user);
    await user.click(await screen.findByRole('button', { name: 'Set up repository' }));
    expect(await screen.findByText('Repository setup failed')).toBeInTheDocument();
    expect(screen.getByText(/no repository manifest at \/repo\/\.agentbox\/config\.yaml/)).toBeInTheDocument();
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
    await user.click(within(row).getByRole('button', { name: 'Details for active-run' }));
    await user.click(within(await screen.findByRole('complementary', { name: 'Agent details' })).getByRole('button', { name: 'Sync' }));
    expect(within(row).getByText('Syncing…')).toBeInTheDocument();
    expect(within(await screen.findByRole('complementary', { name: 'Agent details' })).getByRole('button', { name: 'Syncing…' })).toBeInTheDocument();
    expect(within(row).getByRole('button', { name: 'Details for active-run' })).toBeInTheDocument();
    resolveSync(runs[0]);
    await waitFor(() => expect(screen.getByText('Sync requested for active-run')).toBeInTheDocument());
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
    await user.click(within(row).getByRole('button', { name: 'Details for active-run' }));
    await user.click(within(await screen.findByRole('complementary', { name: 'Agent details' })).getByRole('button', { name: 'Stop' }));
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

  it('prefills run approval from the selected profile and preserves an explicit prompted override', async () => {
    const promptedProfile = { ...overview.profiles[0], name: 'prompted-profile', approval: 'prompted' };
    const unattendedProfile = { ...overview.profiles[0], name: 'unattended-profile', approval: 'unattended' };
    const fetchMock = mockRoutes({
      ...defaultRoutes(),
      'GET /api/overview': { ...overview, profiles: [promptedProfile, unattendedProfile] },
      'POST /api/runs': runs[0]
    });
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole('button', { name: 'New agent' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByLabelText('Override image, model, and approval'));

    await waitFor(() => expect(within(dialog).getByLabelText(/^Approval/)).toHaveValue('prompted'));
    await user.selectOptions(within(dialog).getByLabelText(/^Profile/), 'unattended-profile');
    await waitFor(() => expect(within(dialog).getByLabelText(/^Approval/)).toHaveValue('unattended'));

    await user.selectOptions(within(dialog).getByLabelText(/^Approval/), 'prompted');
    await user.selectOptions(within(dialog).getByLabelText(/^Profile/), 'prompted-profile');
    await user.selectOptions(within(dialog).getByLabelText(/^Profile/), 'unattended-profile');
    expect(within(dialog).getByLabelText(/^Approval/)).toHaveValue('prompted');

    await user.click(within(dialog).getByRole('button', { name: 'Start' }));
    const request = fetchMock.mock.calls.find(([input, init]) => input.toString() === '/api/runs' && init?.method === 'POST');
    expect(request).toBeDefined();
    expect(JSON.parse(String(request?.[1]?.body))).toMatchObject({ profile: 'unattended-profile', approval: 'prompted' });
  });

  it('falls back to unattended when the start dialog has no profile', async () => {
    const fetchMock = mockRoutes({
      ...defaultRoutes(),
      'GET /api/overview': { ...overview, profiles: [] },
      'POST /api/runs': runs[0]
    });
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole('button', { name: 'New agent' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByLabelText('Override image, model, and approval'));

    const approval = within(dialog).getByLabelText(/^Approval/);
    expect(approval).toHaveValue('unattended');
    expect(within(approval).getByRole('option', { name: 'unattended — no permission prompts' })).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Start' }));

    const request = fetchMock.mock.calls.find(([input, init]) => input.toString() === '/api/runs' && init?.method === 'POST');
    expect(request).toBeDefined();
    expect(JSON.parse(String(request?.[1]?.body))).toMatchObject({ approval: 'unattended' });
  });

  it('preserves a prompted run approval when copying an agent', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByText('idle-run'));
    await user.click(screen.getByRole('button', { name: 'Copy agent' }));
    const dialog = await screen.findByRole('dialog');

    expect(within(dialog).getByLabelText(/^Approval/)).toHaveValue('prompted');
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
    expect(button).toHaveAttribute('aria-disabled', 'true');
    expect(button).toHaveAttribute('aria-describedby');
    expect(within(row).getByText('In use by profile codex-default')).toBeInTheDocument();
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
    'GET /api/skills/status': skillStatus,
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
    'GET /api/skills/status': skillStatus,
      'GET /api/mcps/local': [],
      'POST /api/mcps/codex-work/delete': { deleted: true }
    });
    const user = userEvent.setup();
    render(<McpPanel />);
    const row = (await screen.findByText('codex-work')).closest('tr');
    if (!row) throw new Error('entry row missing');
    await user.click(within(row).getByRole('button', { name: 'Delete' }));
    expect(await screen.findByText(/Global and project MCP configuration is kept/i));
    await user.click(screen.getByRole('button', { name: 'Delete' }));
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
      'GET /api/skills': skillEntries,
      'GET /api/skills/status': skillStatus
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
      if (url === '/api/skills/status' && method === 'GET') return jsonResponse(skillStatus);
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

  it('shows installed agent roles and their states', async () => {
    mockRoutes({
      'GET /api/skills': skillEntries,
      'GET /api/skills/status': skillStatus
    });
    render(<SkillsPanel />);
    expect(await screen.findByText('Agent roles')).toBeInTheDocument();
    expect(screen.getAllByText('agentbox-planner').length).toBeGreaterThan(0);
    expect(screen.getByText('stale')).toBeInTheDocument();
    expect(screen.getByText('missing')).toBeInTheDocument();
  });

  it('deletes an entry', async () => {
    mockRoutes({
      'GET /api/skills': skillEntries,
    'GET /api/skills/status': skillStatus,
      'POST /api/skills/codex-skills/delete': { deleted: true }
    });
    const user = userEvent.setup();
    render(<SkillsPanel />);
    const row = (await screen.findByText('codex-skills')).closest('tr');
    if (!row) throw new Error('entry row missing');
    await user.click(within(row).getByRole('button', { name: 'Delete' }));
    expect(await screen.findByText(/Host skills, plugins, and project files are kept/i));
    await user.click(screen.getByRole('button', { name: 'Delete' }));
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

describe('allow host', () => {
  async function openDialog(user: ReturnType<typeof userEvent.setup>) {
    render(<App />);
    const labels = await screen.findAllByText('active-run');
    await user.click(labels[0]);
    await user.click(await screen.findByRole('button', { name: 'Allow host' }));
    return await screen.findByRole('dialog');
  }

  it('shows the selected run and its derived agent binary', async () => {
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    expect(within(dialog).getByLabelText('Run')).toHaveValue('active-run');
    await waitFor(() => expect(within(dialog).getByLabelText('Agent binary')).toHaveValue('/usr/local/bin/codex'));
    expect(within(dialog).queryByLabelText('Agent')).toBeNull();
  });

  it('posts only the trimmed host and reports a live apply', async () => {
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/runs/run-active/policy/host': {
        outcome: 'live-applied',
        diff: '--- current\n+++ proposed\n+host-api-example-com',
        policyPath: '/policies/run-active.yaml'
      }
    });
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.type(within(dialog).getByLabelText(/^Host/), '  api.example.com  ');
    await user.click(within(dialog).getByRole('button', { name: 'Allow host' }));
    expect(await within(dialog).findByText('Network rule applied live')).toBeInTheDocument();
    expect(within(dialog).getByText(/host-api-example-com/)).toBeInTheDocument();
    expect(within(dialog).getByText('/policies/run-active.yaml')).toBeInTheDocument();
    expect(within(dialog).getByText('Close')).toBeEnabled();
    expect(within(dialog).getByRole('button', { name: 'Allow another host' })).toBeEnabled();
    expect(within(dialog).queryByRole('button', { name: 'Allow host' })).toBeNull();
    expect(within(dialog).getByLabelText(/^Host/)).toBeDisabled();
    const call = vi.mocked(fetch).mock.calls.find(([input, init]) =>
      input.toString() === '/api/runs/run-active/policy/host' && init?.method === 'POST'
    );
    expect(call).toBeTruthy();
    expect(String(call?.[1]?.body)).toBe('{"host":"api.example.com"}');
  });

  it('reports a static pending change', async () => {
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/runs/run-active/policy/host': { mockStatus: 202, body: { outcome: 'static-pending', diff: 'static diff' } }
    });
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.type(within(dialog).getByLabelText(/^Host/), 'api.example.com');
    await user.click(within(dialog).getByRole('button', { name: 'Allow host' }));
    expect(await within(dialog).findByText('Static change saved for a future sandbox')).toBeInTheDocument();
    expect(within(dialog).getByText('static diff')).toBeInTheDocument();
  });

  it('keeps the dialog open and shows the rollback error on OpenShell rejection', async () => {
    const message = 'OpenShell rejected the policy update; the local policy was restored: policy rejected';
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/runs/run-active/policy/host': {
        mockStatus: 500,
        body: { outcome: 'openshell-rejected', error: message, diff: '--- diff', rolledBack: true }
      }
    });
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.type(within(dialog).getByLabelText(/^Host/), 'api.example.com');
    await user.click(within(dialog).getByRole('button', { name: 'Allow host' }));
    expect(await within(dialog).findByText('Unable to allow host')).toBeInTheDocument();
    expect(within(dialog).getByText(message)).toBeInTheDocument();
    expect(within(dialog).getByText('Rolled back: yes')).toBeInTheDocument();
    expect(within(dialog).getByLabelText(/^Host/)).toHaveValue('api.example.com');
    expect(screen.queryByText('Unable to load fleet')).toBeNull();
  });

  it('disables the action for terminal runs', async () => {
    const user = userEvent.setup();
    render(<App />);
    const labels = await screen.findAllByText('finished-run');
    await user.click(labels[0]);
    const button = await screen.findByRole('button', { name: 'Allow host' });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('title', 'Allow host is available only for Ready, Working, and Waiting agents.');
  });

  it('shows Allowing while the request is pending', async () => {
    let release: ((value: unknown) => void) | undefined;
    const pending = new Promise((resolve) => { release = resolve; });
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/runs/run-active/policy/host': () => pending
    });
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.type(within(dialog).getByLabelText(/^Host/), 'api.example.com');
    await user.click(within(dialog).getByRole('button', { name: 'Allow host' }));
    expect(await within(dialog).findByRole('button', { name: 'Allowing' })).toBeInTheDocument();
    release?.({ outcome: 'live-applied', diff: '--- diff' });
    expect(await within(dialog).findByText('Network rule applied live')).toBeInTheDocument();
  });

  it('refreshes the fleet after success without opening the policy drawer', async () => {
    let overviewCalls = 0;
    let policyCalls = 0;
    mockRoutes({
      ...defaultRoutes(),
      'GET /api/overview': () => { overviewCalls += 1; return overview; },
      'GET /api/runs/run-active/policy': () => { policyCalls += 1; return { effective: { version: 1 } }; },
      'POST /api/runs/run-active/policy/host': { outcome: 'live-applied', diff: '--- diff' }
    });
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    const baselineOverview = overviewCalls;
    await user.type(within(dialog).getByLabelText(/^Host/), 'api.example.com');
    await user.click(within(dialog).getByRole('button', { name: 'Allow host' }));
    expect(await within(dialog).findByText('Network rule applied live')).toBeInTheDocument();
    await waitFor(() => expect(overviewCalls).toBeGreaterThan(baselineOverview));
    expect(policyCalls).toBe(0);
    expect(screen.queryByText(/Policy · active-run/)).toBeNull();
  });

  it('opens the policy drawer only when View policy is clicked', async () => {
    let policyCalls = 0;
    mockRoutes({
      ...defaultRoutes(),
      'GET /api/runs/run-active/policy': () => { policyCalls += 1; return { effective: { version: 1 } }; },
      'POST /api/runs/run-active/policy/host': { outcome: 'live-applied', diff: '--- diff' }
    });
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.type(within(dialog).getByLabelText(/^Host/), 'api.example.com');
    await user.click(within(dialog).getByRole('button', { name: 'Allow host' }));
    await within(dialog).findByText('Network rule applied live');
    await user.click(within(dialog).getByRole('button', { name: 'View policy' }));
    expect(await screen.findByText(/Policy · active-run/)).toBeInTheDocument();
    await waitFor(() => expect(policyCalls).toBe(1));
    expect(screen.queryByText('Network rule applied live')).toBeNull();
  });

  it('resets to a fresh form with Allow another host', async () => {
    mockRoutes({
      ...defaultRoutes(),
      'POST /api/runs/run-active/policy/host': { outcome: 'live-applied', diff: '--- diff' }
    });
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    const input = within(dialog).getByLabelText(/^Host/);
    await user.type(input, 'api.example.com');
    await user.click(within(dialog).getByRole('button', { name: 'Allow host' }));
    await within(dialog).findByText('Network rule applied live');
    await user.click(within(dialog).getByRole('button', { name: 'Allow another host' }));
    expect(within(dialog).queryByText('Network rule applied live')).toBeNull();
    expect(within(dialog).getByLabelText(/^Host/)).toHaveValue('');
    expect(within(dialog).getByLabelText(/^Host/)).toBeEnabled();
    expect(within(dialog).getByRole('button', { name: 'Allow host' })).toBeDisabled();
  });
});

describe('readable fleet layout', () => {
  it('labels narrow-layout cells and opens grouped actions from the row', async () => {
    const user = userEvent.setup();
    render(<App />);
    const table = await screen.findByRole('table', { name: 'Agents in demo' });
    expect(table).toHaveClass('agentbox-table');

    const row = (await screen.findByText('active-run')).closest('tr');
    if (!row) throw new Error('agent row missing');
    const labels = within(row).getAllByRole('cell').map((cell) => cell.getAttribute('data-label'));
    expect(labels).toEqual(expect.arrayContaining(['Type', 'Status', 'Sync', 'Branch', 'Actions']));
    expect(within(row).getByLabelText('Type: codex')).toBeInTheDocument();
    expect(within(row).getByLabelText('Status: Working')).toBeInTheDocument();
    expect(within(row).getByLabelText('Sync: Sync needed')).toBeInTheDocument();

    await user.click(within(row).getByRole('button', { name: 'Details for active-run' }));
    const panel = await screen.findByRole('complementary', { name: 'Agent details' });
    expect(within(panel).getByRole('button', { name: 'Close agent details' })).toBeInTheDocument();
    expect(within(panel).getByRole('heading', { name: 'Open' })).toBeInTheDocument();
    expect(within(panel).getByRole('heading', { name: 'Manage' })).toBeInTheDocument();
    expect(within(panel).getByRole('button', { name: 'Copy Image for active-run' })).toBeInTheDocument();
    expect(within(panel).getAllByText('Show full').length).toBeGreaterThan(0);
  });

  it('renders every sync state with text and a glyph in Fleet rows', async () => {
    const states = ['no_changes', 'up_to_date', 'needs_sync', 'blocked', 'unknown'] as const;
    const labels = {
      no_changes: 'No changes',
      up_to_date: 'Up to date',
      needs_sync: 'Sync needed',
      blocked: 'Sync blocked',
      unknown: 'Unable to check'
    } as const;
    const stateRuns = states.map((state) => ({
      ...runs[0],
      id: `run-state-${state}`,
      name: `state-${state}`,
      status: 'ready' as const,
      sync_state: state,
      last_successful_sync_at: state === 'no_changes' ? undefined : '2026-02-03T04:05:00Z'
    }));
    mockRoutes({
      ...defaultRoutes(),
      'GET /api/overview': { ...overview, tree: [], runs: stateRuns },
      'GET /api/runs': stateRuns
    });
    render(<App />);
    const table = await screen.findByRole('table', { name: 'Agents in demo' });
    for (const state of states) {
      const row = within(table).getByText(`state-${state}`).closest('tr');
      if (!row) throw new Error(`row for ${state} missing`);
      const syncCell = within(row).getByLabelText(`Sync: ${labels[state]}`);
      expect(syncCell).toHaveAttribute('data-label', 'Sync');
      expect(within(row).getByText(labels[state])).toBeInTheDocument();
      expect(within(row).getByLabelText('Status: Ready')).toBeInTheDocument();
      expect(syncCell.querySelector('svg')).not.toBeNull();
    }
  });

  it('shows sync state, explanation, localized timestamp, and Never in agent details', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(within((await screen.findByText('active-run')).closest('tr') as HTMLElement).getByRole('button', { name: 'Details for active-run' }));
    let panel = await screen.findByRole('complementary', { name: 'Agent details' });
    expect(within(panel).getByText('Sync state')).toBeInTheDocument();
    expect(within(panel).getByText('Sync needed')).toBeInTheDocument();
    expect(within(panel).getByText('Last sync')).toBeInTheDocument();
    expect(within(panel).getByText(formatSyncTimestamp('2026-01-01T10:30:00Z'))).toBeInTheDocument();
    expect(within(panel).getByText('Sandbox changes have not been transferred to the host run worktree.')).toBeInTheDocument();

    await user.click(within((await screen.findByText('idle-run')).closest('tr') as HTMLElement).getByRole('button', { name: 'Details for idle-run' }));
    panel = await screen.findByRole('complementary', { name: 'Agent details' });
    expect(within(panel).getByText('Unable to check')).toBeInTheDocument();
    expect(within(panel).getByText('Never')).toBeInTheDocument();
    expect(within(panel).getByText('AgentBox could not inspect the sandbox.')).toBeInTheDocument();
  });

  it('explains a blocked sync when the host worktree changed', async () => {
    const blockedRun = {
      ...runs[0],
      id: 'run-blocked',
      name: 'blocked-run',
      sync_state: 'blocked' as const,
      last_successful_sync_at: '2026-01-01T10:30:00Z'
    };
    mockRoutes({
      ...defaultRoutes(),
      'GET /api/overview': { ...overview, tree: [], runs: [blockedRun] },
      'GET /api/runs': [blockedRun]
    });
    const user = userEvent.setup();
    render(<App />);
    const row = (await screen.findByText('blocked-run')).closest('tr');
    if (!row) throw new Error('blocked row missing');
    await user.click(within(row).getByRole('button', { name: 'Details for blocked-run' }));
    const panel = await screen.findByRole('complementary', { name: 'Agent details' });
    expect(within(panel).getByText('Sync blocked')).toBeInTheDocument();
    expect(within(panel).getByText('The host run worktree changed, so sync will refuse to overwrite it.')).toBeInTheDocument();
  });

  it('groups resolved profile defaults behind a disclosure when starting an agent', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole('button', { name: 'New agent' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Resolved configuration')).toBeInTheDocument();
    expect(within(dialog).getByText('localhost/openshell:codex-1.0.0-r3')).toBeInTheDocument();
    expect(within(dialog).getByText('model-x')).toBeInTheDocument();
    expect(within(dialog).queryByLabelText(/^Image/)).toBeNull();
    expect(within(dialog).getByLabelText(/^Project/)).toBeInTheDocument();
  });

  it('renders ANSI logs readably in the drawer', async () => {
    mockRoutes({ ...defaultRoutes(), 'GET /api/runs/run-active/logs': { logs: '\u001b[32mready\u001b[0m line\n' } });
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /logs/i }))[0]);
    expect(await screen.findByText(/ready line/)).toBeInTheDocument();
    expect(document.body.textContent).not.toContain('\u001b');
  });

  it('shows an empty state instead of a blank drawer', async () => {
    mockRoutes({ ...defaultRoutes(), 'GET /api/runs/run-active/logs': { logs: '' } });
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /logs/i }))[0]);
    expect(await screen.findByText('No output recorded.')).toBeInTheDocument();
  });

  it('shows a load failure distinctly from empty output', async () => {
    mockRoutes({
      ...defaultRoutes(),
      'GET /api/runs/run-active/logs': { mockStatus: 500, body: { error: 'log read failed' } }
    });
    const user = userEvent.setup();
    render(<App />);
    await user.click((await screen.findAllByRole('button', { name: /logs/i }))[0]);
    expect(await screen.findByText('Unable to load output')).toBeInTheDocument();
    expect(screen.getByText('log read failed')).toBeInTheDocument();
    expect(screen.queryByText('No output recorded.')).toBeNull();
  });

  it('dismisses the narrow detail sheet when an output drawer opens', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: true,
      media: '(max-width: 767.98px)',
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn()
    })));
    const user = userEvent.setup();
    render(<App />);
    const row = (await screen.findByText('active-run')).closest('tr');
    if (!row) throw new Error('agent row missing');
    await user.click(within(row).getByRole('button', { name: 'Details for active-run' }));
    const panel = await screen.findByRole('complementary', { name: 'Agent details' });
    await user.click(within(panel).getByRole('button', { name: 'View logs' }));
    await waitFor(() => expect(screen.queryByRole('complementary', { name: 'Agent details' })).toBeNull());
    expect(await screen.findByText('agent log output')).toBeInTheDocument();
  });
});
