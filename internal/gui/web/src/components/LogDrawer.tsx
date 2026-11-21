import { useEffect, useMemo, useState } from 'react';
import { Dialog } from 'radix-ui';
import { Loader2, ShieldAlert, Square, X } from 'lucide-react';
import { api } from '../api';
import type { DenialApplyResult, DenialError, DenialPreview, PolicyDenial } from '../types';
import { Button } from './ui/button';
import { CopyButton } from './ui/copy-value';
import { Modal } from './ui/dialog';
import { Input } from './ui/input';
import { StatusBadge, canAttachRun } from './StatusBadge';
import { normalizeTerminalText } from '../lib/text';

export interface LogDrawerState {
  open: boolean;
  title: string;
  content: string;
  loading?: boolean;
  error?: string;
  taskId?: string;
  taskStatus?: string;
  runId?: string;
  runStatus?: string;
  denials?: PolicyDenial[];
  denialsError?: string;
}

function denialKey(denial: PolicyDenial): string {
  return `${denial.host}:${denial.port}:${denial.binary}`;
}

function entryYaml(denial: PolicyDenial): string {
  return `- host: ${denial.host}\n  port: ${denial.port}\n  binaries: [${denial.binary}]`;
}

export function LogDrawer({
  state,
  onClose,
  onStop,
  stopping = false
}: {
  state: LogDrawerState;
  onClose: () => void;
  onStop?: (taskId: string) => void;
  stopping?: boolean;
}) {
  // Render terminal-shaped text, not raw escape sequences.
  const content = useMemo(() => normalizeTerminalText(state.content), [state.content]);
  const empty = !content.trim();
  const [applied, setApplied] = useState<Set<string>>(new Set());
  const [selected, setSelected] = useState<PolicyDenial | null>(null);
  const [persist, setPersist] = useState(false);
  const [preview, setPreview] = useState<DenialPreview | null>(null);
  const [result, setResult] = useState<DenialApplyResult | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [dialogError, setDialogError] = useState('');
  const [rollback, setRollback] = useState<boolean | undefined>();
  const [failedDiff, setFailedDiff] = useState('');

  useEffect(() => {
    setApplied(new Set());
    setSelected(null);
    setPersist(false);
    setPreview(null);
    setResult(null);
    setDialogError('');
    setRollback(undefined);
    setFailedDiff('');
  }, [state.runId, state.title, state.open]);

  const denials = (state.denials ?? []).filter((denial) => !applied.has(denialKey(denial)));
  const runIsLive = Boolean(state.runId) && canAttachRun(state.runStatus ?? '');

  const loadPreview = async (denial: PolicyDenial, nextPersist: boolean) => {
    if (!state.runId) return;
    setPreviewing(true);
    setDialogError('');
    setFailedDiff('');
    setRollback(undefined);
    try {
      setPreview(await api.previewDenial(state.runId, {
        host: denial.host,
        port: denial.port,
        binary: denial.binary,
        persist: nextPersist
      }));
    } catch (err) {
      setPreview(null);
      setDialogError((err as Error).message);
    } finally {
      setPreviewing(false);
    }
  };

  const openDenial = (denial: PolicyDenial) => {
    setSelected(denial);
    setPersist(false);
    setResult(null);
    setPreview(null);
    setDialogError('');
    void loadPreview(denial, false);
  };

  const closeDialog = () => {
    if (submitting) return;
    setSelected(null);
    setPreview(null);
    setResult(null);
    setDialogError('');
    setRollback(undefined);
    setFailedDiff('');
  };

  const togglePersist = (checked: boolean) => {
    if (!selected) return;
    setPersist(checked);
    void loadPreview(selected, checked);
  };

  const applyDenial = async () => {
    if (!selected || !state.runId || !preview) return;
    setSubmitting(true);
    setDialogError('');
    setRollback(undefined);
    setFailedDiff('');
    try {
      const appliedResult = await api.applyDenial(state.runId, {
        host: selected.host,
        port: selected.port,
        binary: selected.binary,
        persist
      });
      setResult(appliedResult);
      setPreview(appliedResult);
      setApplied((current) => {
        const next = new Set(current);
        next.add(denialKey(selected));
        return next;
      });
    } catch (err) {
      const failure = err as DenialError;
      setDialogError(failure.message);
      setFailedDiff(failure.policyDiff ?? '');
      if (failure.outcome === 'openshell-rejected') setRollback(Boolean(failure.rolledBack));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog.Root open={state.open} onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-30 bg-black/35" />
        <Dialog.Content
          className="fixed bottom-0 left-0 right-0 z-40 flex h-[42vh] max-h-[70vh] flex-col border-t border-border bg-background shadow-2xl"
          aria-describedby={undefined}
        >
          <div className="flex min-h-10 shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border px-3 py-1.5">
            <div className="flex min-w-0 flex-1 items-center gap-2">
              <Dialog.Title className="min-w-0 truncate text-sm font-semibold" title={state.title}>{state.title}</Dialog.Title>
              {state.taskStatus ? <StatusBadge status={state.taskStatus} /> : null}
              {state.loading ? (
                <span className="inline-flex items-center gap-1 text-xs text-muted" role="status">
                  <Loader2 className="size-3.5 animate-spin" aria-hidden /> Loading output
                </span>
              ) : null}
            </div>
            <div className="flex items-center gap-1">
              {!empty ? <CopyButton value={content} label="output" /> : null}
              {state.taskId && state.taskStatus === 'running' ? (
                <Button size="xs" variant="danger" loading={stopping} onClick={() => onStop?.(state.taskId!)}>
                  <Square className="size-3" /> {stopping ? 'Stopping' : 'Stop'}
                </Button>
              ) : null}
              <Dialog.Close asChild>
                <Button size="icon" variant="ghost" aria-label="Close log drawer"><X className="size-4" /></Button>
              </Dialog.Close>
            </div>
          </div>
          <div className="agentbox-scrollbar min-h-0 flex-1 overflow-auto bg-surface-2 p-3">
            {denials.length > 0 || state.denialsError ? (
              <section aria-label="Network denials" className="mb-3 space-y-2">
                <div className="flex flex-wrap items-baseline justify-between gap-1">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted">Network denials</h3>
                  <span className="text-xs text-muted">Structured sandbox policy events, separate from raw output</span>
                </div>
                {state.denialsError ? (
                  <div role="status" className="rounded-lg border border-warning/40 bg-warning/10 p-2 text-sm text-warning">
                    <div className="font-semibold">Denials unavailable</div>
                    <div className="break-words text-xs">{state.denialsError}</div>
                    <div className="text-xs">Raw output below is still available.</div>
                  </div>
                ) : null}
                {denials.map((denial) => {
                  const canAdd = runIsLive && denial.valid;
                  return (
                    <div key={denialKey(denial)} className="rounded-lg border border-warning/40 bg-background p-2">
                      <div className="flex flex-wrap items-start justify-between gap-2">
                        <div className="min-w-0 space-y-1">
                          <div className="flex items-center gap-1.5 text-sm font-semibold">
                            <ShieldAlert className="size-4 text-warning" aria-hidden />
                            <span>Network request denied</span>
                          </div>
                          <div className="break-all font-mono text-sm">{denial.host}:{denial.port}</div>
                          <div className="break-all font-mono text-xs text-muted">{denial.binary}</div>
                          {denial.observedBinary ? <div className="break-all font-mono text-xs text-muted">Observed: {denial.observedBinary}</div> : null}
                          {denial.reason ? <div className="text-xs text-muted">Reason: {denial.reason}</div> : null}
                          {!denial.valid ? (
                            <div className="text-xs text-danger">Cannot add this rule: {denial.invalidReason}</div>
                          ) : null}
                        </div>
                        <Button
                          size="xs"
                          variant="outline"
                          disabled={!canAdd}
                          aria-label={`Add host ${denial.host}:${denial.port}`}
                          title={!denial.valid
                            ? denial.invalidReason
                            : !runIsLive
                              ? 'Add host is available only for Ready, Working, and Waiting agents.'
                              : undefined}
                          onClick={() => openDenial(denial)}
                        >
                          Add host
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </section>
            ) : null}
            {state.error ? (
              <div role="alert" className="space-y-1 text-sm text-danger">
                <div className="font-semibold">{state.taskStatus === 'failed' ? 'Task failed' : 'Unable to load output'}</div>
                <div className="break-words">{state.error}</div>
              </div>
            ) : null}
            {empty ? (
              state.error ? null : (
                <div className="text-sm text-muted" role={state.loading ? 'status' : undefined}>
                  {state.loading
                    ? 'Loading output…'
                    : denials.length > 0 || state.denialsError
                      ? 'No raw output recorded.'
                      : 'No output recorded.'}
                </div>
              )
            ) : (
              <>
                {denials.length > 0 || state.denialsError ? (
                  <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted">Raw output</h3>
                ) : null}
                <pre className={state.error ? 'agentbox-output mt-2 font-mono text-xs leading-5 text-foreground' : 'agentbox-output font-mono text-xs leading-5 text-foreground'}>{content}</pre>
              </>
            )}
          </div>
        </Dialog.Content>
      </Dialog.Portal>

      <Modal
        open={Boolean(selected)}
        onOpenChange={(open) => { if (!open) closeDialog(); }}
        title={selected ? `Add host ${selected.host}:${selected.port}` : 'Add host'}
        description={selected ? 'The rule is scoped to the exact host, port, and binary from the sandbox denial. It applies to this run only unless you persist it.' : undefined}
        footer={result ? (
          <Button onClick={closeDialog}>Close</Button>
        ) : (
          <>
            <Button variant="ghost" onClick={closeDialog} disabled={submitting}>Cancel</Button>
            <Button loading={submitting} disabled={!preview || previewing} onClick={() => void applyDenial()}>
              {submitting ? 'Adding' : persist ? 'Add host and persist' : 'Add host'}
            </Button>
          </>
        )}
      >
        {selected ? (
          <div className="space-y-3">
            {result ? (
              <div role="status" className="rounded-lg border border-success/40 bg-success/10 p-2 text-sm text-success">
                <div className="font-semibold">
                  {result.outcome === 'live-applied' ? 'Network rule applied' : 'Saved for the next sandbox'}
                </div>
                <div className="text-xs">
                  {result.outcome === 'live-applied'
                    ? 'Retry the blocked command in the agent or a shell tab.'
                    : 'Start a new run to use this policy.'}
                </div>
                {result.persisted ? (
                  <div className="mt-1 text-xs text-foreground">
                    Persisted to <span className="break-all font-mono">{result.manifestPath}</span>. Commit it in the run worktree to keep it.
                    {result.fragmentStatus ? <span> Policy fragment: {result.fragmentStatus}.</span> : null}
                  </div>
                ) : null}
              </div>
            ) : null}
            <div className="grid gap-2 rounded-lg border border-border p-2 text-sm sm:grid-cols-3">
              <div><div className="text-xs text-muted">Host</div><div className="break-all font-mono">{selected.host}</div></div>
              <div><div className="text-xs text-muted">Port</div><div className="font-mono">{selected.port}</div></div>
              <div className="sm:col-span-1">
                <div className="text-xs text-muted">Binary</div>
                <div className="break-all font-mono text-xs">{selected.binary}</div>
                {selected.observedBinary ? <div className="break-all font-mono text-xs text-muted">Observed: {selected.observedBinary}</div> : null}
              </div>
            </div>
            <div>
              <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted">Manifest entry</div>
              <pre className="overflow-auto whitespace-pre-wrap rounded-lg border border-border bg-surface/50 p-2 font-mono text-xs leading-4">{entryYaml(selected)}</pre>
            </div>
            {previewing ? <div className="text-sm text-muted" role="status">Computing the scoped diff…</div> : null}
            {preview ? (
              <div className="space-y-2">
                <div>
                  <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted">Run policy diff</div>
                  <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded-lg border border-border bg-surface/50 p-2 font-mono text-xs leading-4">{preview.policyDiff}</pre>
                </div>
                <label className="flex items-start gap-2 rounded-lg border border-border p-2 text-sm">
                  <input
                    type="checkbox"
                    className="mt-0.5 size-4"
                    checked={persist}
                    disabled={!preview.persistAvailable || Boolean(result)}
                    onChange={(event) => togglePersist(event.target.checked)}
                  />
                  <span>
                    <span className="font-medium">Persist in .agentbox/config.yaml</span>
                    <span className="block text-xs text-muted">Adds the same exact entry to future runs without rebuilding the image.</span>
                    {!preview.persistAvailable ? <span className="block text-xs text-warning">{preview.persistReason}</span> : null}
                  </span>
                </label>
                {persist && preview.persistAvailable ? (
                  <div className="space-y-2">
                    <div>
                      <div className="mb-1 break-all text-xs font-semibold uppercase tracking-wide text-muted">Manifest diff · {preview.manifestPath}</div>
                      <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded-lg border border-border bg-surface/50 p-2 font-mono text-xs leading-4">{preview.manifestDiff}</pre>
                    </div>
                    <div>
                      <div className="mb-1 break-all text-xs font-semibold uppercase tracking-wide text-muted">Policy fragment diff · {preview.fragmentPath}</div>
                      <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded-lg border border-border bg-surface/50 p-2 font-mono text-xs leading-4">{preview.fragmentDiff}</pre>
                    </div>
                  </div>
                ) : null}
              </div>
            ) : null}
            {dialogError ? (
              <div role="alert" className="rounded-lg border border-danger/40 bg-danger/10 p-2 text-sm text-danger">
                <div className="font-semibold">Unable to add host</div>
                <div className="break-words">{dialogError}</div>
                {rollback !== undefined ? <div className="text-xs">Rolled back: {rollback ? 'yes' : 'no'}</div> : null}
                {failedDiff ? <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap font-mono text-xs">{failedDiff}</pre> : null}
              </div>
            ) : null}
          </div>
        ) : (
          <Input readOnly value="" aria-hidden />
        )}
      </Modal>
    </Dialog.Root>
  );
}
