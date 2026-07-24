import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { queryKeys } from '../api/queries';
import type { ClassroomDetail } from '../api/types';
import { ClassroomDetailPage } from './ClassroomDetailPage';

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, className }: { children: ReactNode; className?: string }) => (
    <a href="#" className={className}>{children}</a>
  ),
  useNavigate: () => vi.fn(),
}));

vi.mock('sonner', () => ({
  toast: Object.assign(vi.fn(), { error: vi.fn() }),
}));

const classroomId = '62ec47f8-8e69-43ce-a6a0-85c4cc8cba70';

function classroomWithSeats(seatCount: number): ClassroomDetail {
  return {
    id: classroomId,
    name: '云教室 A',
    site: '主校区',
    status: 'READY',
    timezone: 'Asia/Shanghai',
    template_name: 'Windows 教学镜像',
    template_version: 'v1.0.0',
    active_session: null,
    updated_at: '2026-07-24T08:00:00Z',
    seats: Array.from({ length: seatCount }, (_, index) => ({
      id: `seat-${index + 1}`,
      label: `A-${String(index + 1).padStart(2, '0')}`,
      terminal: {
        id: `terminal-${index + 1}`,
        name: `终端 ${index + 1}`,
        online: true,
        last_seen_at: '2026-07-24T08:00:00Z',
      },
      desktop: {
        id: `desktop-${index + 1}`,
        name: `desktop-a-${index + 1}`,
        desired_state: 'RUNNING',
        observed_state: 'STOPPED',
        template_version: 'v1.0.0',
        guest_agent_ready: true,
        last_reconciled_at: '2026-07-24T08:00:00Z',
      },
      operation_state: 'IDLE',
      user_name: null,
    })),
  };
}

const clients: QueryClient[] = [];

function renderPage(classroom: ClassroomDetail): QueryClient {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Number.POSITIVE_INFINITY },
      mutations: { retry: false },
    },
  });
  clients.push(client);
  client.setQueryData(queryKeys.classroom(classroomId), classroom);
  render(
    <QueryClientProvider client={client}>
      <ClassroomDetailPage classroomId={classroomId} />
    </QueryClientProvider>,
  );
  return client;
}

afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  window.sessionStorage.clear();
});

describe('ClassroomDetailPage', () => {
  it('reuses the same idempotency key when an uncertain start request is retried', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('网络连接失败'));
    vi.stubGlobal('fetch', fetchMock);
    renderPage(classroomWithSeats(1));

    const startButton = screen.getByRole('button', { name: '整班开机' });
    await user.click(startButton);
    await waitFor(() => expect(startButton).toBeEnabled());
    await user.click(startButton);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));

    const firstKey = new Headers((fetchMock.mock.calls[0][1] as RequestInit).headers).get('Idempotency-Key');
    const secondKey = new Headers((fetchMock.mock.calls[1][1] as RequestInit).headers).get('Idempotency-Key');
    expect(firstKey).toBeTruthy();
    expect(secondKey).toBe(firstKey);
  });

  it('does not reference a missing scope description in an empty classroom', () => {
    renderPage(classroomWithSeats(0));

    const startButton = screen.getByRole('button', { name: '整班开机' });
    expect(startButton).toBeDisabled();
    expect(startButton).not.toHaveAttribute('aria-describedby');
    expect(startButton).toHaveAttribute('title', '这间教室还没有可开机的座位');
  });
});
