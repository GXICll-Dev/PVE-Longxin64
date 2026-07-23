import { queryOptions } from '@tanstack/react-query';
import { getClassroom, getClassrooms, getDashboard, getOperations } from './client';

export const queryKeys = {
  dashboard: ['dashboard'] as const,
  classrooms: ['classrooms'] as const,
  classroom: (classroomId: string) => ['classrooms', classroomId] as const,
  operations: ['operations'] as const,
};

export const dashboardQueryOptions = queryOptions({
  queryKey: queryKeys.dashboard,
  queryFn: ({ signal }) => getDashboard(signal),
  staleTime: 10_000,
  refetchInterval: 30_000,
});

export const classroomsQueryOptions = queryOptions({
  queryKey: queryKeys.classrooms,
  queryFn: ({ signal }) => getClassrooms(signal),
  staleTime: 10_000,
  refetchInterval: 30_000,
});

export function classroomQueryOptions(classroomId: string) {
  return queryOptions({
    queryKey: queryKeys.classroom(classroomId),
    queryFn: ({ signal }) => getClassroom(classroomId, signal),
    staleTime: 5_000,
    refetchInterval: 15_000,
  });
}

export const operationsQueryOptions = queryOptions({
  queryKey: queryKeys.operations,
  queryFn: ({ signal }) => getOperations(signal),
  staleTime: 3_000,
  refetchInterval: 5_000,
});
