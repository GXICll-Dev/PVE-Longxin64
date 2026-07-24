import { AlertTriangle, Inbox, LoaderCircle, RefreshCw, WifiOff } from 'lucide-react';
import { ApiError } from '../api/client';
import { Button } from './ui/Button';

export function LoadingState({ label = '正在加载数据' }: { label?: string }) {
  return (
    <div className="state-panel" aria-busy="true" aria-live="polite">
      <LoaderCircle className="spinner" aria-hidden="true" size={22} />
      <div>
        <strong>{label}</strong>
        <p>正在从控制平面获取最新状态。</p>
      </div>
    </div>
  );
}

interface ErrorStateProps {
  error: unknown;
  onRetry: () => void;
}

export function ErrorState({ error, onRetry }: ErrorStateProps) {
  const requestId = error instanceof ApiError ? error.requestId : undefined;
  const message = error instanceof Error ? error.message : '暂时无法获取数据。';

  return (
    <div className="state-panel state-panel--error" role="alert">
      <AlertTriangle aria-hidden="true" size={22} />
      <div className="state-panel__content">
        <strong>数据加载失败</strong>
        <p>{message}</p>
        {requestId ? <code>请求 ID：{requestId}</code> : null}
      </div>
      <Button size="sm" onClick={onRetry}>
        <RefreshCw aria-hidden="true" size={15} />
        重试
      </Button>
    </div>
  );
}

interface StaleDataNoticeProps {
  error: unknown;
  isRetrying?: boolean;
  onRetry: () => void;
}

export function StaleDataNotice({ error, isRetrying = false, onRetry }: StaleDataNoticeProps) {
  const requestId = error instanceof ApiError ? error.requestId : undefined;
  const message = error instanceof Error ? error.message : '控制平面暂时无法连接。';

  return (
    <div className="stale-data-notice">
      <WifiOff aria-hidden="true" size={20} />
      <div className="stale-data-notice__content" role="status">
        <strong>连接中断，正在显示上次成功获取的数据</strong>
        <p>最新刷新失败：{message} 页面中的状态可能已经发生变化，请谨慎执行操作。</p>
        {requestId ? <code>请求 ID：{requestId}</code> : null}
      </div>
      <Button size="sm" onClick={onRetry} disabled={isRetrying}>
        {isRetrying ? <LoaderCircle className="spinner" aria-hidden="true" size={15} /> : <RefreshCw aria-hidden="true" size={15} />}
        {isRetrying ? '正在重连' : '重新连接'}
      </Button>
    </div>
  );
}

interface EmptyStateProps {
  title: string;
  description: string;
}

export function EmptyState({ title, description }: EmptyStateProps) {
  return (
    <div className="state-panel state-panel--empty">
      <Inbox aria-hidden="true" size={22} />
      <div>
        <strong>{title}</strong>
        <p>{description}</p>
      </div>
    </div>
  );
}
