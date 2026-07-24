import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import {
  AlertCircle,
  ArrowRight,
  Building2,
  CheckCircle2,
  CircleAlert,
  ListChecks,
  MonitorCheck,
  MonitorUp,
  RefreshCw,
  UsersRound,
} from 'lucide-react';
import type { ReactNode } from 'react';
import { dashboardQueryOptions } from '../api/queries';
import type { DashboardAlert } from '../api/types';
import { EmptyState, ErrorState, LoadingState, StaleDataNotice } from '../components/AsyncState';
import { LastUpdated } from '../components/LastUpdated';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/Button';
import { ProgressBar } from '../components/ui/ProgressBar';

interface StatCardProps {
  label: string;
  value: ReactNode;
  detail: string;
  icon: ReactNode;
  tone?: 'default' | 'warning';
}

function StatCard({ label, value, detail, icon, tone = 'default' }: StatCardProps) {
  return (
    <article className={`stat-card${tone === 'warning' ? ' stat-card--warning' : ''}`}>
      <div className="stat-card__icon" aria-hidden="true">
        {icon}
      </div>
      <div>
        <p className="stat-card__label">{label}</p>
        <strong className="stat-card__value">{value}</strong>
        <p className="stat-card__detail">{detail}</p>
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

  return (
    <div className="page-stack">
      <PageHeader
        title="运营总览"
        description="聚焦今天能否稳定开课：教室就绪、终端在线、桌面运行和后台任务一目了然。"
        actions={
          <Button onClick={() => void query.refetch()} disabled={query.isFetching}>
            <RefreshCw aria-hidden="true" size={16} />
            刷新状态
          </Button>
        }
      />

      {query.isError ? (
        <StaleDataNotice error={query.error} isRetrying={query.isFetching} onRetry={() => void query.refetch()} />
      ) : null}

      <LastUpdated value={generatedAt} isFetching={query.isFetching} />

      <section className="stat-grid" aria-label="关键运营指标">
        <StatCard
          label="云教室"
          value={summary.classrooms_total}
          detail={`${summary.classrooms_ready} 间就绪 · ${summary.classrooms_active} 间上课中`}
          icon={<Building2 size={20} />}
        />
        <StatCard
          label="就绪座位"
          value={`${readiness}%`}
          detail={`${summary.seats_ready} / ${summary.seats_total} 个座位可教学`}
          icon={<UsersRound size={20} />}
        />
        <StatCard
          label="在线终端"
          value={summary.thin_clients_online}
          detail="已上报有效心跳的瘦客户机"
          icon={<MonitorCheck size={20} />}
        />
        <StatCard
          label="运行桌面"
          value={summary.desktops_running}
          detail="最近一次对账观察为运行中"
          icon={<MonitorUp size={20} />}
        />
        <StatCard
          label="执行中任务"
          value={summary.operations_running}
          detail="包含校验、执行、等待与验证阶段"
          icon={<ListChecks size={20} />}
        />
        <StatCard
          label="失败任务"
          value={summary.operations_failed}
          detail="需要查看单机结果并按原因处理"
          icon={<CircleAlert size={20} />}
          tone={summary.operations_failed > 0 ? 'warning' : 'default'}
        />
      </section>

      <div className="dashboard-grid">
        <section className="panel" aria-labelledby="readiness-title">
          <div className="panel__header">
            <div>
              <p className="panel__eyebrow">课堂准备</p>
              <h2 id="readiness-title">整体座位就绪率</h2>
            </div>
            <strong className="readiness-value">{readiness}%</strong>
          </div>
          <ProgressBar value={readiness} label={`整体座位就绪率 ${readiness}%`} />
          <div className="readiness-breakdown">
            <div>
              <CheckCircle2 aria-hidden="true" size={18} />
              <span>已就绪</span>
              <strong>{summary.seats_ready}</strong>
            </div>
            <div>
              <AlertCircle aria-hidden="true" size={18} />
              <span>待处理</span>
              <strong>{Math.max(0, summary.seats_total - summary.seats_ready)}</strong>
            </div>
          </div>
          <Link to="/classrooms" className="panel-link">
            查看所有教室
            <ArrowRight aria-hidden="true" size={15} />
          </Link>
        </section>

        <section className="panel" aria-labelledby="alerts-title">
          <div className="panel__header">
            <div>
              <p className="panel__eyebrow">需要关注</p>
              <h2 id="alerts-title">平台告警</h2>
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
            <EmptyState title="当前没有平台告警" description="控制平面返回的最新快照中没有需要处理的告警。" />
          )}
        </section>
      </div>
    </div>
  );
}
