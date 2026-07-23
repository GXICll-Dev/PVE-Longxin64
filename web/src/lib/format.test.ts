import { describe, expect, it } from 'vitest';
import { getOperationProgress } from './format';

describe('getOperationProgress', () => {
  it('only counts terminal operation items as completed', () => {
    expect(
      getOperationProgress({
        total: 10,
        queued: 1,
        running: 2,
        succeeded: 5,
        failed: 1,
        skipped: 1,
        unknown: 0,
      }),
    ).toEqual({ completed: 7, percent: 70 });
  });

  it('does not invent progress for an operation with no items', () => {
    expect(
      getOperationProgress({
        total: 0,
        queued: 0,
        running: 0,
        succeeded: 0,
        failed: 0,
        skipped: 0,
        unknown: 0,
      }),
    ).toEqual({ completed: 0, percent: 0 });
  });
});
