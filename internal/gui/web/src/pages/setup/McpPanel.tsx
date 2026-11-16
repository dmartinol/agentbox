import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { LocalMCPFiles, MCPTools } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { EmptyState } from '../../components/ui/empty';
import { Field } from '../../components/ui/field';
import { Input, Select } from '../../components/ui/input';
import { Modal } from '../../components/ui/dialog';
import { ConfirmDialog } from '../../components/ui/confirm';
import { ExpandableValue } from '../../components/ui/copy-value';
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/table';

const agents = ['claude', 'codex', 'opencode'];

export function McpPanel() {
  const [entries, setEntries] = useState<MCPTools[]>([]);
  const [localFiles, setLocalFiles] = useState<LocalMCPFiles[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState({ name: '', agent: 'claude' });
  const [available, setAvailable] = useState<string[]>([]);
  const [selectedServers, setSelectedServers] = useState<string[]>([]);
  const [busy, setBusy] = useState<string>();
  const [editing, setEditing] = useState(false);
  const [formError, setFormError] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<MCPTools | null>(null);
  const [deleteError, setDeleteError] = useState('');
  const [deleteBusy, setDeleteBusy] = useState(false);

  const load = async () => {
    try {
      const [nextEntries, nextLocal] = await Promise.all([api.mcps(), api.localMCPs()]);
      setEntries(nextEntries ?? []);
      setLocalFiles(nextLocal ?? []);
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  const loadServers = async (agent: string) => {
    try {
      const names = await api.mcpServers(agent);
      setAvailable(names ?? []);
      setError(null);
    } catch (err) {
      setAvailable([]);
      setError(err as Error);
    }
  };

  const openCreate = async () => {
    setEditing(false);
    setCreateForm({ name: '', agent: 'claude' });
    setSelectedServers([]);
    setAvailable([]);
    setCreateOpen(true);
    await loadServers('claude');
  };

  const changeAgent = async (agent: string) => {
    setCreateForm({ ...createForm, agent });
    setSelectedServers([]);
    await loadServers(agent);
  };

  const openEdit = async (entry: MCPTools) => {
    setEditing(true);
    setCreateForm({ name: entry.name, agent: entry.agent });
    setSelectedServers(entry.servers ?? []);
    setAvailable([]);
    setFormError('');
    setCreateOpen(true);
    await loadServers(entry.agent);
  };

  const toggleServer = (server: string) => {
    setSelectedServers((current) => current.includes(server) ? current.filter((item) => item !== server) : [...current, server]);
  };

  const create = async () => {
    setBusy('create');
    try {
      await api.createMCP({ version: 1, name: createForm.name.trim(), agent: createForm.agent, servers: selectedServers });
      setCreateOpen(false);
      setNotice(`MCP tools entry ${editing ? 'saved' : 'created'}`);
      await load();
    } catch (err) {
      setFormError((err as Error).message);
    } finally {
      setBusy(undefined);
    }
  };

  const remove = async () => {
    if (!deleteTarget) return;
    setDeleteBusy(true);
    setDeleteError('');
    try {
      await api.deleteMCP(deleteTarget.name);
      setDeleteTarget(null);
      setNotice(`MCP tools entry ${deleteTarget.name} deleted`);
      await load();
    } catch (err) {
      setDeleteError((err as Error).message);
    } finally {
      setDeleteBusy(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap justify-end"><Button size="sm" onClick={() => void openCreate()}>Create MCP tools</Button></div>
      {notice ? <Alert variant="success" title={notice} /> : null}
      {error ? <Alert variant="danger" title="Unable to load MCP tools">{error.message}</Alert> : null}
      {loading ? <EmptyState title="Loading MCP tools" /> : entries.length === 0 ? (
        <EmptyState title="No MCP tools entries" description="Create an entry that selects global MCP servers by name for one agent, then pick it when starting a run." />
      ) : (
        <Table>
          <THead><TR><TH className="w-[24%]">Name</TH><TH className="w-[16%]">Agent</TH><TH>Servers</TH><TH className="w-[16%]">Actions</TH></TR></THead>
          <TBody>
            {entries.map((entry) => (
              <TR key={entry.name}>
                <TD className="truncate font-medium"><span title={entry.name}>{entry.name}</span></TD>
                <TD label="Agent" className="truncate">{entry.agent}</TD>
                <TD label="Servers">
                  <ExpandableValue value={entry.servers.join(', ')} label={`servers in ${entry.name}`} max={44} mono={false} />
                </TD>
                <TD label="Actions">
                  <div className="flex flex-wrap gap-1">
                    <Button size="xs" variant="ghost" onClick={() => void openEdit(entry)}>Edit</Button>
                    <Button size="xs" variant="ghost" loading={busy === entry.name} onClick={() => { setDeleteTarget(entry); setDeleteError(''); }}>Delete</Button>
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}
      <div className="rounded-lg border border-border bg-surface/40 p-2.5 text-sm text-muted">
        <div className="mb-1 font-medium text-foreground">How MCP tools work</div>
        <div>Entries select servers by name from the agent's own global MCP configuration on this host and deliver them into the run at user scope. MCP files committed in a project always ride along and win name conflicts. Credentials must be environment-variable references; secret values are rejected at start.</div>
        {localFiles.length > 0 ? (
          <div className="mt-2 space-y-1">
            <div className="font-medium text-foreground">Local MCP files detected in projects</div>
            {localFiles.map((item) => (
              <div key={item.project}>{item.project}: <span className="font-mono text-xs">{item.files.join(', ')}</span></div>
            ))}
          </div>
        ) : null}
      </div>

      <Modal open={createOpen} onOpenChange={setCreateOpen} title={editing ? `Edit ${createForm.name}` : 'Create MCP tools'} footer={<><Button variant="ghost" onClick={() => setCreateOpen(false)}>Cancel</Button><Button loading={busy === 'create'} disabled={!createForm.name.trim() || selectedServers.length === 0} onClick={() => void create()}>{editing ? 'Save' : 'Create'}</Button></>}>
        <div className="grid gap-3">
          {formError ? <div className="sm:col-span-2"><Alert variant="danger" title="Unable to save MCP tools">{formError}</Alert></div> : null}
          <Field label="Name" htmlFor="mcp-name" required><Input id="mcp-name" readOnly={editing} value={createForm.name} onChange={(event) => setCreateForm({ ...createForm, name: event.target.value })} /></Field>
          <Field label="Agent" htmlFor="mcp-agent" required helper="Global MCP servers are configured per agent on this host.">
            <Select id="mcp-agent" value={createForm.agent} onChange={(event) => void changeAgent(event.target.value)}>
              {agents.map((agent) => <option key={agent} value={agent}>{agent}</option>)}
            </Select>
          </Field>
          <Field label="Servers" htmlFor="mcp-servers" required helper={available.length === 0 ? `No global ${createForm.agent} MCP servers found on this host; configure them with the agent first.` : 'Selected servers are copied into the run at start.'}>
            <div id="mcp-servers" className="max-h-44 space-y-1 overflow-y-auto rounded-lg border border-border p-2">
              {selectedServers.filter((server) => !available.includes(server)).length ? (
                <div className="text-xs text-warning">Unavailable saved servers are shown below and can be removed from the entry.</div>
              ) : null}
              {selectedServers.filter((server) => !available.includes(server)).map((server) => (
                <label key={`saved-${server}`} className="flex items-center gap-2 text-sm">
                  <input type="checkbox" checked onChange={() => toggleServer(server)} className="size-3.5 accent-accent" />
                  {server} <span className="text-xs text-muted">(unavailable)</span>
                </label>
              ))}
              {available.map((server) => (
                <label key={server} className="flex items-center gap-2 text-sm">
                  <input type="checkbox" checked={selectedServers.includes(server)} onChange={() => toggleServer(server)} className="size-3.5 accent-accent" />
                  {server}
                </label>
              ))}
            </div>
          </Field>
        </div>
      </Modal>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
            setDeleteError('');
          }
        }}
        title={deleteTarget ? `Delete ${deleteTarget.name}` : 'Delete MCP tools'}
        confirmLabel="Delete"
        busyLabel="Deleting"
        busy={deleteBusy}
        onConfirm={() => void remove()}
      >
        {deleteError ? <Alert variant="danger" title="MCP deletion failed" className="mb-3">{deleteError}</Alert> : null}
        <p className="text-sm text-muted">Remove only this AgentBox selection. Global and project MCP configuration is kept.</p>
      </ConfirmDialog>
    </div>
  );
}
