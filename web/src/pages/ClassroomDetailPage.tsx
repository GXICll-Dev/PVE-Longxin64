import { useEffect, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useNavigate } from '@tanstack/react-router';
import {
  ChevronLeft,
  ChevronRight,
  CircleAlert,
  CircleCheckBig,
  LoaderCircle,
  MonitorCheck,
  MonitorDot,
  Play,
  RefreshCw,
  ServerCog,
  UsersRound,
  X,
} from 'lucide-react';
import { toast } from 'sonner';
import { ApiError, createClassroomOperation } from '../api/client';
import { classroomQueryOptions, queryKeys } from '../api/queries';
import type { Seat } from '../api/types';
import { EmptyState, ErrorState, LoadingState, StaleDataNotice } from '../components/AsyncState';
import { LastUpdated } from '../components/LastUpdated';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/Button';
import { ProgressBar } from '../components/ui/ProgressBar';
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
  const [inspectedSeatId, setInspectedSeatId] = useState<string | null>(null);
  const inspectorCloseRef = useRef<HTMLButtonElement>(null);
  const inspectButtonRefs = useRef(new Map<string, HTMLButtonElement>());
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

  const inspectedSeat = useMemo(
    () => query.data?.seats.find((seat) => seat.id === inspectedSeatId) ?? null,
    [inspectedSeatId, query.data?.seats],
  );

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

  useEffect(() => {
    if (!inspectedSeatId) return;
    const frame = window.requestAnimationFrame(() => inspectorCloseRef.current?.focus());
    return () => window.cancelAnimationFrame(frame);
  }, [inspectedSeatId]);

  useEffect(() => {
    if (!inspectedSeatId) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      event.preventDefault();
      const previousSeatId = inspectedSeatId;
      setInspectedSeatId(null);
      window.requestAnimationFrame(() => inspectButtonRefs.current.get(previousSeatId)?.focus());
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [inspectedSeatId]);

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
  const readinessPercent = classroom.seats.length > 0 ? Math.round((readySeats / classroom.seats.length) * 100) : 0;
  const inspectedAttentionReason = inspectedSeat ? getSeatAttentionReason(inspectedSeat) : null;
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
        ? stoppedActionTargets < validSelectedIds.length
          ? `补启 ${stoppedActionTargets} 台桌面`
          : `启动 ${validSelectedIds.length} 台桌面`
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

  function startOperation() {
    startMutation.mutate({
      seatIds: targetSeatIds,
      idempotencyKey: idempotencyKeys.keyFor(startIntent),
      intent: startIntent,
    });
  }

  function closeInspector() {
    const previousSeatId = inspectedSeatId;
    setInspectedSeatId(null);
    window.requestAnimationFrame(() => {
      if (previousSeatId) inspectButtonRefs.current.get(previousSeatId)?.focus();
    });
  }

  return (
    <div className={`page-stack classroom-detail-page${validSelectedIds.length > 0 ? ' classroom-detail-page--selection-active' : ''}`}>
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
          <>
            <Button
              className="toolbar-button"
              aria-label={query.isFetching ? '正在刷新课堂状态' : '刷新课堂状态'}
              onClick={() => void query.refetch()}
              disabled={query.isFetching}
            >
              <RefreshCw className={query.isFetching ? 'spinner' : undefined} aria-hidden="true" size={16} />
              <span>{query.isFetching ? '刷新中' : '刷新状态'}</span>
            </Button>
            {validSelectedIds.length === 0 ? (
              <Button
                variant="primary"
                disabled={startButtonDisabled}
                aria-describedby={classroom.seats.length > 0 ? 'operation-scope-description' : undefined}
                title={classroom.seats.length === 0 ? '这间教室还没有可开机的座位' : undefined}
                onClick={startOperation}
              >
                {startMutation.isPending ? (
                  <LoaderCircle className="spinner" aria-hidden="true" size={16} />
                ) : (
                  <Play aria-hidden="true" size={15} fill="currentColor" />
                )}
                {startButtonLabel}
              </Button>
            ) : null}
          </>
        }
      />

      {query.isError ? (
        <StaleDataNotice error={query.error} isRetrying={query.isFetching} onRetry={() => void query.refetch()} />
      ) : null}

      <div className="detail-meta-row">
        <StatusBadge status={classroom.status} />
        <LastUpdated value={classroom.updated_at} isFetching={query.isFetching} timezone={classroom.timezone} />
      </div>

      <section
        className={`classroom-health-summary${attentionSeats > 0 ? ' classroom-health-summary--attention' : ''}`}
        aria-labelledby="classroom-health-title"
      >
        <div className="classroom-health-summary__message">
          <span className="classroom-health-summary__icon">
            {attentionSeats > 0 ? <CircleAlert aria-hidden="true" size={18} /> : <CircleCheckBig aria-hidden="true" size={18} />}
          </span>
          <div>
            <p className="section-kicker">课堂状态</p>
            <h2 id="classroom-health-title">
              {attentionSeats > 0 ? `${attentionSeats} 个座位需要处理` : '课堂已经可以开始'}
            </h2>
            <p id="operation-scope-description">
              {classroom.seats.length === 0
                ? '绑定座位后即可执行课堂操作。'
                : allActionTargetsRunning
                  ? '目标桌面均已运行，无需重复提交。'
                  : validSelectedIds.length > 0
                    ? `已选择 ${validSelectedIds.length} 个座位，操作只会作用于所选范围。`
                    : `${readySeats} / ${classroom.seats.length} 个座位可教学，未选择座位时操作整间教室。`}
            </p>
          </div>
        </div>

        <div className="classroom-health-summary__readiness">
          <div>
            <span>教学就绪</span>
            <strong>{readySeats} / {classroom.seats.length}</strong>
          </div>
          <ProgressBar value={readinessPercent} label={`${classroom.name} 教学就绪率 ${readinessPercent}%`} />
        </div>

        <div className="classroom-health-summary__signals" aria-label="教室状态摘要">
          <div><MonitorCheck aria-hidden="true" size={16} /><span>终端</span><strong>{onlineTerminals}/{classroom.seats.length}</strong></div>
          <div><MonitorDot aria-hidden="true" size={16} /><span>桌面</span><strong>{runningDesktops}/{classroom.seats.length}</strong></div>
          <div className={operationSeats > 0 ? 'is-attention' : undefined}><ServerCog aria-hidden="true" size={16} /><span>任务</span><strong>{operationSeats}</strong></div>
          <div className={attentionSeats > 0 ? 'is-attention' : undefined}><UsersRound aria-hidden="true" size={16} /><span>问题</span><strong>{attentionSeats}</strong></div>
        </div>
      </section>

      {classroom.seats.length === 0 ? (
        <EmptyState title="这间教室还没有座位" description="添加座位并绑定终端、桌面后，课堂状态将在这里显示。" />
      ) : (
        <section className={`classroom-workspace${inspectedSeat ? ' classroom-workspace--inspecting' : ''}`}>
          <div className="panel panel--flush classroom-seats-panel" aria-labelledby="seat-table-title">
            <div className="panel__header panel__header--padded">
              <div>
                <p className="section-kicker">课堂控制台</p>
                <h2 id="seat-table-title">座位与桌面</h2>
                <p className="panel__description">选择座位执行批量操作，打开详情查看终端、模板与对账信息。</p>
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
                    <th scope="col">座位与学生</th>
                    <th scope="col">瘦客户机</th>
                    <th scope="col">虚拟桌面</th>
                    <th scope="col">当前状态</th>
                    <th scope="col" className="disclosure-column"><span className="sr-only">详情</span></th>
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
                        data-inspected={inspectedSeatId === seat.id || undefined}
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
                          <div className="seat-identity-cell">
                            <strong>{seat.label}</strong>
                            <span>{seat.user_name || '未分配学生'}</span>
                          </div>
                        </td>
                        <td>
                          <div className="primary-cell primary-cell--compact">
                            <StatusBadge status={terminal.status} label={terminal.label} />
                            <span>{seat.terminal?.name ?? '无终端'}</span>
                          </div>
                        </td>
                        <td>
                          <div className="primary-cell primary-cell--compact">
                            <StatusBadge status={seat.desktop?.observed_state ?? 'UNBOUND'} label={seat.desktop ? undefined : '未分配'} />
                            <span>{seat.desktop?.name ?? '无桌面'}</span>
                          </div>
                        </td>
                        <td>
                          <span className={attentionReason ? 'seat-health seat-health--attention' : 'seat-health'}>
                            {attentionReason ?? '可教学'}
                          </span>
                        </td>
                        <td className="disclosure-column">
                          <button
                            ref={(node) => {
                              if (node) inspectButtonRefs.current.set(seat.id, node);
                              else inspectButtonRefs.current.delete(seat.id);
                            }}
                            type="button"
                            className="row-disclosure"
                            aria-label={`查看 ${seat.label} 详情`}
                            aria-expanded={inspectedSeatId === seat.id}
                            aria-controls={inspectedSeatId === seat.id ? 'seat-inspector' : undefined}
                            onClick={() => setInspectedSeatId(seat.id)}
                          >
                            <ChevronRight aria-hidden="true" size={17} />
                          </button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>

          {inspectedSeat ? (
            <aside id="seat-inspector" className="seat-inspector" aria-labelledby="seat-inspector-title">
              <header className="seat-inspector__header">
                <div>
                  <p className="section-kicker">座位详情</p>
                  <h2 id="seat-inspector-title">{inspectedSeat.label}</h2>
                  <span>{inspectedSeat.user_name || '未分配学生'}</span>
                </div>
                <button
                  ref={inspectorCloseRef}
                  type="button"
                  className="icon-button"
                  aria-label="关闭座位详情"
                  onClick={closeInspector}
                >
                  <X aria-hidden="true" size={17} />
                </button>
              </header>

              <div className={`inspector-health${inspectedAttentionReason ? ' inspector-health--attention' : ''}`}>
                {inspectedAttentionReason ? <CircleAlert aria-hidden="true" size={17} /> : <CircleCheckBig aria-hidden="true" size={17} />}
                <div>
                  <strong>{inspectedAttentionReason ?? '可教学'}</strong>
                  <span>{inspectedAttentionReason ? '请先处理后再开始课堂' : '最近一次检查未发现阻塞项'}</span>
                </div>
              </div>

              <dl className="inspector-list">
                <div>
                  <dt>瘦客户机</dt>
                  <dd><StatusBadge status={terminalStatus(inspectedSeat).status} label={terminalStatus(inspectedSeat).label} /></dd>
                  <dd>{inspectedSeat.terminal?.name ?? '未绑定'}{inspectedSeat.terminal?.ip_address ? ` · ${inspectedSeat.terminal.ip_address}` : ''}</dd>
                </div>
                <div>
                  <dt>虚拟桌面</dt>
                  <dd><StatusBadge status={inspectedSeat.desktop?.observed_state ?? 'UNBOUND'} label={inspectedSeat.desktop ? undefined : '未分配'} /></dd>
                  <dd>{inspectedSeat.desktop?.name ?? '未分配'}{inspectedSeat.desktop?.pve_vmid ? ` · VMID ${inspectedSeat.desktop.pve_vmid}` : ''}</dd>
                </div>
                <div>
                  <dt>教学模板</dt>
                  <dd>{inspectedSeat.desktop?.template_version ?? '—'}</dd>
                  <dd className={inspectedSeat.desktop?.guest_agent_ready ? undefined : 'inline-attention'}>
                    {inspectedSeat.desktop?.guest_agent_ready ? 'Guest Agent 正常' : 'Guest Agent 未就绪'}
                  </dd>
                </div>
                <div>
                  <dt>当前任务</dt>
                  <dd><StatusBadge status={inspectedSeat.operation_state} /></dd>
                  <dd>最后对账 {formatDateTime(inspectedSeat.desktop?.last_reconciled_at, classroom.timezone)}</dd>
                </div>
              </dl>

              <Button
                className="seat-inspector__selection"
                variant={selectedSeatIds.has(inspectedSeat.id) ? 'secondary' : 'primary'}
                onClick={() => toggleSeat(inspectedSeat.id)}
              >
                {selectedSeatIds.has(inspectedSeat.id) ? '取消选择此座位' : '选择此座位'}
              </Button>
            </aside>
          ) : null}
        </section>
      )}

      {validSelectedIds.length > 0 ? (
        <section className="selection-bar" aria-label="所选座位操作" aria-live="polite">
          <div>
            <strong>已选 {validSelectedIds.length} 个座位</strong>
            <span>{allActionTargetsRunning ? '所选桌面均已运行' : `其中 ${stoppedActionTargets} 台桌面需要启动`}</span>
          </div>
          <div className="selection-bar__actions">
            <Button
              className="selection-bar__cancel"
              variant="ghost"
              aria-label="取消选择全部座位"
              onClick={() => setSelectedSeatIds(new Set())}
            >
              <X aria-hidden="true" size={16} />
              <span>取消选择</span>
            </Button>
            <Button
              variant="primary"
              disabled={startButtonDisabled}
              aria-describedby="operation-scope-description"
              onClick={startOperation}
            >
              {startMutation.isPending ? (
                <LoaderCircle className="spinner" aria-hidden="true" size={16} />
              ) : (
                <Play aria-hidden="true" size={15} fill="currentColor" />
              )}
              {startButtonLabel}
            </Button>
          </div>
        </section>
      ) : null}
    </div>
  );
}
