import type {
  Check,
  Gateway,
  ImageArtifact,
  AvailableSkills,
  LocalMCPFiles,
  MCPTools,
  RepoSetupResult,
  SkillStatus,
  SkillsEntry,
  Overview,
  DenialApplyResult,
  DenialError,
  DenialPreview,
  DenialRequest,
  PolicyDenial,
  Profile,
  Project,
  Provider,
  Registry,
  Run,
  RunPolicyContent,
  Settings,
  StartRunRequest,
  GuiTask
} from './types';

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: init?.body ? { 'Content-Type': 'application/json', ...init?.headers } : init?.headers
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    const message = typeof body.error === 'string' ? body.error : `Request failed (${response.status})`;
    throw new Error(message);
  }
  return body as T;
}

const post = <T>(path: string, body?: unknown) =>
  request<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) });

export interface RunLogsResponse {
  logs: string;
  /** Rendered tmux pane, saved snapshot, or the raw terminal stream. */
  logSource?: 'screen' | 'snapshot' | 'stream';
  denials?: PolicyDenial[];
  denialsError?: string;
}

async function postDenial<T>(path: string, input: DenialRequest): Promise<T> {
  const response = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input)
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error(
      typeof body.error === 'string' ? body.error : `Request failed (${response.status})`
    ) as DenialError;
    error.outcome = body.outcome;
    error.policyDiff = body.policyDiff;
    error.rolledBack = body.rolledBack;
    throw error;
  }
  return body as T;
}

