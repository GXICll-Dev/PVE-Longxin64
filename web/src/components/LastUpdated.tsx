import { Clock3 } from 'lucide-react';
import { formatDateTime } from '../lib/format';

interface LastUpdatedProps {
  value?: string | null;
  isFetching?: boolean;
  timezone?: string;
  label?: string;
}

export function LastUpdated({ value, isFetching = false, timezone, label = '数据时间' }: LastUpdatedProps) {
  return (
    <div className="last-updated" aria-live="polite">
      <Clock3 aria-hidden="true" size={14} />
      <span>
        {isFetching ? '正在刷新 · ' : `${label} · `}
        <time dateTime={value ?? undefined}>{formatDateTime(value, timezone)}</time>
      </span>
    </div>
  );
}
