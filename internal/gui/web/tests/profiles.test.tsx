import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfilesPanel } from '../src/pages/setup/ProfilesPanel';
import type { PolicyConfiguration, Profile } from '../src/types';
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
  approval: 'prompted',
  base_url: 'https://models.example.test/v1',
  api_key_env: 'UNIT_MODEL_KEY'
};

const sharedPolicy: PolicyConfiguration = {
  name: 'shared-policy',
  path: '/policies/shared-policy.yaml',
  used_by: ['codex-default', 'claude-default']
};

const defaultPolicy: PolicyConfiguration = {
  name: profile.name,
  path: profile.policy,
  used_by: [profile.name]
};

function jsonResponse(value: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => value
  };
}

function setupApi({
  profiles = [profile],
  policies = [sharedPolicy],
  getPolicies
}: {
  profiles?: Profile[];
  policies?: PolicyConfiguration[];
  getPolicies?: () => Promise<PolicyConfiguration[]>;
} = {}) {
  let currentProfiles = profiles;
  const savedProfiles: Profile[] = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = input.toString();
    const method = init?.method ?? 'GET';
    if (method === 'GET' && url === '/api/profiles') return jsonResponse(currentProfiles);
    if (method === 'GET' && url === '/api/images') return jsonResponse(overview.images);
    if (method === 'GET' && url === '/api/providers') return jsonResponse(providers);
    if (method === 'GET' && url === '/api/policies') return jsonResponse(getPolicies ? await getPolicies() : policies);
    if (method === 'POST' && url === '/api/profiles') {
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

describe('profile policy selection', () => {
  it('selects the AgentBox default first and submits an empty policy', async () => {
    const { savedProfiles } = setupApi({ profiles: [], policies: [sharedPolicy] });
    const user = await openCreate();
    const policySelect = screen.getByRole('combobox', { name: 'Policy' });

    expect(within(policySelect).getAllByRole('option')[0]).toHaveTextContent('Default created by AgentBox');
    expect(within(policySelect).queryByRole('option', { name: /^None$/i })).not.toBeInTheDocument();
    expect(policySelect).toHaveValue('');
    expect(screen.getByText(/A policy is required/)).toBeInTheDocument();

    await fillRequiredFields(user);
    expect(policySelect).toHaveDisplayValue(/Default created by AgentBox \(policies\/new-profile\.yaml\)/);
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(savedProfiles).toHaveLength(1));
    expect(savedProfiles[0]?.policy).toBe('');
  });

  it('lists reusable policies by name with use context and submits the selected full path', async () => {
    const beta: PolicyConfiguration = { name: 'beta', path: '/policies/beta.yaml', used_by: ['one'] };
    const alpha: PolicyConfiguration = { name: 'alpha', path: '/policies/alpha.yaml', used_by: ['one', 'two'] };
    const { savedProfiles } = setupApi({ profiles: [], policies: [beta, alpha] });
    const user = await openCreate();
    const policySelect = screen.getByRole('combobox', { name: 'Policy' });
    const options = within(policySelect).getAllByRole('option');

    expect(options.map((option) => option.textContent)).toEqual([
      'Default created by AgentBox (policies/<profile-name>.yaml)',
      'alpha — used by 2 profiles',
      'beta — used by 1 profile'
    ]);
    await user.selectOptions(policySelect, '/policies/alpha.yaml');
    await fillRequiredFields(user);
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(savedProfiles).toHaveLength(1));
    expect(savedProfiles[0]?.policy).toBe('/policies/alpha.yaml');
  });

  it.each([
    { name: 'default policy', target: { ...profile, policy: '/policies/codex-default.yaml' }, expected: '' },
    { name: 'reusable policy', target: { ...profile, policy: '/policies/shared-policy.yaml' }, expected: '/policies/shared-policy.yaml' }
  ])('preselects the $name when editing', async ({ target, expected }) => {
    setupApi({ profiles: [target], policies: [
      { name: 'codex-default', path: '/policies/codex-default.yaml', used_by: [profile.name] },
      sharedPolicy
    ] });
    await openEdit(target);
    const policySelect = await screen.findByRole('combobox', { name: 'Policy' });

    await waitFor(() => expect(policySelect).toHaveValue(expected));
    if (!expected) {
      expect(within(policySelect).queryByRole('option', { name: /codex-default — used by/ })).not.toBeInTheDocument();
    }
  });

  it('preserves the original generated policy path when renaming its profile', async () => {
    const { savedProfiles } = setupApi({
      profiles: [profile],
      policies: [defaultPolicy, sharedPolicy]
    });
    const user = await openEdit(profile);
    const nameInput = screen.getByRole('textbox', { name: 'Name' });
    const policySelect = screen.getByRole('combobox', { name: 'Policy' });

    await screen.findByRole('option', { name: /shared-policy/ });
    await user.clear(nameInput);
    await user.type(nameInput, 'renamed-codex');

    expect(policySelect).toHaveValue(profile.policy);
    expect(within(policySelect).getByRole('option', { name: new RegExp(`Original default — ${profile.policy}`) })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled();
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(savedProfiles).toHaveLength(1));
    expect(savedProfiles[0]?.name).toBe('renamed-codex');
    expect(savedProfiles[0]?.policy).toBe(profile.policy);
  });

  it('lets a renamed profile switch to its new AgentBox default', async () => {
    const { savedProfiles } = setupApi({
      profiles: [profile],
      policies: [defaultPolicy, sharedPolicy]
    });
    const user = await openEdit(profile);
    await screen.findByRole('option', { name: /shared-policy/ });
    await user.clear(screen.getByRole('textbox', { name: 'Name' }));
    await user.type(screen.getByRole('textbox', { name: 'Name' }), 'renamed-codex');

    const policySelect = screen.getByRole('combobox', { name: 'Policy' });
    expect(policySelect).toHaveValue(profile.policy);
    await user.selectOptions(policySelect, '');
    expect(policySelect).toHaveValue('');
    expect(within(policySelect).getByRole('option', { name: /Original default —/ })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(savedProfiles).toHaveLength(1));
    expect(savedProfiles[0]?.name).toBe('renamed-codex');
    expect(savedProfiles[0]?.policy).toBe('');
  });

  it('keeps the current selection visible and disables the select while policies load', async () => {
    let resolvePolicies!: (value: PolicyConfiguration[]) => void;
    const pendingPolicies = new Promise<PolicyConfiguration[]>((resolve) => { resolvePolicies = resolve; });
    setupApi({ profiles: [{ ...profile, policy: sharedPolicy.path }], getPolicies: () => pendingPolicies });
    await openEdit({ ...profile, policy: sharedPolicy.path });
    const policySelect = screen.getByRole('combobox', { name: 'Policy' });

    expect(await screen.findByRole('status')).toHaveTextContent('Loading policies…');
    expect(policySelect).toBeDisabled();
    expect(policySelect).toHaveValue(sharedPolicy.path);

    resolvePolicies([sharedPolicy]);
    await waitFor(() => expect(policySelect).toBeEnabled());
    expect(policySelect).toHaveValue(sharedPolicy.path);
  });

  it('shows a policy load error with retry and preserves the current selection', async () => {
    let calls = 0;
    setupApi({
      profiles: [{ ...profile, policy: sharedPolicy.path }],
      getPolicies: async () => {
        calls += 1;
        if (calls === 1) throw new Error('Policies are temporarily unavailable.');
        return [sharedPolicy];
      }
    });
    const user = await openEdit({ ...profile, policy: sharedPolicy.path });
    const policySelect = screen.getByRole('combobox', { name: 'Policy' });

    expect(await screen.findByRole('alert')).toHaveTextContent('Policies are temporarily unavailable.');
    expect(policySelect).toHaveValue(sharedPolicy.path);
    await user.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(policySelect).toHaveValue(sharedPolicy.path));
    expect(await screen.findByRole('option', { name: /shared-policy — used by 2 profiles/ })).toBeInTheDocument();
    expect(calls).toBe(2);
  });

  it('shows guidance when there are no reusable policies', async () => {
    setupApi({ profiles: [], policies: [] });
    await openCreate();
    const policySelect = await screen.findByRole('combobox', { name: 'Policy' });

    expect(await screen.findByText('No reusable policies yet — create one under Setup → Policies.')).toBeInTheDocument();
    expect(within(policySelect).getAllByRole('option')).toHaveLength(1);
    expect(within(policySelect).getAllByRole('option')[0]).toHaveTextContent('Default created by AgentBox');
  });

  it('keeps a missing policy selected, explains how to fix it, and blocks save', async () => {
    setupApi({ profiles: [{ ...profile, policy: '/policies/deleted-policy.yaml' }], policies: [sharedPolicy] });
    await openEdit({ ...profile, policy: '/policies/deleted-policy.yaml' });
    const policySelect = screen.getByRole('combobox', { name: 'Policy' });

    expect(await screen.findByRole('option', { name: 'deleted-policy.yaml (missing)' })).toHaveValue('/policies/deleted-policy.yaml');
    expect(policySelect).toHaveValue('/policies/deleted-policy.yaml');
    expect(screen.getByText('Policy /policies/deleted-policy.yaml no longer exists; pick another policy.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
  });
});
