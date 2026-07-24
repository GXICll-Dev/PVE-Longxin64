import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import {
  AlertCircle,
  ArrowRight,
  CheckCircle2,
  CircleAlert,
  Gauge,
  ListChecks,
  MonitorCheck,
  MonitorUp,
  RefreshCw,
  School,
  UsersRound,
} from 'lucide-react';
import type { CSSProperties, ReactNode } from 'react';
import { dashboardQueryOptions } from '../api/queries';
import type { DashboardAlert } from '../api/types';
import { EmptyState, ErrorState, LoadingState, StaleDataNotice } from '../components/AsyncState';
import { LastUpdated } from '../components/LastUpdated';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/Button';

interface MetricProps {
  label: string;
  value: ReactNode;
  detail: string;
  icon: ReactNode;
  tone?: 'default' | 'warning';
}

function Metric({ label, value, detail, icon, tone = 'default' }: MetricProps) {
  return (
    <article className={`metric-strip__item${tone === 'warning' ? ' metric-strip__item--warning' : ''}`}>
      <div className="metric-strip__icon" aria-hidden="true">
        {icon}
      </div>
      <div className="metric-strip__copy">
        <p>{label}</p>
        <strong>{value}</strong>
        <span>{detail}</span>
      </div>
    </article>
  );
}

function AlertItem({ alert }: { alert: DashboardAlert }) {
  const isCritical = alert.severity.toLowerCase() === 'critical';
  const Icon = isCritical ? CircleAlert : AlertCircle;
  return (
    <li className={`alert-item alert-item--${alert.severity.toLowerCase()}`}>
      <Icon aria-hidden="true" size={19} />
      <div>
        <strong>{alert.title}</strong>
        <p>{alert.description}</p>
      </div>
      {alert.resource_id ? (
        <Link to="/classrooms/$classroomId" params={{ classroomId: alert.resource_id }} aria-label={`查看${alert.title}`}>
          查看
          <ArrowRight aria-hidden="true" size={14} />
        </Link>
      ) : null}
    </li>
  );
}

export function DashboardPage() {
  const query = useQuery(dashboardQueryOptions);

  if (!query.data) {
    if (query.isPending) return <LoadingState label="正在加载运营总览" />;
    return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;
  }

  const { summary, alerts, generated_at: generatedAt } = query.data;
  const readiness = summary.seats_total > 0 ? Math.round((summary.seats_ready / summary.seats_total) * 100) : 0;
  const pendingSeats = Math.max(0, summary.seats_total - summary.seats_ready);
  const needsAttention = pendingSeats > 0 || summary.operations_failed > 0 || alerts.length > 0;
  const readinessStyle = { '--readiness-angle': `${readiness * 3.6}deg` } as CSSProperties;

  return (
    <div className="page-stack">
      <PageHeader
        title="运营总览"
        description="把基础设施状态翻译成课堂结论，让值班老师先看到能否开课，再决定要处理什么。"
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

      <div className="dashboard-primary-grid">
        <section className={`readiness-hero${needsAttention ? ' readiness-hero--attention' : ''}`} aria-labelledby="readiness-title">
          <div className="readiness-hero__copy">
            <p className="section-kicker">
              <Gauge aria-hidden="true" size={15} />
              今日课堂准备
            </p>
            <h2 id="readiness-title">
              {pendingSeats > 0 ? `${pendingSeats} 个座位需要课前处理` : '当前教学环境可以稳定开课'}
            </h2>
            <p>
              {pendingSeats > 0
                ? `${summary.seats_ready} 个座位已经具备教学条件。优先处理离线终端与未就绪桌面，再开始整班操作。`
                : `全部 ${summary.seats_total} 个座位已具备教学条件，当前没有阻塞开课的问题。`}
            </p>
            <div className="readiness-hero__actions">
              <Link to="/classrooms" className="hero-action-link">
                {pendingSeats > 0 ? '定位异常教室' : '进入云教室'}
                <ArrowRight aria-hidden="true" size={16} />
              </Link>
              <span>
                <CheckCircle2 aria-hidden="true" size={15} />
                {summary.classrooms_ready} / {summary.classrooms_total} 间教室已就绪
              </span>
            </div>
          </div>
          <div className="readiness-ring" style={readinessStyle} aria-label={`整体座位就绪率 ${readiness}%`}>
            <div>
              <strong>{readiness}%</strong>
              <span>座位就绪</span>
            </div>
          </div>
        </section>

        <section className="attention-panel" aria-labelledby="alerts-title">
          <div className="panel__header">
            <div>
              <p className="section-kicker">需要关注</p>
              <h2 id="alerts-title">课前阻塞</h2>
            </div>
            <span className="panel-count">{alerts.length}</span>
          </div>
          {alerts.length > 0 ? (
            <ul className="alert-list">
              {alerts.map((alert) => (
                <AlertItem key={alert.id} alert={alert} />
              ))}
            </ul>
          ) : (
            <EmptyState title="当前没有课前阻塞" description="控制平面返回的最新快照中没有需要处理的问题。" />
          )}
        </section>
      </div>

      <section className="metric-strip" aria-label="课堂运行证据">
        <Metric
          label="云教室"
          value={summary.classrooms_total}
          detail={`${summary.classrooms_active} 间正在上课`}
          icon={<School size={20} />}
        />
        <Metric
          label="在线瘦客户机"
          value={`${summary.thin_clients_online} / ${summary.seats_total}`}
          detail="已上报有效心跳"
          icon={<MonitorCheck size={20} />}
        />
        <Metric
          label="运行中桌面"
          value={`${summary.desktops_running} / ${summary.seats_total}`}
          detail="最近一次对账结果"
          icon={<MonitorUp size={20} />}
        />
        <Metric
          label="后台任务"
          value={summary.operations_running}
          detail={summary.operations_failed > 0 ? `${summary.operations_failed} 个失败任务` : '没有失败任务'}
          icon={summary.operations_failed > 0 ? <CircleAlert size={20} /> : <ListChecks size={20} />}
          tone={summary.operations_failed > 0 ? 'warning' : 'default'}
        />
      </section>

      <div className="dashboard-footer-meta">
        <LastUpdated value={generatedAt} isFetching={query.isFetching} />
        <span><UsersRound aria-hidden="true" size={14} /> {summary.seats_ready} 个座位可教学</span>
      </div>
    </div>
  );
}
