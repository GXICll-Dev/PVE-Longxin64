import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import {
  AlertCircle,
  CheckCircle2,
  ChevronRight,
  CircleAlert,
  ListChecks,
  MonitorCheck,
  MonitorUp,
  RefreshCw,
  School,
  UsersRound,
} from 'lucide-react';
import { classroomsQueryOptions, dashboardQueryOptions } from '../api/queries';
import type { ClassroomSummary, DashboardAlert } from '../api/types';
import { EmptyState, ErrorState, LoadingState, StaleDataNotice } from '../components/AsyncState';
import { LastUpdated } from '../components/LastUpdated';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/Button';
import { formatActiveSession, formatStatus, ratioLabel } from '../lib/format';

function classroomReadiness(classroom: ClassroomSummary): number {
  if (classroom.seats_total <= 0) return 0;
  return Math.round((classroom.seats_ready / classroom.seats_total) * 100);
}

function ClassroomListRow({ classroom }: { classroom: ClassroomSummary }) {
  const readiness = classroomReadiness(classroom);
  const isReady = classroom.seats_total > 0 && classroom.seats_ready === classroom.seats_total;
  const blockedSeats = Math.max(0, classroom.seats_total - classroom.seats_ready);
  const issueLabel = classroom.seats_total === 0
    ? '未配置座位'
    : blockedSeats > 0
      ? `${blockedSeats} 个问题`
      : '可开课';

  return (
    <li>
      <Link
        to="/classrooms/$classroomId"
        params={{ classroomId: classroom.id }}
        className={`classroom-grouped-row classroom-grouped-row--compact${isReady ? '' : ' classroom-grouped-row--attention'}`}
        aria-label={`${classroom.name}，${formatStatus(classroom.status)}，${classroom.seats_ready} / ${classroom.seats_total} 个座位可教学，${issueLabel}`}
      >
        <span className={`classroom-state-dot${isReady ? ' classroom-state-dot--ready' : ' classroom-state-dot--attention'}`} aria-hidden="true" />
        <span className="classroom-grouped-row__identity">
          <span className="classroom-grouped-row__title-line">
            <strong>{classroom.name}</strong>
            <span className="classroom-state-label">{formatStatus(classroom.status)}</span>
          </span>
          <span>{classroom.site} · {formatActiveSession(classroom.active_session)}</span>
        </span>

        <span className="classroom-grouped-row__readiness">
          <span>
            <span>课堂准备度</span>
            <strong>{readiness}%</strong>
          </span>
          <progress max={100} value={readiness} aria-label={`${classroom.name}课堂准备度 ${readiness}%`}>
            {readiness}%
          </progress>
        </span>

        <span className="classroom-grouped-row__signals" aria-label="终端和桌面状态">
          <span><MonitorCheck aria-hidden="true" size={15} /> {ratioLabel(classroom.thin_clients_online, classroom.seats_total)} 终端</span>
          <span><MonitorUp aria-hidden="true" size={15} /> {ratioLabel(classroom.desktops_running, classroom.seats_total)} 桌面</span>
        </span>

        <span className={`classroom-grouped-row__issue${isReady ? '' : ' classroom-grouped-row__issue--attention'}`}>
          {issueLabel}
        </span>
        <ChevronRight className="classroom-grouped-row__chevron" aria-hidden="true" size={17} />
      </Link>
    </li>
  );
}

function AlertItem({ alert }: { alert: DashboardAlert }) {
  const isCritical = alert.severity.toLowerCase() === 'critical';
  const Icon = isCritical ? CircleAlert : AlertCircle;
  const content = (
    <>
      <span className="dashboard-problem-row__icon" aria-hidden="true">
        <Icon size={18} />
      </span>
      <span className="dashboard-problem-row__copy">
        <strong>{alert.title}</strong>
        <span>{alert.description}</span>
      </span>
      {alert.resource_id ? <ChevronRight aria-hidden="true" size={17} /> : null}
    </>
  );

  return (
    <li>
      {alert.resource_id ? (
        <Link
          to="/classrooms/$classroomId"
          params={{ classroomId: alert.resource_id }}
          className={`dashboard-problem-row dashboard-problem-row--${alert.severity.toLowerCase()}`}
          aria-label={`查看${alert.title}`}
        >
          {content}
        </Link>
      ) : (
        <div className={`dashboard-problem-row dashboard-problem-row--${alert.severity.toLowerCase()}`}>
          {content}
        </div>
      )}
    </li>
  );
}

