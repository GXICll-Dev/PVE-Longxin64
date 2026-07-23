import type { ActiveSession, OperationCounts } from '../api/types';

const statusLabels: Record<string, string> = {
  READY: '已就绪',
  ACTIVE: '上课中',
  DEGRADED: '部分异常',
  OFFLINE: '离线',
  RUNNING: '运行中',
  STOPPED: '已关机',
  STARTING: '启动中',
  STOPPING: '关机中',
  ERROR: '异常',
  UNKNOWN: '未知',
  QUEUED: '排队中',
  VALIDATING: '校验中',
  WAITING_PVE: '等待 PVE',
  VERIFYING: '验证中',
  SUCCEEDED: '已完成',
  PARTIALLY_SUCCEEDED: '部分完成',
  FAILED: '失败',
  CANCEL_REQUESTED: '取消中',
  CANCELLED: '已取消',
  SKIPPED: '已跳过',
  ONLINE: '在线',
  STALE: '心跳延迟',
  IDLE: '空闲',
};

const operationLabels: Record<string, string> = {
  PRECHECK: '课前检查',
  START: '批量开机',
  SHUTDOWN: '批量关机',
  RESTORE: '基线还原',
};

export function normalizeStatus(status?: string | null): string {
  return status?.trim().toUpperCase() || 'UNKNOWN';
}

export function formatStatus(status?: string | null): string {
  const normalized = normalizeStatus(status);
  return statusLabels[normalized] ?? status ?? '未知';
}

export function formatOperationType(type: string): string {
  return operationLabels[normalizeStatus(type)] ?? type;
}

export function formatDateTime(value?: string | null, timezone?: string): string {
  if (!value) return '暂无数据';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '时间格式异常';

  try {
    return new Intl.DateTimeFormat('zh-CN', {
      dateStyle: 'medium',
      timeStyle: 'medium',
      hour12: false,
      ...(timezone ? { timeZone: timezone } : {}),
    }).format(date);
  } catch {
    return new Intl.DateTimeFormat('zh-CN', {
      dateStyle: 'medium',
      timeStyle: 'medium',
      hour12: false,
    }).format(date);
  }
}

export function formatCompactId(id: string): string {
  return id.length > 12 ? `${id.slice(0, 8)}…${id.slice(-4)}` : id;
}

export function formatActiveSession(session: ActiveSession | string | null): string {
  if (!session) return '无进行中课程';
  if (typeof session === 'string') return session;
  return session.name;
}

export function getOperationProgress(counts: OperationCounts): { completed: number; percent: number } {
  const completed = counts.succeeded + counts.failed + counts.skipped;
  if (counts.total <= 0) return { completed, percent: 0 };
  return { completed, percent: Math.min(100, Math.round((completed / counts.total) * 100)) };
}

export function ratioLabel(current: number, total: number): string {
  return `${current} / ${total}`;
}
