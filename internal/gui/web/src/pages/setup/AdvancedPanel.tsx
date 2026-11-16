import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { Check, Settings } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Field } from '../../components/ui/field';
import { Input } from '../../components/ui/input';
import { Disclosure } from '../../components/ui/disclosure';
import { CopyButton } from '../../components/ui/copy-value';
import { middleEllipsis } from '../../lib/text';

const remediations: Record<string, string> = {
  platform: 'This spike currently targets darwin/arm64.',
  git: 'Install Git.',
  podman: 'Install Podman.',
  openshell: 'Repair the OpenShell CLI installation.',
  npm: 'Install Node.js and npm.',
  'podman connection': 'Start Podman with `podman machine start <machine>`.',
  gateway: 'Restart OpenShell with `brew services restart openshell`.'
};

function readableDetail(detail: string) {
  const trimmed = detail.trim();
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) return detail;
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2);
  } catch {
    return detail;
  }
}

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
      <section className="space-y-3 rounded-lg border border-border p-3">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-muted">Worktrees</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Root" htmlFor="settings-root" required><Input id="settings-root" value={settings?.worktrees.root ?? ''} onChange={(event) => settings && setSettings({ ...settings, worktrees: { ...settings.worktrees, root: event.target.value } })} /></Field>
          <Field label="Branch prefix" htmlFor="settings-prefix" required><Input id="settings-prefix" value={settings?.worktrees.branch_prefix ?? ''} onChange={(event) => settings && setSettings({ ...settings, worktrees: { ...settings.worktrees, branch_prefix: event.target.value } })} /></Field>
        </div>
        <Button size="sm" onClick={() => void save()}>Save settings</Button>
      </section>

      <section className="space-y-3 rounded-lg border border-border p-3">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-semibold uppercase tracking-wide text-muted">Doctor</h3>
          <Button size="xs" variant="ghost" onClick={() => void load()}>Refresh</Button>
        </div>
        {failed.length > 0 ? (
          <Alert variant="danger" title={`${failed.length} of ${checks.length} checks failed`}>
            <div className="space-y-1">
              <div>{remediations[failed[0].name] ?? 'Run `./agentbox doctor` for the full diagnostic report.'}</div>
              <div className="text-xs opacity-90">Expand a failing check for its complete output and action.</div>
            </div>
          </Alert>
        ) : checks.length > 0 ? (
          <Alert variant="success" title={`${checks.length} checks passed`} />
        ) : null}
        {loading ? (
          <p className="text-sm text-muted" role="status">Loading diagnostics…</p>
        ) : error ? null : checks.length === 0 ? (
          <p className="text-sm text-muted">No diagnostics were reported.</p>
        ) : (
          <div className="space-y-2">
            {[...checks].sort((left, right) => Number(left.ok) - Number(right.ok)).map((check) => {
              const detail = readableDetail(check.detail || '');
              const action = check.ok ? '' : remediations[check.name] ?? 'Run `./agentbox doctor` for details.';
              return (
                <Disclosure
                  key={check.name}
                  className={check.ok ? undefined : 'border-danger/40'}
                  summary={(
                    <span className="flex min-w-0 flex-wrap items-center gap-2">
                      <Badge tone={check.ok ? 'success' : 'danger'}>{check.ok ? 'OK' : 'Fail'}</Badge>
                      <span>{check.name}</span>
                      <span className="min-w-0 text-xs font-normal text-muted" title={check.detail || undefined}>
                        {middleEllipsis(check.detail || 'No output recorded.', 64)}
                      </span>
                    </span>
                  )}
                >
                  <div className="space-y-2">
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-xs font-semibold uppercase tracking-wide text-muted">Full check output</span>
                      {check.detail ? <CopyButton value={check.detail} label={`${check.name} output`} /> : null}
                    </div>
                    <pre className="agentbox-output max-h-52 overflow-auto rounded-md border border-border bg-surface-2 p-2 font-mono text-xs leading-5">
                      {detail || 'No output recorded.'}
                    </pre>
                    {action ? <div className="text-sm text-muted">Action: {action}</div> : null}
                  </div>
                </Disclosure>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
}
