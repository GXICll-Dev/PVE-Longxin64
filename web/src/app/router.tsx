import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router';
import { AppShell } from './AppShell';
import { DashboardPage } from '../pages/DashboardPage';
import { ClassroomDetailPage } from '../pages/ClassroomDetailPage';
import { ClassroomsPage } from '../pages/ClassroomsPage';
import { NotFoundPage } from '../pages/NotFoundPage';
import { OperationsPage } from '../pages/OperationsPage';

const rootRoute = createRootRoute({
  component: AppShell,
  notFoundComponent: NotFoundPage,
});

const dashboardRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: DashboardPage,
});

const classroomsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/classrooms',
  component: ClassroomsPage,
});

const classroomDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/classrooms/$classroomId',
  component: ClassroomDetailRoute,
});

function ClassroomDetailRoute() {
  const { classroomId } = classroomDetailRoute.useParams();
  return <ClassroomDetailPage classroomId={classroomId} />;
}

const operationsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/operations',
  component: OperationsPage,
});

const routeTree = rootRoute.addChildren([dashboardRoute, classroomsRoute, classroomDetailRoute, operationsRoute]);

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
  defaultPreloadStaleTime: 10_000,
  scrollRestoration: true,
});

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
