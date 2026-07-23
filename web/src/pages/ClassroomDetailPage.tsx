import { useEffect, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useNavigate } from '@tanstack/react-router';
import {
  ChevronLeft,
  LoaderCircle,
  MonitorCheck,
  MonitorDot,
  Play,
  RefreshCw,
  ServerCog,
  UsersRound,
} from 'lucide-react';
import { toast } from 'sonner';
import { ApiError, createClassroomOperation } from '../api/client';
import { classroomQueryOptions, queryKeys } from '../api/queries';
import type { Seat } from '../api/types';
import { EmptyState, ErrorState, LoadingState } from '../components/AsyncState';
import { LastUpdated } from '../components/LastUpdated';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/Button';
import { StatusBadge } from '../components/ui/StatusBadge';
import { formatCompactId, formatDateTime, normalizeStatus } from '../lib/format';

function isTerminalOnline(seat: Seat): boolean {
  if (!seat.terminal) return false;
  if (typeof seat.terminal.online === 'boolean') return seat.terminal.online;
  return normalizeStatus(seat.terminal.status) === 'ONLINE';
}

function isDesktopRunning(seat: Seat): boolean {
  if (!seat.desktop) return false;
  return normalizeStatus(seat.desktop.observed_state ?? seat.desktop.power_state) === 'RUNNING';
}

function isSeatReady(seat: Seat): boolean {
  const operationState = normalizeStatus(seat.operation_state);
  return (
    isTerminalOnline(seat) &&
    Boolean(seat.desktop?.guest_agent_ready) &&
    operationState !== 'FAILED' &&
    operationState !== 'UNKNOWN'
  );
}

function terminalStatus(seat: Seat): { status: string; label?: string } {
  if (!seat.terminal) return { status: 'UNBOUND', label: '未绑定' };
  if (typeof seat.terminal.online === 'boolean') {
    return { status: seat.terminal.online ? 'ONLINE' : 'OFFLINE' };
  }
  return { status: seat.terminal.status ?? 'UNKNOWN' };
}

