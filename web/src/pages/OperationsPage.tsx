import { useQuery } from '@tanstack/react-query';
import {
  CheckCircle2,
  ChevronDown,
  CircleAlert,
  ClipboardCheck,
  Clock3,
  Play,
  Power,
  RefreshCw,
  RotateCcw,
} from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useOperationEventStreams } from '../api/operationEvents';
import { operationsQueryOptions } from '../api/queries';
import type { Operation, OperationType } from '../api/types';
import { EmptyState, ErrorState, LoadingState, StaleDataNotice } from '../components/AsyncState';
import { LastUpdated } from '../components/LastUpdated';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/Button';
import { ProgressBar } from '../components/ui/ProgressBar';
import { StatusBadge } from '../components/ui/StatusBadge';
import { formatCompactId, formatDateTime, formatOperationType, getOperationProgress } from '../lib/format';

const EMPTY_OPERATIONS: Operation[] = [];
const ACTIVE_STATUSES = ['QUEUED', 'VALIDATING', 'RUNNING', 'WAITING_PVE', 'VERIFYING', 'CANCEL_REQUESTED'];

function OperationTypeIcon({ type }: { type: OperationType }) {
  switch (type) {
    case 'PRECHECK':
      return <ClipboardCheck aria-hidden="true" size={19} />;
    case 'START':
      return <Play aria-hidden="true" size={19} fill="currentColor" />;
    case 'SHUTDOWN':
      return <Power aria-hidden="true" size={19} />;
    case 'RESTORE':
      return <RotateCcw aria-hidden="true" size={19} />;
  }
}

