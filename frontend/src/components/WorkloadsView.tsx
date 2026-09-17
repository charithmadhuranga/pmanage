import { useEffect, useRef, useState } from 'react';
import { Service as ClassifyService } from '../../bindings/pmanage/pkg/classify';

import type { ProcUsage, ClassifiedProc, WorkloadKind } from '../lib/types';
import { shortName } from '../lib/types';

const KIND_META: Record<WorkloadKind, { label: string; color: string }> = {
  training:  { label: 'Training',  color: '#38bdf8' },
  inference: { label: 'Inference', color: '#818cf8' },
  codec:     { label: 'Codec',     color: '#34d399' },
  crypto:    { label: 'Crypto',    color: '#f87171' },
  unknown:   { label: 'Unknown',   color: '#a1a1aa' },
};

interface Props {
  accelProcs: ProcUsage[];
}

export default function WorkloadsView({ accelProcs }: Props) {
  const [classified, setClassified] = useState<Record<number, ClassifiedProc>>({});
  const [loading, setLoading] = useState(false);
  const inflight = useRef(new Set<number>());
  const classifiedPids = useRef(new Set<number>());

  // Classify each accel process on arrival / pid change
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const pids = accelProcs.map(p => p.pid);
      // Only classify PIDs we haven't seen yet
      const newPids = pids.filter(pid => !classifiedPids.current.has(pid));
      if (newPids.length === 0) return;

      setLoading(true);
      for (const pid of newPids) {
        if (cancelled || inflight.current.has(pid)) continue;
        inflight.current.add(pid);
        try {
          const c = await ClassifyService.ClassifyPID(pid);
          if (cancelled) return;
          classifiedPids.current.add(pid);
          setClassified(prev => ({ ...prev, [pid]: c as unknown as ClassifiedProc }));
        } catch {
          // pid vanished between telemetry and classify
          classifiedPids.current.add(pid); // don't retry
        } finally {
          inflight.current.delete(pid);
        }
      }
      // prune classified entries whose pid no longer appears
      setClassified(prev => {
        const keep: Record<number, ClassifiedProc> = {};
        const activePids = new Set(accelProcs.map(p => p.pid));
        for (const pid of Object.keys(prev).map(Number)) {
          if (activePids.has(pid)) keep[pid] = prev[pid];
          else classifiedPids.current.delete(pid); // allow re-classify if it returns
        }
        return keep;
      });
      if (!cancelled) setLoading(false);
    })();
    return () => { cancelled = true; };
  }, [accelProcs]);

  const counts: Record<WorkloadKind, number> = {
    training: 0, inference: 0, codec: 0, crypto: 0, unknown: 0,
  };
  let totalScore = 0;
  let scored = 0;
  for (const c of Object.values(classified)) {
    counts[c.kind]++;
    if (c.kind !== 'unknown') { totalScore += c.score; scored++; }
  }

  const rows = accelProcs.map(p => {
    const c = classified[p.pid];
    return { proc: p, c: c ?? null };
  });

  return (
    <div className="space-y-4">
      {/* Summary stripline */}
      <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
        {(Object.keys(KIND_META) as WorkloadKind[]).map(k => (
          <div key={k} className="rounded-lg bg-surface ring-1 ring-hairline p-3">
            <div className="flex items-center gap-2 mb-1">
              <span className="w-2 h-2 rounded-full" style={{ background: KIND_META[k].color }} />
              <span className="text-[10px] uppercase tracking-[0.12em] text-faint font-semibold">{KIND_META[k].label}</span>
            </div>
            <div className="text-xl font-semibold" style={{ color: KIND_META[k].color }}>
              {counts[k]}
            </div>
          </div>
        ))}
      </div>

      {loading && classified && (
        <div className="flex items-center gap-2 text-xs text-faint">
          <span className="w-3 h-3 animate-spin rounded-full border border-faint border-t-transparent" />
          Classifying workloads…
        </div>
      )}

      <div className="rounded-lg bg-surface ring-1 ring-hairline overflow-hidden">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-hairline text-faint">
              <th className="text-left px-3 py-2 font-medium">Workload</th>
              <th className="text-left px-3 py-2 font-medium">Process</th>
              <th className="text-left px-3 py-2 font-medium">PID</th>
              <th className="text-left px-3 py-2 font-medium">Device</th>
              <th className="text-right px-3 py-2 font-medium">GPU %</th>
              <th className="text-left px-3 py-2 font-medium">Confidence</th>
              <th className="text-left px-3 py-2 font-medium">Matched</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 && (
              <tr><td colSpan={7} className="px-3 py-4 text-center text-faint">No accelerator processes to classify</td></tr>
            )}
            {[...rows]
              .sort((a, b) => (a.c?.score ?? 0) - (b.c?.score ?? 0))
              .reverse()
              .map(({ proc, c }) => {
                const kind = c?.kind ?? 'unknown';
                const meta = KIND_META[kind];
                return (
                  <tr key={proc.pid} className="border-b border-hairline/50 hover:bg-white/[0.02]">
                    <td className="px-3 py-1.5">
                      <span className="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[10px] font-medium"
                        style={{ color: meta.color, background: meta.color + '1a' }}>
                        <span className="w-1.5 h-1.5 rounded-full" style={{ background: meta.color }} />
                        {meta.label}
                      </span>
                    </td>
                    <td className="px-3 py-1.5 text-zinc-200 font-medium" title={proc.name}>{shortName(proc.name)}</td>
                    <td className="px-3 py-1.5 text-faint tabular-nums">{proc.pid}</td>
                    <td className="px-3 py-1.5 text-faint tabular-nums">{proc.deviceId}</td>
                    <td className="px-3 py-1.5 text-right tabular-nums text-zinc-200">{proc.utilPct.toFixed(0)}%</td>
                    <td className="px-3 py-1.5 text-faint tabular-nums">
                      {c ? `${(c.score * 100).toFixed(0)}%` : '…'}
                    </td>
                    <td className="px-3 py-1.5 text-faint max-w-[14rem] truncate" title={c?.hits?.map(h => h.rule).join(', ') ?? ''}>
                      {c?.hits?.map(h => h.signal).filter(Boolean).slice(0, 3).join(' · ') || '—'}
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