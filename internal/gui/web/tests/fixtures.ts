import type { Overview, Run } from '../src/types';

export const runs: Run[] = [
  {
    id: 'run-active',
    name: 'active-run',
    created: '2026-01-01T10:00:00Z',
    source: '/repo',
    project: 'demo',
    worktree: '/trees/demo/active-run',
    worktree_isolated: true,
    branch: 'agentbox/demo/active-run',
    baseline: 'abc',
    last_synced: 'abc',
    sandbox: 'sandbox-active',
    gateway: 'openshell',
    agent: 'codex',
    image: 'localhost/openshell:codex-1.0.0-r3',
    mode: 'gateway',
    model: 'model-x',
    approval: 'unattended',
    status: 'running',
    synced: true,
    policy: '/policies/run-active.yaml'
  },
  {
    id: 'run-idle',
    name: 'idle-run',
    created: '2026-01-01T11:00:00Z',
    source: '/repo',
    project: 'demo',
    worktree: '/trees/demo/idle-run',
    worktree_isolated: true,
    branch: 'agentbox/demo/idle-run',
    baseline: 'abc',
    last_synced: 'abc',
    sandbox: 'sandbox-idle',
    gateway: 'openshell',
    agent: 'claude',
    image: 'localhost/openshell:claude-1.0.0-r3',
    mode: 'gateway',
    model: 'model-y',
    approval: 'prompted',
    status: 'stopped',
    synced: false,
    policy: '/policies/run-idle.yaml'
  },
  {
    id: 'run-finished',
    name: 'finished-run',
    created: '2026-01-01T12:00:00Z',
    source: '/repo',
    project: 'demo',
    worktree: '/trees/demo/finished-run',
    worktree_isolated: true,
    branch: 'agentbox/demo/finished-run',
    baseline: 'abc',
    last_synced: 'abc',
    sandbox: 'sandbox-finished',
    gateway: 'openshell',
    agent: 'opencode',
    image: 'localhost/openshell:opencode-1.0.0-r3',
    mode: 'gateway',
    model: 'model-z',
    approval: 'prompted',
    status: 'finished',
    synced: true,
    policy: '/policies/run-finished.yaml'
  }
];

export const providers = [
  {
    name: 'model-gateway',
    type: 'gateway-bearer',
    profile: 'agentbox-gateway-bearer-models-example-com',
    credential: 'MY_GATEWAY_KEY',
    credential_keys: ['MY_GATEWAY_KEY'],
    base_url: 'https://models.example.test/v1',
    gateway: 'openshell',
    used_by: ['codex-default']
  },
  {
    name: 'openai',
    type: 'openai',
    profile: 'agentbox-openai',
    credential: 'OPENAI_API_KEY',
    credential_keys: ['OPENAI_API_KEY'],
    base_url: 'https://api.openai.com/v1',
    gateway: 'openshell',
    builtin: true,
    registered: true
  },
  {
    name: 'anthropic',
    type: 'anthropic',
    profile: 'agentbox-anthropic',
    credential: 'ANTHROPIC_API_KEY',
    credential_keys: ['ANTHROPIC_API_KEY'],
    base_url: 'https://api.anthropic.com/v1',
    gateway: 'openshell',
    builtin: true,
    registered: true
  },
  {
    name: 'unused-gateway',
    type: 'gateway-header',
    profile: 'agentbox-gateway-header-models-example-com',
    credential: 'UNUSED_GATEWAY_KEY',
    credential_keys: ['UNUSED_GATEWAY_KEY'],
    base_url: 'https://unused.example.test/v1',
    gateway: 'openshell'
  }
];

export const overview: Overview = {
  summary: { active: 1, finished: 1, failed: 0, unsynced: 1, images: 3, gateways: 1, profiles: 1, projects: 1 },
  tree: [
    {
      project: 'demo',
      root: '/repo',
      isolation: 'worktree',
      activeRuns: [runs[0]],
      idleRuns: [runs[1]],
      finishedRuns: [runs[2]]
    }
  ],
  runs,
  projects: [{ version: 1, name: 'demo', root: '/repo', worktree: true }],
  profiles: [{ version: 1, name: 'codex-default', agent: 'codex', image: 'localhost/openshell:codex-1.0.0-r3', mode: 'gateway', model: 'model-x', provider: 'model-gateway', base_url: 'https://models.example.test/v1', api_key_env: 'UNIT_MODEL_KEY', policy: '/policies/default.yaml', approval: 'unattended' }],
  images: [
    { reference: 'localhost/openshell:opencode-2.0.0-r3', agent: 'opencode', version: '2.0.0', recipe: 3, os: 'linux', arch: 'arm64' },
    { reference: 'localhost/openshell:codex-1.0.0-r3', agent: 'codex', version: '1.0.0', recipe: 3, os: 'linux', arch: 'arm64' },
    { reference: 'localhost/openshell:codex-0.9.0-r3', agent: 'codex', version: '0.9.0', recipe: 3, os: 'linux', arch: 'arm64' }
  ],
  gateways: [{ name: 'openshell', active: true, endpoint: 'https://localhost:17670', type: 'podman', auth: 'bearer', source: 'local' }],
  doctor: [{ name: 'git', ok: true, detail: '/usr/bin/git' }]
};
