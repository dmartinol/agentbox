import { useEffect, useMemo, useState } from 'react';
import { api } from '../../api';
import type { Gateway, Provider } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { EmptyState } from '../../components/ui/empty';
import { Field } from '../../components/ui/field';
import { Input, Select } from '../../components/ui/input';
import { Modal } from '../../components/ui/dialog';
import { ConfirmDialog } from '../../components/ui/confirm';
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/table';

const providerKinds = ['gateway-bearer', 'gateway-header', 'openai', 'anthropic'];

export function ProvidersPanel({ onChanged }: { onChanged?: () => void }) {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [gateways, setGateways] = useState<Gateway[]>([]);
  const [gateway, setGateway] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [setupOpen, setSetupOpen] = useState(false);
  const [setupError, setSetupError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [form, setForm] = useState({ kind: 'gateway-bearer', name: '', host: 'https://api.enmaas.devshift.net/v1', credential: 'ENMAAS_KEY', gateway: '' });
  const [editProvider, setEditProvider] = useState<Provider>();
  const [editCredential, setEditCredential] = useState('');
  const [editGateway, setEditGateway] = useState('');
  const [editError, setEditError] = useState('');
  const [editSubmitting, setEditSubmitting] = useState(false);
  const [deleteProvider, setDeleteProvider] = useState<Provider>();
  const [deleteError, setDeleteError] = useState('');
  const [deleteSubmitting, setDeleteSubmitting] = useState(false);

  const load = async (selectedGateway = gateway) => {
    try {
      const [nextProviders, nextGateways] = await Promise.all([api.providers(selectedGateway), api.gateways()]);
      setProviders(nextProviders ?? []);
      setGateways(nextGateways ?? []);
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void (async () => {
      const nextGateways = (await api.gateways()) ?? [];
      const preferred = nextGateways.find((item) => item.active)?.name ?? nextGateways[0]?.name ?? '';
      setGateways(nextGateways);
      setGateway(preferred);
      setForm((current) => ({ ...current, gateway: current.gateway || preferred }));
      await load(preferred);
    })();
  }, []);

  const gatewayOptions = useMemo(() => {
    const names = gateways.map((item) => item.name).filter(Boolean);
    return names.length ? names : ['openshell'];
  }, [gateways]);

  const setup = async () => {
    setSetupError('');
    setSubmitting(true);
    try {
      await api.setupProvider(form);
      setSetupOpen(false);
      setNotice('Provider setup completed');
      await load(form.gateway);
      onChanged?.();
    } catch (err) {
      console.error('[AgentBox] provider setup failed:', err);
      setSetupError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  const update = async () => {
    if (!editProvider) return;
    setEditError('');
    setEditSubmitting(true);
    try {
      await api.updateProvider(editProvider.name, { credential: editCredential, gateway: editGateway });
      setEditProvider(undefined);
      setNotice(`Provider ${editProvider.name} updated`);
      await load(editGateway);
    } catch (err) {
      setEditError((err as Error).message);
    } finally {
      setEditSubmitting(false);
    }
  };

  const remove = async () => {
    if (!deleteProvider) return;
    setDeleteError('');
    setDeleteSubmitting(true);
    try {
      await api.deleteProvider(deleteProvider.name, deleteProvider.gateway || gateway);
      setNotice(`Provider ${deleteProvider.name} deleted`);
      setDeleteProvider(undefined);
      await load(deleteProvider.gateway || gateway);
    } catch (err) {
      setDeleteError((err as Error).message);
    } finally {
      setDeleteSubmitting(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <Select aria-label="Provider gateway" className="w-40" value={gateway} onChange={(event) => { setGateway(event.target.value); setForm((current) => ({ ...current, gateway: event.target.value })); void load(event.target.value); }}>
          {gatewayOptions.map((name) => <option key={name} value={name}>{name}</option>)}
        </Select>
        <Button size="sm" onClick={() => { setSetupError(''); setSetupOpen(true); }}>Set up provider</Button>
      </div>
      {notice ? <Alert variant="success" title={notice} /> : null}
      {error ? <Alert variant="danger" title="Unable to load providers">{error.message}</Alert> : null}
      {loading ? <EmptyState title="Loading providers" /> : providers.length === 0 ? (
        <EmptyState title="No providers" description="Set up an EnMaaS or direct provider before creating gateway profiles." />
      ) : (
        <Table>
          <THead><TR><TH className="w-[18%]">Name</TH><TH className="w-[18%]">Type</TH><TH className="w-[18%]">Credential</TH><TH className="w-[20%]">Used by</TH><TH>Actions</TH></TR></THead>
          <TBody>
            {providers.map((provider) => {
              const usedCount = provider.used_by?.length ?? 0;
              return (
                <TR key={`${provider.gateway ?? gateway}/${provider.name}`}>
                  <TD className="truncate font-medium">{provider.name}</TD>
                  <TD className="truncate text-muted">{provider.type || '-'}</TD>
                  <TD className="truncate font-mono text-xs">{provider.credential_keys?.length ? provider.credential_keys.join(', ') : provider.credential || '-'}</TD>
                  <TD className="truncate">
                    {usedCount ? <><Badge tone="warning">{usedCount}</Badge> <span className="text-muted">{provider.used_by?.join(', ')}</span></> : <span className="text-muted">—</span>}
                  </TD>
                  <TD>
                    <div className="flex gap-1">
                      <Button size="xs" variant="ghost" onClick={() => { setEditProvider(provider); setEditCredential(provider.credential ?? provider.credential_keys?.[0] ?? ''); setEditGateway(provider.gateway || gateway); setEditError(''); }}>Edit</Button>
                      <Button size="xs" variant="ghost" className="text-danger" disabled={usedCount > 0} title={usedCount ? 'Delete dependent profiles first' : undefined} onClick={() => { setDeleteError(''); setDeleteProvider(provider); }}>Delete</Button>
                    </div>
                  </TD>
                </TR>
              );
            })}
          </TBody>
        </Table>
      )}

      <Modal
        open={setupOpen}
        onOpenChange={setSetupOpen}
        title="Set up provider"
        description="Create an OpenShell inference provider."
        footer={<><Button variant="ghost" onClick={() => setSetupOpen(false)}>Cancel</Button><Button loading={submitting} onClick={() => void setup()}>{submitting ? 'Setting up' : 'Set up'}</Button></>}
      >
        {setupError ? <Alert variant="danger" title="Provider setup failed" className="mb-3">{setupError}</Alert> : null}
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Type" htmlFor="provider-kind" required helper="Bearer for Codex/OpenCode; header for Claude.">
            <Select id="provider-kind" value={form.kind} onChange={(event) => {
              const kind = event.target.value;
              setForm({
                ...form,
                kind,
                host: kind === 'openai' ? 'api.openai.com' : kind === 'anthropic' ? 'api.anthropic.com' : form.host,
                credential: kind === 'openai' ? 'OPENAI_API_KEY' : kind === 'anthropic' ? 'ANTHROPIC_API_KEY' : form.credential
              });
            }}>
              {providerKinds.map((kind) => <option key={kind} value={kind}>{kind}</option>)}
            </Select>
          </Field>
          <Field label="Name" htmlFor="provider-name" required>
            <Input id="provider-name" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
          </Field>
          <Field label="Provider host" htmlFor="provider-host" required helper="A hostname or URL; AgentBox extracts the hostname for OpenShell.">
            <Input id="provider-host" disabled={form.kind === 'openai' || form.kind === 'anthropic'} value={form.host} onChange={(event) => setForm({ ...form, host: event.target.value })} />
          </Field>
          <Field label="Credential env" htmlFor="provider-credential" required helper="Name only; value stays external.">
            <Input id="provider-credential" disabled={form.kind === 'openai' || form.kind === 'anthropic'} value={form.credential} onChange={(event) => setForm({ ...form, credential: event.target.value })} />
          </Field>
          <Field label="Gateway" htmlFor="provider-gateway" required>
            <Select id="provider-gateway" value={form.gateway} onChange={(event) => setForm({ ...form, gateway: event.target.value })}>
              {gatewayOptions.map((name) => <option key={name} value={name}>{name}</option>)}
            </Select>
          </Field>
        </div>
      </Modal>

      <Modal
        open={Boolean(editProvider)}
        onOpenChange={(open) => !open && setEditProvider(undefined)}
        title={editProvider ? `Edit ${editProvider.name}` : 'Edit provider'}
        footer={<><Button variant="ghost" onClick={() => setEditProvider(undefined)}>Cancel</Button><Button loading={editSubmitting} disabled={(editProvider?.credential_keys?.length ?? 0) > 1} onClick={() => void update()}>{editSubmitting ? 'Saving' : 'Save'}</Button></>}
      >
        {editError ? <Alert variant="danger" title="Provider update failed" className="mb-3">{editError}</Alert> : null}
        {editProvider ? (
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Name" htmlFor="edit-provider-name"><Input id="edit-provider-name" readOnly value={editProvider.name} /></Field>
            <Field label="Type" htmlFor="edit-provider-type"><Input id="edit-provider-type" readOnly value={editProvider.type || '-'} /></Field>
            <Field label="Credential env" htmlFor="edit-provider-credential" required>
              <Input id="edit-provider-credential" readOnly={(editProvider.credential_keys?.length ?? 0) > 1} value={editCredential} onChange={(event) => setEditCredential(event.target.value)} />
            </Field>
            <Field label="Gateway" htmlFor="edit-provider-gateway" required>
              <Select id="edit-provider-gateway" value={editGateway} onChange={(event) => setEditGateway(event.target.value)}>
                {gatewayOptions.map((name) => <option key={name} value={name}>{name}</option>)}
              </Select>
            </Field>
          </div>
        ) : null}
      </Modal>

      <ConfirmDialog
        open={Boolean(deleteProvider)}
        onOpenChange={(open) => !open && setDeleteProvider(undefined)}
        title={deleteProvider ? `Delete ${deleteProvider.name}` : 'Delete provider'}
        confirmLabel="Delete"
        busyLabel="Deleting"
        busy={deleteSubmitting}
        disabled={Boolean(deleteProvider?.used_by?.length)}
        onConfirm={() => void remove()}
      >
        {deleteError ? <Alert variant="danger" title="Provider deletion failed" className="mb-3">{deleteError}</Alert> : null}
        {deleteProvider?.used_by?.length ? (
          <Alert variant="warning" title="Provider is in use">Edit or delete these profiles first: {deleteProvider.used_by.join(', ')}</Alert>
        ) : <p className="text-sm text-muted">Delete this OpenShell provider?</p>}
      </ConfirmDialog>
    </div>
  );
}
