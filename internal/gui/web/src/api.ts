import type {
  Check,
  Gateway,
  ImageArtifact,
  Overview,
  PolicyConfiguration,
  Profile,
  Project,
  Provider,
  Registry,
  Run,
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

export const api = {
  overview: () => request<Overview>('/api/overview'),
  runs: () => request<Run[]>('/api/runs'),
  run: (id: string) => request<Run>(`/api/runs/${id}`),
  startRun: (input: StartRunRequest) => post<Run>('/api/runs', input),
  stopRun: (id: string) => post<Run>(`/api/runs/${id}/stop`),
  syncRun: (id: string) => post<Run>(`/api/runs/${id}/sync`),
  deleteRun: (id: string) => post<{ deleted: true }>(`/api/runs/${id}/delete`, { discard: true }),
  logs: (id: string) => request<{ logs: string }>(`/api/runs/${id}/logs`),
  diff: (id: string) => request<{ diff: string }>(`/api/runs/${id}/diff`),
  policy: (id: string) => request<Record<string, unknown>>(`/api/runs/${id}/policy`),

  projects: () => request<Project[]>('/api/projects'),
  saveProject: (project: Project) => post<Project>('/api/projects', project),
  profiles: () => request<Profile[]>('/api/profiles'),
  saveProfile: (profile: Profile) => post<Profile>('/api/profiles', profile),
  providers: (gateway = '') => request<Provider[]>(`/api/providers${gateway ? `?gateway=${encodeURIComponent(gateway)}` : ''}`),
  setupProvider: (input: { kind: string; name: string; host: string; credential: string; gateway: string }) => post<{ ok: true }>('/api/providers', input),
  updateProvider: (name: string, input: { credential: string; gateway: string }) => post<{ ok: true }>(`/api/providers/${encodeURIComponent(name)}/update`, input),
  deleteProvider: (name: string, gateway: string) => post<{ deleted: true }>(`/api/providers/${encodeURIComponent(name)}/delete?gateway=${encodeURIComponent(gateway)}`),

  images: () => request<ImageArtifact[]>('/api/images'),
  imageVersion: (agent: string) => post<{ version: string }>('/api/images/version', { agent }),
  buildImage: (agent: string, version: string) => post<GuiTask>('/api/images/build', { agent, version }),
  task: (id: string) => request<GuiTask>(`/api/tasks/${encodeURIComponent(id)}`),
  stopTask: (id: string) => post<GuiTask>(`/api/tasks/${encodeURIComponent(id)}/stop`),
  pullImage: (reference: string) => post<{ ok: true }>('/api/images/pull', { reference }),
  pushImage: (localTag: string, remote: string) => post<{ ok: true }>('/api/images/push', { localTag, remote }),
  registry: () => request<Registry>('/api/registry'),
  saveRegistry: (reference: string) => post<Registry>('/api/registry', { reference }),
  discoverRegistry: () => post<ImageArtifact[]>('/api/registry/discover'),

  gateways: () => request<Gateway[]>('/api/gateways'),
  connectGateway: (input: { url: string; name: string }) => post<{ ok: true }>('/api/gateways/connect', input),
  startGateway: () => post<{ ok: true }>('/api/gateways/start'),
  gatewayInfo: (name: string) => request<unknown>(`/api/gateways/${encodeURIComponent(name)}/info`),

  policies: () => request<PolicyConfiguration[]>('/api/policies'),
  createPolicy: (name: string) => post<PolicyConfiguration>('/api/policies', { name }),
  policyContent: (name: string) => request<{ name: string; content: string }>(`/api/policies/${encodeURIComponent(name)}`),
  savePolicyContent: (name: string, content: string) => post<PolicyConfiguration>(`/api/policies/${encodeURIComponent(name)}`, { content }),
  addPolicyHost: (name: string, agent: string, host: string) => post<PolicyConfiguration>(`/api/policies/${encodeURIComponent(name)}/host`, { agent, host }),
  applyPolicy: (name: string, runId: string) => post<{ ok: true }>(`/api/policies/${encodeURIComponent(name)}/apply`, { runId }),

  settings: () => request<Settings>('/api/settings'),
  saveSettings: (settings: Settings) => post<Settings>('/api/settings', settings),
  doctor: () => request<Check[]>('/api/doctor')
};
