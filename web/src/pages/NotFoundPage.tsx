import { Link } from '@tanstack/react-router';
import { ArrowLeft } from 'lucide-react';

export function NotFoundPage() {
  return (
    <div className="not-found">
      <span>404</span>
      <h1>找不到这个页面</h1>
      <p>地址可能已变更，或当前纵切片尚未提供该功能。</p>
      <Link to="/" className="button button--primary button--md">
        <ArrowLeft aria-hidden="true" size={16} />
        返回总览
      </Link>
    </div>
  );
}
