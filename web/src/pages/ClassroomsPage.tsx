import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import {
  ChevronRight,
  CircleAlert,
  Clock3,
  MonitorCheck,
  MonitorUp,
  RefreshCw,
  UsersRound,
} from 'lucide-react';
import { useState } from 'react';
import { classroomsQueryOptions } from '../api/queries';
import type { ClassroomSummary } from '../api/types';
import { EmptyState, ErrorState, LoadingState, StaleDataNotice } from '../components/AsyncState';
import { LastUpdated } from '../components/LastUpdated';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/Button';
import { formatActiveSession, formatDateTime, formatStatus, ratioLabel } from '../lib/format';

type ClassroomFilter = 'all' | 'ready' | 'attention';

function isClassroomReady(classroom: ClassroomSummary): boolean {
  return classroom.seats_total > 0 && classroom.seats_ready === classroom.seats_total;
}

function classroomReadiness(classroom: ClassroomSummary): number {
  if (classroom.seats_total <= 0) return 0;
  return Math.round((classroom.seats_ready / classroom.seats_total) * 100);
}

function ClassroomRow({ classroom }: { classroom: ClassroomSummary }) {
  const readiness = classroomReadiness(classroom);
  const isReady = isClassroomReady(classroom);
  const blockedSeats = Math.max(0, classroom.seats_total - classroom.seats_ready);
  const stateTone = !isReady ? 'attention' : classroom.status === 'ACTIVE' ? 'active' : 'ready';
  const issueLabel = classroom.seats_total === 0
    ? '未配置座位'
    : blockedSeats > 0
      ? `${blockedSeats} 个问题`
      : classroom.status === 'ACTIVE'
        ? '课堂进行中'
        : '可开课';

  return (
    <li>
      <Link
        to="/classrooms/$classroomId"
        params={{ classroomId: classroom.id }}
        className={`classroom-grouped-row${isReady ? '' : ' classroom-grouped-row--attention'}`}
        aria-label={`${classroom.name}，${formatStatus(classroom.status)}，课堂准备度 ${readiness}%，${classroom.seats_ready} / ${classroom.seats_total} 个座位可教学，${issueLabel}`}
      >
        <span className={`classroom-state-dot classroom-state-dot--${stateTone}`} aria-hidden="true" />

        <span className="classroom-grouped-row__identity">
          <span className="classroom-grouped-row__title-line">
            <strong>{classroom.name}</strong>
            <span className="classroom-state-label">{formatStatus(classroom.status)}</span>
          </span>
          <span>{classroom.site} · {formatActiveSession(classroom.active_session)}</span>
          <span className="classroom-grouped-row__updated">
            <Clock3 aria-hidden="true" size={13} />
            <time dateTime={classroom.updated_at}>{formatDateTime(classroom.updated_at, classroom.timezone)}</time>
          </span>
        </span>

        <span className="classroom-grouped-row__readiness">
          <span>
            <span>课堂准备度</span>
            <strong>{readiness}%</strong>
          </span>
          <progress max={100} value={readiness} aria-label={`${classroom.name}课堂准备度 ${readiness}%`}>
            {readiness}%
          </progress>
          <small>{blockedSeats > 0 ? `${blockedSeats} 个座位阻塞开课` : `${classroom.seats_total} 个座位均可教学`}</small>
        </span>

        <dl className="classroom-grouped-row__signals">
          <div>
            <dt><UsersRound aria-hidden="true" size={14} /> 座位</dt>
            <dd>{ratioLabel(classroom.seats_ready, classroom.seats_total)}</dd>
          </div>
          <div>
            <dt><MonitorCheck aria-hidden="true" size={14} /> 终端</dt>
            <dd>{ratioLabel(classroom.thin_clients_online, classroom.seats_total)}</dd>
          </div>
          <div>
            <dt><MonitorUp aria-hidden="true" size={14} /> 桌面</dt>
            <dd>{ratioLabel(classroom.desktops_running, classroom.seats_total)}</dd>
          </div>
        </dl>

        <span className="classroom-grouped-row__template">
          <span>教学模板</span>
          <strong>{classroom.template_version}</strong>
          <small>{classroom.template_name}</small>
        </span>

        <span className={`classroom-grouped-row__issue${isReady ? '' : ' classroom-grouped-row__issue--attention'}`}>
          {issueLabel}
        </span>
        <ChevronRight className="classroom-grouped-row__chevron" aria-hidden="true" size={18} />
      </Link>
    </li>
  );
}