export function DashboardPage() {
  const dashboardQuery = useQuery(dashboardQueryOptions);
  const classroomsQuery = useQuery(classroomsQueryOptions);

  if (!dashboardQuery.data) {
    if (dashboardQuery.isPending) return <LoadingState label="正在加载运营总览" />;
    return <ErrorState error={dashboardQuery.error} onRetry={() => void dashboardQuery.refetch()} />;
  }

  const { summary, alerts, generated_at: generatedAt } = dashboardQuery.data;
  const readiness = summary.seats_total > 0
    ? Math.round((summary.seats_ready / summary.seats_total) * 100)
    : 0;
  const pendingSeats = Math.max(0, summary.seats_total - summary.seats_ready);
  const needsAttention = pendingSeats > 0 || summary.operations_failed > 0 || alerts.length > 0;
  const fallbackProblemCount = (pendingSeats > 0 ? 1 : 0) + (summary.operations_failed > 0 ? 1 : 0);
  const classroomItems = classroomsQuery.data?.items ?? [];
  const sortedClassrooms = [...classroomItems].sort((left, right) => {
    const leftPriority = left.status === 'ACTIVE' ? 0 : classroomReadiness(left) < 100 ? 1 : 2;
    const rightPriority = right.status === 'ACTIVE' ? 0 : classroomReadiness(right) < 100 ? 1 : 2;
    return leftPriority - rightPriority;
  });
  const isRefreshing = dashboardQuery.isFetching || classroomsQuery.isFetching;
  const statusTitle = pendingSeats > 0
    ? `${pendingSeats} 个座位需要课前处理`
    : summary.operations_failed > 0
      ? `${summary.operations_failed} 个后台任务需要确认`
      : alerts.length > 0
        ? `${alerts.length} 项课堂问题需要确认`
        : '当前课堂均可正常开始';
  const statusDescription = pendingSeats > 0
    ? `${summary.seats_ready} / ${summary.seats_total} 个座位已具备教学条件，先处理阻塞项再执行整班操作。`
    : summary.operations_failed > 0
      ? '座位准备度正常，但最近的批量任务存在失败结果，请先确认影响范围。'
      : alerts.length > 0
        ? '座位准备度正常，但控制平面仍有需要值班老师确认的课堂事项。'
        : `${summary.classrooms_ready} / ${summary.classrooms_total} 间教室已就绪，没有阻塞开课的问题。`;

  function refreshPage() {
    void dashboardQuery.refetch();
    void classroomsQuery.refetch();
  }

  return (
    <div className="page-stack dashboard-page">
      <PageHeader
        title="总览"
        description="查看今天的课堂、阻塞问题和关键运行状态。"
        actions={
          <Button onClick={refreshPage} disabled={isRefreshing}>
            <RefreshCw className={isRefreshing ? 'spinner' : undefined} aria-hidden="true" size={16} />
            {isRefreshing ? '刷新中' : '刷新'}
          </Button>
        }
      />

      {dashboardQuery.isError ? (
        <StaleDataNotice error={dashboardQuery.error} isRetrying={dashboardQuery.isFetching} onRetry={refreshPage} />
      ) : null}
      {classroomsQuery.isError && classroomsQuery.data ? (
        <StaleDataNotice error={classroomsQuery.error} isRetrying={classroomsQuery.isFetching} onRetry={refreshPage} />
      ) : null}

      <section
        className={`dashboard-status-summary grouped-surface${needsAttention ? ' dashboard-status-summary--attention' : ''}`}
        aria-labelledby="today-status-title"
      >
        <div className="dashboard-status-summary__message">
          <span className="dashboard-status-summary__icon" aria-hidden="true">
            {needsAttention ? <CircleAlert size={21} /> : <CheckCircle2 size={21} />}
          </span>
          <div>
            <p className="section-kicker">今日课堂状态</p>
            <h2 id="today-status-title">{statusTitle}</h2>
            <p>{statusDescription}</p>
          </div>
        </div>

        <div className="dashboard-status-summary__readiness">
          <div>
            <span>座位就绪</span>
            <strong>{summary.seats_ready} / {summary.seats_total}</strong>
          </div>
          <progress max={100} value={readiness} aria-label={`整体座位就绪率 ${readiness}%`}>
            {readiness}%
          </progress>
          <span>{readiness}%</span>
        </div>

        {pendingSeats === 0 && summary.operations_failed > 0 ? (
          <Link to="/operations" className="dashboard-status-summary__action">
            查看 {summary.operations_failed} 个失败任务
            <ChevronRight aria-hidden="true" size={17} />
          </Link>
        ) : (
          <Link to="/classrooms" className="dashboard-status-summary__action">
            {pendingSeats > 0 ? `查看 ${pendingSeats} 个问题` : '打开云教室'}
            <ChevronRight aria-hidden="true" size={17} />
          </Link>
        )}
      </section>

      <div className="dashboard-content-grid">
        <section className="dashboard-group grouped-surface" aria-labelledby="today-classrooms-title">
          <header className="dashboard-group__header">
            <div>
              <p className="section-kicker">课堂</p>
              <h2 id="today-classrooms-title">今日课堂</h2>
              <p>按正在上课和需要处理的顺序显示。</p>
            </div>
            <Link to="/classrooms" className="dashboard-group__action">
              查看全部
              <ChevronRight aria-hidden="true" size={16} />
            </Link>
          </header>

          {classroomsQuery.isPending ? (
            <LoadingState label="正在加载今日课堂" />
          ) : classroomsQuery.isError && !classroomsQuery.data ? (
            <ErrorState error={classroomsQuery.error} onRetry={() => void classroomsQuery.refetch()} />
          ) : sortedClassrooms.length === 0 ? (
            <EmptyState title="今天没有可显示的教室" description="配置云教室后，课堂状态会出现在这里。" />
          ) : (
            <ul className="classroom-grouped-list dashboard-classroom-list">
              {sortedClassrooms.slice(0, 4).map((classroom) => (
                <ClassroomListRow key={classroom.id} classroom={classroom} />
              ))}
            </ul>
          )}
        </section>

        <section className="dashboard-group grouped-surface" aria-labelledby="problems-title">
          <header className="dashboard-group__header">
            <div>
              <p className="section-kicker">异常</p>
              <h2 id="problems-title">需要处理</h2>
              <p>只显示会影响课堂的事项。</p>
            </div>
            <span className="dashboard-group__count">{alerts.length || fallbackProblemCount}</span>
          </header>

          {alerts.length > 0 ? (
            <ul className="dashboard-problem-list">
              {alerts.map((alert) => (
                <AlertItem key={alert.id} alert={alert} />
              ))}
            </ul>
          ) : pendingSeats > 0 || summary.operations_failed > 0 ? (
            <ul className="dashboard-problem-list">
              {pendingSeats > 0 ? (
                <li>
                  <Link to="/classrooms" className="dashboard-problem-row dashboard-problem-row--warning">
                    <span className="dashboard-problem-row__icon" aria-hidden="true"><CircleAlert size={18} /></span>
                    <span className="dashboard-problem-row__copy">
                      <strong>{pendingSeats} 个座位尚未就绪</strong>
                      <span>进入云教室查看终端、桌面和 Guest Agent 状态。</span>
                    </span>
                    <ChevronRight aria-hidden="true" size={17} />
                  </Link>
                </li>
              ) : null}
              {summary.operations_failed > 0 ? (
                <li>
                  <Link to="/operations" className="dashboard-problem-row dashboard-problem-row--critical">
                    <span className="dashboard-problem-row__icon" aria-hidden="true"><AlertCircle size={18} /></span>
                    <span className="dashboard-problem-row__copy">
                      <strong>{summary.operations_failed} 个任务执行失败</strong>
                      <span>打开任务中心定位失败座位并决定是否重试。</span>
                    </span>
                    <ChevronRight aria-hidden="true" size={17} />
                  </Link>
                </li>
              ) : null}
            </ul>
          ) : (
            <div className="dashboard-problem-empty">
              <CheckCircle2 aria-hidden="true" size={20} />
              <div>
                <strong>当前没有课堂阻塞</strong>
                <span>最新状态中没有需要值班老师处理的问题。</span>
              </div>
            </div>
          )}
        </section>
      </div>

      <section className="dashboard-metric-summary grouped-surface" aria-labelledby="runtime-summary-title">
        <header className="dashboard-metric-summary__header">
          <div>
            <p className="section-kicker">运行状态</p>
            <h2 id="runtime-summary-title">关键指标</h2>
          </div>
          <LastUpdated value={generatedAt} isFetching={isRefreshing} />
        </header>
        <dl className="dashboard-metric-summary__grid">
          <div>
            <dt><School aria-hidden="true" size={16} /> 正在上课</dt>
            <dd>{summary.classrooms_active} <span>/ {summary.classrooms_total} 间</span></dd>
          </div>
          <div>
            <dt><MonitorCheck aria-hidden="true" size={16} /> 在线终端</dt>
            <dd>{summary.thin_clients_online} <span>/ {summary.seats_total} 台</span></dd>
          </div>
          <div>
            <dt><MonitorUp aria-hidden="true" size={16} /> 运行桌面</dt>
            <dd>{summary.desktops_running} <span>/ {summary.seats_total} 台</span></dd>
          </div>
          <div className={summary.operations_failed > 0 ? 'dashboard-metric-summary__attention' : undefined}>
            <dt><ListChecks aria-hidden="true" size={16} /> 后台任务</dt>
            <dd>{summary.operations_running} <span>执行中 · {summary.operations_failed} 失败</span></dd>
          </div>
          <div>
            <dt><UsersRound aria-hidden="true" size={16} /> 可教学座位</dt>
            <dd>{summary.seats_ready} <span>/ {summary.seats_total} 个</span></dd>
          </div>
        </dl>
      </section>
    </div>
  );
}
