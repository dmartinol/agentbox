import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfilesPanel } from '../src/pages/setup/ProfilesPanel';
import type { Profile } from '../src/types';
import { overview, providers } from './fixtures';

const profile: Profile = {
  version: 1,
  name: 'codex-default',
  agent: 'codex',
  image: 'localhost/openshell:codex-1.0.0-r3',
  mode: 'gateway',
  model: 'model-x',
  provider: 'model-gateway',
  policy: '/policies/codex-default.yaml',
  default_policy: '/policies/codex-default.yaml',
  approval: 'prompted',
  base_url: 'https://models.example.test/v1',
  api_key_env: 'UNIT_MODEL_KEY'
};

function jsonResponse(value: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => value
  };
}

function setupApi({ profiles = [profile], saveError }: { profiles?: Profile[]; saveError?: string } = {}) {
  let currentProfiles = profiles;
  const savedProfiles: Profile[] = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = input.toString();
    const method = init?.method ?? 'GET';
    if (method === 'GET' && url === '/api/profiles') return jsonResponse(currentProfiles);
    if (method === 'GET' && url === '/api/images') return jsonResponse(overview.images);
    if (method === 'GET' && url === '/api/providers') return jsonResponse(providers);
    if (method === 'POST' && url === '/api/profiles') {
      if (saveError) return jsonResponse({ error: saveError }, 400);
      const body = JSON.parse(String(init?.body)) as Profile;
      savedProfiles.push(body);
      currentProfiles = [...currentProfiles.filter((item) => item.name !== body.name), body];
      return jsonResponse(body);
    }
    throw new Error(`Unmocked request ${method} ${url}`);
  });
  vi.stubGlobal('fetch', fetchMock);
  return { savedProfiles, fetchMock };
}

async function openCreate() {
  const user = userEvent.setup();
  render(<ProfilesPanel />);
  await user.click(screen.getByRole('button', { name: 'Create profile' }));
  await screen.findByRole('dialog');
  return user;
}

async function openEdit(target: Profile = profile) {
  const user = userEvent.setup();
  render(<ProfilesPanel />);
  await screen.findByText(target.name);
  await user.click(screen.getByRole('button', { name: 'Edit' }));
  await screen.findByRole('dialog');
  return user;
}

async function fillRequiredFields(user: ReturnType<typeof userEvent.setup>, name = 'new-profile') {
  await user.type(screen.getByRole('textbox', { name: 'Name' }), name);
  await user.type(screen.getByRole('textbox', { name: 'Model' }), 'model-x');
}

afterEach(() => {
  vi.unstubAllGlobals();
  cleanup();
});

describe('profile policy', () => {
  it('shows the generated AgentBox default, makes no policy request, and submits an empty policy', async () => {
    const { savedProfiles, fetchMock } = setupApi({ profiles: [] });
    const user = await openCreate();
    const policy = screen.getByRole('textbox', { name: 'Policy' });

    expect(policy).toHaveValue('AgentBox default (generated per profile)');
    expect(policy).toHaveAttribute('readonly');
    expect(screen.getByText(/AgentBox derives the run policy at start/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Use AgentBox default' })).not.toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([input]) => input.toString() === '/api/policies')).toBe(false);

    await fillRequiredFields(user);
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(savedProfiles).toHaveLength(1));
    expect(savedProfiles[0]?.policy).toBe('');
  });

  it('shows an existing generated-default profile as the AgentBox default when editing', async () => {
    const { savedProfiles } = setupApi({ profiles: [profile] });
    const user = await openEdit(profile);

    expect(screen.getByRole('textbox', { name: 'Policy' })).toHaveValue('AgentBox default (generated per profile)');
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(savedProfiles).toHaveLength(1));
    expect(savedProfiles[0]?.policy).toBe('');
  });

  it('keeps a legacy policy path read-only and switches to the AgentBox default on request', async () => {
    const legacyProfile: Profile = { ...profile, policy: '/tmp/legacy/codex-default.yaml' };
    const { savedProfiles } = setupApi({ profiles: [legacyProfile] });
    const user = await openEdit(legacyProfile);
    const policy = screen.getByRole('textbox', { name: 'Policy' });

    expect(policy).toHaveValue('/tmp/legacy/codex-default.yaml');
    expect(policy).toHaveAttribute('readonly');
    expect(screen.getByText(/this legacy file contributes its network rules only/i)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Use AgentBox default' }));
    expect(policy).toHaveValue('AgentBox default (generated per profile)');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(savedProfiles).toHaveLength(1));
    expect(savedProfiles[0]?.policy).toBe('');
  });

  it('surfaces the backend legacy-policy error instead of blocking Save locally', async () => {
    const legacyProfile: Profile = { ...profile, policy: '/policies/deleted-policy.yaml' };
    setupApi({
      profiles: [legacyProfile],
      saveError: 'profile "codex-default" references missing policy file /policies/deleted-policy.yaml; restore the policy or use the AgentBox default'
    });
    const user = await openEdit(legacyProfile);

    expect(screen.getByRole('textbox', { name: 'Policy' })).toHaveValue('/policies/deleted-policy.yaml');
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled();
    await user.click(screen.getByRole('button', { name: 'Save' }));

    expect(await screen.findByText(/references missing policy file/)).toBeInTheDocument();
    expect(screen.getByText(/use the AgentBox default/)).toBeInTheDocument();
  });
});

describe('profile image selection', () => {
  it('keeps a missing profile image visible and saves a newly selected image', async () => {
    const staleImage = 'localhost/openshell:codex-1.0.0-r4';
    const staleProfile = { ...profile, image: staleImage };
    const { savedProfiles } = setupApi({ profiles: [staleProfile] });
    const user = await openEdit(staleProfile);
    const imageSelect = screen.getByRole('combobox', { name: 'Image' });

    expect(await screen.findByRole('option', { name: `${staleImage} (not found locally)` })).toHaveValue(staleImage);
    expect(imageSelect).toHaveValue(staleImage);

    await user.selectOptions(imageSelect, 'localhost/openshell:codex-1.0.0-r3');
    expect(imageSelect).toHaveValue('localhost/openshell:codex-1.0.0-r3');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(savedProfiles).toHaveLength(1));
    expect(savedProfiles[0]?.image).toBe('localhost/openshell:codex-1.0.0-r3');
  });
});

describe('profile approval defaults', () => {
  it('creates profiles with unattended approval and an explicit no-prompt label', async () => {
    const { savedProfiles } = setupApi({ profiles: [] });
    const user = await openCreate();
    const approval = screen.getByRole('combobox', { name: 'Approval' });

    expect(approval).toHaveValue('unattended');
    expect(within(approval).getByRole('option', { name: 'unattended — no permission prompts' })).toBeInTheDocument();

    await fillRequiredFields(user);
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(savedProfiles).toHaveLength(1));
    expect(savedProfiles[0]?.approval).toBe('unattended');
  });

  it('keeps prompted approval when editing an existing profile', async () => {
    setupApi({ profiles: [profile] });
    await openEdit(profile);

    expect(screen.getByRole('combobox', { name: 'Approval' })).toHaveValue('prompted');
  });
});
