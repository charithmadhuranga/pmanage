import { useEffect, useMemo, useState } from 'react';
import * as echarts from 'echarts';

import { Service as HistoryService } from '../../bindings/pmanage/pkg/history';
import { useEChart } from '../hooks/useEChart';

import type { HistoryPoint, DeviceMetrics } from '../lib/types';

const METRICS = [
  { key: 'utilPct',  label: 'Utilization %',    color: '#38bdf8', max: 100 },
  { key: 'vramUsed', label: 'VRAM Used (B)',    color: '#818cf8', max: undefined },
  { key: 'tempC',    label: 'Temperature °C',   color: '#34d399', max: undefined },
  { key: 'powerW',   label: 'Power W',          color: '#facc15', max: undefined },
  { key: 'clockMhz', label: 'Clock MHz',        color: '#22d3ee', max: undefined },
] as const;

const WINDOWS = [
  { label: '1m',  sec: 60 },
  { label: '5m',  sec: 300 },
  { label: '15m', sec: 900 },
  { label: '1h',  sec: 3600 },
];

interface Props {
  devices: DeviceMetrics[];
}

export default function HistoryView({ devices }: Props) {
  const [windowSec, setWindowSec] = useState(300);
  const [metric, setMetric] = useState<(typeof METRICS)[number]['key']>('utilPct');
  const [selected, setSelected] = useState<string | null>(null);
  const [pointsMap, setPointsMap] = useState<Record<string, HistoryPoint[]>>({});
  const [devList, setDevList] = useState<string[]>([]);

  // device list first-seen from history, fallback to current live devices
  useEffect(() => {
    let cancelled = false;
    HistoryService.Devices().then((ids: string[]) => {
      if (cancelled) return;
      setDevList(ids);
      if (!selected && ids.length) setSelected(ids[0]);
    }).catch(() => { /* bindings not ready */ });
    return () => { cancelled = true; };
  }, []);

  // When live devices know more than history (fresh boot), absorb them.
  useEffect(() => {
    const ids = devices.map(d => d.deviceId).filter(id => !devList.includes(id));
    if (ids.length) setDevList(prev => [...prev, ...ids]);
  }, [devices, devList]);

  useEffect(() => {
    if (!selected) return;
    let cancelled = false;
    const load = async () => {
      try {
        const pts = await HistoryService.Range(selected, windowSec) as HistoryPoint[];
        if (!cancelled && pts.length) {
          setPointsMap(prev => ({ ...prev, [selected]: pts }));
        }
      } catch { /* ignore */ }
    };
    load();
    const iv = setInterval(load, 5000);
    return () => { cancelled = true; clearInterval(iv); };
  }, [selected, windowSec]);

  const meta = METRICS.find(m => m.key === metric)!;
  const points = selected ? (pointsMap[selected] ?? []) : [];

  const option: echarts.EChartsOption = useMemo(() => ({
    backgroundColor: 'transparent',
    grid: { top: 28, right: 16, bottom: 28, left: 48 },
    tooltip: {
      trigger: 'axis',
      backgroundColor: '#18181b',
      borderColor: '#27272a',
      textStyle: { color: '#e4e4e7', fontSize: 11 },
      valueFormatter: (v: unknown) => (typeof v === 'number' ? v.toFixed(1) : String(v)),
    },
    xAxis: {
      type: 'time',
      axisLine: { lineStyle: { color: '#3f3f46' } },
      axisLabel: { color: '#71717a', fontSize: 10 },
      splitLine: { lineStyle: { color: '#27272a' } },
    },
    yAxis: {
      type: 'value',
      max: meta.max,
      axisLabel: { color: '#71717a', fontSize: 10 },
      splitLine: { lineStyle: { color: '#27272a' } },
    },
    series: [{
      name: meta.label,
      type: 'line',
      data: points.map(p => [p.ts * 1000, p[metric]]),
      smooth: true,
      symbol: 'none',
      lineStyle: { color: meta.color, width: 2 },
      areaStyle: {
        color: {
          type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [
            { offset: 0, color: meta.color + '2e' },
            { offset: 1, color: meta.color + '06' },
          ],
        },
      },
    }],
    animation: true,
  }), [points, metric, meta]);

  const chartRef = useEChart(option, [option]);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3 flex-wrap">
        {/* Device selector */}
        <div className="flex items-center gap-1 rounded-lg bg-surface ring-1 ring-hairline p-1">
          {devList.map(id => (
            <button
              key={id}
              onClick={() => setSelected(id)}
              className={`px-2.5 py-1 rounded text-[11px] font-medium transition-colors ${
                selected === id ? 'bg-brand/15 text-brand' : 'text-faint hover:text-zinc-200'}`}
            >
              {id}
            </button>
          ))}
        </div>

        {/* Metric selector */}
        <div className="flex items-center gap-1 rounded-lg bg-surface ring-1 ring-hairline p-1">
          {METRICS.map(m => (
            <button
              key={m.key}
              onClick={() => setMetric(m.key)}
              className={`px-2.5 py-1 rounded text-[11px] font-medium transition-colors ${
                metric === m.key ? 'bg-brand/15 text-brand' : 'text-faint hover:text-zinc-200'}`}
            >
              {m.label.replace(' (B)', '')}
            </button>
          ))}
        </div>

        {/* Time window selector */}
        <div className="flex items-center gap-1 rounded-lg bg-surface ring-1 ring-hairline p-1">
          {WINDOWS.map(w => (
            <button
              key={w.sec}
              onClick={() => setWindowSec(w.sec)}
              className={`px-2.5 py-1 rounded text-[11px] font-medium transition-colors ${
                windowSec === w.sec ? 'bg-brand/15 text-brand' : 'text-faint hover:text-zinc-200'}`}
            >
              {w.label}
            </button>
          ))}
        </div>

        <div className="flex-1" />
        <span className="text-[11px] text-faint">{points.length} samples · {meta.label}</span>
      </div>

      <div className="rounded-lg bg-surface ring-1 ring-hairline p-2">
        <div ref={chartRef} className="h-64 w-full" />
      </div>

      {points.length > 1 && (
        <div className="grid gap-3" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))' }}>
          {METRICS.map(m => {
            const vals = points.map(p => p[m.key]);
            const last = vals[vals.length - 1] ?? 0;
            const avg = vals.reduce((a, b) => a + b, 0) / vals.length;
            let max = -Infinity;
            for (const v of vals) if (v > max) max = v;
            const fmt = (v: number) =>
              m.key === 'vramUsed' ? `${(v / 1e9).toFixed(1)}G` : `${v.toFixed(1)}`;
            return (
              <div key={m.key} className="rounded-lg bg-canvas/50 ring-1 ring-hairline p-3">
                <div className="text-[10px] uppercase tracking-[0.12em] text-faint font-semibold mb-1">{m.label}</div>
                <div className="flex items-baseline gap-2">
                  <span className="text-lg font-semibold text-zinc-100 tabular-nums">{fmt(last)}</span>
                </div>
                <div className="text-[11px] text-faint tabular-nums mt-1">
                  avg {fmt(avg)} · max {fmt(max)}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}