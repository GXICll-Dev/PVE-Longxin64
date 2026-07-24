import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { queryKeys } from './queries';
import type { Operation, OperationEvent, OperationList, OperationProgress } from './types';
import { useOperationEventStreams } from './operationEvents';

type MockListener = EventListenerOrEventListenerObject;

class MockEventSource {
  static instances: MockEventSource[] = [];

  readonly url: string;
  readonly withCredentials: boolean;
  closed = false;
  onerror: ((event: Event) => void) | null = null;
  private readonly listeners = new Map<string, Set<MockListener>>();

  constructor(url: string | URL, options?: EventSourceInit) {
    this.url = String(url);
    this.withCredentials = options?.withCredentials ?? false;
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: MockListener): void {
    const listeners = this.listeners.get(type) ?? new Set<MockListener>();
    listeners.add(listener);
    this.listeners.set(type, listeners);
  }

  close(): void {
    this.closed = true;
  }

  emit(type: string, payload: OperationEvent): void {
    const event = new MessageEvent(type, {
      data: JSON.stringify(payload),
      lastEventId: String(payload.sequence),
    });
    for (const listener of this.listeners.get(type) ?? []) {
      if (typeof listener === 'function') listener.call(this, event);
      else listener.handleEvent(event);
    }
  }

  fail(): void {
    this.onerror?.(new Event('error'));
  }
}

const clients: QueryClient[] = [];

function createClient(list: OperationList): QueryClient {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(queryKeys.operations, list);
  clients.push(client);
  return client;
}

function renderStreams(client: QueryClient, operations: Operation[]) {
  return render(
    <QueryClientProvider client={client}>
      <StreamHarness operations={operations} />
    </QueryClientProvider>,
  );
}

function StreamHarness({ operations }: { operations: Operation[] }) {
  useOperationEventStreams(operations);
  return null;
}

function makeOperation(index: number, status: Operation['status'] = 'QUEUED'): Operation {
  return {
    id: `operation-${index}`,
    classroom_id: 'classroom-1',
    classroom_name: '云教室 A',
    type: 'START',
    status,
    counts: { total: 1, queued: 1, running: 0, succeeded: 0, failed: 0, skipped: 0, unknown: 0 },
    items: [
      {
        id: `item-${index}`,
        operation_id: `operation-${index}`,
        seat_id: `seat-${index}`,
        seat_label: `A-${index}`,
        target_name: `desktop-${index}`,
        status: 'QUEUED',
        updated_at: `2026-07-24T08:00:0${index}Z`,
      },
    ],
    resource_version: 1,
    created_at: `2026-07-24T08:00:0${index}Z`,
    updated_at: `2026-07-24T08:00:0${index}Z`,
  };
}

function progress(overrides: Partial<OperationProgress> = {}): OperationProgress {
  return {
    total: 1,
    completed: 0,
    queued: 1,
    running: 0,
    succeeded: 0,
    failed: 0,
    skipped: 0,
    unknown: 0,
    ...overrides,
  };
}

afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  MockEventSource.instances = [];
  vi.unstubAllGlobals();
});

