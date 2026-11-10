import { useEffect, useMemo, useState } from 'react';
import { api } from '../../api';
import type { Profile } from '../../types';
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

export function ProfilesPanel({ onChanged }: { onChanged?: () => void }) {
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [images, setImages] = useState<string[]>([]);
  const [providers, setProviders] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<Profile>(emptyProfile);

  const load = async () => {
    try {
      const [nextProfiles, nextImages, nextProviders] = await Promise.all([api.profiles(), api.images(), api.providers()]);
      setProfiles(nextProfiles ?? []);
      setImages((nextImages ?? []).map((image) => image.reference));
      setProviders((nextProviders ?? []).map((provider) => provider.name).filter(Boolean));
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  const providerOptions = useMemo(() => {
    const values = new Set(providers);
    if (form.provider) values.add(form.provider);
    return Array.from(values).sort();
  }, [providers, form.provider]);

  const save = async () => {
    try {
      await api.saveProfile({ ...form, policy: form.policy || `~/.agentbox/policies/${form.name}.yaml` });
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
        <Button size="sm" onClick={() => { setForm({ ...emptyProfile, image: images[0] ?? '', provider: providerOptions[0] ?? '' }); setOpen(true); }}>Create profile</Button>
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
                <TD><Button size="xs" variant="ghost" onClick={() => { setForm(profile); setOpen(true); }}>Edit</Button></TD>
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
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Name" htmlFor="profile-name" required><Input id="profile-name" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} /></Field>
          <Field label="Agent" htmlFor="profile-agent" required>
            <Select id="profile-agent" value={form.agent} onChange={(event) => setForm({ ...form, agent: event.target.value })}>
              {agents.map((agent) => <option key={agent} value={agent}>{agent}</option>)}
            </Select>
          </Field>
          <Field label="Image" htmlFor="profile-image" required><Input id="profile-image" list="profile-images" value={form.image} onChange={(event) => setForm({ ...form, image: event.target.value })} /></Field>
          <datalist id="profile-images">{images.map((image) => <option key={image} value={image} />)}</datalist>
          <Field label="Mode" htmlFor="profile-mode" required>
            <Select id="profile-mode" value={form.mode} onChange={(event) => setForm({ ...form, mode: event.target.value })}>
              {modes.map((mode) => <option key={mode} value={mode}>{mode}</option>)}
            </Select>
          </Field>
          <Field label="Model" htmlFor="profile-model"><Input id="profile-model" value={form.model} onChange={(event) => setForm({ ...form, model: event.target.value })} /></Field>
          <Field label="Base URL" htmlFor="profile-base-url" helper="Gateway mode; usually ends in /v1."><Input id="profile-base-url" value={form.base_url ?? ''} onChange={(event) => setForm({ ...form, base_url: event.target.value })} /></Field>
          <Field label="API key env" htmlFor="profile-api-key-env"><Input id="profile-api-key-env" value={form.api_key_env ?? ''} onChange={(event) => setForm({ ...form, api_key_env: event.target.value })} /></Field>
          <Field label="Provider" htmlFor="profile-provider" required>
            <Select id="profile-provider" value={form.provider} onChange={(event) => setForm({ ...form, provider: event.target.value })}>
              {providerOptions.map((provider) => <option key={provider} value={provider}>{provider}</option>)}
            </Select>
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
