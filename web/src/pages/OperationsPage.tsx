import { useQuery } from '@tanstack/react-query';
import { RefreshCw } from 'lucide-react';
import { useState } from 'react';
import { useOperationEventStreams } from '../api/operationEvents';
import { operationsQueryOptions } from '../api/queries';
import type { Operation } from '../api/types';
import { EmptyState, ErrorState, LoadingState, StaleDataNotice } from '../components/AsyncState';
import { LastUpdated } from '../components/LastUpdated';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/Button';
import { ProgressBar } from '../components/ui/ProgressBar';
import { StatusBadge } from '../components/ui/StatusBadge';
import { formatCompactId, formatDateTime, formatOperationType, getOperationProgress } from '../lib/format';

const EMPTY_OPERATIONS: Operation[] = [];

function OperationRows({ operation }: { operation: Operation }) {
  const progress = getOperationProgress(operation.counts);
  const hasAttentionItems = operation.items.some((item) =>
    ['FAILED', 'UNKNOWN'].includes(item.status.toUpperCase()),
  );
  const shouldAutoOpen = hasAttentionItems || ['FAILED', 'PARTIALLY_SUCCEEDED'].includes(operation.status.toUpperCase());
  const [itemsOpen, setItemsOpen] = useState(shouldAutoOpen);

  return (
    <>
      <tr>
        <td>
          <div className="primary-cell">
            <strong>{formatOperationType(operation.type)}</strong>
            <code title={operation.id}>{formatCompactId(operation.id)}</code>
          </div>
        </td>
        <td>{operation.classroom_name || operation.classroom_id}</td>
        <td><StatusBadge status={operation.status} /></td>
        <td>
          <div className="operation-progress-cell">
            <div>
              <span>{progress.completed} / {operation.counts.total} 已有终态</span>
              <strong>{progress.percent}%</strong>
            </div>
            <ProgressBar value={progress.percent} label={`${formatOperationType(operation.type)}完成 ${progress.percent}%`} />
            <small>
              {operation.counts.succeeded} 成功 · {operation.counts.running} 执行中 · {operation.counts.queued} 排队 ·{' '}
              {operation.counts.skipped} 跳过 · {operation.counts.unknown} 未知
            </small>
          </div>
        </td>
        <td>
          <span className={operation.counts.failed > 0 || operation.counts.unknown > 0 ? 'attention-count' : 'muted-cell'}>
            {operation.counts.failed} / {operation.counts.unknown}
          </span>
        </td>
        <td className="muted-cell">{formatDateTime(operation.created_at)}</td>
        <td className="muted-cell">{formatDateTime(operation.completed_at)}</td>
      </tr>
      <tr className="operation-items-row">
        <td colSpan={7}>
          <details
            className="operation-items"
            open={itemsOpen}
            onToggle={(event) => setItemsOpen(event.currentTarget.open)}
          >
            <summary>
              <span className="operation-items__summary-content">
                <span>单机结果（{operation.items.length}）</span>
                <span>
                  {hasAttentionItems
                    ? `${operation.counts.failed} 项失败 · ${operation.counts.unknown} 项未知${itemsOpen ? '，已展开' : '，展开查看'}`
                    : '展开查看座位级执行结果'}
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
                      <tr key={item.id}>
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
        </td>
      </tr>
    </>
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
    ['QUEUED', 'VALIDATING', 'RUNNING', 'WAITING_PVE', 'VERIFYING', 'CANCEL_REQUESTED'].includes(operation.status.toUpperCase()),
  ).length;
  const attentionCount = query.data.items.filter((operation) =>
    ['FAILED', 'PARTIALLY_SUCCEEDED'].includes(operation.status.toUpperCase()),
  ).length;
  const visibleCount = query.data.items.length;
  const hiddenCount = Math.max(0, query.data.total - visibleCount);

  return (
    <div className="page-stack">
      <PageHeader
        title="任务中心"
        description="查看批量父任务的阶段与单机结果。请求已受理、正在执行和已经成功在这里严格区分。"
        actions={
          <Button onClick={() => void query.refetch()} disabled={query.isFetching}>
            <RefreshCw aria-hidden="true" size={16} />
            刷新任务
          </Button>
        }
      />
      {query.isError ? (
        <StaleDataNotice error={query.error} isRetrying={query.isFetching} onRetry={() => void query.refetch()} />
      ) : null}
      <div className="page-meta-row">
        <div className="task-summary" aria-label="任务摘要">
          <span>{visibleCount > 0 ? `当前显示 1–${visibleCount} / ${query.data.total} 个任务` : '当前没有可显示任务'}</span>
          <span>{runningCount} 个执行中</span>
          <span>{attentionCount} 个需要关注</span>
        </div>
        <LastUpdated value={refreshedAt} isFetching={query.isFetching} label="刷新时间" />
      </div>

      {query.data.items.length === 0 ? (
        <EmptyState title="暂无批量任务" description="在课堂控制台提交课前检查、开机、关机或还原后，任务会出现在这里。" />
      ) : (
        <div className="operation-list-stack">
          {hiddenCount > 0 ? (
            <p className="list-scope-note">当前范围仅包含最近 {visibleCount} 个任务，另有 {hiddenCount} 个历史任务未显示。</p>
          ) : null}
          <div className="table-shell">
            <table className="data-table operation-table">
              <caption className="sr-only">批量任务进度列表</caption>
              <thead>
                <tr>
                  <th scope="col">任务</th>
                  <th scope="col">教室</th>
                  <th scope="col">状态</th>
                  <th scope="col">执行进度</th>
                  <th scope="col">失败 / 未知</th>
                  <th scope="col">创建时间</th>
                  <th scope="col">完成时间</th>
                </tr>
              </thead>
              <tbody>
                {query.data.items.map((operation) => (
                  <OperationRows
                    key={`${operation.id}:${operation.status}:${operation.counts.failed}:${operation.counts.unknown}`}
                    operation={operation}
                  />
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
