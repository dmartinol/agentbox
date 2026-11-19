import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ActionButton } from '../src/components/ui/action';
import { ProjectsPanel } from '../src/pages/setup/ProjectsPanel';
import { ProfilesPanel } from '../src/pages/setup/ProfilesPanel';
import { GatewaysPanel } from '../src/pages/setup/GatewaysPanel';
import { ImagesPanel } from '../src/pages/setup/ImagesPanel';
import { McpPanel } from '../src/pages/setup/McpPanel';
import { SkillsPanel } from '../src/pages/setup/SkillsPanel';
import type { MCPTools, SkillsEntry } from '../src/types';

function response(value: unknown, status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => value };
}

function mockRoutes(routes: Record<string, unknown>) {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = input.toString();
    const method = init?.method ?? 'GET';
    const key = `${method} ${url}`;
    const route = routes[key];
    if (!(key in routes)) throw new Error(`unmocked request ${key}`);
    if (typeof route === 'function') return route(init);
    if (route && typeof route === 'object' && 'mockStatus' in route) {
      const mocked = route as { mockStatus: number; body: unknown };
      return response(mocked.body, mocked.mockStatus);
    }
    return response(route);
  }));
}

afterEach(() => {
  vi.unstubAllGlobals();
  cleanup();
});

describe('ActionButton', () => {
  it('keeps a guarded action focusable and visible without invoking it', async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(<ActionButton disabledReason="Used by run demo" onClick={onClick}>Delete</ActionButton>);
    const button = screen.getByRole('button', { name: 'Delete' });
    button.focus();
    expect(button).toHaveFocus();
    expect(button).toHaveAttribute('aria-disabled', 'true');
    expect(button).toHaveAttribute('aria-describedby');
    expect(screen.getByText('Used by run demo')).toBeInTheDocument();
    await user.click(button);
    expect(onClick).not.toHaveBeenCalled();
  });
});

