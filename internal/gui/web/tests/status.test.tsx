import { render, screen } from '@testing-library/react';
import { StatusBadge, canAttachRun, canStopRun, isActiveRun, statusLabel } from '../src/components/StatusBadge';

describe('agent status presentation', () => {
  it('maps lifecycle states to compact semantic badges', () => {
    render(
      <>
        <StatusBadge status="running" />
        <StatusBadge status="idle" />
        <StatusBadge status="policy-blocked" />
        <StatusBadge status="finished" />
      </>
    );
    expect(screen.getByText('Running')).toBeInTheDocument();
    expect(screen.getByText('Idle')).toBeInTheDocument();
    expect(screen.getByText('Policy blocked')).toBeInTheDocument();
    expect(screen.getByText('Finished')).toBeInTheDocument();
  });

  it('identifies active states without color-only communication', () => {
    expect(isActiveRun('RUNNING')).toBe(true);
    expect(isActiveRun('created')).toBe(true);
    expect(isActiveRun('stopped')).toBe(false);
    expect(canAttachRun('idle')).toBe(true);
    expect(canStopRun('idle')).toBe(true);
    expect(statusLabel('unreachable')).toBe('Unreachable');
  });
});
