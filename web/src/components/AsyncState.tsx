import { AlertTriangle, Inbox, LoaderCircle, RefreshCw } from 'lucide-react';
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
