import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { queryKeys } from '../api/queries';
import type { OperationList } from '../api/types';
import { OperationsPage } from './OperationsPage';

const operationList: OperationList = {
  items: [
    {
      id: '879f1d29-e65d-4f23-a453-3b14939751dc',
      classroom_id: '62ec47f8-8e69-43ce-a6a0-85c4cc8cba70',
      classroom_name: '云教室 A',
      type: 'START',
      status: 'PARTIALLY_SUCCEEDED',
      counts: { total: 2, queued: 0, running: 0, succeeded: 1, failed: 0, skipped: 0, unknown: 1 },
      items: [
        {
          id: 'item-1',
          seat_id: 'seat-1',
          seat_label: 'A-01',
          target_name: 'desktop-a-01',
          status: 'SUCCEEDED',
          message: '桌面已运行',
          updated_at: '2026-07-24T08:01:00Z',
        },
        {
          id: 'item-2',
          seat_id: 'seat-2',
          seat_label: 'A-02',
          target_name: 'desktop-a-02',
          status: 'UNKNOWN',
          error_code: 'PVE_TIMEOUT',
          message: '等待 PVE 任务结果超时，当前状态尚未确认。',
          updated_at: '2026-07-24T08:02:00Z',
        },
      ],
      created_at: '2026-07-24T08:00:00Z',
      updated_at: '2026-07-24T08:02:00Z',
      completed_at: '2026-07-24T08:02:00Z',
    },
  ],
  total: 4,
  generated_at: '2026-07-24T08:03:00Z',
};

const clients: QueryClient[] = [];

function createClient(): QueryClient {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Number.POSITIVE_INFINITY },
    },
  });
  clients.push(client);
  return client;
}

function renderPage(client: QueryClient) {
  return render(
    <QueryClientProvider client={client}>
      <OperationsPage />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
});

describe('OperationsPage', () => {
  it('shows the current result range and seat-level partial failures', () => {
    const client = createClient();
    client.setQueryData(queryKeys.operations, operationList);

    renderPage(client);

    expect(screen.getByText('当前显示 1–1 / 4 个任务')).toBeInTheDocument();
    expect(screen.getByText('当前范围仅包含最近 1 个任务，另有 3 个历史任务未显示。')).toBeInTheDocument();
    expect(screen.getByText('A-02')).toBeInTheDocument();
    expect(screen.getByText('PVE_TIMEOUT')).toBeInTheDocument();
    expect(screen.getByText('等待 PVE 任务结果超时，当前状态尚未确认。')).toBeInTheDocument();

    const itemDetails = screen.getByText('单机结果（2）').closest('details');
    expect(itemDetails).toHaveAttribute('open');
  });

  it('retains the last snapshot and marks it stale after a background refresh fails', async () => {
    const client = createClient();
    client.setQueryData(queryKeys.operations, operationList);
    await client.invalidateQueries({ queryKey: queryKeys.operations, refetchType: 'none' });
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('网络连接失败')));

    renderPage(client);

    expect(screen.getByText('批量开机')).toBeInTheDocument();
    expect(await screen.findByText('连接中断，正在显示上次成功获取的数据')).toBeInTheDocument();
    expect(screen.getByText('A-02')).toBeInTheDocument();
  });

  it('uses the full error state when no successful snapshot exists', async () => {
    const client = createClient();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('网络连接失败')));

    renderPage(client);

    expect(await screen.findByText('数据加载失败')).toBeInTheDocument();
    expect(screen.queryByText('批量开机')).not.toBeInTheDocument();
  });
});
