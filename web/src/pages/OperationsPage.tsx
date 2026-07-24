import { useQuery } from '@tanstack/react-query';
import {
  ChevronDown,
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

function OperationTaskRow({ operation }: { operation: Operation }) {
  const progress = getOperationProgress(operation.counts);
  const hasAttentionItems = operation.items.some((item) =>
    ['FAILED', 'UNKNOWN'].includes(item.status.toUpperCase()),
  ) || operation.counts.failed > 0 || operation.counts.unknown > 0;
  const needsAttention = hasAttentionItems || ['FAILED', 'PARTIALLY_SUCCEEDED'].includes(operation.status.toUpperCase());
  const isActive = ACTIVE_STATUSES.includes(operation.status.toUpperCase());
  const [itemsOpen, setItemsOpen] = useState(needsAttention);
  const previouslyNeededAttention = useRef(needsAttention);

  useEffect(() => {
    if (needsAttention && !previouslyNeededAttention.current) {
      setItemsOpen(true);
    }
    previouslyNeededAttention.current = needsAttention;
  }, [needsAttention]);

  const timeValue = isActive
    ? operation.created_at
    : operation.completed_at ?? operation.updated_at ?? operation.created_at;
  const timeLabel = isActive ? '创建时间' : operation.completed_at ? '完成时间' : '更新时间';
  const completionHint = needsAttention
    ? `${operation.counts.failed} 失败 · ${operation.counts.unknown} 未知`
    : isActive
      ? `${operation.counts.running} 执行中 · ${operation.counts.queued} 排队`
      : `${operation.counts.succeeded} 成功`;

  return (
    <article
      className={`operation-task-row${isActive ? ' operation-task-row--active' : ''}${needsAttention ? ' operation-task-row--attention' : ''}`}
    >
      <header className="operation-task-row__main">
        <div className="operation-task-row__action">
          <span className="operation-task-row__icon">
            <OperationTypeIcon type={operation.type} />
          </span>
          <div>
            <strong>{formatOperationType(operation.type)}</strong>
            <code title={operation.id}>{formatCompactId(operation.id)}</code>
          </div>
        </div>

        <div className="operation-task-row__classroom">
          <span>教室</span>
          <strong>{operation.classroom_name || operation.classroom_id}</strong>
        </div>

        <div className="operation-task-row__status">
          <span className="sr-only">任务状态</span>
          <StatusBadge status={operation.status} />
        </div>

        <div className="operation-task-row__completion">
          <span>完成数</span>
          <strong>{progress.completed} / {operation.counts.total}</strong>
          <small>{completionHint}</small>
        </div>

        <time className="operation-task-row__time" dateTime={timeValue}>
          <Clock3 aria-hidden="true" size={14} />
          <span>
            <small>{timeLabel}</small>
            <strong>{formatDateTime(timeValue)}</strong>
          </span>
        </time>
      </header>

      {isActive ? (
        <div className="operation-task-row__progress" aria-label="任务执行进度">
          <ProgressBar value={progress.percent} label={`${formatOperationType(operation.type)}完成 ${progress.percent}%`} />
          <div className="operation-task-row__progress-meta">
            <span>{progress.completed} / {operation.counts.total} 已有终态</span>
            <strong>{progress.percent}%</strong>
          </div>
        </div>
      ) : null}

      <details
        className="operation-items operation-task-row__details"
        open={itemsOpen}
        onToggle={(event) => setItemsOpen(event.currentTarget.open)}
      >
        <summary className="operation-task-row__details-summary">
          <span className="operation-items__summary-content">
            <span>
              单机结果（{operation.items.length}）
              <ChevronDown aria-hidden="true" size={16} />
            </span>
            <span>
              {hasAttentionItems
                ? `${operation.counts.failed} 项失败 · ${operation.counts.unknown} 项未知${itemsOpen ? '，已展开' : '，展开查看'}`
                : operation.items.length > 0
                  ? '查看座位级执行结果'
                  : '尚无座位级结果'}
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
                <ul className="operation-grouped-list">
                  {group.items.map((operation) => (
                    <li className="operation-grouped-list__item" key={operation.id}>
                      <OperationTaskRow operation={operation} />
                    </li>
                  ))}
                </ul>
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}
