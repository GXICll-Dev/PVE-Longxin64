import { Link, Outlet, useRouterState } from '@tanstack/react-router';
import {
  BookOpenCheck,
  Building2,
  CloudCog,
  LayoutDashboard,
  ListChecks,
  ShieldCheck,
} from 'lucide-react';

const navigation = [
  { to: '/', label: '总览', icon: LayoutDashboard, exact: true },
  { to: '/classrooms', label: '云教室', icon: Building2, exact: false },
  { to: '/operations', label: '任务中心', icon: ListChecks, exact: false },
] as const;

function getCurrentSection(pathname: string): string {
  if (pathname.startsWith('/classrooms/')) return '课堂控制台';
  if (pathname.startsWith('/classrooms')) return '云教室';
  if (pathname.startsWith('/operations')) return '任务中心';
  return '运营总览';
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
            <small>教学控制平面</small>
          </span>
        </div>

        <nav className="sidebar__nav" aria-label="主导航">
          <p className="nav-group-label">课堂工作台</p>
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
        </nav>

        <div className="sidebar__footer">
          <ShieldCheck aria-hidden="true" size={17} />
          <span>
            <strong>受控运维</strong>
            <small>批量操作全程留痕</small>
          </span>
        </div>
      </aside>

      <div className="workspace">
        <header className="topbar">
          <div className="topbar__context" aria-label="当前位置">
            <BookOpenCheck aria-hidden="true" size={17} />
            <span>教学运营</span>
            <span className="topbar__divider" aria-hidden="true" />
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