describe('operation event streams', () => {
  it('subscribes only to the four most recent non-terminal operations', async () => {
    vi.stubGlobal('EventSource', MockEventSource);
    const operations = [
      makeOperation(1),
      makeOperation(2),
      makeOperation(3),
      makeOperation(4),
      makeOperation(5),
      makeOperation(6),
      makeOperation(7, 'SUCCEEDED'),
    ];
    const list = { items: operations, total: operations.length };
    const client = createClient(list);

    renderStreams(client, operations);

    await waitFor(() => expect(MockEventSource.instances).toHaveLength(4));
    expect(MockEventSource.instances.map((source) => source.url)).toEqual([
      '/api/v1/operations/operation-6/events',
      '/api/v1/operations/operation-5/events',
      '/api/v1/operations/operation-4/events',
      '/api/v1/operations/operation-3/events',
    ]);
    expect(MockEventSource.instances.every((source) => source.withCredentials)).toBe(true);
  });

  it('patches snapshots, parent updates and item updates without accepting stale events', async () => {
    vi.stubGlobal('EventSource', MockEventSource);
    const operation = makeOperation(1);
    const list: OperationList = { items: [operation], total: 1 };
    const client = createClient(list);
    renderStreams(client, list.items);
    await waitFor(() => expect(MockEventSource.instances).toHaveLength(1));
    const source = MockEventSource.instances[0];

    act(() => {
      source.emit('operation.updated', {
        event_type: 'operation.updated',
        operation_id: operation.id,
        sequence: 2,
        timestamp: '2026-07-24T08:01:00Z',
        operation_status: 'RUNNING',
        progress: progress({ queued: 0, running: 1 }),
        resource_version: 2,
      });
      source.emit('operation.item.updated', {
        event_type: 'operation.item.updated',
        operation_id: operation.id,
        item_id: operation.items[0].id,
        sequence: 3,
        timestamp: '2026-07-24T08:01:01Z',
        operation_status: 'RUNNING',
        item_status: 'RUNNING',
        seat_id: operation.items[0].seat_id,
        seat_label: operation.items[0].seat_label,
        target_name: operation.items[0].target_name,
        item_updated_at: '2026-07-24T08:01:01Z',
        progress: progress({ queued: 0, running: 1 }),
        resource_version: 2,
      });
    });

    let cached = client.getQueryData<OperationList>(queryKeys.operations)?.items[0];
    expect(cached).toMatchObject({ status: 'RUNNING', resource_version: 2 });
    expect(cached?.items[0]).toMatchObject({ status: 'RUNNING', updated_at: '2026-07-24T08:01:01Z' });

    act(() => {
      source.emit('operation.updated', {
        event_type: 'operation.updated',
        operation_id: operation.id,
        sequence: 2,
        timestamp: '2026-07-24T08:02:00Z',
        operation_status: 'FAILED',
        progress: progress({ queued: 0, failed: 1, completed: 1 }),
        resource_version: 5,
      });
      source.emit('operation.updated', {
        event_type: 'operation.updated',
        operation_id: operation.id,
        sequence: 4,
        timestamp: '2026-07-24T08:02:01Z',
        operation_status: 'FAILED',
        progress: progress({ queued: 0, failed: 1, completed: 1 }),
        resource_version: 1,
      });
    });

    cached = client.getQueryData<OperationList>(queryKeys.operations)?.items[0];
    expect(cached).toMatchObject({ status: 'RUNNING', resource_version: 2 });

    act(() => {
      source.emit('operation.snapshot', {
        event_type: 'operation.snapshot',
        operation_id: operation.id,
        sequence: 5,
        timestamp: '2026-07-24T08:03:00Z',
        operation_status: 'WAITING_PVE',
        progress: progress({ queued: 0, running: 1 }),
        resource_version: 3,
        reset: true,
        items: [
          {
            item_id: operation.items[0].id,
            seat_id: operation.items[0].seat_id,
            seat_label: operation.items[0].seat_label ?? '',
            target_name: operation.items[0].target_name ?? '',
            status: 'WAITING_PVE',
            updated_at: '2026-07-24T08:03:00Z',
          },
        ],
      });
    });

    cached = client.getQueryData<OperationList>(queryKeys.operations)?.items[0];
    expect(cached).toMatchObject({ status: 'WAITING_PVE', resource_version: 3 });
    expect(cached?.items[0]).toMatchObject({ status: 'WAITING_PVE', seat_label: 'A-1' });

    const snapshotBeforeFailure = client.getQueryData<OperationList>(queryKeys.operations);
    act(() => source.fail());
    expect(source.closed).toBe(false);
    expect(client.getQueryData(queryKeys.operations)).toBe(snapshotBeforeFailure);
  });

  it('closes a terminal stream and invalidates the list for complete final timestamps', async () => {
    vi.stubGlobal('EventSource', MockEventSource);
    const operation = makeOperation(1);
    const list: OperationList = { items: [operation], total: 1 };
    const client = createClient(list);
    const invalidate = vi.spyOn(client, 'invalidateQueries').mockResolvedValue();
    renderStreams(client, list.items);
    await waitFor(() => expect(MockEventSource.instances).toHaveLength(1));
    const source = MockEventSource.instances[0];

    act(() => {
      source.emit('operation.updated', {
        event_type: 'operation.updated',
        operation_id: operation.id,
        sequence: 2,
        timestamp: '2026-07-24T08:04:00Z',
        operation_status: 'SUCCEEDED',
        progress: progress({ queued: 0, succeeded: 1, completed: 1 }),
        resource_version: 2,
      });
    });

    expect(source.closed).toBe(true);
    expect(client.getQueryData<OperationList>(queryKeys.operations)?.items[0].status).toBe('SUCCEEDED');
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.operations });
  });
});
