export type RunStatus =
  | 'creating'
  | 'ready'
  | 'working'
  | 'waiting'
  | 'failed'
  | 'unreachable'
  | 'stopped'
  | 'completed'
  | 'exited';

export type SyncState = 'no_changes' | 'up_to_date' | 'needs_sync' | 'blocked' | 'wrong_checkout' | 'unknown';

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
  last_successful_sync_at?: string;
  synced_sandbox_fingerprint?: string;
  sync_state: SyncState;
  sandbox: string;
  gateway: string;
  agent: string;
  agent_binary?: string;
  image: string;
  base_image?: string;
  image_source?: string;
  image_repo?: string;
  manifest_digest?: string;
  image_warnings?: string[];
  policy_fragment?: string;
  policy_fragment_digest?: string;
  policy_provenance?: RunPolicyProvenance;
  runtime_hosts?: RuntimeHost[];
  mode: string;
  model: string;
  approval: string;
  status: RunStatus;
  status_detail?: string;
  exited?: boolean;
  exit_code?: number;
  profile?: string;
  synced: boolean;
  policy: string;
  pending_policy?: string;
  mcp?: string;
  mcp_warnings?: string[];
  skills?: string;
  skills_warnings?: string[];
}

export interface RunPolicyProvenance {
  source: 'derived' | 'derived+legacy';
  gateway: string;
  provider: string;
  mode: string;
  host: string;
  port: number;
  rule: string;
  agent_binary: string;
  legacy_path?: string;
}

export interface ProjectRepoStatus {
  manifest_present: boolean;
  manifest_path: string;
  manifest_digest?: string;
  fragment_path?: string;
  fragment_digest?: string;
  fragment_status: 'absent' | 'current' | 'stale' | 'invalid';
  host_rules?: RepoHostRule[];
  worker_images: Array<{
    agent: string;
    status: 'current' | 'stale' | 'missing' | 'unknown';
    reference?: string;
    manifest_digest?: string;
  }>;
  warnings?: string[];
}

export interface RepoHostRule {
  host: string;
  port: number;
  binaries: string[];
}

export type CheckoutState = 'up-to-date' | 'behind' | 'ahead' | 'diverged' | 'no-upstream' | 'unknown';

export interface ProjectCheckout {
  state: CheckoutState;
  behind: number;
  ahead: number;
  reason?: string;
}

export interface Project {
  version: number;
  name: string;
  root: string;
  worktree?: boolean;
  used_by?: string[];
  manifest?: boolean;
  repo?: string;
  repo_status?: ProjectRepoStatus;
  checkout?: ProjectCheckout;
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
  default_policy?: string;
  approval: string;
  base_url?: string;
  api_key_env?: string;
  args?: string[];
  used_by?: string[];
}

export interface ImageArtifact {
  reference: string;
  agent: string;
  version: string;
  recipe: number;
  os?: string;
  arch?: string;
  digest?: string;
  worker?: boolean;
  repo?: string;
  manifest_digest?: string;
  base_reference?: string;
  used_by?: string[];
}

export interface Gateway {
  name: string;
  active: boolean;
  endpoint: string;
  type: string;
  auth: string;
  source: string;
  used_by?: string[];
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
  mode?: string;
  error?: string;
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

export interface SkillInstallation {
  target: string;
  path: string;
  present: boolean;
  version?: string;
  state: string;
  detail?: string;
}

export interface SkillFileStatus {
  name: string;
  path: string;
  state: string;
  detail?: string;
}

export interface SkillStatus {
  name: string;
  kind?: 'skill' | 'role';
  version: string;
  installations: SkillInstallation[];
  files?: SkillFileStatus[];
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
  state?: 'ok' | 'fail' | 'unavailable';
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

export interface RepoSetupImage {
  agent: string;
  base: string;
  tag: string;
  manifest_digest: string;
  built: boolean;
  image_id?: string;
  digest?: string;
}

export interface RepoSetupResult {
  repo: string;
  root: string;
  manifest_path: string;
  manifest_digest: string;
  agents: string[];
  fragment_path: string;
  fragment_status: string;
  images: RepoSetupImage[];
  dry?: boolean;
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
  model?: string;
  approval?: string;
  prompt?: string;
  mcp?: string;
  skills?: string;
}

export type PolicyEditOutcome =
  | 'network-preview'
  | 'static-preview'
  | 'live-applied'
  | 'static-pending'
  | 'no-live-sandbox'
  | 'openshell-rejected';

export interface RuntimeHost {
  host: string;
  port: number;
  binary: string;
  rule: string;
  source: 'run' | 'run+manifest' | string;
  added_at: string;
}

export interface PolicyDenial {
  host: string;
  port: number;
  binary: string;
  observedBinary?: string;
  reason?: string;
  logLine?: string;
  valid: boolean;
  invalidReason?: string;
}

export interface DenialRequest {
  host: string;
  port: number;
  binary: string;
  persist?: boolean;
}

export interface RepoHostRule {
  host: string;
  port: number;
  binaries: string[];
}

export interface DenialPreview {
  entry: RepoHostRule;
  policyPath: string;
  policyDiff: string;
  persistAvailable: boolean;
  persistReason?: string;
  manifestPath?: string;
  manifestDiff?: string;
  fragmentPath?: string;
  fragmentDiff?: string;
}

export interface DenialApplyResult extends DenialPreview {
  outcome: PolicyEditOutcome;
  rolledBack: boolean;
  runtimeHost: RuntimeHost;
  persisted: boolean;
  fragmentStatus?: string;
}

export interface RunPolicyContent {
  path: string;
  content: string;
}

export interface DenialError extends Error {
  outcome?: PolicyEditOutcome;
  policyDiff?: string;
  rolledBack?: boolean;
}
