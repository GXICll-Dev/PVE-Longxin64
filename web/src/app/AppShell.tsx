import { Link, Outlet, useRouterState } from '@tanstack/react-router';
import { Building2, CloudCog, LayoutDashboard, ListChecks } from 'lucide-react';

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

      <aside className="app-navigation sidebar" aria-label="应用导航">
        <Link to="/" className="app-navigation__brand brand" aria-label="返回运营总览">
          <span className="app-navigation__brand-mark brand__mark" aria-hidden="true">
            <CloudCog size={22} />
          </span>
          <span className="app-navigation__brand-copy">
            <strong>PVE 云教室</strong>
          </span>
        </Link>

        <nav className="app-navigation__items sidebar__nav" aria-label="主导航">
          {navigation.map(({ to, label, icon: Icon, exact }) => (
            <Link
              key={to}
              to={to}
              className="app-navigation__link nav-link"
              activeProps={{
                className: 'app-navigation__link--active nav-link--active',
                'aria-current': 'page',
              }}
              activeOptions={{ exact }}
            >
              <span className="app-navigation__icon" aria-hidden="true">
                <Icon size={20} />
              </span>
              <span className="app-navigation__label">{label}</span>
            </Link>
          ))}
        </nav>
      </aside>

      <div className="app-workspace workspace">
        <header className="app-topbar topbar">
          <div className="app-topbar__title topbar__context" aria-label="当前位置">
            <strong>{currentSection}</strong>
          </div>
          <Link
            to="/operations"
            className="app-topbar__task-link topbar__task-link"
            aria-label="打开任务中心"
          >
            <ListChecks aria-hidden="true" size={18} />
            <span className="app-topbar__task-label">任务中心</span>
          </Link>
        </header>

        <main id="main-content" className="app-main main-content" tabIndex={-1}>
          <Outlet />
        </main>
      </div>
    </div>
  );
}
