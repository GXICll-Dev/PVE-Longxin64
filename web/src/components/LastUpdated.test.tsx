import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { LastUpdated } from './LastUpdated';

describe('LastUpdated', () => {
  it('does not announce every background polling transition', () => {
    const { container, rerender } = render(<LastUpdated value="2026-07-24T08:00:00Z" />);
    const status = container.querySelector('.last-updated');

    expect(status).not.toHaveAttribute('aria-live');

    rerender(<LastUpdated value="2026-07-24T08:00:00Z" isFetching />);
    expect(screen.getByText(/正在刷新/)).toBeInTheDocument();
    expect(status).not.toHaveAttribute('aria-live');
  });
});
