import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { Events } from '@wailsio/runtime';

import NavRail, { type RailMode } from './components/NavRail';
import Header from './components/Header';
import StatusBar from './components/StatusBar';
import KpiCard from './components/KpiCard';
import DeviceCard from './components/DeviceCard';
import ProcessTable from './components/ProcessTable';
import SourcesView from './components/SourcesView';
import WorkloadsView from './components/WorkloadsView';
import HistoryView from './components/HistoryView';
import AlertsView from './components/AlertsView';
import CommandPalette, { type PaletteItem } from './components/CommandPalette';
import ToastList, { useToasts } from './components/Toast';
import useServerMode from './hooks/useServerMode';

import { Service as ProcService } from '../bindings/pmanage/pkg/process';

import type { TelemetrySnapshot, DeviceMetrics, ProcUsage, MergedProcess, AlertFire } from './lib/types';

const SPARK_LEN = 60;
const POLL_MS = 3000;
const VENDOR_COLORS: Record<string, string> = {
  nvidia: '#76b900', amd: '#e8744a', intel: '#38bdf8', google: '#818cf8',
};

function push(arr: number[], v: number, max = SPARK_LEN) {
  const next = [...arr, v];
  return next.length > max ? next.slice(next.length - max) : next;
}

function guessVendor(deviceId: string): string {
  if (deviceId.startsWith('gpu0')) return 'nvidia';
  if (deviceId.startsWith('gpu1')) return 'amd';
  if (deviceId.startsWith('gpu2')) return 'intel';
  if (deviceId.startsWith('npu0')) return 'hailo';
  if (deviceId.startsWith('tpu0')) return 'google';
  return 'unknown';
}

interface AccelGroup {
  label: string;
  color: string;
  deviceCount: number;
  avgUtil: number;
  avgPower: number;
  totalVram: number;
  totalVramMax: number;
  spark: number[];
}

