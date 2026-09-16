import { useState, useRef, useEffect, useMemo } from 'react';

export interface PaletteItem {
  id: string;
  kind: 'view' | 'action' | 'device' | 'process';
  label: string;
  sub?: string;
  action?: () => void;
}

interface Props {
  open: boolean;
  onClose: () => void;
  items: PaletteItem[];
}

export default function CommandPalette({ open, onClose, items }: Props) {
  const [query, setQuery] = useState('');
  const [idx, setIdx] = useState(0);
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      setQuery('');
      setIdx(0);
      setTimeout(() => input.current?.focus(), 0);
    }
  }, [open]);

  const matches = useMemo(() => {
    const q = query.toLowerCase();
    if (!q) return items;
    return items.filter(i =>
      i.label.toLowerCase().includes(q) ||
      i.sub?.toLowerCase().includes(q) ||
      i.kind.includes(q)
    );
  }, [items, query]);

  useEffect(() => setIdx(0), [query]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'ArrowDown') { e.preventDefault(); setIdx(i => Math.min(i + 1, matches.length - 1)); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); setIdx(i => Math.max(i - 1, 0)); }
      else if (e.key === 'Enter' && matches[idx]) { matches[idx].action?.(); onClose(); }
      else if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, idx, matches, onClose]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[20vh]" onClick={onClose}>
      <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" />
      <div
        className="relative w-[480px] bg-surface border border-hairline rounded-xl shadow-2xl overflow-hidden"
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 px-4 border-b border-hairline">
          <svg className="w-4 h-4 text-faint shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/></svg>
          <input
            ref={input}
            type="text"
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="Search views, devices, processes..."
            className="flex-1 bg-transparent py-3 text-sm text-zinc-100 placeholder-faint outline-none"
          />
          <kbd className="text-[10px] px-1.5 py-0.5 rounded bg-canvas/60 border border-hairline text-faint">esc</kbd>
        </div>
        <ul className="max-h-[300px] overflow-y-auto py-1">
          {matches.length === 0 && (
            <li className="px-4 py-3 text-sm text-faint">No results</li>
          )}
          {matches.map((m, i) => (
            <li key={m.id}>
              <button
                onClick={() => { m.action?.(); onClose(); }}
                onMouseEnter={() => setIdx(i)}
                className={`w-full text-left px-4 py-2 text-sm flex items-center gap-3
                  ${i === idx ? 'bg-brand/10 text-zinc-100' : 'text-zinc-300 hover:bg-white/[0.03]'}`}
              >
                <span className="text-[10px] font-medium uppercase w-14 shrink-0 text-faint">{m.kind}</span>
                <span className="truncate">{m.label}</span>
                {m.sub && <span className="ml-auto text-[10px] text-faint truncate">{m.sub}</span>}
              </button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}