import { useEffect, useMemo, useState } from 'react';
import { api } from '../../api';
import { approvalOptions, defaultApproval } from '../../approvals';
import type { ImageArtifact, Profile, Provider } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { ActionButton } from '../../components/ui/action';
import { ConfirmDialog } from '../../components/ui/confirm';
import { ExpandableValue } from '../../components/ui/copy-value';
import { EmptyState } from '../../components/ui/empty';
import { Field } from '../../components/ui/field';
import { Input, Select } from '../../components/ui/input';
import { Modal } from '../../components/ui/dialog';
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/table';

const agents = ['codex', 'claude', 'opencode'];

const emptyProfile: Profile = {
  version: 1, name: '', agent: 'codex', image: '', mode: '', model: '',
  provider: '', policy: '', approval: defaultApproval, base_url: '', api_key_env: ''
};

function defaultImageFor(images: ImageArtifact[], agent: string, current = '') {
  const currentImage = images.find((image) => image.reference === current);
  if (currentImage?.agent === agent) return currentImage.reference;
  return images
    .filter((image) => !image.worker && image.agent === agent)
    .sort((left, right) => right.version.localeCompare(left.version, undefined, { numeric: true }))[0]?.reference ?? '';
}

function providerValues(provider: Provider | undefined) {
  return {
    base_url: provider?.base_url ?? '',
    api_key_env: provider?.credential ?? provider?.credential_keys?.[0] ?? ''
  };
}

function usesProviderDefaults(profile: Profile, provider: Provider | undefined) {
  const defaults = providerValues(provider);
  return Boolean(provider && defaults.base_url && defaults.api_key_env &&
    profile.base_url === defaults.base_url && profile.api_key_env === defaults.api_key_env);
}

function cleanPath(value: string) {
  if (!value) return '';
  const normalized = value.replace(/\\/g, '/');
  const drive = /^[a-zA-Z]:/.test(normalized);
  const absolute = normalized.startsWith('/');
  const parts: string[] = [];
  for (const part of normalized.split('/')) {
    if (!part || part === '.') continue;
    if (part === '..' && parts.length && parts[parts.length - 1] !== '..') {
      parts.pop();
      continue;
    }
    parts.push(part);
  }
  const cleaned = `${absolute ? '/' : ''}${parts.join('/')}`;
  return drive ? cleaned.charAt(0).toLowerCase() + cleaned.slice(1) : cleaned;
}

function isDefaultProfilePolicy(profile: Profile) {
  if (!profile.default_policy) return false;
  return cleanPath(profile.policy) === cleanPath(profile.default_policy);
}