function OperationCard({ operation }: { operation: Operation }) {
  const progress = getOperationProgress(operation.counts);
  const hasAttentionItems = operation.items.some((item) =>
    ['FAILED', 'UNKNOWN'].includes(item.status.toUpperCase()),
  );
  const needsAttention = hasAttentionItems || ['FAILED', 'PARTIALLY_SUCCEEDED'].includes(operation.status.toUpperCase());
  const isActive = ACTIVE_STATUSES.includes(operation.status.toUpperCase());
  const [itemsOpen, setItemsOpen] = useState(needsAttention);
  const hasAutoOpened = useRef(needsAttention);

  useEffect(() => {
    if (needsAttention && !hasAutoOpened.current) {
      hasAutoOpened.current = true;
      setItemsOpen(true);
    }
  }, [needsAttention]);

  const unresolvedCount = operation.counts.failed + operation.counts.unknown;

  return (
    <article className={`operation-card${needsAttention ? ' operation-card--attention' : ''}`}>
      <header className="operation-card__header">
        <div className="operation-card__identity">
          <span className="operation-card__icon">
            <OperationTypeIcon type={operation.type} />
          </span>
          <div>
            <p>{operation.classroom_name || operation.classroom_id}</p>
            <h2>{formatOperationType(operation.type)}</h2>
            <code title={operation.id}>{formatCompactId(operation.id)}</code>
          </div>
        </div>
        <div className="operation-card__meta">
          <StatusBadge status={operation.status} />
          <span><Clock3 aria-hidden="true" size={13} /> {formatDateTime(operation.created_at)}</span>
        </div>
      </header>

      {isActive ? (
        <section className="operation-card__progress" aria-label="任务执行进度">
          <div className="operation-progress-heading">
            <div>
              <span>正在处理座位</span>
              <strong>{progress.completed} / {operation.counts.total} 已有终态</strong>
            </div>
            <strong>{progress.percent}%</strong>
          </div>
          <ProgressBar value={progress.percent} label={`${formatOperationType(operation.type)}完成 ${progress.percent}%`} />
        </section>
      ) : (
        <section className={`operation-outcome${needsAttention ? ' operation-outcome--attention' : ''}`} aria-label="任务结果">
          {needsAttention ? <CircleAlert aria-hidden="true" size={20} /> : <CheckCircle2 aria-hidden="true" size={20} />}
          <div>
            <strong>
              {needsAttention
                ? `${unresolvedCount} 个座位需要确认`
                : `${operation.counts.succeeded} 个座位已完成`}
            </strong>
            <span>
              {operation.completed_at ? `完成于 ${formatDateTime(operation.completed_at)}` : '任务已进入终态'}
            </span>
          </div>
        </section>
      )}

      <dl className="operation-counts">
        <div><dt>成功</dt><dd>{operation.counts.succeeded}</dd></div>
        <div><dt>执行中</dt><dd>{operation.counts.running}</dd></div>
        <div><dt>排队</dt><dd>{operation.counts.queued}</dd></div>
        <div className={operation.counts.failed > 0 ? 'operation-counts__attention' : undefined}>
          <dt>失败</dt><dd>{operation.counts.failed}</dd>
        </div>
        <div className={operation.counts.unknown > 0 ? 'operation-counts__attention' : undefined}>
          <dt>未知</dt><dd>{operation.counts.unknown}</dd>
        </div>
        <div><dt>跳过</dt><dd>{operation.counts.skipped}</dd></div>
      </dl>

      <details
        className="operation-items"
        open={itemsOpen}
        onToggle={(event) => setItemsOpen(event.currentTarget.open)}
      >
        <summary>
          <span className="operation-items__summary-content">
            <span>
              单机结果（{operation.items.length}）
              <ChevronDown aria-hidden="true" size={16} />
            </span>
            <span>
              {hasAttentionItems
                ? `${operation.counts.failed} 项失败 · ${operation.counts.unknown} 项未知${itemsOpen ? '，已展开' : '，展开查看'}`
                : '查看座位级执行结果'}
            </span>
          </span>
        </summary>
        {operation.items.length > 0 ? (
          <div className="operation-items__table-shell">
            <table className="operation-items__table">
              <caption className="sr-only">任务 {formatCompactId(operation.id)} 的单机执行结果</caption>
              <thead>
                <tr>
                  <th scope="col">座位</th>
                  <th scope="col">状态</th>
                  <th scope="col">错误代码</th>
                  <th scope="col">结果说明</th>
                  <th scope="col">更新时间</th>
                </tr>
              </thead>
              <tbody>
                {operation.items.map((item) => (
                  <tr key={item.id} data-attention={['FAILED', 'UNKNOWN'].includes(item.status.toUpperCase()) || undefined}>
                    <td>
                      <div className="primary-cell primary-cell--compact">
                        <strong>{item.seat_label || item.target_name || item.seat_id}</strong>
                        {item.target_name && item.target_name !== item.seat_label ? <span>{item.target_name}</span> : null}
                      </div>
                    </td>
                    <td><StatusBadge status={item.status} /></td>
                    <td>{item.error_code ? <code>{item.error_code}</code> : <span className="muted-cell">无</span>}</td>
                    <td className={item.message || item.error_message ? undefined : 'muted-cell'}>
                      {item.message || item.error_message || '暂无补充信息'}
                    </td>
                    <td className="muted-cell">{formatDateTime(item.updated_at ?? item.completed_at ?? item.started_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="operation-items__empty">控制平面尚未返回单机结果。</p>
        )}
      </details>
    </article>
  );
}

export function OperationsPage() {
  const query = useQuery(operationsQueryOptions);
  useOperationEventStreams(query.data?.items ?? EMPTY_OPERATIONS);

  if (!query.data) {
    if (query.isPending) return <LoadingState label="正在加载任务中心" />;
    return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;
  }

  const refreshedAt = query.data.generated_at ?? new Date(query.dataUpdatedAt).toISOString();
  const runningCount = query.data.items.filter((operation) =>
    ACTIVE_STATUSES.includes(operation.status.toUpperCase()),
  ).length;
  const attentionCount = query.data.items.filter((operation) =>
    ['FAILED', 'PARTIALLY_SUCCEEDED'].includes(operation.status.toUpperCase()),
  ).length;
  const visibleCount = query.data.items.length;
  const hiddenCount = Math.max(0, query.data.total - visibleCount);
  const activeOperations = query.data.items.filter((operation) =>
    ACTIVE_STATUSES.includes(operation.status.toUpperCase()),
  );
  const attentionOperations = query.data.items.filter((operation) =>
    !ACTIVE_STATUSES.includes(operation.status.toUpperCase())
      && ['FAILED', 'PARTIALLY_SUCCEEDED'].includes(operation.status.toUpperCase()),
  );
  const recentOperations = query.data.items.filter((operation) =>
    !ACTIVE_STATUSES.includes(operation.status.toUpperCase())
      && !['FAILED', 'PARTIALLY_SUCCEEDED'].includes(operation.status.toUpperCase()),
  );
  const operationGroups = [
    {
      key: 'active',
      eyebrow: '实时执行',
      title: '进行中',
      description: '关注当前阶段和座位完成数量。',
      items: activeOperations,
    },
    {
      key: 'attention',
      eyebrow: '异常收件箱',
      title: '需要处理',
      description: '优先确认失败与状态未知的座位。',
      items: attentionOperations,
    },
    {
      key: 'recent',
      eyebrow: '执行记录',
      title: '最近完成',
      description: '终态任务以结果为主，不再强调进度。',
      items: recentOperations,
    },
  ].filter((group) => group.items.length > 0);

  return (
    <div className="page-stack">
      <PageHeader
        title="任务中心"
        description="把批量任务看成可追踪的课堂动作：进行中看进度，完成后看结果，异常时直接定位到座位。"
        actions={
          <Button onClick={() => void query.refetch()} disabled={query.isFetching}>
            <RefreshCw className={query.isFetching ? 'spinner' : undefined} aria-hidden="true" size={16} />
            {query.isFetching ? '刷新中' : '刷新任务'}
          </Button>
        }
      />
      {query.isError ? (
        <StaleDataNotice error={query.error} isRetrying={query.isFetching} onRetry={() => void query.refetch()} />
      ) : null}

      <section className="task-summary" aria-label="任务摘要">
        <div>
          <span>当前范围</span>
          <strong>{visibleCount > 0 ? `当前显示 1–${visibleCount} / ${query.data.total} 个任务` : '当前没有可显示任务'}</strong>
        </div>
        <div>
          <span>执行中</span>
          <strong>{runningCount}</strong>
        </div>
        <div className={attentionCount > 0 ? 'task-summary__attention' : undefined}>
          <span>需要关注</span>
          <strong>{attentionCount}</strong>
        </div>
        <LastUpdated value={refreshedAt} isFetching={query.isFetching} label="刷新时间" />
      </section>

      {query.data.items.length === 0 ? (
        <EmptyState title="暂无批量任务" description="在课堂控制台提交课前检查、开机、关机或还原后，任务会出现在这里。" />
      ) : (
        <div className="operation-list-stack">
          {hiddenCount > 0 ? (
            <p className="list-scope-note">当前范围仅包含最近 {visibleCount} 个任务，另有 {hiddenCount} 个历史任务未显示。</p>
          ) : null}
          {operationGroups.map((group) => (
            <section className="operation-group" aria-labelledby={`operation-group-${group.key}`} key={group.key}>
              <header className="operation-group__header">
                <div>
                  <p className="section-kicker">{group.eyebrow}</p>
                  <h2 id={`operation-group-${group.key}`}>{group.title}</h2>
                  <span>{group.description}</span>
                </div>
                <strong>{group.items.length}</strong>
              </header>
              <div className="operation-group__list">
                {group.items.map((operation) => (
                  <OperationCard key={operation.id} operation={operation} />
                ))}
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}
