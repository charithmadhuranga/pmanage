import { useState, useMemo } from 'react';

import type { MergedProcess } from '../lib/types';
import { hasProcValid, shortName } from '../lib/types';

interface Props {
  procs: MergedProcess[];
  selection: Set<number>;
  onSelect: (sel: Set<number>) => void;
  onAction: (action: string, pids: number[]) => void;
}

type SortCol = 'pid' | 'name' | 'cpu' | 'memBytes' | 'threads' | 'elapsedSec' | 'gpu' | 'vram' | 'status';

const COLUMNS: { key: SortCol; label: string; right?: boolean }[] = [
  { key: 'pid',       label: 'PID' },
  { key: 'name',      label: 'Name' },
  { key: 'status',    label: 'Status' },
  { key: 'cpu',       label: 'CPU %', right: true },
  { key: 'memBytes',  label: 'Memory', right: true },
  { key: 'threads',   label: 'Thr', right: true },
  { key: 'elapsedSec', label: 'Time', right: true },
  { key: 'gpu',       label: 'GPU %', right: true },
  { key: 'vram',      label: 'VRAM', right: true },
];

function fmtBytes(b: number): string {
  if (b < 1e6) return `${(b / 1e3).toFixed(0)} KB`;
  if (b < 1e9) return `${(b / 1e6).toFixed(1)} MB`;
  return `${(b / 1e9).toFixed(2)} GB`;
}

