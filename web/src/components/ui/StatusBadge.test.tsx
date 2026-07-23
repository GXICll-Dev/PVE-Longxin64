import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { StatusBadge, getStatusTone } from './StatusBadge';

describe('StatusBadge', () => {
  it('uses text and a warning tone for a partial result', () => {
    render(<StatusBadge status="PARTIALLY_SUCCEEDED" />);

    const badge = screen.getByText('部分完成');
    expect(badge).toHaveClass('status-badge--warning');
    expect(getStatusTone('PARTIALLY_SUCCEEDED')).toBe('warning');
  });

  it('normalizes lower-case API states', () => {
    render(<StatusBadge status="online" />);
    expect(screen.getByText('在线')).toHaveClass('status-badge--positive');
  });
});
