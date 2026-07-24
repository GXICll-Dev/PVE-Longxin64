import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { ArrowRight, RefreshCw } from 'lucide-react';
import { classroomsQueryOptions } from '../api/queries';
import { EmptyState, ErrorState, LoadingState, StaleDataNotice } from '../components/AsyncState';
import { LastUpdated } from '../components/LastUpdated';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/Button';
import { StatusBadge } from '../components/ui/StatusBadge';
import { formatActiveSession, formatDateTime, ratioLabel } from '../lib/format';

export function ClassroomsPage() {
  const query = useQuery(classroomsQueryOptions);

  if (!query.data) {
    if (query.isPending) return <LoadingState label="正在加载云教室" />;
    return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;
  }

  const refreshedAt = query.data.generated_at ?? new Date(query.dataUpdatedAt).toISOString();

  return (
    <div className="page-stack">
      <PageHeader
        title="云教室"
        description="从课堂视角查看座位、瘦客户机、虚拟桌面和课程状态，不需要进入 PVE 基础设施页面。"
        actions={
          <Button onClick={() => void query.refetch()} disabled={query.isFetching}>
            <RefreshCw aria-hidden="true" size={16} />
            刷新列表
          </Button>
        }
      />
      {query.isError ? (
        <StaleDataNotice error={query.error} isRetrying={query.isFetching} onRetry={() => void query.refetch()} />
      ) : null}
      <div className="page-meta-row">
        <span className="result-count">共 {query.data.total} 间教室</span>
        <LastUpdated value={refreshedAt} isFetching={query.isFetching} label="刷新时间" />
      </div>

      {query.data.items.length === 0 ? (
        <EmptyState title="尚未配置云教室" description="完成 PVE 集群接入后，可由学校运维创建教室与座位。" />
      ) : (
        <div className="table-shell">
          <table className="data-table">
            <caption className="sr-only">云教室状态列表</caption>
            <thead>
              <tr>
                <th scope="col">教室</th>
                <th scope="col">状态</th>
                <th scope="col">座位就绪</th>
                <th scope="col">终端在线</th>
                <th scope="col">桌面运行</th>
                <th scope="col">模板版本</th>
                <th scope="col">当前课程</th>
                <th scope="col">更新时间</th>
                <th scope="col"><span className="sr-only">操作</span></th>
              </tr>
            </thead>
            <tbody>
              {query.data.items.map((classroom) => (
                <tr key={classroom.id}>
                  <td>
                    <div className="primary-cell">
                      <strong>{classroom.name}</strong>
                      <span>{classroom.site}</span>
                    </div>
                  </td>
                  <td><StatusBadge status={classroom.status} /></td>
                  <td className="numeric-cell">{ratioLabel(classroom.seats_ready, classroom.seats_total)}</td>
                  <td className="numeric-cell">{ratioLabel(classroom.thin_clients_online, classroom.seats_total)}</td>
                  <td className="numeric-cell">{ratioLabel(classroom.desktops_running, classroom.seats_total)}</td>
                  <td>
                    <div className="primary-cell primary-cell--compact">
                      <strong>{classroom.template_version}</strong>
                      <span>{classroom.template_name}</span>
                    </div>
                  </td>
                  <td>{formatActiveSession(classroom.active_session)}</td>
                  <td className="muted-cell">{formatDateTime(classroom.updated_at, classroom.timezone)}</td>
                  <td>
                    <Link
                      to="/classrooms/$classroomId"
                      params={{ classroomId: classroom.id }}
                      className="row-action"
                      aria-label={`打开${classroom.name}课堂控制台`}
                    >
                      进入
                      <ArrowRight aria-hidden="true" size={14} />
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