export function ClassroomDetailPage({ classroomId }: { classroomId: string }) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const query = useQuery(classroomQueryOptions(classroomId));
  const [selectedSeatIds, setSelectedSeatIds] = useState<Set<string>>(() => new Set());
  const selectAllRef = useRef<HTMLInputElement>(null);

  const validSelectedIds = useMemo(() => {
    const seatIds = new Set(query.data?.seats.map((seat) => seat.id) ?? []);
    return [...selectedSeatIds].filter((id) => seatIds.has(id));
  }, [query.data?.seats, selectedSeatIds]);

  const allSelected = Boolean(query.data?.seats.length) && validSelectedIds.length === query.data?.seats.length;

  useEffect(() => {
    if (selectAllRef.current) {
      selectAllRef.current.indeterminate = validSelectedIds.length > 0 && !allSelected;
    }
  }, [allSelected, validSelectedIds.length]);

  const startMutation = useMutation({
    mutationFn: ({ seatIds, idempotencyKey }: { seatIds: string[]; idempotencyKey: string }) =>
      createClassroomOperation(
        classroomId,
        {
          type: 'START',
          ...(seatIds.length > 0 ? { seat_ids: seatIds } : {}),
        },
        idempotencyKey,
      ),
    onSuccess: (operation) => {
      setSelectedSeatIds(new Set());
      toast('开机任务已创建', {
        description: `请求已受理，任务 ${formatCompactId(operation.id)} 正在后台处理。`,
        action: {
          label: '查看任务',
          onClick: () => void navigate({ to: '/operations' }),
        },
      });
      void queryClient.invalidateQueries({ queryKey: queryKeys.dashboard });
      void queryClient.invalidateQueries({ queryKey: queryKeys.classrooms });
      void queryClient.invalidateQueries({ queryKey: queryKeys.classroom(classroomId) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.operations });
    },
    onError: (error) => {
      const message = error instanceof Error ? error.message : '控制平面拒绝了本次请求。';
      const requestId = error instanceof ApiError && error.requestId ? `请求 ID：${error.requestId}` : undefined;
      toast.error('开机任务创建失败', { description: requestId ? `${message} · ${requestId}` : message });
    },
  });

  if (query.isPending) return <LoadingState label="正在加载课堂控制台" />;
  if (query.isError) return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;

  const classroom = query.data;
  const readySeats = classroom.seats.filter(isSeatReady).length;
  const onlineTerminals = classroom.seats.filter(isTerminalOnline).length;
  const runningDesktops = classroom.seats.filter(isDesktopRunning).length;
  const operationSeats = classroom.seats.filter((seat) => !['IDLE', 'SUCCEEDED'].includes(normalizeStatus(seat.operation_state))).length;

  function toggleSeat(seatId: string) {
    setSelectedSeatIds((current) => {
      const next = new Set(current);
      if (next.has(seatId)) next.delete(seatId);
      else next.add(seatId);
      return next;
    });
  }

  function toggleAll() {
    if (allSelected) {
      setSelectedSeatIds(new Set());
      return;
    }
    setSelectedSeatIds(new Set(classroom.seats.map((seat) => seat.id)));
  }

  return (
    <div className="page-stack classroom-detail-page">
      <PageHeader
        eyebrow={
          <Link to="/classrooms" className="breadcrumb-link">
            <ChevronLeft aria-hidden="true" size={15} />
            返回云教室
          </Link>
        }
        title={classroom.name}
        description={`${classroom.site} · ${classroom.timezone} · 模板 ${classroom.template_name} ${classroom.template_version}`}
        actions={
          <div className="header-action-group">
            <Button onClick={() => void query.refetch()} disabled={query.isFetching}>
              <RefreshCw aria-hidden="true" size={16} />
              刷新状态
            </Button>
            <Button
              variant="primary"
              disabled={classroom.seats.length === 0 || startMutation.isPending}
              aria-describedby="operation-scope-description"
              onClick={() =>
                startMutation.mutate({
                  seatIds: validSelectedIds,
                  idempotencyKey: crypto.randomUUID(),
                })
              }
            >
              {startMutation.isPending ? (
                <LoaderCircle className="spinner" aria-hidden="true" size={16} />
              ) : (
                <Play aria-hidden="true" size={16} fill="currentColor" />
              )}
              {startMutation.isPending
                ? '任务受理中'
                : validSelectedIds.length > 0
                  ? `启动 ${validSelectedIds.length} 台桌面`
                  : '整班开机'}
            </Button>
          </div>
        }
      />

      <div className="detail-meta-row">
        <StatusBadge status={classroom.status} />
        <LastUpdated value={classroom.updated_at} isFetching={query.isFetching} timezone={classroom.timezone} />
      </div>

      <section className="compact-stat-grid" aria-label="教室状态摘要">
        <article>
          <UsersRound aria-hidden="true" size={19} />
          <span>座位就绪</span>
          <strong>{readySeats} / {classroom.seats.length}</strong>
        </article>
        <article>
          <MonitorCheck aria-hidden="true" size={19} />
          <span>终端在线</span>
          <strong>{onlineTerminals} / {classroom.seats.length}</strong>
        </article>
        <article>
          <MonitorDot aria-hidden="true" size={19} />
          <span>桌面运行</span>
          <strong>{runningDesktops} / {classroom.seats.length}</strong>
        </article>
        <article>
          <ServerCog aria-hidden="true" size={19} />
          <span>处理中座位</span>
          <strong>{operationSeats}</strong>
        </article>
      </section>

      {classroom.seats.length === 0 ? (
        <EmptyState title="这间教室还没有座位" description="添加座位并绑定终端、桌面后，课堂状态将在这里显示。" />
      ) : (
        <section className="panel panel--flush" aria-labelledby="seat-table-title">
          <div className="panel__header panel__header--padded">
            <div>
              <p className="panel__eyebrow">课堂控制台</p>
              <h2 id="seat-table-title">座位状态</h2>
            </div>
            <span id="operation-scope-description" className="selection-summary" aria-live="polite">
              {validSelectedIds.length > 0 ? `已选择 ${validSelectedIds.length} 个座位` : '未选择时操作整间教室'}
            </span>
          </div>

          <div className="table-shell table-shell--embedded">
            <table className="data-table seat-table">
              <caption className="sr-only">{classroom.name}座位、终端和虚拟桌面状态</caption>
              <thead>
                <tr>
                  <th scope="col" className="checkbox-column">
                    <input
                      ref={selectAllRef}
                      className="selection-checkbox"
                      type="checkbox"
                      checked={allSelected}
                      onChange={toggleAll}
                      aria-label={allSelected ? '取消选择全部座位' : '选择全部座位'}
                    />
                  </th>
                  <th scope="col">座位</th>
                  <th scope="col">学生</th>
                  <th scope="col">瘦客户机</th>
                  <th scope="col">虚拟桌面</th>
                  <th scope="col">模板合规</th>
                  <th scope="col">当前任务</th>
                  <th scope="col">最后对账</th>
                </tr>
              </thead>
              <tbody>
                {classroom.seats.map((seat) => {
                  const terminal = terminalStatus(seat);
                  return (
                    <tr key={seat.id} data-selected={selectedSeatIds.has(seat.id) || undefined}>
                      <td className="checkbox-column">
                        <input
                          className="selection-checkbox"
                          type="checkbox"
                          checked={selectedSeatIds.has(seat.id)}
                          onChange={() => toggleSeat(seat.id)}
                          aria-label={`选择座位 ${seat.label}`}
                        />
                      </td>
                      <td><strong>{seat.label}</strong></td>
                      <td>{seat.user_name || <span className="muted-cell">未分配</span>}</td>
                      <td>
                        <div className="primary-cell primary-cell--compact">
                          <StatusBadge status={terminal.status} label={terminal.label} />
                          <span>{seat.terminal?.name ?? '无终端'}{seat.terminal?.ip_address ? ` · ${seat.terminal.ip_address}` : ''}</span>
                        </div>
                      </td>
                      <td>
                        <div className="primary-cell primary-cell--compact">
                          <StatusBadge status={seat.desktop?.observed_state ?? 'UNBOUND'} label={seat.desktop ? undefined : '未分配'} />
                          <span>{seat.desktop?.name ?? '无桌面'}{seat.desktop?.pve_vmid ? ` · VMID ${seat.desktop.pve_vmid}` : ''}</span>
                        </div>
                      </td>
                      <td>
                        {seat.desktop ? (
                          <div className="primary-cell primary-cell--compact">
                            <strong>{seat.desktop.template_version}</strong>
                            <span>{seat.desktop.guest_agent_ready ? 'Guest Agent 正常' : 'Guest Agent 未就绪'}</span>
                          </div>
                        ) : <span className="muted-cell">无桌面</span>}
                      </td>
                      <td><StatusBadge status={seat.operation_state} /></td>
                      <td className="muted-cell">{formatDateTime(seat.desktop?.last_reconciled_at, classroom.timezone)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </div>
  );
}
