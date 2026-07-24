import { describe, expect, it, vi } from 'vitest';
import { createOperationIntentSignature, OperationIdempotencyKeyManager } from './operationIdempotency';

describe('OperationIdempotencyKeyManager', () => {
  it('reuses a key until the intent succeeds or changes', () => {
    const createKey = vi.fn()
      .mockReturnValueOnce('key-1')
      .mockReturnValueOnce('key-2')
      .mockReturnValueOnce('key-3');
    const manager = new OperationIdempotencyKeyManager({ createKey });
    const wholeClass = createOperationIntentSignature({ classroomId: 'classroom-1', type: 'START', seatIds: [] });
    const selectedSeats = createOperationIntentSignature({
      classroomId: 'classroom-1',
      type: 'START',
      seatIds: ['seat-2', 'seat-1'],
    });

    expect(manager.keyFor(wholeClass)).toBe('key-1');
    expect(manager.keyFor(wholeClass)).toBe('key-1');
    expect(createKey).toHaveBeenCalledTimes(1);

    manager.synchronize(selectedSeats);
    expect(manager.keyFor(selectedSeats)).toBe('key-2');

    manager.acknowledge(selectedSeats);
    expect(manager.keyFor(selectedSeats)).toBe('key-3');
  });

  it('treats the same selected seat set as the same business intent', () => {
    const first = createOperationIntentSignature({
      classroomId: 'classroom-1',
      type: 'START',
      seatIds: ['seat-2', 'seat-1', 'seat-2'],
    });
    const second = createOperationIntentSignature({
      classroomId: 'classroom-1',
      type: 'START',
      seatIds: ['seat-1', 'seat-2'],
    });

    expect(first).toBe(second);
  });

  it('restores an uncertain intent key after a page reload', () => {
    const values = new Map<string, string>();
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key),
    };
    const intent = createOperationIntentSignature({ classroomId: 'classroom-1', type: 'START', seatIds: [] });
    const firstPage = new OperationIdempotencyKeyManager({
      createKey: () => 'reload-safe-key',
      storage,
      storageKey: 'classroom-start',
    });

    expect(firstPage.keyFor(intent)).toBe('reload-safe-key');

    const reloadedPage = new OperationIdempotencyKeyManager({
      createKey: () => 'unexpected-new-key',
      storage,
      storageKey: 'classroom-start',
    });
    expect(reloadedPage.keyFor(intent)).toBe('reload-safe-key');
  });
});
