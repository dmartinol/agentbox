import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { Check, Settings } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Field } from '../../components/ui/field';
import { Input } from '../../components/ui/input';
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/table';

const remediations: Record<string, string> = {
  platform: 'This spike currently targets darwin/arm64.',
  git: 'Install Git.',
  podman: 'Install Podman.',
  openshell: 'Repair the OpenShell CLI installation.',
  npm: 'Install Node.js and npm.',
  'podman connection': 'Start Podman with `podman machine start <machine>`.',
  gateway: 'Restart OpenShell with `brew services restart openshell`.'
};

export function AdvancedPanel() {
  const [settings, setSettings] = useState<Settings>();
  const [checks, setChecks] = useState<Check[]>([]);
  const [error, setError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [loading, setLoading] = useState(true);

  const load = async () => {
    setLoading(true);
    try {
      const [nextSettings, nextChecks] = await Promise.all([api.settings(), api.doctor()]);
      setSettings(nextSettings);
      setChecks(nextChecks);
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  const save = async () => {
    if (!settings) return;
    try {
      setSettings(await api.saveSettings(settings));
      setNotice('Settings saved');
      setError(null);
    } catch (err) {
      setError(err as Error);
    }
  };

  const failed = checks.filter((check) => !check.ok);

  return (
    <div className="space-y-4">
      {notice ? <Alert variant="success" title={notice} /> : null}
      {error ? <Alert variant="danger" title="Unable to load advanced settings">{error.message}</Alert> : null}
      <section className="space-y-3 rounded-lg border border-zinc-800 p-3">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-zinc-500">Worktrees</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Root" htmlFor="settings-root" required><Input id="settings-root" value={settings?.worktrees.root ?? ''} onChange={(event) => settings && setSettings({ ...settings, worktrees: { ...settings.worktrees, root: event.target.value } })} /></Field>
          <Field label="Branch prefix" htmlFor="settings-prefix" required><Input id="settings-prefix" value={settings?.worktrees.branch_prefix ?? ''} onChange={(event) => settings && setSettings({ ...settings, worktrees: { ...settings.worktrees, branch_prefix: event.target.value } })} /></Field>
        </div>
        <Button size="sm" onClick={() => void save()}>Save settings</Button>
      </section>

      <section className="space-y-3 rounded-lg border border-zinc-800 p-3">
        <div className="flex items-center justify-between">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-zinc-500">Doctor</h3>
          <Button size="xs" variant="ghost" onClick={() => void load()}>Refresh</Button>
        </div>
        {failed.length > 0 ? <Alert variant="danger" title={`${failed.length} checks require attention`}>{remediations[failed[0].name] ?? 'Run `./agentbox doctor` for details.'}</Alert> : null}
        {loading ? <p className="text-xs text-zinc-500">Loading diagnostics…</p> : (
          <Table>
            <THead><TR><TH className="w-[8%]">Status</TH><TH className="w-[18%]">Check</TH><TH>Detail</TH><TH className="w-[28%]">Action</TH></TR></THead>
            <TBody>
              {[...checks].sort((left, right) => Number(left.ok) - Number(right.ok)).map((check) => (
                <TR key={check.name} className={check.ok ? undefined : 'bg-red-500/5'}>
                  <TD><Badge tone={check.ok ? 'success' : 'danger'}>{check.ok ? 'OK' : 'Fail'}</Badge></TD>
                  <TD>{check.name}</TD>
                  <TD className="truncate text-zinc-400">{check.detail || '—'}</TD>
                  <TD className="text-zinc-400">{check.ok ? '—' : remediations[check.name] ?? 'Run CLI doctor'}</TD>
                </TR>
              ))}
            </TBody>
          </Table>
        )}
      </section>
    </div>
  );
}