describe('resource lifecycle panels', () => {
  it('guards and deletes projects with explicit retention', async () => {
    const user = userEvent.setup();
    mockRoutes({
      'GET /api/projects': [
        { version: 1, name: 'held', root: '/held', used_by: ['active-run'] },
        { version: 1, name: 'unused', root: '/unused' }
      ],
      'POST /api/projects/unused/delete': { deleted: true },
      'POST /api/projects/held/delete': { mockStatus: 409, body: { error: 'project held is used', dependents: ['active-run'] } }
    });
    render(<ProjectsPanel />);
    const held = (await screen.findByText('held')).closest('tr');
    if (!held) throw new Error('held row missing');
    expect(within(held).getByRole('button', { name: 'Delete' })).toHaveAttribute('aria-disabled', 'true');
    expect(within(held).getByText('Used by runs: active-run')).toBeInTheDocument();
    await user.click(within(held).getByRole('button', { name: 'Delete' }));
    expect(screen.queryByRole('dialog')).toBeNull();

    const unused = screen.getByText('unused').closest('tr');
    if (!unused) throw new Error('unused row missing');
    await user.click(within(unused).getByRole('button', { name: 'Delete' }));
    expect(await screen.findByText(/Git checkout at \/unused and existing run records are kept/i)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Delete registration' }));
    expect(await screen.findByText('Project unused deleted')).toBeInTheDocument();
  });

  it('guards profiles for active runs and the last profile for an agent', async () => {
    mockRoutes({
      'GET /api/profiles': [
        { version: 1, name: 'active', agent: 'codex', mode: 'standard', used_by: ['active-run'] },
        { version: 1, name: 'only', agent: 'claude', mode: 'standard', last_for_agent: true },
        { version: 1, name: 'extra', agent: 'claude', mode: 'standard' }
      ],
      'GET /api/images': [],
      'GET /api/providers': []
    });
    render(<ProfilesPanel />);
    const active = (await screen.findByText('active')).closest('tr');
    const only = screen.getByText('only').closest('tr');
    if (!active || !only) throw new Error('profile rows missing');
    expect(within(active).getByText('Used by active runs: active-run')).toBeInTheDocument();
    expect(within(only).getByText('Last profile for its agent')).toBeInTheDocument();
    expect(within(active).getByRole('button', { name: 'Delete' })).toHaveAttribute('aria-disabled', 'true');
    expect(within(only).getByRole('button', { name: 'Delete' })).toHaveAttribute('aria-disabled', 'true');
  });

  it('shows compact project repository status and textual worker state', async () => {
    const user = userEvent.setup();
    mockRoutes({
      'GET /api/projects': [{
        version: 1,
        name: 'demo',
        root: '/repo',
        worktree: true,
        manifest: true,
        repo: 'github.com/example/demo',
        repo_status: {
          manifest_present: true,
          manifest_path: '/repo/.agentbox/config.yaml',
          manifest_digest: 'sha256:manifest',
          fragment_path: '/state/policies/repo-github-com-example-demo.yaml',
          fragment_digest: 'sha256:fragment',
          fragment_status: 'current',
          host_rules: [{ host: 'proxy.golang.org', port: 443, binaries: ['/usr/local/go/bin/go'] }],
          worker_images: [
            { agent: 'claude', status: 'missing' },
            { agent: 'codex', status: 'current', reference: 'repo-image' },
            { agent: 'opencode', status: 'stale', reference: 'old-image' }
          ],
          warnings: ['local image inventory unavailable']
        }
      }]
    });
    render(<ProjectsPanel />);
    await user.click(await screen.findByText('Repository details'));
    const details = screen.getByText('Repository details').closest('details');
    if (!details) throw new Error('repository details missing');
    expect(await within(details).findByText((_, node) => node?.tagName === 'DD'
      && node.textContent === 'present · /repo/.agentbox/config.yaml · sha256:manifest')).toBeInTheDocument();
    expect(within(details).getByText((_, node) => node?.tagName === 'DD'
      && node.textContent === 'current · /state/policies/repo-github-com-example-demo.yaml · sha256:fragment')).toBeInTheDocument();
    expect(within(details).getByText(/proxy\.golang\.org:443 — \/usr\/local\/go\/bin\/go/)).toBeInTheDocument();
    expect(screen.getByText(/codex: current · repo-image/)).toBeInTheDocument();
    expect(screen.getByText(/opencode: stale · old-image/)).toBeInTheDocument();
    expect(screen.getByText(/claude: missing/)).toBeInTheDocument();
    expect(screen.getByText(/local image inventory unavailable/)).toBeInTheDocument();
  });

  it('guards gateways with active runs and removes only local registration', async () => {
    const user = userEvent.setup();
    mockRoutes({
      'GET /api/gateways': [{ name: 'openshell', active: true, endpoint: 'https://gateway', type: 'podman' }],
      'POST /api/gateways/openshell/delete': { deleted: true }
    });
    render(<GatewaysPanel />);
    const row = (await screen.findByText('openshell')).closest('tr');
    if (!row) throw new Error('gateway row missing');
    await user.click(within(row).getByRole('button', { name: 'Remove' }));
    expect(screen.getByText(/remote gateway, its sandboxes, and run records are kept/i)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Remove local registration' }));
    expect(await screen.findByText('Gateway openshell removed locally')).toBeInTheDocument();
  });

  it('guards image deletion and clears only the saved registry reference', async () => {
    const user = userEvent.setup();
    mockRoutes({
      'GET /api/images': [
        { reference: 'held-image', agent: 'codex', version: '1', recipe: 1, used_by: ['profile codex'] }
      ],
      'GET /api/registry': { reference: 'quay.io/team/openshell' },
      'GET /api/projects': [],
      'GET /api/profiles': [],
      'POST /api/registry/clear': { cleared: true }
    });
    render(<ImagesPanel openLog={() => undefined} updateLog={() => undefined} />);
    const row = (await screen.findByText('held-image')).closest('tr');
    if (!row) throw new Error('image row missing');
    expect(within(row).getByText('In use by profile codex')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Clear' }));
    expect(screen.getByText(/Podman images and registry credentials are kept/i)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Clear registry' }));
    expect(await screen.findByText('Saved registry reference removed')).toBeInTheDocument();
  });

  it('disables registry clear when no reference is saved', async () => {
    const user = userEvent.setup();
    mockRoutes({ 'GET /api/images': [], 'GET /api/registry': { reference: '' }, 'GET /api/projects': [], 'GET /api/profiles': [] });
    render(<ImagesPanel openLog={() => undefined} updateLog={() => undefined} />);
    const clear = await screen.findByRole('button', { name: 'Clear' });
    expect(clear).toHaveAttribute('aria-disabled', 'true');
    expect(screen.getByText('No saved registry reference')).toBeInTheDocument();
    await user.click(clear);
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});

describe('MCP and skills editing', () => {
  it('edits an MCP entry with unavailable saved selections', async () => {
    const user = userEvent.setup();
    let body: Record<string, unknown> | undefined;
    const entry: MCPTools = { version: 1, name: 'codex-work', agent: 'codex', servers: ['github', 'gone'] };
    mockRoutes({
      'GET /api/mcps': [entry],
      'GET /api/mcps/local': [],
      'GET /api/mcps/servers?agent=codex': ['github'],
      'POST /api/mcps': (init?: RequestInit) => {
        body = JSON.parse(String(init?.body));
        return response(entry);
      }
    });
    render(<McpPanel />);
    const row = (await screen.findByText('codex-work')).closest('tr');
    if (!row) throw new Error('MCP row missing');
    await user.click(within(row).getByRole('button', { name: 'Edit' }));
    const name = await screen.findByLabelText(/^Name/);
    expect(name).toHaveValue('codex-work');
    expect(name).toHaveAttribute('readonly');
    expect(await screen.findByText(/Unavailable saved servers/i)).toBeInTheDocument();
    expect(screen.getByText('gone')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('MCP tools entry saved')).toBeInTheDocument();
    expect(body).toEqual({ version: 1, name: 'codex-work', agent: 'codex', servers: ['github', 'gone'] });
  });

  it('edits a skills entry with unavailable saved selections', async () => {
    const user = userEvent.setup();
    let body: Record<string, unknown> | undefined;
    const entry: SkillsEntry = { version: 1, name: 'codex-skills', agent: 'codex', skills: ['orca-cli', 'gone-skill'], plugins: ['gone-plugin'] };
    mockRoutes({
      'GET /api/skills': [entry],
      'GET /api/skills/status': [],
      'GET /api/skills/available?agent=codex': { agent: 'codex', skills: [{ name: 'orca-cli' }], plugins: [] },
      'POST /api/skills': (init?: RequestInit) => {
        body = JSON.parse(String(init?.body));
        return response(entry);
      }
    });
    render(<SkillsPanel />);
    const row = (await screen.findByText('codex-skills')).closest('tr');
    if (!row) throw new Error('skills row missing');
    await user.click(within(row).getByRole('button', { name: 'Edit' }));
    const name = await screen.findByLabelText(/^Name/);
    expect(name).toHaveValue('codex-skills');
    expect(name).toHaveAttribute('readonly');
    expect(await screen.findByText(/Unavailable saved skills/i)).toBeInTheDocument();
    expect(screen.getByText(/Unavailable saved plugins/i)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Skills entry saved')).toBeInTheDocument();
    expect(body).toEqual({ version: 1, name: 'codex-skills', agent: 'codex', skills: ['orca-cli', 'gone-skill'], plugins: ['gone-plugin'] });
  });
});
