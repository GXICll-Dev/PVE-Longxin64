import { describe, expect, it, vi } from 'vitest';
import {
  ApiContractError,
  createClassroomOperation,
  decodeClassroomList,
  decodeDashboard,
  decodeOperationList,
  getDashboard,
} from './client';

describe('API response decoders', () => {
  it('decodes a direct dashboard response without inventing values', () => {
    const payload = {
      generated_at: '2026-07-24T08:00:00Z',
      summary: {
        classrooms_total: 2,
        classrooms_ready: 1,
        classrooms_active: 1,
        seats_total: 50,
        seats_ready: 46,
        thin_clients_online: 48,
        desktops_running: 47,
        operations_running: 2,
        operations_failed: 1,
      },
      alerts: [],
    };

    expect(decodeDashboard(payload)).toEqual(payload);
  });

  it('accepts a data envelope while preserving list metadata', () => {
    const classroom = {
      id: '62ec47f8-8e69-43ce-a6a0-85c4cc8cba70',
      name: '云教室 A',
      site: '主校区',
    };

    expect(
      decodeClassroomList({
        data: { items: [classroom], total: 1, generated_at: '2026-07-24T08:00:00Z' },
      }),
    ).toMatchObject({ items: [classroom], total: 1 });
  });

  it('rejects malformed operation data instead of presenting an empty success state', () => {
    expect(() => decodeOperationList({ total: 4 })).toThrow(ApiContractError);
  });

  it('sends the stable idempotency key when creating a classroom operation', async () => {
    const operation = {
      id: '879f1d29-e65d-4f23-a453-3b14939751dc',
      classroom_id: '62ec47f8-8e69-43ce-a6a0-85c4cc8cba70',
      classroom_name: '云教室 A',
      type: 'START',
      status: 'QUEUED',
      counts: { total: 1, queued: 1, running: 0, succeeded: 0, failed: 0, skipped: 0, unknown: 0 },
      items: [],
      created_at: '2026-07-24T08:00:00Z',
      updated_at: '2026-07-24T08:00:00Z',
    };
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(operation), {
        status: 202,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await createClassroomOperation(
      operation.classroom_id,
      { type: 'START', seat_ids: ['seat-1'] },
      '56f2e419-0f2b-40e6-a492-d42ef4010802',
    );

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(new Headers(init.headers).get('Idempotency-Key')).toBe('56f2e419-0f2b-40e6-a492-d42ef4010802');
    expect(init.body).toBe(JSON.stringify({ type: 'START', seat_ids: ['seat-1'] }));
  });

  it('preserves request IDs from an error envelope', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            error: {
              error_code: 'PVE_UNAVAILABLE',
              message: 'PVE 集群暂时不可用。',
              request_id: 'req-frontend-contract-test',
            },
          }),
          { status: 503, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    );

    await expect(getDashboard()).rejects.toMatchObject({
      name: 'ApiError',
      code: 'PVE_UNAVAILABLE',
      requestId: 'req-frontend-contract-test',
      message: 'PVE 集群暂时不可用。',
    });
  });
});