function fmtElapsed(s: number): string {
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

function statusColor(status: string): string {
  switch (status.toLowerCase()) {
    case 'running': return 'text-ok';
    case 'sleeping':
    case 'sleep':
    case 'idle': return 'text-faint';
    case 'stop':
    case 'stopped':
    case 't':
    case 't (stopped)': return 'text-warn';
    case 'zombie':
    case 'z': return 'text-crit';
    default: return 'text-faint';
  }
}

export default function ProcessTable({ procs, selection, onSelect, onAction }: Props) {
  const [sortCol, setSortCol] = useState<SortCol>('cpu');
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc');
  const [filter, setFilter] = useState('');

  const filtered = useMemo(() => {
    let list = procs;
    const q = filter.trim().toLowerCase();
    if (q.startsWith('pid:')) {
      const pid = Number(q.slice(4).trim());
      if (!isNaN(pid)) list = list.filter(p => p.pid === pid);
    } else if (q.startsWith('name:')) {
      const pat = q.slice(5).trim();
      list = list.filter(p => p.name.toLowerCase().includes(pat));
    } else if (q) {
      list = list.filter(p =>
        p.name.toLowerCase().includes(q) ||
        String(p.pid).includes(q) ||
        p.username.toLowerCase().includes(q)
      );
    }

    list.sort((a, b) => {
      let va: number, vb: number;
      switch (sortCol) {
        case 'pid': va = a.pid; vb = b.pid; break;
        case 'name': va = a.name.localeCompare(b.name); vb = 0; break;
        case 'status': va = a.status.localeCompare(b.status); vb = 0; break;
        case 'cpu': va = a.cpu; vb = b.cpu; break;
        case 'memBytes': va = a.memBytes; vb = b.memBytes; break;
        case 'threads': va = a.threads; vb = b.threads; break;
        case 'elapsedSec': va = a.elapsedSec; vb = b.elapsedSec; break;
        case 'gpu': va = a.accel?.utilPct ?? -1; vb = b.accel?.utilPct ?? -1; break;
        case 'vram': va = a.accel?.vramUsed ?? -1; vb = b.accel?.vramUsed ?? -1; break;
        default: va = 0; vb = 0;
      }
      if (sortCol === 'name' || sortCol === 'status') {
        return sortDir === 'asc' ? va as unknown as number : -(va as unknown as number);
      }
      return sortDir === 'asc' ? va - vb : vb - va;
    });
    return list;
  }, [procs, sortCol, sortDir, filter]);

  const toggleSort = (col: SortCol) => {
    if (col === sortCol) setSortDir(d => d === 'asc' ? 'desc' : 'asc');
    else { setSortCol(col); setSortDir(col === 'name' || col === 'status' ? 'asc' : 'desc'); }
  };

  const allPids = filtered.map(p => p.pid);
  const allSelected = allPids.length > 0 && allPids.every(p => selection.has(p));

  const toggleAll = () => {
    if (allSelected) onSelect(new Set());
    else onSelect(new Set(allPids));
  };
  const toggle = (pid: number) => {
    const next = new Set(selection);
    if (next.has(pid)) next.delete(pid); else next.add(pid);
    onSelect(next);
  };

  return (
    <div className="flex flex-col h-full">
      {/* Top bar: filter + batch actions */}
      <div className="flex items-center gap-2 px-3 py-2 bg-surface border-b border-hairline shrink-0">
        <input
          type="text"
          value={filter}
          onChange={e => setFilter(e.target.value)}
          placeholder="Filter: name, pid:123, user..."
          className="w-64 px-2 py-1 text-xs bg-canvas/60 rounded border border-hairline text-zinc-100 placeholder-faint outline-none focus:border-brand"
        />
        <span className="text-[10px] text-faint tabular-nums ml-auto">
          {filtered.length} of {procs.length} processes
        </span>
        {selection.size > 0 && (
          <div className="flex items-center gap-1 ml-3 pl-3 border-l border-hairline">
            <span className="text-[10px] text-brand font-medium mr-1">{selection.size} selected</span>
            <button onClick={() => onAction('suspend', [...selection])} className="px-2 py-0.5 text-[10px] rounded bg-warn/10 text-warn hover:bg-warn/20">Suspend</button>
            <button onClick={() => onAction('resume', [...selection])} className="px-2 py-0.5 text-[10px] rounded bg-ok/10 text-ok hover:bg-ok/20">Resume</button>
            <button onClick={() => onAction('kill', [...selection])} className="px-2 py-0.5 text-[10px] rounded bg-crit/10 text-crit hover:bg-crit/20">Terminate</button>
          </div>
        )}
      </div>

      {/* Table */}
      <div className="flex-1 overflow-y-auto overflow-x-auto">
        <table className="w-full text-xs">
          <thead className="sticky top-0 bg-surface z-10">
            <tr className="border-b border-hairline text-faint">
              <th className="w-8 px-2 py-1.5 text-center">
                <input
                  type="checkbox"
                  checked={allSelected}
                  onChange={toggleAll}
                  className="rounded border-hairline"
                />
              </th>
              {COLUMNS.map(c => (
                <th
                  key={c.key}
                  onClick={() => toggleSort(c.key)}
                  className={`px-2 py-1.5 font-medium select-none cursor-pointer hover:text-zinc-200 ${c.right ? 'text-right' : 'text-left'}`}
                >
                  {c.label}
                  {sortCol === c.key && (
                    <span className="ml-0.5">{sortDir === 'asc' ? '↑' : '↓'}</span>
                  )}
                </th>
              ))}
              <th className="w-14 px-2 py-1.5" />
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 && (
              <tr>
                <td colSpan={COLUMNS.length + 2} className="px-3 py-8 text-center text-faint">
                  {procs.length === 0 ? 'No processes available' : 'No processes match filter'}
                </td>
              </tr>
            )}
            {filtered.map(p => {
              const selected = selection.has(p.pid);
              const accel = p.accel;
              return (
                <tr
                  key={p.pid}
                  onClick={() => toggle(p.pid)}
                  className={`border-b border-hairline/40 cursor-pointer transition-colors
                    ${selected ? 'bg-brand/8' : 'hover:bg-white/[0.02]'}`}
                >
                  <td className="px-2 py-1 text-center" onClick={e => e.stopPropagation()}>
                    <input
                      type="checkbox"
                      checked={selected}
                      onChange={() => toggle(p.pid)}
                      className="rounded border-hairline"
                    />
                  </td>
                  <td className="px-2 py-1 text-zinc-300 tabular-nums">{p.pid}</td>
                  <td className="px-2 py-1 text-zinc-100 font-medium truncate max-w-[200px]" title={p.name}>{shortName(p.name)}</td>
                  <td className={`px-2 py-1 text-[10px] uppercase ${statusColor(p.status)}`}>{p.status}</td>
                  <td className="px-2 py-1 text-right tabular-nums text-zinc-200">{p.cpu.toFixed(1)}</td>
                  <td className="px-2 py-1 text-right tabular-nums text-zinc-300">{fmtBytes(p.memBytes)}</td>
                  <td className="px-2 py-1 text-right tabular-nums text-zinc-400">{p.threads}</td>
                  <td className="px-2 py-1 text-right tabular-nums text-zinc-500">{fmtElapsed(p.elapsedSec)}</td>
                  <td className="px-2 py-1 text-right tabular-nums text-zinc-200">
                    {accel && hasProcValid(accel, 'util') ? `${accel.utilPct.toFixed(0)}%` : '—'}
                  </td>
                  <td className="px-2 py-1 text-right tabular-nums text-zinc-300">
                    {accel && hasProcValid(accel, 'vramUsed') && accel.vramUsed > 0 ? `${(accel.vramUsed / 1e9).toFixed(2)}G` : '—'}
                  </td>
                  <td className="px-2 py-1" onClick={e => e.stopPropagation()}>
                    <button
                      onClick={() => onAction('kill', [p.pid])}
                      title="Terminate"
                      className="p-0.5 rounded text-faint hover:text-crit hover:bg-crit/10"
                    >
                      <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><path d="M18 6L6 18M6 6l12 12"/></svg>
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}