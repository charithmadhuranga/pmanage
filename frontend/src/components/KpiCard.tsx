interface Props {
  label: string;
  value: string;
  unit?: string;
  color: string;
  sparkData: number[];
  detail?: string;
  devices?: string;
}

export default function KpiCard({ label, value, unit, color, sparkData, detail, devices }: Props) {
  return (
    <section className="rounded-lg bg-surface ring-1 ring-hairline overflow-hidden flex flex-col">
      <header className="flex items-center justify-between px-3 h-8 border-b border-hairline">
        <h2 className="text-[10px] uppercase tracking-[0.12em] text-faint font-semibold">{label}</h2>
        {devices && <span className="text-[11px] text-faint">{devices}</span>}
      </header>
      <div className="px-3 pt-3 pb-1 flex items-baseline justify-between">
        <div className="flex items-baseline gap-1">
          <span className="text-2xl font-semibold num" style={{ color }}>{value}</span>
          {unit && <span className="text-xs text-faint">{unit}</span>}
        </div>
        {detail && <span className="text-[11px] text-faint">{detail}</span>}
      </div>
      <div className="px-3 pb-2 flex-1">
        <Sparkline data={sparkData} color={color} />
      </div>
    </section>
  );
}

function Sparkline({ data, color }: { data: number[]; color: string }) {
  // Inline tiny SVG sparkline (no ECharts overhead for single-value KPI spark)
  if (data.length < 2) return null;
  const max = Math.max(...data, 1);
  const w = 200;
  const h = 24;
  const pts = data.map((v, i) => {
    const x = (i / (data.length - 1)) * w;
    const y = h - (v / max) * h;
    return `${x},${y}`;
  }).join(' ');
  const areaPts = `0,${h} ${pts} ${w},${h}`;
  return (
    <svg viewBox={`0 0 ${w} ${h}`} className="w-full" style={{ height: h }} preserveAspectRatio="none">
      <polygon points={areaPts} fill={color + '18'} />
      <polyline points={pts} fill="none" stroke={color} strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}
