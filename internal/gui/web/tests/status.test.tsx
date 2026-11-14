import { render, screen } from '@testing-library/react';
import {
  StatusBadge,
  canAttachRun,
  canStopRun,
  isActiveRun,
  isWaitingRun,
  statusLabel
} from '../src/components/StatusBadge';
import type { RunStatus } from '../src/types';

const statuses: RunStatus[] = ['creating', 'ready', 'working', 'waiting', 'failed', 'unreachable', 'stopped', 'completed'];

describe('agent status presentation', () => {
  it('renders a glyph and label for every derived status', () => {
    render(
      <>
        {statuses.map((status) => <StatusBadge key={status} status={status} />)}
      </>
    );
    expect(screen.getByText('Creating')).toBeInTheDocument();
    expect(screen.getByText('Ready')).toBeInTheDocument();
    expect(screen.getByText('Working')).toBeInTheDocument();
    const waiting = screen.getByLabelText('Waiting for input');
    expect(waiting).toHaveTextContent('Waiting');
    expect(waiting).toHaveAttribute('title', 'Waiting for input');
    expect(screen.queryByText('Waiting for input')).not.toBeInTheDocument();
    expect(screen.getByText('Failed')).toBeInTheDocument();
    expect(screen.getByText('Unreachable')).toBeInTheDocument();
    expect(screen.getByText('Stopped')).toBeInTheDocument();
    expect(screen.getByText('Completed')).toBeInTheDocument();
  });

  it('gates actions on the derived status, not user input', () => {
    expect(isActiveRun('CREATING')).toBe(true);
    expect(isActiveRun('waiting')).toBe(true);
    expect(isActiveRun('completed')).toBe(false);
    expect(isActiveRun('stopped')).toBe(false);
    expect(canAttachRun('ready')).toBe(true);
    expect(canAttachRun('working')).toBe(true);
    expect(canAttachRun('completed')).toBe(false);
    expect(canStopRun('waiting')).toBe(true);
    expect(canStopRun('failed')).toBe(false);
    expect(isWaitingRun('waiting')).toBe(true);
    expect(isWaitingRun('ready')).toBe(false);
    expect(statusLabel('unreachable')).toBe('Unreachable');
  });

  it('labels in-flight action states', () => {
    render(
      <>
        <StatusBadge status="stopping" />
        <StatusBadge status="deleting" />
      </>
    );
    expect(screen.getByText('Stopping')).toBeInTheDocument();
    expect(screen.getByText('Deleting')).toBeInTheDocument();
  });
});
