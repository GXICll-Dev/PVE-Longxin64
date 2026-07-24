import type {
  ClassroomDetail,
  ClassroomList,
  CreateClassroomOperationInput,
  Dashboard,
  Operation,
  OperationList,
  ProblemDetails,
} from './types';

const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.replace(/\/$/, '') || '/api/v1';

export function getOperationEventsUrl(operationId: string): string {
  return `${API_BASE_URL}/operations/${encodeURIComponent(operationId)}/events`;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly requestId?: string;
  readonly fieldErrors?: Record<string, string>;

  constructor(message: string, status: number, problem?: ProblemDetails) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = problem?.error_code;
    this.requestId = problem?.request_id;
    this.fieldErrors = problem?.field_errors;
  }
}

export class ApiContractError extends Error {
  constructor(resource: string) {
    super(`服务端返回了无法识别的${resource}数据。`);
    this.name = 'ApiContractError';
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function unwrapData(value: unknown): unknown {
  if (isRecord(value) && 'data' in value) {
    return value.data;
  }
  return value;
}

function decodeProblem(value: unknown): ProblemDetails | undefined {
  if (!isRecord(value)) return undefined;

  if (isRecord(value.error)) {
    const nested = value.error as ProblemDetails;
    return {
      ...nested,
      request_id: nested.request_id ?? (typeof value.request_id === 'string' ? value.request_id : undefined),
    };
  }

  return value;
}

async function requestJson(path: string, init?: RequestInit): Promise<unknown> {
  const headers = new Headers(init?.headers);
  headers.set('Accept', 'application/json');
  if (init?.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers,
    credentials: 'same-origin',
  });

  const contentType = response.headers.get('content-type') ?? '';
  const body: unknown = contentType.includes('application/json') ? await response.json() : undefined;

  if (!response.ok) {
    const problem = decodeProblem(body);
    throw new ApiError(problem?.message || `请求失败（HTTP ${response.status}）`, response.status, problem);
  }

  return body;
}

export function decodeDashboard(payload: unknown): Dashboard {
  const value = unwrapData(payload);
  if (!isRecord(value) || !isRecord(value.summary) || !Array.isArray(value.alerts) || typeof value.generated_at !== 'string') {
    throw new ApiContractError('总览');
  }
  return value as unknown as Dashboard;
}

export function decodeClassroomList(payload: unknown): ClassroomList {
  const value = unwrapData(payload);
  if (!isRecord(value) || !Array.isArray(value.items) || typeof value.total !== 'number') {
    throw new ApiContractError('教室列表');
  }
  return value as unknown as ClassroomList;
}

export function decodeClassroomDetail(payload: unknown): ClassroomDetail {
  const value = unwrapData(payload);
  if (!isRecord(value) || typeof value.id !== 'string' || !Array.isArray(value.seats)) {
    throw new ApiContractError('教室详情');
  }
  return value as unknown as ClassroomDetail;
}

export function decodeOperation(payload: unknown): Operation {
  const value = unwrapData(payload);
  if (!isRecord(value) || typeof value.id !== 'string' || !isRecord(value.counts) || !Array.isArray(value.items)) {
    throw new ApiContractError('任务');
  }
  return value as unknown as Operation;
}

export function decodeOperationList(payload: unknown): OperationList {
  const value = unwrapData(payload);
  if (Array.isArray(value)) {
    return { items: value as Operation[], total: value.length };
  }
  if (!isRecord(value) || !Array.isArray(value.items) || typeof value.total !== 'number') {
    throw new ApiContractError('任务列表');
  }
  return value as unknown as OperationList;
}

export async function getDashboard(signal?: AbortSignal): Promise<Dashboard> {
  return decodeDashboard(await requestJson('/dashboard', { signal }));
}

export async function getClassrooms(signal?: AbortSignal): Promise<ClassroomList> {
  return decodeClassroomList(await requestJson('/classrooms', { signal }));
}

export async function getClassroom(classroomId: string, signal?: AbortSignal): Promise<ClassroomDetail> {
  return decodeClassroomDetail(await requestJson(`/classrooms/${encodeURIComponent(classroomId)}`, { signal }));
}

export async function getOperations(signal?: AbortSignal): Promise<OperationList> {
  return decodeOperationList(await requestJson('/operations', { signal }));
}

export async function createClassroomOperation(
  classroomId: string,
  input: CreateClassroomOperationInput,
  idempotencyKey: string,
): Promise<Operation> {
  return decodeOperation(
    await requestJson(`/classrooms/${encodeURIComponent(classroomId)}/operations`, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
      body: JSON.stringify(input),
    }),
  );
}