export function ClassroomsPage() {
  const query = useQuery(classroomsQueryOptions);
  const [filter, setFilter] = useState<ClassroomFilter>('all');

  if (!query.data) {
    if (query.isPending) return <LoadingState label="正在加载云教室" />;
    return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;
  }

  const refreshedAt = query.data.generated_at ?? new Date(query.dataUpdatedAt).toISOString();
  const readyClassrooms = query.data.items.filter(isClassroomReady).length;
  const attentionClassrooms = Math.max(0, query.data.total - readyClassrooms);
  const filteredClassrooms = query.data.items.filter((classroom) => {
    const isReady = isClassroomReady(classroom);
    if (filter === 'ready') return isReady;
    if (filter === 'attention') return !isReady;
    return true;
  });

  return (
    <div className="page-stack classrooms-page">
      <PageHeader
        title="云教室"
        description="查看课堂准备度，进入教室处理座位或执行整班操作。"
        actions={
          <Button onClick={() => void query.refetch()} disabled={query.isFetching}>
            <RefreshCw className={query.isFetching ? 'spinner' : undefined} aria-hidden="true" size={16} />
            {query.isFetching ? '刷新中' : '刷新'}
          </Button>
        }
      />

      {query.isError ? (
        <StaleDataNotice error={query.error} isRetrying={query.isFetching} onRetry={() => void query.refetch()} />
      ) : null}

      {query.data.items.length === 0 ? (
        <EmptyState title="尚未配置云教室" description="完成 PVE 集群接入后，可由学校运维创建教室与座位。" />
      ) : (
        <section className="classroom-browser grouped-surface" aria-labelledby="classroom-browser-title">
          <header className="classroom-browser__toolbar">
            <div className="classroom-browser__heading">
              <div>
                <h2 id="classroom-browser-title">教室列表</h2>
                <span aria-live="polite">
                  当前显示 {filteredClassrooms.length} / {query.data.total} 间
                </span>
              </div>
              <div className="classroom-browser__summary" aria-label="教室摘要">
                <span><strong>{readyClassrooms}</strong> 可开课</span>
                <span className={attentionClassrooms > 0 ? 'classroom-browser__attention' : undefined}>
                  {attentionClassrooms > 0 ? <CircleAlert aria-hidden="true" size={14} /> : null}
                  <strong>{attentionClassrooms}</strong> 需处理
                </span>
              </div>
            </div>

            <div className="classroom-browser__controls">
              <div className="segmented-control" role="group" aria-label="筛选云教室">
                <button
                  type="button"
                  aria-pressed={filter === 'all'}
                  data-active={filter === 'all' || undefined}
                  onClick={() => setFilter('all')}
                >
                  全部 <span>{query.data.total}</span>
                </button>
                <button
                  type="button"
                  aria-pressed={filter === 'ready'}
                  data-active={filter === 'ready' || undefined}
                  onClick={() => setFilter('ready')}
                >
                  可开课 <span>{readyClassrooms}</span>
                </button>
                <button
                  type="button"
                  aria-pressed={filter === 'attention'}
                  data-active={filter === 'attention' || undefined}
                  onClick={() => setFilter('attention')}
                >
                  需处理 <span>{attentionClassrooms}</span>
                </button>
              </div>
              <LastUpdated value={refreshedAt} isFetching={query.isFetching} label="刷新时间" />
            </div>
          </header>

          <div className="classroom-browser__columns" aria-hidden="true">
            <span />
            <span>教室</span>
            <span>准备度</span>
            <span>课堂状态</span>
            <span>教学模板</span>
            <span>处理</span>
            <span />
          </div>

          {filteredClassrooms.length === 0 ? (
            <div className="classroom-browser__empty">
              <EmptyState title="这个筛选下没有教室" description="切换到其他状态即可继续查看。" />
            </div>
          ) : (
            <ul className="classroom-grouped-list">
              {filteredClassrooms.map((classroom) => (
                <ClassroomRow key={classroom.id} classroom={classroom} />
              ))}
            </ul>
          )}
        </section>
      )}
    </div>
  );
}