export default function App() {
  const [view, setView] = useState('overview');
  const [devices, setDevices] = useState<DeviceMetrics[]>([]);
  const [accelProcs, setAccelProcs] = useState<ProcUsage[]>([]);
  const [gpuSpark, setGpuSpark] = useState<number[]>([]);
  const [npuSpark, setNpuSpark] = useState<number[]>([]);
  const [tpuSpark, setTpuSpark] = useState<number[]>([]);
  const gpuVendors = useRef(new Set<string>());
  const npuVendors = useRef(new Set<string>());
  const tpuVendors = useRef(new Set<string>());

  // Phase 1 — system process state
  const [sysProcs, setSysProcs] = useState<MergedProcess[]>([]);
  const [procSelection, setProcSelection] = useState(new Set<number>());
  const [paused, setPaused] = useState(false);
  const pausedRef = useRef(false);

  // Shell state
  const [railMode, setRailMode] = useState<RailMode>('collapsed');
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [toasts, notify] = useToasts();
  const serverInfo = useServerMode();

  // Confirm dialog state
  const [confirm, setConfirm] = useState<{ action: string; pids: number[] } | null>(null);

  const handleTelemetry = useCallback((data: TelemetrySnapshot) => {
    if (pausedRef.current) return;

    setDevices(data.devices);
    setAccelProcs(data.procs);

    gpuVendors.current.clear();
    npuVendors.current.clear();
    tpuVendors.current.clear();

    let gpuAvg = 0, gpuN = 0, npuAvg = 0, npuN = 0, tpuAvg = 0, tpuN = 0;
    for (const d of data.devices) {
      const vendor = guessVendor(d.deviceId);
      if (d.deviceId.startsWith('gpu')) { gpuAvg += d.utilPct; gpuN++; gpuVendors.current.add(vendor); }
      else if (d.deviceId.startsWith('npu')) { npuAvg += d.utilPct; npuN++; npuVendors.current.add(vendor); }
      else { tpuAvg += d.utilPct; tpuN++; tpuVendors.current.add(vendor); }
    }
    setGpuSpark(s => push(s, gpuN ? gpuAvg / gpuN : 0));
    setNpuSpark(s => push(s, npuN ? npuAvg / npuN : 0));
    setTpuSpark(s => push(s, tpuN ? tpuAvg / tpuN : 0));
  }, []);

  useEffect(() => {
    const unsub = Events.On('telemetry', (ev) => handleTelemetry(ev.data as TelemetrySnapshot));
    return unsub;
  }, [handleTelemetry]);

  // Alert events: push a toast for each incoming fire from Go-side engine
  useEffect(() => {
    const unsub = Events.On('alert', (ev) => {
      const f = ev.data as AlertFire;
      if (!f?.message) return;
      const kind = f.severity === 'crit' ? 'error' : f.severity === 'warn' ? 'error' : 'info';
      notify(f.message, kind);
    });
    return unsub;
  }, [notify]);

  // System process polling
  const refreshProcs = useCallback(async () => {
    if (pausedRef.current) return;
    try {
      const list = await ProcService.ListProcesses();
      const accelMap = new Map(accelProcs.map(p => [p.pid, p]));
      setSysProcs((list ?? []).map(p => ({ ...p, accel: accelMap.get(p.pid) })));
    } catch { /* ignore transient poll errors */ }
  }, [accelProcs]);

  useEffect(() => {
    refreshProcs();
    const iv = setInterval(refreshProcs, POLL_MS);
    return () => clearInterval(iv);
  }, [refreshProcs]);

  // Build accel map for live merge
  const accelMap = useMemo(() => new Map(accelProcs.map(p => [p.pid, p])), [accelProcs]);

  // Re-merge sysProcs with latest accel when accelProcs change
  useEffect(() => {
    setSysProcs(prev => prev.map(p => ({ ...p, accel: accelMap.get(p.pid) })));
  }, [accelMap]);

  // Pause toggle
  const togglePause = useCallback(() => {
    setPaused(p => {
      pausedRef.current = !p;
      return !p;
    });
  }, []);

  // Rail mode toggle: collapsed → expanded → hidden → collapsed
  const cycleRail = useCallback(() => {
    setRailMode(m => m === 'collapsed' ? 'expanded' : m === 'expanded' ? 'hidden' : 'collapsed');
  }, []);

  // Global ⌘K / ⌘\ shortcuts
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') { e.preventDefault(); setPaletteOpen(p => !p); }
      if ((e.metaKey || e.ctrlKey) && e.key === '\\') { e.preventDefault(); cycleRail(); }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [cycleRail]);

  // Process actions
  const doAction = useCallback(async (action: string, pids: number[]) => {
    if (serverInfo?.readOnly) {
      notify('Read-only mode: control verbs are disabled on this remote session', 'error');
      return;
    }
    const label = action === 'kill' ? 'Terminate' : action.charAt(0).toUpperCase() + action.slice(1);
    let failures = 0;
    let lastMsg = '';
    for (const pid of pids) {
      try {
        let res;
        switch (action) {
          case 'suspend': res = await ProcService.Suspend(pid); break;
          case 'resume':  res = await ProcService.Resume(pid);  break;
          case 'kill':    res = await ProcService.Terminate(pid); break;
          case 'nice':    res = await ProcService.SetPriority(pid, 0); break;
          default: continue;
        }
        lastMsg = res.message;
        if (!res.ok) failures++;
      } catch (err: any) {
        lastMsg = err?.message || String(err);
        failures++;
      }
    }
    if (failures > 0) notify(`${label} failed for ${failures} process(es): ${lastMsg}`, 'error');
    else if (pids.length > 1) notify(`${label} sent to ${pids.length} processes`, 'ok');
    else notify(`${label} sent to pid ${pids[0]}`, 'ok');

    setProcSelection(new Set());
    refreshProcs();
  }, [notify, refreshProcs, serverInfo]);

  const handleAction = useCallback((action: string, pids: number[]) => {
    if (action === 'kill' && pids.length <= 3) {
      setConfirm({ action, pids });
    } else {
      doAction(action, pids);
    }
  }, [doAction]);

  // Compute per-kind summary
  const groups: Record<string, AccelGroup> = {};
  for (const d of devices) {
    const kind = d.deviceId.startsWith('gpu') ? 'gpu' : d.deviceId.startsWith('npu') ? 'npu' : 'tpu';
    if (!groups[kind]) {
      groups[kind] = {
        label: kind.toUpperCase(), color: kind === 'gpu' ? '#76b900' : kind === 'npu' ? '#facc15' : '#818cf8',
        deviceCount: 0, avgUtil: 0, avgPower: 0, totalVram: 0, totalVramMax: 0, spark: [],
      };
    }
    const g = groups[kind];
    g.deviceCount++;
    if (d.status !== 'n/a') {
      g.avgUtil += d.utilPct; g.avgPower += d.powerW; g.totalVram += d.vramUsed; g.totalVramMax += d.vramTotal;
    }
  }
  for (const g of Object.values(groups)) {
    if (g.deviceCount > 1) { g.avgUtil /= g.deviceCount; g.avgPower /= g.deviceCount; }
  }
  groups.gpu && (groups.gpu.spark = gpuSpark);
  groups.npu && (groups.npu.spark = npuSpark);
  groups.tpu && (groups.tpu.spark = tpuSpark);

  // Command palette items
  const paletteItems: PaletteItem[] = useMemo(() => [
    { id: 'v:overview',  kind: 'view', label: 'Overview',  sub: 'Dashboard', action: () => setView('overview') },
    { id: 'v:devices',   kind: 'view', label: 'Devices',   sub: 'GPU / NPU / TPU grid', action: () => setView('devices') },
    { id: 'v:processes', kind: 'view', label: 'Processes', sub: 'All running processes', action: () => setView('processes') },
    { id: 'v:workloads', kind: 'view', label: 'Workloads', sub: 'Training / Inference / Codec / Crypto', action: () => setView('workloads') },
    { id: 'v:alerts',    kind: 'view', label: 'Alerts',    sub: 'Rules & active fires', action: () => setView('alerts') },
    { id: 'v:history',   kind: 'view', label: 'History',   sub: 'Per-device metric charts', action: () => setView('history') },
    { id: 'v:sources',   kind: 'view', label: 'Sources',   sub: 'Enable telemetry sources & capabilities', action: () => setView('sources') },
    ...devices.map(d => ({
      id: `d:${d.deviceId}`, kind: 'device' as const,
      label: d.deviceId, sub: `${d.utilPct.toFixed(0)}% util · ${d.tempC}°C`,
      action: () => setView('devices'),
    })),
    ...accelProcs.slice(0, 50).map(p => ({
      id: `p:${p.pid}`, kind: 'process' as const,
      label: `${p.name} (pid ${p.pid})`, sub: `GPU ${p.utilPct.toFixed(0)}% · ${p.deviceId}`,
      action: () => setView('processes'),
    })),
  ], [devices, accelProcs]);

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-canvas text-zinc-100">
      <NavRail active={view} onNav={setView} mode={railMode} onToggleMode={cycleRail} />

      <div className="flex flex-col flex-1 min-w-0">
        <Header view={view} paused={paused} onPause={togglePause} onOpenPalette={() => setPaletteOpen(true)} server={serverInfo} />

        <main className="flex-1 overflow-y-auto p-4">
          {view === 'overview' && (
            <div className="space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
                {Object.values(groups).map(g => (
                  <KpiCard
                    key={g.label}
                    label={g.label}
                    value={g.avgUtil.toFixed(0)}
                    unit="%"
                    color={g.color}
                    devices={`${g.deviceCount} device${g.deviceCount > 1 ? 's' : ''}`}
                    detail={g.totalVramMax > 0 ? `${(g.totalVram / 1e9).toFixed(1)}/${(g.totalVramMax / 1e9).toFixed(0)} GB` : undefined}
                    sparkData={g.spark}
                  />
                ))}
                <KpiCard
                  label="Processes"
                  value={String(sysProcs.length)}
                  unit="active"
                  color="#60a5fa"
                  sparkData={[]}
                  devices=""
                />
              </div>

              <section>
                <h2 className="text-[10px] uppercase tracking-[0.12em] text-faint font-semibold mb-2">All Devices</h2>
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
                  {devices.map(d => <DeviceCard key={d.deviceId} device={d} />)}
                </div>
              </section>

              <section>
                <h2 className="text-[10px] uppercase tracking-[0.12em] text-faint font-semibold mb-2">Top Accelerator Processes</h2>
                <div className="rounded-lg bg-surface ring-1 ring-hairline overflow-hidden">
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="border-b border-hairline text-faint">
                        <th className="text-left px-3 py-2 font-medium">Process</th>
                        <th className="text-left px-3 py-2 font-medium">PID</th>
                        <th className="text-left px-3 py-2 font-medium">Device</th>
                        <th className="text-right px-3 py-2 font-medium">GPU %</th>
                        <th className="text-right px-3 py-2 font-medium">vMem</th>
                      </tr>
                    </thead>
                    <tbody>
                      {accelProcs.length === 0 && (
                        <tr><td colSpan={5} className="px-3 py-4 text-center text-faint">No processes using accelerators</td></tr>
                      )}
                      {[...accelProcs].sort((a, b) => b.utilPct - a.utilPct).map(p => (
                        <tr key={p.pid} className="border-b border-hairline/50 hover:bg-white/[0.02]">
                          <td className="px-3 py-1.5 text-zinc-200 font-medium">{p.name}</td>
                          <td className="px-3 py-1.5 text-faint tabular-nums">{p.pid}</td>
                          <td className="px-3 py-1.5">
                            <span className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium"
                              style={{
                                color: VENDOR_COLORS[guessVendor(p.deviceId)] ?? '#a1a1aa',
                                background: (VENDOR_COLORS[guessVendor(p.deviceId)] ?? '#a1a1aa') + '18',
                              }}
                            >
                              {p.deviceId}
                            </span>
                          </td>
                          <td className="px-3 py-1.5 text-right tabular-nums text-zinc-200">{p.utilPct.toFixed(0)}%</td>
                          <td className="px-3 py-1.5 text-right tabular-nums text-zinc-300">
                            {p.vramUsed > 0 ? `${(p.vramUsed / 1e9).toFixed(1)}G` : '—'}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
            </div>
          )}

          {view === 'devices' && (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
              {devices.map(d => <DeviceCard key={d.deviceId} device={d} />)}
            </div>
          )}

          {view === 'processes' && (
            <div className="rounded-lg bg-surface ring-1 ring-hairline overflow-hidden h-full">
              <ProcessTable
                procs={sysProcs}
                selection={procSelection}
                onSelect={setProcSelection}
                onAction={handleAction}
              />
            </div>
          )}

          {view === 'sources' && (
            <div className="rounded-lg bg-surface ring-1 ring-hairline overflow-hidden h-full p-2">
              <SourcesView notify={notify} />
            </div>
          )}

          {view === 'workloads' && (
            <div className="rounded-lg bg-surface ring-1 ring-hairline overflow-hidden h-full p-4">
              <WorkloadsView accelProcs={accelProcs} />
            </div>
          )}

          {view === 'alerts' && (
            <div className="rounded-lg bg-surface ring-1 ring-hairline overflow-hidden h-full p-4">
              <AlertsView onToast={notify} />
            </div>
          )}

          {view === 'history' && (
            <div className="rounded-lg bg-surface ring-1 ring-hairline overflow-hidden h-full p-4">
              <HistoryView devices={devices} />
            </div>
          )}
        </main>

        <StatusBar
          deviceCount={devices.length}
          gpuUtil={groups.gpu ? groups.gpu.avgUtil : null}
          npuUtil={groups.npu ? groups.npu.avgUtil : null}
          tpuUtil={groups.tpu ? groups.tpu.avgUtil : null}
        />
      </div>

      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} items={paletteItems} />
      <ToastList toasts={toasts} />

      {/* Confirm dialog */}
      {confirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={() => setConfirm(null)}>
          <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" />
          <div className="relative w-80 bg-surface border border-hairline rounded-xl p-4 shadow-2xl" onClick={e => e.stopPropagation()}>
            <p className="text-sm text-zinc-100 mb-3">
              Terminate {confirm.pids.length > 1 ? `${confirm.pids.length} processes` : `pid ${confirm.pids[0]}`}?
            </p>
            <p className="text-xs text-faint mb-4">This sends SIGTERM. The process may not exit cleanly.</p>
            <div className="flex gap-2 justify-end">
              <button onClick={() => setConfirm(null)} className="px-3 py-1 text-xs rounded bg-canvas/60 border border-hairline text-zinc-300 hover:text-zinc-100">Cancel</button>
              <button onClick={() => { doAction(confirm.action, confirm.pids); setConfirm(null); }} className="px-3 py-1 text-xs rounded bg-crit/20 text-crit border border-crit/40 hover:bg-crit/30">Terminate</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}