export const api = {
  overview: () => request<Overview>('/api/overview'),
  runs: () => request<Run[]>('/api/runs'),
  run: (id: string) => request<Run>(`/api/runs/${id}`),
  startRun: (input: StartRunRequest) => post<Run>('/api/runs', input),
  sleepRun: (id: string) => post<Run>(`/api/runs/${id}/sleep`),
  resumeRun: (id: string) => post<Run>(`/api/runs/${id}/resume`),
  finishRun: (id: string) => post<Run>(`/api/runs/${id}/finish`),
  syncRun: (id: string) => post<Run>(`/api/runs/${id}/sync`),
  deleteRun: (id: string) => post<{ deleted: true }>(`/api/runs/${id}/delete`, { discard: true }),
  logs: (id: string, tail?: number) => request<RunLogsResponse>(`/api/runs/${id}/logs${tail ? `?tail=${tail}` : ''}`),
  shells: (id: string) => request<string[]>(`/api/runs/${id}/shells`),
  createShell: (id: string) => post<{ name: string }>(`/api/runs/${id}/shells`),
  deleteShell: (id: string, name: string) => post<{ deleted: true }>(`/api/runs/${id}/shells/${encodeURIComponent(name)}/delete`),
  diff: (id: string) => request<{ diff: string }>(`/api/runs/${id}/diff`),
  policy: (id: string) => request<Record<string, unknown>>(`/api/runs/${id}/policy`),
  runPolicyContent: (id: string) => request<RunPolicyContent>(`/api/runs/${id}/policy/content`),
  previewDenial: (id: string, input: DenialRequest) => postDenial<DenialPreview>(`/api/runs/${id}/policy/denial/preview`, input),
  applyDenial: (id: string, input: DenialRequest) => postDenial<DenialApplyResult>(`/api/runs/${id}/policy/denial`, input),

  projects: () => request<Project[]>('/api/projects'),
  saveProject: (project: Project) => post<Project>('/api/projects', project),
  deleteProject: (name: string) => post<{ deleted: true }>(`/api/projects/${encodeURIComponent(name)}/delete`),
  profiles: () => request<Profile[]>('/api/profiles'),
  saveProfile: (profile: Profile) => post<Profile>('/api/profiles', profile),
  deleteProfile: (name: string) => post<{ deleted: true }>(`/api/profiles/${encodeURIComponent(name)}/delete`),
  providers: (gateway = '') => request<Provider[]>(`/api/providers${gateway ? `?gateway=${encodeURIComponent(gateway)}` : ''}`),
  providerModels: (name: string, gateway = '') => request<string[]>(`/api/providers/${encodeURIComponent(name)}/models${gateway ? `?gateway=${encodeURIComponent(gateway)}` : ''}`),
  setupProvider: (input: { kind: string; name: string; host: string; credential: string; gateway: string }) => post<{ ok: true }>('/api/providers', input),
  updateProvider: (name: string, input: { credential: string; gateway: string }) => post<{ ok: true }>(`/api/providers/${encodeURIComponent(name)}/update`, input),
  deleteProvider: (name: string, gateway: string) => post<{ deleted: true }>(`/api/providers/${encodeURIComponent(name)}/delete?gateway=${encodeURIComponent(gateway)}`),

  images: () => request<ImageArtifact[]>('/api/images'),
  imageVersion: (agent: string) => post<{ version: string }>('/api/images/version', { agent }),
  buildImage: (agent: string, version: string) => post<GuiTask>('/api/images/build', { agent, version }),
  task: (id: string) => request<GuiTask>(`/api/tasks/${encodeURIComponent(id)}`),
  stopTask: (id: string) => post<GuiTask>(`/api/tasks/${encodeURIComponent(id)}/stop`),
  refreshImages: () => post<GuiTask>('/api/images/refresh'),
  deleteImage: (reference: string) => post<{ deleted: true }>('/api/images/delete', { reference }),
  pruneImages: () => post<{ removed: string[] }>('/api/images/prune'),
  pullImage: (reference: string) => post<{ ok: true }>('/api/images/pull', { reference }),
  pushImage: (localTag: string, remote: string) => post<{ ok: true }>('/api/images/push', { localTag, remote }),
  validateRepoSetup: (input: { project: string; agents: string[] }) => post<RepoSetupResult>('/api/repos/setup', { ...input, dry: true }),
  setupRepo: (input: { project: string; agents: string[] }) => post<GuiTask>('/api/repos/setup', { ...input, dry: false }),
  registry: () => request<Registry>('/api/registry'),
  saveRegistry: (reference: string) => post<Registry>('/api/registry', { reference }),
  clearRegistry: () => post<{ cleared: true }>('/api/registry/clear'),
  discoverRegistry: () => post<ImageArtifact[]>('/api/registry/discover'),

  gateways: () => request<Gateway[]>('/api/gateways'),
  connectGateway: (input: { url: string; name: string }) => post<{ ok: true }>('/api/gateways/connect', input),
  startGateway: () => post<{ ok: true }>('/api/gateways/start'),
  deleteGateway: (name: string) => post<{ deleted: true }>(`/api/gateways/${encodeURIComponent(name)}/delete`),
  gatewayInfo: (name: string) => request<unknown>(`/api/gateways/${encodeURIComponent(name)}/info`),

  mcps: () => request<MCPTools[]>('/api/mcps'),
  createMCP: (entry: MCPTools) => post<MCPTools>('/api/mcps', entry),
  deleteMCP: (name: string) => post<{ deleted: true }>(`/api/mcps/${encodeURIComponent(name)}/delete`),
  mcpServers: (agent: string) => request<string[]>(`/api/mcps/servers?agent=${encodeURIComponent(agent)}`),
  localMCPs: () => request<LocalMCPFiles[]>('/api/mcps/local'),

  skills: () => request<SkillsEntry[]>('/api/skills'),
  createSkills: (entry: SkillsEntry) => post<SkillsEntry>('/api/skills', entry),
  deleteSkills: (name: string) => post<{ deleted: true }>(`/api/skills/${encodeURIComponent(name)}/delete`),
  availableSkills: (agent: string) => request<AvailableSkills>(`/api/skills/available?agent=${encodeURIComponent(agent)}`),
  skillsStatus: () => request<SkillStatus[]>('/api/skills/status'),

  settings: () => request<Settings>('/api/settings'),
  saveSettings: (settings: Settings) => post<Settings>('/api/settings', settings),
  doctor: () => request<Check[]>('/api/doctor')
};
