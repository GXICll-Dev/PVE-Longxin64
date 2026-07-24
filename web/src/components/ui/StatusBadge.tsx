import { cva } from 'class-variance-authority';
import { Circle } from 'lucide-react';
import { formatStatus, normalizeStatus } from '../../lib/format';

const statusBadge = cva('status-badge', {
  variants: {
    tone: {
      positive: 'status-badge--positive',
      informative: 'status-badge--informative',
      warning: 'status-badge--warning',
      critical: 'status-badge--critical',
      neutral: 'status-badge--neutral',
    },
  },
  defaultVariants: { tone: 'neutral' },
});

export type StatusTone = 'positive' | 'informative' | 'warning' | 'critical' | 'neutral';

export function getStatusTone(status?: string | null): StatusTone {
  switch (normalizeStatus(status)) {
    case 'READY':
    case 'RUNNING':
    case 'SUCCEEDED':
    case 'ONLINE':
      return 'positive';
    case 'ACTIVE':
    case 'QUEUED':
    case 'VALIDATING':
    case 'WAITING_PVE':
    case 'VERIFYING':
    case 'STARTING':
      return 'informative';
    case 'DEGRADED':
    case 'PARTIALLY_SUCCEEDED':
    case 'STALE':
    case 'CANCEL_REQUESTED':
    case 'UNKNOWN':
      return 'warning';
    case 'FAILED':
    case 'ERROR':
    case 'OFFLINE':
      return 'critical';
    default:
      return 'neutral';
  }
}

interface StatusBadgeProps {
  status?: string | null;
  label?: string;
}

export function StatusBadge({ status, label }: StatusBadgeProps) {
  const tone = getStatusTone(status);
  return (
    <span className={statusBadge({ tone })}>
      <Circle aria-hidden="true" size={7} fill="currentColor" strokeWidth={0} />
      {label ?? formatStatus(status)}
    </span>
  );
}
