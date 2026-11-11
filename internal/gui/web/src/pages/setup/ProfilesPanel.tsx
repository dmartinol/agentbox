import { useEffect, useMemo, useState } from 'react';
import { api } from '../../api';
import type { ImageArtifact, Profile, Provider } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { EmptyState } from '../../components/ui/empty';
import { Field } from '../../components/ui/field';
import { Input, Select } from '../../components/ui/input';
import { Modal } from '../../components/ui/dialog';
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/table';

const agents = ['codex', 'claude', 'opencode'];
const modes = ['standard', 'gateway'];
const approvals = ['prompted', 'unattended'];

const emptyProfile: Profile = {
  version: 1, name: '', agent: 'codex', image: '', mode: 'gateway', model: '',
  provider: '', policy: '', approval: 'prompted', base_url: '', api_key_env: ''
};

function defaultImageFor(images: ImageArtifact[], agent: string, current = '') {
  const currentImage = images.find((image) => image.reference === current);
  if (currentImage?.agent === agent) return currentImage.reference;
  return images
    .filter((image) => image.agent === agent)
    .sort((left, right) => right.version.localeCompare(left.version, undefined, { numeric: true }))[0]?.reference ?? '';
}

function providerValues(provider: Provider | undefined) {
  return {
    base_url: provider?.base_url ?? '',
    api_key_env: provider?.credential ?? provider?.credential_keys?.[0] ?? ''
  };
}

function providerMode(provider: Provider | undefined) {
  switch (provider?.type) {
    case 'openai':
    case 'anthropic':
      return 'standard';
    case 'gateway-bearer':
    case 'gateway-header':
      return 'gateway';
    default:
      return undefined;
  }
}

function isDirectProvider(provider: Provider | undefined) {
  return providerMode(provider) === 'standard';
}

function usesProviderDefaults(profile: Profile, provider: Provider | undefined) {
  const defaults = providerValues(provider);
  return Boolean(provider && defaults.base_url && defaults.api_key_env &&
    profile.base_url === defaults.base_url && profile.api_key_env === defaults.api_key_env);
}

export function ProfilesPanel({ onChanged }: { onChanged?: () => void }) {
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [images, setImages] = useState<ImageArtifact[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<Profile>(emptyProfile);
  const [formError, setFormError] = useState('');
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
  const modelOptions = useMemo(() => {
    const values = new Set(models);
    if (form.model) values.add(form.model);
    return Array.from(values).sort();
  }, [models, form.model]);

  const agentImages = useMemo(() => images
    .filter((image) => image.agent === form.agent)
    .sort((left, right) => right.version.localeCompare(left.version, undefined, { numeric: true })), [images, form.agent]);

  const openCreate = () => {
    const provider = providers[0];
    setForm({
      ...emptyProfile,
      image: defaultImageFor(images, 'codex'),
      provider: provider?.name ?? '',
      mode: providerMode(provider) ?? 'gateway',
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
    setForm(profile);
    setUseProviderSettings(usesProviderDefaults(profile, provider));
    setFormError('');
    setModels([]);
    setModelError('');
    setOpen(true);
  };

  const selectProvider = (name: string) => {
    const provider = providers.find((item) => item.name === name);
    const mode = providerMode(provider);
    setFormError('');
    setModels([]);
    setModelError('');
    setForm((current) => ({
      ...current,
      provider: name,
      mode: mode ?? current.mode,
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
    if (isDirectProvider(selectedProvider) && form.mode === 'gateway') {
      setFormError('Direct OpenAI and Anthropic providers require standard mode.');
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
      setError(err as Error);
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
                <TD className="truncate font-medium">{profile.name}</TD>
                <TD>{profile.agent}</TD>
                <TD>{profile.mode}</TD>
                <TD className="truncate font-mono text-xs text-muted">{profile.model || '—'}</TD>
                <TD className="truncate">{profile.provider}</TD>
                <TD><Button size="xs" variant="ghost" onClick={() => openEdit(profile)}>Edit</Button></TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}

      <Modal
        open={open}
        onOpenChange={setOpen}
        title={profiles.some((profile) => profile.name === form.name) ? `Edit ${form.name}` : 'Create profile'}
        description="Profiles reference an OpenShell provider and define agent defaults."
        footer={<><Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button><Button onClick={() => void save()}>Save</Button></>}
      >
        {formError ? <Alert variant="danger" title="Unable to save profile" className="mb-3">{formError}</Alert> : null}
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Name" htmlFor="profile-name" required><Input id="profile-name" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} /></Field>
          <Field label="Agent" htmlFor="profile-agent" required>
            <Select id="profile-agent" value={form.agent} onChange={(event) => selectAgent(event.target.value)}>
              {agents.map((agent) => <option key={agent} value={agent}>{agent}</option>)}
            </Select>
          </Field>
          <Field label="Image" htmlFor="profile-image" required helper="Only images matching the selected agent are listed.">
            <Select id="profile-image" value={form.image} onChange={(event) => setForm({ ...form, image: event.target.value })}>
              {agentImages.length ? agentImages.map((image) => <option key={image.reference} value={image.reference}>{image.reference}</option>) : <option value="">No matching image</option>}
            </Select>
          </Field>
          <Field label="Mode" htmlFor="profile-mode" required>
            <Select id="profile-mode" value={form.mode} onChange={(event) => setForm({ ...form, mode: event.target.value })}>
              {modes.map((mode) => <option key={mode} value={mode}>{mode}</option>)}
            </Select>
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
          <Field label="Policy" htmlFor="profile-policy" helper="Leave blank to create and use the profile's default policy." className="sm:col-span-2">
            <Input id="profile-policy" value={form.policy} onChange={(event) => setForm({ ...form, policy: event.target.value })} />
          </Field>
          <Field label="Approval" htmlFor="profile-approval" required>
            <Select id="profile-approval" value={form.approval} onChange={(event) => setForm({ ...form, approval: event.target.value })}>
              {approvals.map((approval) => <option key={approval} value={approval}>{approval}</option>)}
            </Select>
          </Field>
        </div>
      </Modal>
    </div>
  );
}
