export type RunStatus =
  | 'creating'
  | 'ready'
  | 'working'
  | 'waiting'
  | 'failed'
  | 'unreachable'
  | 'stopped'
  | 'completed';

export interface Run {
  id: string;
  name?: string;
  created: string;
  source: string;
  project?: string;
  worktree: string;
  worktree_isolated?: boolean;
  branch: string;
  baseline: string;
  last_synced: string;
  sandbox: string;
  gateway: string;
  agent: string;
  image: string;
  mode: string;
  model: string;
  approval: string;
  status: RunStatus;
  status_detail?: string;
  synced: boolean;
  policy: string;
  mcp?: string;
  mcp_warnings?: string[];
  skills?: string;
  skills_warnings?: string[];
}

export interface Project {
  version: number;
  name: string;
  root: string;
  worktree?: boolean;
}

export interface Profile {
  version: number;
  name: string;
  agent: string;
  image: string;
  mode: string;
  model: string;
  provider: string;
  policy: string;
  approval: string;
  base_url?: string;
  api_key_env?: string;
  args?: string[];
}

export interface ImageArtifact {
  reference: string;
  agent: string;
  version: string;
  recipe: number;
  os?: string;
  arch?: string;
  digest?: string;
  used_by?: string[];
}

export interface Gateway {
  name: string;
  active: boolean;
  endpoint: string;
  type: string;
  auth: string;
  source: string;
}

export interface Provider {
  name: string;
  type?: string;
  profile?: string;
  credential?: string;
  credential_keys?: string[];
  base_url?: string;
  gateway?: string;
  used_by?: string[];
  builtin?: boolean;
  registered?: boolean;
  credential_present?: boolean;
  error?: string;
}

export interface PolicyConfiguration {
  name: string;
  path: string;
  used_by?: string[];
}

export interface MCPTools {
  version: number;
  name: string;
  agent: string;
  servers: string[];
}

export interface SkillsEntry {
  version: number;
  name: string;
  agent: string;
  skills: string[];
  plugins: string[];
}

export interface SkillInfo {
  name: string;
  description?: string;
}

export interface PluginInfo {
  id: string;
  plugin: string;
  marketplace: string;
  skills: number;
}

export interface AvailableSkills {
  agent: string;
  skills: SkillInfo[];
  plugins: PluginInfo[];
}

export interface LocalMCPFiles {
  project: string;
  root: string;
  files: string[];
}

export interface Check {
  name: string;
  ok: boolean;
  detail: string;
}

export interface Settings {
  version: number;
  worktrees: {
    root: string;
    branch_prefix: string;
  };
}

export interface Registry {
  reference: string;
}

export interface GuiTask {
  id: string;
  kind: string;
  status: 'running' | 'succeeded' | 'failed' | 'canceled';
  output: string;
  error?: string;
  created_at: string;
  updated_at: string;
  stop_requested?: boolean;
}

export interface Summary {
  active: number;
  finished: number;
  failed: number;
  unsynced: number;
  images: number;
  gateways: number;
  profiles: number;
  projects: number;
}

export interface ProjectRunNode {
  project: string;
  root: string;
  isolation: 'worktree' | 'direct';
  activeRuns: Run[];
  idleRuns: Run[];
  finishedRuns: Run[];
}

export interface Overview {
  summary: Summary;
  tree: ProjectRunNode[];
  runs: Run[];
  projects: Project[];
  profiles: Profile[];
  images: ImageArtifact[];
  gateways: Gateway[];
  doctor: Check[];
}

export interface StartRunRequest {
  profile: string;
  project: string;
  name?: string;
  gateway: string;
  image?: string;
  mode?: string;
  model?: string;
  approval?: string;
  prompt?: string;
  mcp?: string;
  skills?: string;
}
