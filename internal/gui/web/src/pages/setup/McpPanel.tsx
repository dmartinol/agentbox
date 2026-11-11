import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { LocalMCPFiles, MCPTools } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { EmptyState } from '../../components/ui/empty';
import { Field } from '../../components/ui/field';
import { Input, Select } from '../../components/ui/input';
import { Modal } from '../../components/ui/dialog';
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

  const toggleServer = (server: string) => {
    setSelectedServers((current) => current.includes(server) ? current.filter((item) => item !== server) : [...current, server]);
  };

  const create = async () => {
    setBusy('create');
    try {
      await api.createMCP({ version: 1, name: createForm.name.trim(), agent: createForm.agent, servers: selectedServers });
      setCreateOpen(false);
      setNotice('MCP tools entry created');
      await load();
    } catch (err) {
      setError(err as Error);
    } finally {
      setBusy(undefined);
    }
  };

  const remove = async (name: string) => {
    setBusy(name);
    try {
      await api.deleteMCP(name);
      setNotice(`MCP tools entry ${name} deleted`);
      await load();
    } catch (err) {
      setError(err as Error);
    } finally {
      setBusy(undefined);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex justify-end"><Button size="sm" onClick={() => void openCreate()}>Create MCP tools</Button></div>
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
                <TD className="truncate font-medium">{entry.name}</TD>
                <TD className="truncate">{entry.agent}</TD>
                <TD className="truncate text-muted">{entry.servers.join(', ')}</TD>
                <TD>
                  <Button size="xs" variant="ghost" loading={busy === entry.name} onClick={() => void remove(entry.name)}>Delete</Button>
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

      <Modal open={createOpen} onOpenChange={setCreateOpen} title="Create MCP tools" footer={<><Button variant="ghost" onClick={() => setCreateOpen(false)}>Cancel</Button><Button loading={busy === 'create'} disabled={!createForm.name.trim() || selectedServers.length === 0} onClick={() => void create()}>Create</Button></>}>
        <div className="grid gap-3">
          <Field label="Name" htmlFor="mcp-name" required><Input id="mcp-name" value={createForm.name} onChange={(event) => setCreateForm({ ...createForm, name: event.target.value })} /></Field>
          <Field label="Agent" htmlFor="mcp-agent" required helper="Global MCP servers are configured per agent on this host.">
            <Select id="mcp-agent" value={createForm.agent} onChange={(event) => void changeAgent(event.target.value)}>
              {agents.map((agent) => <option key={agent} value={agent}>{agent}</option>)}
            </Select>
          </Field>
          <Field label="Servers" htmlFor="mcp-servers" required helper={available.length === 0 ? `No global ${createForm.agent} MCP servers found on this host; configure them with the agent first.` : 'Selected servers are copied into the run at start.'}>
            <div id="mcp-servers" className="max-h-44 space-y-1 overflow-y-auto rounded-lg border border-border p-2">
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
    </div>
  );
}
