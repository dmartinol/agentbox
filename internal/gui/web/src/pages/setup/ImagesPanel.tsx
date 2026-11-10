import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { GuiTask, ImageArtifact, Registry } from '../../types';
import { Alert } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { EmptyState } from '../../components/ui/empty';
import { Field } from '../../components/ui/field';
import { Input } from '../../components/ui/input';
import { Modal } from '../../components/ui/dialog';
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/table';
import { StatusBadge } from '../../components/StatusBadge';
import type { LogDrawerState } from '../../components/LogDrawer';

export function ImagesPanel({
  openLog,
  updateLog
}: {
  openLog: (state: Omit<LogDrawerState, 'open'>) => void;
  updateLog: (state: Partial<LogDrawerState>) => void;
}) {
  const [images, setImages] = useState<ImageArtifact[]>([]);
  const [registryImages, setRegistryImages] = useState<ImageArtifact[]>([]);
  const [registry, setRegistry] = useState<Registry>({ reference: '' });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [notice, setNotice] = useState('');
  const [buildOpen, setBuildOpen] = useState(false);
  const [buildForm, setBuildForm] = useState({ agent: 'codex', version: '' });
  const [task, setTask] = useState<GuiTask>();
  const [pullOpen, setPullOpen] = useState(false);
  const [pullRef, setPullRef] = useState('');

  const load = async () => {
    try {
      const [nextImages, nextRegistry] = await Promise.all([api.images(), api.registry()]);
      setImages(nextImages ?? []);
      setRegistry(nextRegistry);
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  useEffect(() => {
    if (!task || task.status !== 'running') return;
    let active = true;
    const timer = window.setInterval(async () => {
      try {
        const next = await api.task(task.id);
        if (!active) return;
        setTask(next);
        updateLog({ content: next.output, taskStatus: next.status, loading: false });
        if (next.status === 'succeeded') await load();
      } catch {
        // Keep last output on transient polling failure.
      }
    }, 1000);
    return () => { active = false; window.clearInterval(timer); };
  }, [task, updateLog]);

  const action = async (callback: () => Promise<unknown>, message: string) => {
    try {
      await callback();
      setNotice(message);
      await load();
    } catch (err) {
      setError(err as Error);
    }
  };

  const build = async () => {
    const next = await api.buildImage(buildForm.agent, buildForm.version);
    setTask(next);
    setBuildOpen(false);
    openLog({ title: `Build image · ${buildForm.agent}`, content: next.output, loading: true, taskId: next.id, taskStatus: next.status });
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap justify-end gap-2">
        <Button size="sm" onClick={() => setBuildOpen(true)}>Build image</Button>
        <Button size="sm" variant="outline" onClick={() => setPullOpen(true)}>Pull image</Button>
      </div>
      {notice ? <Alert variant="success" title={notice} /> : null}
      {error ? <Alert variant="danger" title="Unable to load images">{error.message}</Alert> : null}
      {loading ? <EmptyState title="Loading images" /> : images.length === 0 ? (
        <EmptyState title="No local images" description="Build or pull an AgentBox agent image." />
      ) : (
        <Table>
          <THead><TR><TH className="w-[38%]">Reference</TH><TH className="w-[12%]">Agent</TH><TH className="w-[12%]">Version</TH><TH className="w-[10%]">Recipe</TH><TH>Platform</TH></TR></THead>
          <TBody>
            {images.map((image) => (
              <TR key={image.reference}>
                <TD className="truncate font-mono text-[11px]">{image.reference}</TD>
                <TD>{image.agent}</TD>
                <TD>{image.version}</TD>
                <TD>r{image.recipe}</TD>
                <TD>{image.os || 'linux'}/{image.arch || 'arm64'}</TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}

      <div className="rounded-lg border border-zinc-800 p-3">
        <div className="flex items-end gap-2">
          <Field label="Registry repository" htmlFor="registry-reference" className="flex-1">
            <Input id="registry-reference" value={registry.reference} onChange={(event) => setRegistry({ reference: event.target.value })} />
          </Field>
          <Button size="sm" onClick={() => void action(async () => {
            await api.saveRegistry(registry.reference);
            setRegistryImages((await api.discoverRegistry()) ?? []);
          }, 'Registry discovery complete')}>Discover</Button>
        </div>
        {registryImages.length > 0 ? (
          <Table className="mt-3">
            <THead><TR><TH className="w-[12%]">Agent</TH><TH className="w-[12%]">Version</TH><TH>Reference</TH></TR></THead>
            <TBody>
              {registryImages.map((image) => (
                <TR key={image.reference}><TD>{image.agent}</TD><TD>{image.version}</TD><TD className="truncate font-mono text-[11px]">{image.reference}</TD></TR>
              ))}
            </TBody>
          </Table>
        ) : null}
      </div>

      <Modal
        open={buildOpen}
        onOpenChange={setBuildOpen}
        title="Build image"
        description="Build a local AgentBox agent image."
        footer={<><Button variant="ghost" onClick={() => setBuildOpen(false)}>Cancel</Button><Button onClick={() => void action(build, 'Image build started')}>Build</Button></>}
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Agent" htmlFor="image-agent" required>
            <select id="image-agent" className="h-8 rounded-md border border-zinc-700 bg-zinc-900 px-2" value={buildForm.agent} onChange={(event) => setBuildForm({ ...buildForm, agent: event.target.value })}>
              {['codex', 'claude', 'opencode'].map((agent) => <option key={agent} value={agent}>{agent}</option>)}
            </select>
          </Field>
          <Field label="Exact version" htmlFor="image-version" helper="Blank resolves latest."><Input id="image-version" value={buildForm.version} onChange={(event) => setBuildForm({ ...buildForm, version: event.target.value })} /></Field>
          <Button variant="outline" onClick={() => void action(async () => setBuildForm({ ...buildForm, version: (await api.imageVersion(buildForm.agent)).version }), 'Version discovered')}>Discover latest</Button>
        </div>
        {task ? <div className="mt-3 flex items-center gap-2 text-xs text-zinc-400"><StatusBadge status={task.status} /> Build output opens in the log drawer.</div> : null}
      </Modal>

      <Modal
        open={pullOpen}
        onOpenChange={setPullOpen}
        title="Pull image"
        footer={<><Button variant="ghost" onClick={() => setPullOpen(false)}>Cancel</Button><Button onClick={() => void action(async () => { await api.pullImage(pullRef); setPullOpen(false); }, 'Image pull requested')}>Pull</Button></>}
      >
        <Field label="Remote reference" htmlFor="pull-reference" required><Input id="pull-reference" value={pullRef} onChange={(event) => setPullRef(event.target.value)} /></Field>
      </Modal>
    </div>
  );
}
