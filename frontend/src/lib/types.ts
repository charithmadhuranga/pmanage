export interface DeviceInfo {
  id: string;
  name: string;
  vendor: string;
  kind: 'gpu' | 'npu' | 'tpu';
  driver: string;
}

export interface DeviceMetrics {
  deviceId: string;
  utilPct: number;
  vramUsed: number;
  vramTotal: number;
  tempC: number;
  powerW: number;
  clockMhz: number;
  clockMaxMhz: number;
  fanPct: number;
  status: string;
  minKernel?: string;
  memUtilRate?: number;
  effectiveLoad?: number;
  powerMaxW?: number;
  validFields?: string[];
}

export function hasValid(device: DeviceMetrics, field: string): boolean {
  return device.validFields?.includes(field) ?? false;
}

export interface ProcUsage {
  pid: number;
  name: string;
  deviceId: string;
  vramUsed: number;
  utilPct: number;
  engine?: Record<string, number>;
  username?: string;
  cpuUsage?: number;
  memResident?: number;
  memVirtual?: number;
  type?: string;
  validFields?: string[];
}

export function hasProcValid(proc: ProcUsage, field: string): boolean {
  return proc.validFields?.includes(field) ?? false;
}

export interface TelemetrySnapshot {
  devices: DeviceMetrics[];
  procs: ProcUsage[];
  ts: number;
}

/* Phase 1 — process + system types (mirror bindings) */
export interface ProcessInfo {
  pid: number;
  ppid: number;
  name: string;
  username: string;
  status: string;
  cpu: number;
  memBytes: number;
  memPct: number;
  threads: number;
  createTime: number;
  elapsedSec: number;
}

export interface OpResult {
  pid: number;
  action: string;
  ok: boolean;
  code: string;
  message: string;
}

export interface HostInfo {
  os: string;
  platform: string;
  platformVer: string;
  kernelVer: string;
  arch: string;
  hostname: string;
  uptimeSec: number;
  numCpu: number;
  memTotal: number;
  memUsed: number;
  memPct: number;
}

export type MergedProcess = ProcessInfo & { accel?: ProcUsage };

export interface Toast {
  id: number;
  kind: 'ok' | 'error' | 'info';
  msg: string;
}

/* Sources/settings view */
export interface SourceInfo {
  name: string;
  kind: 'gpu' | 'npu' | 'tpu';
  enabled: boolean;
  detected: boolean;
  deviceCount: number;
  capabilities: string[];
}

/* Phase 6 — history, alerts, workload classifier */
export interface HistoryPoint {
  ts: number;
  utilPct: number;
  vramUsed: number;
  vramTotal: number;
  tempC: number;
  powerW: number;
  clockMhz: number;
  fanPct: number;
}

export type AlertMetric = 'util_pct' | 'temp_c' | 'power_w' | 'vram_pct' | 'vram_used';
export type AlertOperator = '>' | '>=' | '<' | '<=';
export type AlertAction = 'toast' | 'notify' | 'auto-suspend' | 'auto-terminate';

export interface AlertRule {
  id: string;
  name: string;
  metric: AlertMetric;
  entity: string; // device id prefix, "*", or "proc" for zombie rule
  operator: AlertOperator;
  threshold: number;
  action: AlertAction;
  enabled: boolean;
  cooldownSec: number;
  message?: string;
}

export interface AlertFire {
  id: string;
  ruleName: string;
  message: string;
  deviceId: string;
  metric: AlertMetric;
  value: number;
  severity: 'info' | 'warn' | 'crit';
  action: AlertAction;
  firedAt: number;
}

export type WorkloadKind = 'training' | 'inference' | 'codec' | 'crypto' | 'unknown';

export interface ClassifiedProc {
  pid: number;
  kind: WorkloadKind;
  score: number;
  hits: { rule: string; signal: string; score: number }[];
}
