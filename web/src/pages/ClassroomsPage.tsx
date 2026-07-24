import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { ArrowRight, CircleAlert, Clock3, MonitorCheck, MonitorUp, RefreshCw, UsersRound } from 'lucide-react';
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
  const readyClassrooms = query.data.items.filter((classroom) => ['READY', 'ACTIVE'].includes(classroom.status)).length;
  const attentionClassrooms = Math.max(0, query.data.total - readyClassrooms);

  return (
    <div className="page-stack">
      <PageHeader
        title="云教室"
        description="按开课条件组织教室状态，先处理阻塞座位，再进入课堂执行整班操作。"
        actions={
          <Button onClick={() => void query.refetch()} disabled={query.isFetching}>
            <RefreshCw className={query.isFetching ? 'spinner' : undefined} aria-hidden="true" size={16} />
            {query.isFetching ? '刷新中' : '刷新列表'}
          </Button>
        }
      />
      {query.isError ? (
        <StaleDataNotice error={query.error} isRetrying={query.isFetching} onRetry={() => void query.refetch()} />
      ) : null}
      <div className="page-meta-row">
        <div className="classroom-summary-line" aria-label="教室摘要">
          <span><strong>{query.data.total}</strong> 间教室</span>
          <span><strong>{readyClassrooms}</strong> 间可开课</span>
          {attentionClassrooms > 0 ? <span className="classroom-summary-line__attention"><CircleAlert size={14} /> {attentionClassrooms} 间需处理</span> : null}
        </div>
        <LastUpdated value={refreshedAt} isFetching={query.isFetching} label="刷新时间" />
      </div>

      {query.data.items.length === 0 ? (
        <EmptyState title="尚未配置云教室" description="完成 PVE 集群接入后，可由学校运维创建教室与座位。" />
      ) : (
        <section className="classroom-list" aria-label="云教室状态列表">
          {query.data.items.map((classroom) => {
            const readiness = classroom.seats_total > 0
              ? Math.round((classroom.seats_ready / classroom.seats_total) * 100)
              : 0;
            const blockedSeats = Math.max(0, classroom.seats_total - classroom.seats_ready);
            const actionLabel = blockedSeats > 0 ? `处理 ${blockedSeats} 个异常` : classroom.status === 'ACTIVE' ? '进入课堂' : '课前准备';

            return (
              <article className={`classroom-row${blockedSeats > 0 ? ' classroom-row--attention' : ''}`} key={classroom.id}>
                <div className="classroom-row__identity">
                  <div className="classroom-row__title-line">
                    <h2>{classroom.name}</h2>
                    <StatusBadge status={classroom.status} />
                  </div>
                  <p>{classroom.site} · {formatActiveSession(classroom.active_session)}</p>
                  <span><Clock3 aria-hidden="true" size={13} /> {formatDateTime(classroom.updated_at, classroom.timezone)}</span>
                </div>

                <div className="classroom-row__readiness">
                  <div>
                    <span>课堂准备度</span>
                    <strong>{readiness}%</strong>
                  </div>
                  <div className="classroom-readiness-track" aria-label={`${classroom.name}课堂准备度 ${readiness}%`}>
                    <span style={{ transform: `scaleX(${readiness / 100})` }} />
                  </div>
                  <small>{blockedSeats > 0 ? `${blockedSeats} 个座位阻塞开课` : `${classroom.seats_total} 个座位均可教学`}</small>
                </div>

                <dl className="classroom-row__signals">
                  <div>
                    <dt><UsersRound aria-hidden="true" size={15} /> 座位</dt>
                    <dd>{ratioLabel(classroom.seats_ready, classroom.seats_total)}</dd>
                  </div>
                  <div>
                    <dt><MonitorCheck aria-hidden="true" size={15} /> 终端</dt>
                    <dd>{ratioLabel(classroom.thin_clients_online, classroom.seats_total)}</dd>
                  </div>
                  <div>
                    <dt><MonitorUp aria-hidden="true" size={15} /> 桌面</dt>
                    <dd>{ratioLabel(classroom.desktops_running, classroom.seats_total)}</dd>
                  </div>
                </dl>

                <div className="classroom-row__template">
                  <span>教学模板</span>
                  <strong>{classroom.template_version}</strong>
                  <small>{classroom.template_name}</small>
                </div>

                <Link
                  to="/classrooms/$classroomId"
                  params={{ classroomId: classroom.id }}
                  className="row-action row-action--button"
                  aria-label={`${actionLabel}：${classroom.name}`}
                >
                  {actionLabel}
                  <ArrowRight aria-hidden="true" size={15} />
                </Link>
              </article>
            );
          })}
        </section>
      )}
    </div>
  );
}
