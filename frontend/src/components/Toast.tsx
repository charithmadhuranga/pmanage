import { useState, useCallback, useRef } from 'react';

import type { Toast } from '../lib/types';

export type Notify = (msg: string, kind?: Toast['kind']) => void;

export function useToasts(): [Toast[], Notify] {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(0);
  const notify: Notify = useCallback((msg, kind = 'info') => {
    const id = ++nextId.current;
    setToasts(prev => [...prev, { id, kind, msg }]);
    setTimeout(() => setToasts(prev => prev.filter(t => t.id !== id)), 4000);
  }, []);
  return [toasts, notify];
}

interface Props {
  toasts: Toast[];
}

const COLOR: Record<Toast['kind'], string> = {
  ok: 'border-ok/40 text-ok',
  error: 'border-crit/40 text-crit',
  info: 'border-brand/40 text-brand',
};

export default function ToastList({ toasts }: Props) {
  if (toasts.length === 0) return null;
  return (
    <div className="fixed bottom-16 right-4 z-50 flex flex-col gap-1.5 pointer-events-none">
      {toasts.map(t => (
        <div
          key={t.id}
          className={`pointer-events-auto px-3 py-1.5 rounded-lg border bg-surface/90 backdrop-blur text-xs font-medium shadow-lg animate-[fadeIn_0.15s] ${COLOR[t.kind]}`}
        >
          {t.msg}
        </div>
      ))}
    </div>
  );
}