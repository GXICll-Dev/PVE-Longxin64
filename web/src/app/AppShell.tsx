import { Link, Outlet, useRouterState } from '@tanstack/react-router';
import {
  BookOpenCheck,
  Building2,
  ChevronRight,
  CloudCog,
  LayoutDashboard,
  ListChecks,
  MonitorSmartphone,
  PanelsTopLeft,
  Settings2,
  ShieldCheck,
} from 'lucide-react';

const navigation = [
  { to: '/', label: '总览', icon: LayoutDashboard, exact: true },
  { to: '/classrooms', label: '云教室', icon: Building2, exact: false },
  { to: '/operations', label: '任务中心', icon: ListChecks, exact: false },
] as const;

const plannedNavigation = [
  { label: '模板与镜像', icon: PanelsTopLeft },
  { label: '终端设备', icon: MonitorSmartphone },
  { label: '自动化与策略', icon: Settings2 },
] as const;

function getCurrentSection(pathname: string): string {
  if (pathname.startsWith('/classrooms/')) return '云教室 / 教室详情';
  if (pathname.startsWith('/classrooms')) return '云教室';
  if (pathname.startsWith('/operations')) return '任务中心';
  return '总览';
}

export function AppShell() {
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const currentSection = getCurrentSection(pathname);

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳到主要内容
      </a>
      <aside className="sidebar">
        <div className="brand">
          <span className="brand__mark" aria-hidden="true">
            <CloudCog size={22} />
          </span>
          <span>
            <strong>PVE 云教室</strong>
            <small>Longxin64 Console</small>
          </span>
        </div>

        <nav className="sidebar__nav" aria-label="主导航">
          <p className="nav-group-label">教学运营</p>
          {navigation.map(({ to, label, icon: Icon, exact }) => (
            <Link
              key={to}
              to={to}
              className="nav-link"
              activeProps={{ className: 'nav-link nav-link--active' }}
              activeOptions={{ exact }}
            >
              <Icon aria-hidden="true" size={18} />
              <span>{label}</span>
            </Link>
          ))}

          <p className="nav-group-label nav-group-label--spaced">平台管理</p>
          {plannedNavigation.map(({ label, icon: Icon }) => (
            <span className="nav-link nav-link--disabled" aria-disabled="true" key={label}>
              <Icon aria-hidden="true" size={18} />
              <span>{label}</span>
              <small>规划中</small>
            </span>
          ))}
        </nav>

        <div className="sidebar__footer">
          <ShieldCheck aria-hidden="true" size={17} />
          <span>
            <strong>安全边界</strong>
            <small>浏览器不接触 PVE Token</small>
          </span>
        </div>
      </aside>

      <div className="workspace">
        <header className="topbar">
          <div className="topbar__context" aria-label="当前位置">
            <BookOpenCheck aria-hidden="true" size={17} />
            <span>控制平面</span>
            <ChevronRight aria-hidden="true" size={14} />
            <strong>{currentSection}</strong>
          </div>
          <Link to="/operations" className="topbar__task-link">
            <ListChecks aria-hidden="true" size={16} />
            查看任务
          </Link>
        </header>

        <main id="main-content" className="main-content" tabIndex={-1}>
          <Outlet />
        </main>
      </div>
    </div>
  );
}
