import { useEffect, useMemo, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { getOperationEventsUrl } from './client';
import { queryKeys } from './queries';
import type {
  Operation,
  OperationCounts,
  OperationEvent,
  OperationEventType,
  OperationItem,
  OperationItemEventState,
  OperationList,
  OperationProgress,
  OperationStatus,
} from './types';

const EVENT_TYPES: OperationEventType[] = ['operation.snapshot', 'operation.updated', 'operation.item.updated'];
const TERMINAL_STATUSES = new Set<OperationStatus>(['SUCCEEDED', 'PARTIALLY_SUCCEEDED', 'FAILED', 'CANCELLED']);
const MAX_ACTIVE_STREAMS = 4;

interface ManagedStream {
  source: EventSource | null;
  lastSequence: number;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isNonNegativeInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;
}

function decodeProgress(value: unknown): OperationProgress | undefined {
  if (!isRecord(value)) return undefined;
  const fields = ['total', 'completed', 'queued', 'running', 'succeeded', 'failed', 'skipped', 'unknown'] as const;
  if (!fields.every((field) => isNonNegativeInteger(value[field]))) return undefined;
  return value as unknown as OperationProgress;
}

function decodeItemState(value: unknown): OperationItemEventState | undefined {
  if (
    !isRecord(value) ||
    typeof value.item_id !== 'string' ||
    typeof value.seat_id !== 'string' ||
    typeof value.seat_label !== 'string' ||
    typeof value.target_name !== 'string' ||
    typeof value.status !== 'string' ||
    typeof value.updated_at !== 'string'
  ) {
    return undefined;
  }
  if (value.error_code !== undefined && typeof value.error_code !== 'string') return undefined;
  if (value.message !== undefined && typeof value.message !== 'string') return undefined;
  return value as unknown as OperationItemEventState;
}

export function decodeOperationEvent(data: unknown, expectedType: OperationEventType): OperationEvent | undefined {
  let value: unknown = data;
  if (typeof data === 'string') {
    try {
      value = JSON.parse(data) as unknown;
    } catch {
      return undefined;
    }
  }
  if (
    !isRecord(value) ||
    value.event_type !== expectedType ||
    typeof value.operation_id !== 'string' ||
    !isNonNegativeInteger(value.sequence) ||
    value.sequence < 1 ||
    typeof value.timestamp !== 'string' ||
    typeof value.operation_status !== 'string' ||
    !isNonNegativeInteger(value.resource_version) ||
    value.resource_version < 1
  ) {
    return undefined;
  }
  const progress = decodeProgress(value.progress);
  if (!progress) return undefined;
  const record = value;
  let items: OperationItemEventState[] | undefined;

  if (expectedType === 'operation.snapshot') {
    if (!Array.isArray(record.items)) return undefined;
    const decodedItems = record.items.map(decodeItemState);
    if (!decodedItems.every((item): item is OperationItemEventState => item !== undefined)) return undefined;
    items = decodedItems;
  }
  if (expectedType === 'operation.item.updated') {
    if (
      typeof record.item_id !== 'string' ||
      typeof record.item_status !== 'string' ||
      typeof record.seat_id !== 'string' ||
      typeof record.seat_label !== 'string' ||
      typeof record.target_name !== 'string' ||
      typeof record.item_updated_at !== 'string'
    ) {
      return undefined;
    }
  }
  if (record.error_code !== undefined && typeof record.error_code !== 'string') return undefined;
  if (record.message !== undefined && typeof record.message !== 'string') return undefined;

  return { ...record, progress, ...(items ? { items } : {}) } as unknown as OperationEvent;
}

function countsFromProgress(progress: OperationProgress): OperationCounts {
  return {
    total: progress.total,
    queued: progress.queued,
    running: progress.running,
    succeeded: progress.succeeded,
    failed: progress.failed,
    skipped: progress.skipped,
    unknown: progress.unknown,
  };
}

function patchItemFromSnapshot(state: OperationItemEventState, existing?: OperationItem): OperationItem {
  return {
    ...existing,
    id: state.item_id,
    seat_id: state.seat_id,
    seat_label: state.seat_label,
    target_name: state.target_name,
    status: state.status,
    error_code: state.error_code,
    error_message: undefined,
    message: state.message,
    updated_at: state.updated_at,
  };
}

function patchOperation(operation: Operation, event: OperationEvent): Operation {
  let items = operation.items;
  if (event.event_type === 'operation.snapshot') {
    const existingItems = new Map(operation.items.map((item) => [item.id, item]));
    items = (event.items ?? []).map((item) => patchItemFromSnapshot(item, existingItems.get(item.item_id)));
  } else if (event.event_type === 'operation.item.updated') {
    const itemIndex = operation.items.findIndex((item) => item.id === event.item_id);
    const existing = itemIndex >= 0 ? operation.items[itemIndex] : undefined;
    const updatedItem: OperationItem = {
      ...existing,
      id: event.item_id ?? '',
      operation_id: existing?.operation_id ?? event.operation_id,
      seat_id: event.seat_id ?? existing?.seat_id ?? '',
      seat_label: event.seat_label,
      target_name: event.target_name,
      status: event.item_status ?? existing?.status ?? 'UNKNOWN',
      error_code: event.error_code,
      error_message: undefined,
      message: event.message,
      updated_at: event.item_updated_at ?? event.timestamp,
    };
    items = [...operation.items];
    if (itemIndex >= 0) items[itemIndex] = updatedItem;
    else items.push(updatedItem);
  }

  return {
    ...operation,
    status: event.operation_status,
    counts: countsFromProgress(event.progress),
    items,
    resource_version: event.resource_version,
    updated_at: event.timestamp,
  };
}

export function applyOperationEventToList(current: OperationList | undefined, event: OperationEvent): OperationList | undefined {
  if (!current) return current;
  let changed = false;
  const items = current.items.map((operation) => {
    if (operation.id !== event.operation_id) return operation;
    if (event.resource_version < (operation.resource_version ?? 0)) return operation;
    changed = true;
    return patchOperation(operation, event);
  });
  if (!changed) return current;
  return { ...current, items, generated_at: event.timestamp };
}

function createdAtValue(operation: Operation): number {
  const value = Date.parse(operation.created_at);
  return Number.isNaN(value) ? 0 : value;
}

export function selectStreamOperationIds(operations: Operation[]): string[] {
  return operations
    .filter((operation) => !TERMINAL_STATUSES.has(operation.status))
    .sort((left, right) => createdAtValue(right) - createdAtValue(left))
    .slice(0, MAX_ACTIVE_STREAMS)
    .map((operation) => operation.id);
}

export function useOperationEventStreams(operations: Operation[]): void {
  const queryClient = useQueryClient();
  const streamsRef = useRef(new Map<string, ManagedStream>());
  const targetIds = useMemo(() => selectStreamOperationIds(operations), [operations]);
  const targetKey = JSON.stringify(targetIds);

  useEffect(() => {
    const desiredIds = new Set(JSON.parse(targetKey) as string[]);
    const streams = streamsRef.current;

    for (const [operationId, entry] of streams) {
      if (desiredIds.has(operationId)) continue;
      streams.delete(operationId);
      entry.source?.close();
    }

    for (const operationId of desiredIds) {
      if (streams.has(operationId)) continue;
      const entry: ManagedStream = { source: null, lastSequence: 0 };
      streams.set(operationId, entry);

      if (typeof EventSource === 'undefined') continue;

      try {
        const source = new EventSource(getOperationEventsUrl(operationId), { withCredentials: true });
        entry.source = source;
        for (const eventType of EVENT_TYPES) {
          source.addEventListener(eventType, (rawEvent) => {
            const active = streamsRef.current.get(operationId);
            if (active?.source !== source) return;
            const event = decodeOperationEvent((rawEvent as MessageEvent<string>).data, eventType);
            if (!event || event.operation_id !== operationId || event.sequence <= active.lastSequence) return;
            active.lastSequence = event.sequence;
            let eventApplied = false;
            queryClient.setQueryData<OperationList>(queryKeys.operations, (current) => {
              const updated = applyOperationEventToList(current, event);
              eventApplied = updated !== current;
              return updated;
            });

            if (eventApplied && TERMINAL_STATUSES.has(event.operation_status)) {
              streamsRef.current.delete(operationId);
              source.close();
              void queryClient.invalidateQueries({ queryKey: queryKeys.operations });
            }
          });
        }
      } catch {
        // The existing five-second query polling remains the fallback path.
      }
    }
  }, [queryClient, targetKey]);

  useEffect(() => {
    const streams = streamsRef.current;
    return () => {
      for (const [operationId, entry] of streams) {
        streams.delete(operationId);
        entry.source?.close();
      }
    };
  }, []);
}
