import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { Gateway } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { EmptyState } from '../../components/ui/empty';
import { Field } from '../../components/ui/field';
import { Input } from '../../components/ui/input';
import { Modal } from '../../components/ui/dialog';
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/table';

export function GatewaysPanel() {
  const [gateways, setGateways] = useState<Gateway[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [connectOpen, setConnectOpen] = useState(false);
  const [form, setForm] = useState({ url: 'https://localhost:17670', name: 'openshell' });
  const [info, setInfo] = useState<unknown>();
  const [busy, setBusy] = useState<string>();

  const load = async () => {
    try {
      setGateways((await api.gateways()) ?? []);
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  const action = async (key: string, callback: () => Promise<unknown>, message: string) => {
    setBusy(key);
    try {
      await callback();
      setNotice(message);
      await load();
    } catch (err) {
      setError(err as Error);
    } finally {
      setBusy(undefined);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex justify-end gap-2">
        <Button size="sm" variant="outline" loading={busy === 'start'} onClick={() => void action('start', () => api.startGateway(), 'Gateway start requested')}>
          {busy === 'start' ? 'Starting' : 'Start service'}
        </Button>
        <Button size="sm" onClick={() => setConnectOpen(true)}>Connect gateway</Button>
      </div>
      {notice ? <Alert variant="success" title={notice} /> : null}
      {error ? <Alert variant="danger" title="Unable to load gateways">{error.message}</Alert> : null}
      {loading ? <EmptyState title="Loading gateways" /> : gateways.length === 0 ? (
        <EmptyState title="No gateways" description="Connect or start an OpenShell gateway." />
      ) : (
        <Table>
          <THead><TR><TH className="w-[16%]">Name</TH><TH className="w-[12%]">Status</TH><TH className="w-[28%]">Endpoint</TH><TH className="w-[12%]">Type</TH><TH>Inspect</TH></TR></THead>
          <TBody>
            {gateways.map((gateway) => (
              <TR key={gateway.name}>
                <TD className="truncate font-medium">{gateway.name}</TD>
                <TD><Badge tone={gateway.active ? 'success' : 'neutral'}>{gateway.active ? 'Active' : 'Inactive'}</Badge></TD>
                <TD className="truncate font-mono text-xs text-muted">{gateway.endpoint}</TD>
                <TD>{gateway.type}</TD>
                <TD><Button size="xs" variant="ghost" loading={busy === `inspect:${gateway.name}`} onClick={() => void action(`inspect:${gateway.name}`, async () => setInfo(await api.gatewayInfo(gateway.name)), 'Gateway inspected')}>Inspect</Button></TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}
      {info !== undefined ? <pre className="agentbox-scrollbar max-h-64 overflow-auto rounded-lg border border-border bg-surface-2 p-3 font-mono text-xs text-foreground">{JSON.stringify(info, null, 2)}</pre> : null}

      <Modal
        open={connectOpen}
        onOpenChange={setConnectOpen}
        title="Connect gateway"
        footer={<><Button variant="ghost" onClick={() => setConnectOpen(false)}>Cancel</Button><Button loading={busy === 'connect'} onClick={() => void action('connect', async () => { await api.connectGateway(form); setConnectOpen(false); }, 'Gateway connected')}>{busy === 'connect' ? 'Connecting' : 'Connect'}</Button></>}
      >
        <div className="grid gap-3">
          <Field label="Endpoint URL" htmlFor="gateway-url" required><Input id="gateway-url" value={form.url} onChange={(event) => setForm({ ...form, url: event.target.value })} /></Field>
          <Field label="Name" htmlFor="gateway-name" required><Input id="gateway-name" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} /></Field>
        </div>
      </Modal>
    </div>
  );
}
