import type { DeviceMetrics } from '../lib/types';

interface Props {
  device: DeviceMetrics;
}

const VENDOR_COLORS: Record<string, string> = {
  gpu0: '#76b900', // nvidia
  gpu1: '#e8744a', // amd
  gpu2: '#38bdf8', // intel
  tpu0: '#818cf8', // google
};

function formatBytes(b: number): string {
  if (b === 0) return '—';
  if (b >= 1e12) return (b / 1e12).toFixed(1) + ' TB';
  if (b >= 1e9) return (b / 1e9).toFixed(1) + ' GB';
  if (b >= 1e6) return (b / 1e6).toFixed(0) + ' MB';
  return (b / 1e3).toFixed(0) + ' KB';
}

function statusColor(status: string): string {
  switch (status) {
    case 'ok': return 'bg-ok';
    case 'busy': return 'bg-warn';
    case 'critical': return 'bg-crit';
    default: return 'bg-faint';
  }
}

function UtilBar({ pct, color }: { pct: number; color: string }) {
  const p = Math.min(pct, 100);
  return (
    <div className="flex items-center gap-2">
      <div className="flex-1 h-1.5 rounded-full bg-white/5 overflow-hidden">
        {pct >= 0 && (
          <div
            className="h-full rounded-full transition-all duration-700"
            style={{ width: `${p}%`, background: color }}
          />
        )}
      </div>
      <span className="text-xs num tabular-nums w-10 text-right" style={{ color: pct >= 0 ? color : undefined }}>
        {pct >= 0 ? `${p.toFixed(0)}%` : 'N/A'}
      </span>
    </div>
  );
}

export default function DeviceCard({ device }: Props) {
  const color = VENDOR_COLORS[device.deviceId] ?? '#a1a1aa';
  const isNA = device.status === 'n/a';

  return (
    <div className="rounded-lg bg-surface ring-1 ring-hairline p-3 flex flex-col gap-2 min-h-[110px]">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className={`w-2 h-2 rounded-full ${statusColor(device.status)}`} />
          <span className="text-xs font-medium text-zinc-100 truncate max-w-[160px]">{device.deviceId}</span>
        </div>
        {!isNA && (
          <span className="text-[10px] text-faint num">{device.tempC > 0 ? `${device.tempC.toFixed(0)}°C` : ''}</span>
        )}
      </div>

      {isNA ? (
        <div className="flex-1 flex items-center justify-center">
          <span className="text-[11px] text-faint border border-dashed border-hairline-strong rounded px-2 py-1">
            N/A — metrics not exposed
          </span>
        </div>
      ) : (
        <>
          {/* Kernel gate (Linux drm fdinfo): explains honest N/A per-process cells */}
          {device.minKernel && (
            <div
              className="text-[9px] text-faint/80 border border-hairline rounded px-1.5 py-0.5 inline-block w-fit"
              title={`Per-process metrics require Linux kernel ≥ ${device.minKernel}`}
            >
              needs kernel ≥ {device.minKernel}
            </div>
          )}

          {/* Utilization */}
          <div className="mt-1">
            <UtilBar pct={device.utilPct} color={color} />
          </div>

          {/* Memory */}
          <div className="text-[11px] text-faint">
            {device.vramTotal > 0 ? (
              <>{formatBytes(device.vramUsed)} / {formatBytes(device.vramTotal)}</>
            ) : (
              <span className="text-faint/60">Shared memory</span>
            )}
          </div>

          {/* Power / Clock */}
          <div className="flex items-center justify-between text-[10px] text-faint/80">
            {device.powerW > 0 && <span>{device.powerW.toFixed(0)}W</span>}
            {device.clockMhz > 0 && <span>{device.clockMhz} MHz</span>}
          </div>
        </>
      )}
    </div>
  );
}