export function ProfilesPanel({ onChanged }: { onChanged?: () => void }) {
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [images, setImages] = useState<ImageArtifact[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [open, setOpen] = useState(false);
  const [editingProfile, setEditingProfile] = useState(false);
  const [form, setForm] = useState<Profile>(emptyProfile);
  const [formError, setFormError] = useState('');
  const [deleteProfile, setDeleteProfile] = useState<Profile | null>(null);
  const [deleteError, setDeleteError] = useState('');
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [useProviderSettings, setUseProviderSettings] = useState(true);
  const [models, setModels] = useState<string[]>([]);
  const [loadingModels, setLoadingModels] = useState(false);
  const [modelError, setModelError] = useState('');

  const load = async () => {
    try {
      const [nextProfiles, nextImages, nextProviders] = await Promise.all([api.profiles(), api.images(), api.providers()]);
      setProfiles(nextProfiles ?? []);
      setImages(nextImages ?? []);
      setProviders(nextProviders ?? []);
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  const providerOptions = useMemo(() => {
    const values = new Set(providers.map((provider) => provider.name).filter(Boolean));
    if (form.provider) values.add(form.provider);
    return Array.from(values).sort();
  }, [providers, form.provider]);

  const selectedProvider = providers.find((provider) => provider.name === form.provider);
  const selectedProviderMode = selectedProvider?.mode;
  const modelOptions = useMemo(() => {
    const values = new Set(models);
    if (form.model) values.add(form.model);
    return Array.from(values).sort();
  }, [models, form.model]);

  const agentImages = useMemo(() => images
    .filter((image) => !image.worker && image.agent === form.agent)
    .sort((left, right) => right.version.localeCompare(left.version, undefined, { numeric: true })), [images, form.agent]);
  const currentImageMissing = Boolean(form.image) && !agentImages.some((image) => image.reference === form.image);

  const openCreate = () => {
    const provider = providers[0];
    setEditingProfile(false);
    setForm({
      ...emptyProfile,
      image: defaultImageFor(images, 'codex'),
      provider: provider?.name ?? '',
      mode: provider?.mode ?? '',
      ...providerValues(provider)
    });
    setUseProviderSettings(true);
    setFormError('');
    setModels([]);
    setModelError('');
    setOpen(true);
  };

  const openEdit = (profile: Profile) => {
    const provider = providers.find((item) => item.name === profile.provider);
    setEditingProfile(true);
    const isDefaultPolicy = isDefaultProfilePolicy(profile);
    setForm({ ...profile, policy: isDefaultPolicy ? '' : profile.policy, mode: provider?.mode ?? '' });
    setUseProviderSettings(usesProviderDefaults(profile, provider));
    setFormError('');
    setModels([]);
    setModelError('');
    setOpen(true);
  };

  const selectProvider = (name: string) => {
    const provider = providers.find((item) => item.name === name);
    setFormError('');
    setModels([]);
    setModelError('');
    setForm((current) => ({
      ...current,
      provider: name,
      mode: provider?.mode ?? '',
      ...(useProviderSettings ? providerValues(provider) : {})
    }));
  };

  const toggleProviderSettings = (checked: boolean) => {
    setUseProviderSettings(checked);
    if (checked) {
      setForm((current) => ({ ...current, ...providerValues(selectedProvider) }));
    }
  };

  const selectAgent = (agent: string) => {
    setForm((current) => ({ ...current, agent, image: defaultImageFor(images, agent, current.image) }));
  };

  const discoverModels = async () => {
    if (!selectedProvider) return;
    setLoadingModels(true);
    setModelError('');
    try {
      const next = await api.providerModels(selectedProvider.name, selectedProvider.gateway ?? '');
      setModels(next ?? []);
      if (!next?.length) {
        setModelError('No models were returned. Enter a model ID manually or check the provider credential.');
      }
    } catch (err) {
      setModelError((err as Error).message);
    } finally {
      setLoadingModels(false);
    }
  };

  const save = async () => {
    if (form.mode === 'gateway' && !form.model.trim()) {
      setFormError('Gateway mode requires an exact model ID.');
      return;
    }
    setFormError('');
    try {
      await api.saveProfile({ ...form, policy: form.policy || '' });
      setOpen(false);
      setNotice('Profile saved');
      await load();
      onChanged?.();
    } catch (err) {
      setFormError((err as Error).message);
    }
  };

  const remove = async () => {
    if (!deleteProfile) return;
    setDeleteBusy(true);
    setDeleteError('');
    try {
      await api.deleteProfile(deleteProfile.name);
      setDeleteProfile(null);
      setNotice(`Profile ${deleteProfile.name} deleted`);
      await load();
      onChanged?.();
    } catch (err) {
      setDeleteError((err as Error).message);
    } finally {
      setDeleteBusy(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <Button size="sm" onClick={openCreate}>Create profile</Button>
      </div>
      {notice ? <Alert variant="success" title={notice} /> : null}
      {error ? <Alert variant="danger" title="Unable to load profiles">{error.message}</Alert> : null}
      {loading ? <EmptyState title="Loading profiles" /> : profiles.length === 0 ? (
        <EmptyState title="No profiles" description="Create a profile after configuring a provider." />
      ) : (
        <Table>
          <THead><TR><TH className="w-[18%]">Name</TH><TH className="w-[10%]">Agent</TH><TH className="w-[10%]">Mode</TH><TH className="w-[22%]">Model</TH><TH className="w-[18%]">Provider</TH><TH>Action</TH></TR></THead>
          <TBody>
            {profiles.map((profile) => (
              <TR key={profile.name}>
                <TD className="truncate font-medium"><span title={profile.name}>{profile.name}</span></TD>
                <TD label="Agent">{profile.agent}</TD>
                <TD label="Mode">{profile.mode}</TD>
                <TD label="Model"><ExpandableValue value={profile.model || '—'} label={`model for ${profile.name}`} max={40} /></TD>
                <TD label="Provider" className="truncate"><span title={profile.provider}>{profile.provider}</span></TD>
                <TD label="Actions">
                  <div className="flex flex-wrap gap-1">
                    <Button size="xs" variant="ghost" onClick={() => openEdit(profile)}>Edit</Button>
                    <ActionButton
                      size="xs"
                      variant="ghost"
                      className="text-danger"
                      disabledReason={
                        profile.used_by?.length
                          ? `Used by active runs: ${profile.used_by.join(', ')}`
                          : undefined
                      }
                      onClick={() => { setDeleteProfile(profile); setDeleteError(''); }}
                    >
                      Delete
                    </ActionButton>
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}

      <Modal
        open={open}
        onOpenChange={setOpen}
        title={editingProfile ? `Edit ${form.name || 'profile'}` : 'Create profile'}
        description="Profiles reference an OpenShell provider and define agent defaults."
        footer={<><Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button><Button disabled={!selectedProviderMode} onClick={() => void save()}>Save</Button></>}
      >
        {formError ? <Alert variant="danger" title="Unable to save profile" className="mb-3">{formError}</Alert> : null}
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Name" htmlFor="profile-name" required><Input id="profile-name" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} /></Field>
          <Field label="Agent" htmlFor="profile-agent" required>
            <Select id="profile-agent" value={form.agent} onChange={(event) => selectAgent(event.target.value)}>
              {agents.map((agent) => <option key={agent} value={agent}>{agent}</option>)}
            </Select>
          </Field>
          <Field
            label="Image"
            htmlFor="profile-image"
            required
            helper={currentImageMissing ? 'The saved image is not installed locally. Choose an installed image or build it.' : 'Only images matching the selected agent are listed.'}
          >
            <Select id="profile-image" value={form.image} onChange={(event) => setForm({ ...form, image: event.target.value })}>
              {currentImageMissing ? <option value={form.image}>{form.image} (not found locally)</option> : null}
              {agentImages.length
                ? agentImages.map((image) => <option key={image.reference} value={image.reference}>{image.reference}</option>)
                : currentImageMissing ? null : <option value="">No matching image</option>}
            </Select>
          </Field>
          <Field
            label="Mode (derived from provider kind)"
            htmlFor="profile-mode"
            helper="The selected provider determines this mode."
            error={selectedProviderMode ? undefined : 'This provider has an unsupported kind. Choose a supported provider before saving.'}
          >
            <Input id="profile-mode" readOnly value={selectedProviderMode ?? 'Unknown'} />
          </Field>
          <Field
            label="Model"
            htmlFor="profile-model"
            helper={form.mode === 'gateway' ? 'Required in gateway mode. Discover models from the selected provider.' : 'Optional in standard mode; leave blank to use the agent default.'}
            error={modelError}
          >
            <div className="flex gap-2">
              {models.length ? (
                <Select id="profile-model" value={form.model} onChange={(event) => setForm({ ...form, model: event.target.value })}>
                  <option value="">{form.mode === 'gateway' ? 'Select a model' : 'Use agent default'}</option>
                  {modelOptions.map((model) => <option key={model} value={model}>{model}</option>)}
                </Select>
              ) : (
                <Input id="profile-model" value={form.model} onChange={(event) => setForm({ ...form, model: event.target.value })} />
              )}
              <Button variant="outline" disabled={!selectedProvider || loadingModels} onClick={() => void discoverModels()}>
                {loadingModels ? 'Discovering…' : 'Discover'}
              </Button>
            </div>
          </Field>
          <Field label="Provider" htmlFor="profile-provider" required>
            <Select id="profile-provider" value={form.provider} onChange={(event) => selectProvider(event.target.value)}>
              {providerOptions.map((provider) => <option key={provider} value={provider}>{provider}</option>)}
            </Select>
          </Field>
          <label className="flex items-center gap-2 text-sm text-muted sm:col-span-2">
            <input type="checkbox" checked={useProviderSettings} onChange={(event) => toggleProviderSettings(event.target.checked)} className="size-3.5 accent-accent" />
            Use provider base URL and credential environment
          </label>
          <Field label="Base URL" htmlFor="profile-base-url" helper="Model API endpoint used by the agent; usually ends in /v1.">
            <Input id="profile-base-url" disabled={useProviderSettings} value={form.base_url ?? ''} onChange={(event) => setForm({ ...form, base_url: event.target.value })} />
          </Field>
          <Field label="API key env" htmlFor="profile-api-key-env" helper="Name only; value stays external.">
            <Input id="profile-api-key-env" disabled={useProviderSettings} value={form.api_key_env ?? ''} onChange={(event) => setForm({ ...form, api_key_env: event.target.value })} />
          </Field>
          <Field
            label="Policy"
            htmlFor="profile-policy"
            className="sm:col-span-2"
            helper={form.policy
              ? 'Read-only here. Policies are no longer managed in Setup; this legacy file contributes its network rules only. Use the AgentBox default to stop relying on it.'
              : 'AgentBox derives the run policy at start from its default, the selected provider, and the repository manifest. No policy setup is required.'}
          >
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <Input id="profile-policy" readOnly value={form.policy || 'AgentBox default (generated per profile)'} />
              {form.policy ? <Button size="sm" variant="outline" onClick={() => setForm({ ...form, policy: '' })}>Use AgentBox default</Button> : null}
            </div>
          </Field>
          <Field label="Approval" htmlFor="profile-approval" required>
            <Select id="profile-approval" value={form.approval} onChange={(event) => setForm({ ...form, approval: event.target.value })}>
              {approvalOptions.map(({ value, label }) => <option key={value} value={value}>{label}</option>)}
            </Select>
          </Field>
        </div>
      </Modal>

      <ConfirmDialog
        open={Boolean(deleteProfile)}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteProfile(null);
            setDeleteError('');
          }
        }}
        title={deleteProfile ? `Delete ${deleteProfile.name}` : 'Delete profile'}
        confirmLabel="Delete profile"
        busyLabel="Deleting…"
        busy={deleteBusy}
        onConfirm={() => void remove()}
      >
        {deleteError ? <Alert variant="danger" title="Profile deletion failed" className="mb-3">{deleteError}</Alert> : null}
        <p className="text-sm text-muted">
          Remove only the profile YAML. Its policy, image, and run records are kept.
        </p>
      </ConfirmDialog>
    </div>
  );
}
