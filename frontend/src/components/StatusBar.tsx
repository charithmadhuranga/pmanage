interface Props {
  deviceCount: number;
  gpuUtil: number | null;
  npuUtil: number | null;
  tpuUtil: number | null;
}

export default function StatusBar({ deviceCount, gpuUtil, npuUtil, tpuUtil }: Props) {
  return (
    <footer className="h-6 flex items-center px-4 border-t border-hairline text-[11px] text-faint font-mono shrink-0 gap-2">
      <span className="flex items-center gap-1.5">
        <span className="w-1.5 h-1.5 rounded-full bg-ok" />
        {deviceCount} accelerators
      </span>
      <span className="text-hairline-strong">│</span>
      {gpuUtil !== null && <span>GPU {gpuUtil.toFixed(0)}%</span>}
      {npuUtil !== null && <>
        <span className="text-hairline-strong">·</span>
        <span>NPU {npuUtil.toFixed(0)}%</span>
      </>}
      {tpuUtil !== null && <>
        <span className="text-hairline-strong">·</span>
        <span>TPU {tpuUtil.toFixed(0)}%</span>
      </>}
      <span className="ml-auto text-faint">v0.1.0</span>
    </footer>
  );
}
