import { useQuery } from '@tanstack/react-query';
import { RefreshCw } from 'lucide-react';
import { operationsQueryOptions } from '../api/queries';
import { EmptyState, ErrorState, LoadingState } from '../components/AsyncState';
import { LastUpdated } from '../components/LastUpdated';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/Button';
import { ProgressBar } from '../components/ui/ProgressBar';
import { StatusBadge } from '../components/ui/StatusBadge';
import { formatCompactId, formatDateTime, formatOperationType, getOperationProgress } from '../lib/format';

export function OperationsPage() {
  const query = useQuery(operationsQueryOptions);

  if (query.isPending) return <LoadingState label="正在加载任务中心" />;
  if (query.isError) return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;

  const refreshedAt = query.data.generated_at ?? new Date(query.dataUpdatedAt).toISOString();
  const runningCount = query.data.items.filter((operation) =>
    ['QUEUED', 'VALIDATING', 'RUNNING', 'WAITING_PVE', 'VERIFYING', 'CANCEL_REQUESTED'].includes(operation.status.toUpperCase()),
  ).length;
  const attentionCount = query.data.items.filter((operation) =>
    ['FAILED', 'PARTIALLY_SUCCEEDED'].includes(operation.status.toUpperCase()),
  ).length;

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
      <div className="page-meta-row">
        <div className="task-summary" aria-label="任务摘要">
          <span>共 {query.data.total} 个任务</span>
          <span>{runningCount} 个执行中</span>
          <span>{attentionCount} 个需要关注</span>
        </div>
        <LastUpdated value={refreshedAt} isFetching={query.isFetching} label="刷新时间" />
      </div>

      {query.data.items.length === 0 ? (
        <EmptyState title="暂无批量任务" description="在课堂控制台提交课前检查、开机、关机或还原后，任务会出现在这里。" />
      ) : (
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
              {query.data.items.map((operation) => {
                const progress = getOperationProgress(operation.counts);
                return (
                  <tr key={operation.id}>
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
                        <small>{operation.counts.succeeded} 成功 · {operation.counts.running} 执行中 · {operation.counts.queued} 排队</small>
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
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
