import { useEffect, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useNavigate } from '@tanstack/react-router';
import {
  ChevronLeft,
  CircleAlert,
  CircleCheckBig,
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
import { EmptyState, ErrorState, LoadingState, StaleDataNotice } from '../components/AsyncState';
import { LastUpdated } from '../components/LastUpdated';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/Button';
import { StatusBadge } from '../components/ui/StatusBadge';
import { formatCompactId, formatDateTime, normalizeStatus } from '../lib/format';
import { createOperationIntentSignature, OperationIdempotencyKeyManager } from '../lib/operationIdempotency';

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

function getSeatAttentionReason(seat: Seat): string | null {
  const operationState = normalizeStatus(seat.operation_state);
  if (['FAILED', 'UNKNOWN', 'ERROR'].includes(operationState)) return '任务状态异常';
  if (!seat.terminal) return '未绑定终端';
  if (!isTerminalOnline(seat)) return '终端离线';
  if (!seat.desktop) return '未分配桌面';
  if (['ERROR', 'UNKNOWN'].includes(normalizeStatus(seat.desktop.observed_state ?? seat.desktop.power_state))) {
    return '桌面状态异常';
  }
  if (!seat.desktop.guest_agent_ready) return 'Guest Agent 未就绪';
  return null;
}

export function ClassroomDetailPage({ classroomId }: { classroomId: string }) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const query = useQuery(classroomQueryOptions(classroomId));
  const [selectedSeatIds, setSelectedSeatIds] = useState<Set<string>>(() => new Set());
  const [idempotencyKeys] = useState(
    () => new OperationIdempotencyKeyManager({
      storage: typeof window === 'undefined' ? undefined : window.sessionStorage,
      storageKey: `pve-classroom:start-operation:${classroomId}`,
    }),
  );
  const selectAllRef = useRef<HTMLInputElement>(null);

  const validSelectedIds = useMemo(() => {
    const seatIds = new Set(query.data?.seats.map((seat) => seat.id) ?? []);
    return [...selectedSeatIds].filter((id) => seatIds.has(id));
  }, [query.data?.seats, selectedSeatIds]);

  const allSelected = Boolean(query.data?.seats.length) && validSelectedIds.length === query.data?.seats.length;
  const targetSeatIds = useMemo(() => [...validSelectedIds].sort(), [validSelectedIds]);
  const startIntent = useMemo(
    () => createOperationIntentSignature({ classroomId, type: 'START', seatIds: targetSeatIds }),
    [classroomId, targetSeatIds],
  );

  useEffect(() => {
    if (selectAllRef.current) {
      selectAllRef.current.indeterminate = validSelectedIds.length > 0 && !allSelected;
    }
  }, [allSelected, validSelectedIds.length]);

  useEffect(() => {
    idempotencyKeys.synchronize(startIntent);
  }, [idempotencyKeys, startIntent]);

  const startMutation = useMutation({
    mutationFn: ({ seatIds, idempotencyKey }: { seatIds: string[]; idempotencyKey: string; intent: string }) =>
      createClassroomOperation(
        classroomId,
        {
          type: 'START',
          ...(seatIds.length > 0 ? { seat_ids: seatIds } : {}),
        },
        idempotencyKey,
      ),
    onSuccess: (operation, variables) => {
      idempotencyKeys.acknowledge(variables.intent);
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
      const detail = requestId ? `${message} · ${requestId}` : message;
      toast.error('尚未确认开机任务是否创建', {
        description: `${detail} 再次提交相同范围时会复用本次幂等键，避免重复创建任务。`,
      });
    },
  });

  if (!query.data) {
    if (query.isPending) return <LoadingState label="正在加载课堂控制台" />;
    return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;
  }

  const classroom = query.data;
  const readySeats = classroom.seats.filter(isSeatReady).length;
  const onlineTerminals = classroom.seats.filter(isTerminalOnline).length;
  const runningDesktops = classroom.seats.filter(isDesktopRunning).length;
  const operationSeats = classroom.seats.filter((seat) => !['IDLE', 'SUCCEEDED'].includes(normalizeStatus(seat.operation_state))).length;
  const attentionSeatDetails = classroom.seats.flatMap((seat) => {
    const reason = getSeatAttentionReason(seat);
    return reason ? [{ id: seat.id, label: seat.label, reason }] : [];
  });
  const attentionSeats = attentionSeatDetails.length;
  const actionTargets = validSelectedIds.length > 0
    ? classroom.seats.filter((seat) => selectedSeatIds.has(seat.id))
    : classroom.seats;
  const stoppedActionTargets = actionTargets.filter((seat) => !isDesktopRunning(seat)).length;
  const allActionTargetsRunning = actionTargets.length > 0 && actionTargets.every(isDesktopRunning);
  const startButtonDisabled = classroom.seats.length === 0 || startMutation.isPending || allActionTargetsRunning;
  const startButtonLabel = startMutation.isPending
    ? '任务受理中'
    : allActionTargetsRunning
      ? validSelectedIds.length > 0 ? '所选桌面已运行' : '桌面已全部启动'
      : validSelectedIds.length > 0
        ? `启动 ${validSelectedIds.length} 台桌面`
        : classroom.status === 'ACTIVE'
          ? `补启 ${stoppedActionTargets} 台桌面`
          : '整班开机';

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
        description={`${classroom.site} · 教学模板 ${classroom.template_name} ${classroom.template_version}`}
        actions={
          <Button onClick={() => void query.refetch()} disabled={query.isFetching}>
            <RefreshCw className={query.isFetching ? 'spinner' : undefined} aria-hidden="true" size={16} />
            {query.isFetching ? '刷新中' : '刷新状态'}
          </Button>
        }
      />

      {query.isError ? (
        <StaleDataNotice error={query.error} isRetrying={query.isFetching} onRetry={() => void query.refetch()} />
      ) : null}

      <div className="detail-meta-row">
        <StatusBadge status={classroom.status} />
        <LastUpdated value={classroom.updated_at} isFetching={query.isFetching} timezone={classroom.timezone} />
      </div>

      <section className={`classroom-briefing${attentionSeats > 0 ? ' classroom-briefing--attention' : ''}`} aria-labelledby="classroom-health-title">
        <div>
          <p className="section-kicker">课堂健康</p>
          <h2 id="classroom-health-title">
            {attentionSeats > 0 ? `${attentionSeats} 个座位阻塞开课` : '全部座位已具备教学条件'}
          </h2>
          <p>
            {attentionSeats > 0
              ? `当前 ${readySeats} / ${classroom.seats.length} 个座位可教学。异常座位已在下方突出显示，请优先处理终端心跳、桌面状态和 Guest Agent。`
              : `终端、虚拟桌面与 Guest Agent 均已通过最近一次课堂检查，可以执行整班操作。`}
          </p>
          {attentionSeatDetails.length > 0 ? (
            <div className="classroom-attention-list" aria-label="需要处理的座位">
              {attentionSeatDetails.slice(0, 4).map((seat) => (
                <span key={seat.id}><strong>{seat.label}</strong>{seat.reason}</span>
              ))}
              {attentionSeatDetails.length > 4 ? <span>另有 {attentionSeatDetails.length - 4} 个座位</span> : null}
            </div>
          ) : null}
        </div>
        <div className="classroom-briefing__score">
          {attentionSeats > 0 ? <CircleAlert aria-hidden="true" size={20} /> : <CircleCheckBig aria-hidden="true" size={20} />}
          <strong>{readySeats}<span> / {classroom.seats.length}</span></strong>
          <small>座位可教学</small>
        </div>
      </section>

      <section className="classroom-signal-strip" aria-label="教室状态摘要">
        <article>
          <MonitorCheck aria-hidden="true" size={18} />
          <div><span>终端在线</span><strong>{onlineTerminals} / {classroom.seats.length}</strong></div>
        </article>
        <article>
          <MonitorDot aria-hidden="true" size={18} />
          <div><span>桌面运行</span><strong>{runningDesktops} / {classroom.seats.length}</strong></div>
        </article>
        <article className={operationSeats > 0 ? 'classroom-signal-strip__attention' : undefined}>
          <ServerCog aria-hidden="true" size={18} />
          <div><span>处理中座位</span><strong>{operationSeats}</strong></div>
        </article>
        <article className={attentionSeats > 0 ? 'classroom-signal-strip__attention' : undefined}>
          <UsersRound aria-hidden="true" size={18} />
          <div><span>需要处理</span><strong>{attentionSeats}</strong></div>
        </article>
      </section>

      <section className="classroom-action-bar" aria-label="课堂批量操作">
        <div>
          <p>本次操作范围</p>
          <strong id="operation-scope-description" aria-live="polite">
            {validSelectedIds.length > 0 ? `已选择 ${validSelectedIds.length} 个座位` : '整间教室'}
          </strong>
          <span>
            {classroom.seats.length === 0
              ? '绑定座位后即可执行批量开机。'
              : allActionTargetsRunning
                ? '目标桌面均已运行，无需重复提交。'
                : validSelectedIds.length > 0 ? '仅对所选座位创建任务。' : '未选择座位时操作整间教室。'}
          </span>
        </div>
        <Button
          className="classroom-start-button"
          variant="primary"
          disabled={startButtonDisabled}
          aria-describedby={classroom.seats.length > 0 ? 'operation-scope-description' : undefined}
          title={classroom.seats.length === 0 ? '这间教室还没有可开机的座位' : undefined}
          onClick={() =>
            startMutation.mutate({
              seatIds: targetSeatIds,
              idempotencyKey: idempotencyKeys.keyFor(startIntent),
              intent: startIntent,
            })
          }
        >
          {startMutation.isPending ? (
            <LoaderCircle className="spinner" aria-hidden="true" size={16} />
          ) : (
            <Play aria-hidden="true" size={16} fill="currentColor" />
          )}
          {startButtonLabel}
        </Button>
      </section>

      {classroom.seats.length === 0 ? (
        <EmptyState title="这间教室还没有座位" description="添加座位并绑定终端、桌面后，课堂状态将在这里显示。" />
      ) : (
        <section className="panel panel--flush" aria-labelledby="seat-table-title">
          <div className="panel__header panel__header--padded">
            <div>
              <p className="section-kicker">课堂控制台</p>
              <h2 id="seat-table-title">座位与桌面</h2>
              <p className="panel__description">正常状态保持安静，阻塞开课的座位会用原因提示突出显示。</p>
            </div>
            <span className="selection-summary">{validSelectedIds.length > 0 ? `已选 ${validSelectedIds.length} / ${classroom.seats.length}` : `${classroom.seats.length} 个座位`}</span>
          </div>

          <div className="table-shell table-shell--embedded">
            <table className="data-table seat-table">
              <caption className="sr-only">{classroom.name}座位、终端和虚拟桌面状态</caption>
              <thead>
                <tr>
                  <th scope="col" className="checkbox-column">
                    <label className="selection-control">
                      <input
                        ref={selectAllRef}
                        className="selection-checkbox"
                        type="checkbox"
                        checked={allSelected}
                        onChange={toggleAll}
                        aria-label={allSelected ? '取消选择全部座位' : '选择全部座位'}
                      />
                    </label>
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
                  const attentionReason = getSeatAttentionReason(seat);
                  return (
                    <tr
                      key={seat.id}
                      data-selected={selectedSeatIds.has(seat.id) || undefined}
                      data-attention={attentionReason ? true : undefined}
                    >
                      <td className="checkbox-column">
                        <label className="selection-control">
                          <input
                            className="selection-checkbox"
                            type="checkbox"
                            checked={selectedSeatIds.has(seat.id)}
                            onChange={() => toggleSeat(seat.id)}
                            aria-label={`选择座位 ${seat.label}`}
                          />
                        </label>
                      </td>
                      <td>
                        <div className="seat-label-cell">
                          <strong>{seat.label}</strong>
                          <span className={attentionReason ? 'seat-health seat-health--attention' : 'seat-health'}>
                            {attentionReason ?? '可教学'}
                          </span>
                        </div>
                      </td>
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
                            <span className={seat.desktop.guest_agent_ready ? undefined : 'inline-attention'}>
                              {seat.desktop.guest_agent_ready ? 'Guest Agent 正常' : 'Guest Agent 未就绪'}
                            </span>
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
