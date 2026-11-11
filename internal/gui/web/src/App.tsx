import { useCallback, useEffect, useState } from 'react';
import { Activity, Boxes, Settings } from 'lucide-react';
import { ThemeToggle } from './components/ThemeToggle';
import { api } from './api';
import type { Check } from './types';
import { Badge } from './components/ui/badge';
import { Button } from './components/ui/button';
import { FleetView } from './pages/FleetView';
import { SetupSheet } from './pages/SetupSheet';
import { LogDrawer, type LogDrawerState } from './components/LogDrawer';

const initialLog: LogDrawerState = { open: false, title: '', content: '' };

export function App() {
  const [setupOpen, setSetupOpen] = useState(false);
  const [log, setLog] = useState<LogDrawerState>(initialLog);
  const [doctor, setDoctor] = useState<Check[]>([]);
  const [stoppingTask, setStoppingTask] = useState(false);

  useEffect(() => {
    void api.doctor().then(setDoctor).catch(() => setDoctor([]));
  }, []);

  const openLog = useCallback((state: Omit<LogDrawerState, 'open'>) => {
    setLog({ ...state, open: true });
  }, []);

  const updateLog = useCallback((state: Partial<LogDrawerState>) => {
    setLog((current) => ({ ...current, ...state }));
  }, []);

  const failed = doctor.filter((check) => !check.ok).length;

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <header className="flex h-11 shrink-0 items-center gap-3 border-b border-border bg-surface px-3">
        <div className="flex items-center gap-2">
          <Boxes className="size-4 text-accent" />
          <span className="text-sm font-semibold tracking-tight">AgentBox</span>
        </div>
        <div className="flex min-w-0 items-center gap-1.5 text-sm text-muted">
          <Activity className="size-3.5" />
          OpenShell fleet
        </div>
        <div className="ml-auto flex items-center gap-2">
          <ThemeToggle />
          <Badge tone={failed === 0 ? 'success' : failed > 1 ? 'danger' : 'warning'}>
            Doctor {failed === 0 ? 'OK' : `${failed} issue${failed === 1 ? '' : 's'}`}
          </Badge>
          <Button size="sm" variant="outline" onClick={() => setSetupOpen(true)}>
            <Settings className="size-3.5" /> Setup
          </Button>
        </div>
      </header>

      <main className="min-h-0 flex-1">
        <FleetView openLog={openLog} updateLog={updateLog} />
      </main>

      <SetupSheet open={setupOpen} onOpenChange={setSetupOpen} openLog={openLog} updateLog={updateLog} />
      <LogDrawer
        state={log}
        stopping={stoppingTask}
        onClose={() => setLog(initialLog)}
        onStop={async (taskId) => {
          setStoppingTask(true);
          try {
            const stopped = await api.stopTask(taskId);
            setLog((current) => ({ ...current, content: stopped.output, taskStatus: stopped.status }));
          } catch {
            // Leave the drawer as-is; the task stays stoppable.
          } finally {
            setStoppingTask(false);
          }
        }}
      />
    </div>
  );
}

export default App;
