import { useMemo } from 'react';
import { useEChart } from '../hooks/useEChart';
import type * as echarts from 'echarts';

interface Props {
  data: number[];
  color: string;
  height?: number;
  max?: number;
}

export default function Sparkline({ data, color, height = 28, max = 100 }: Props) {
  const indices = useMemo(() => data.map((_, i) => i), [data]);

  const option: echarts.EChartsOption = useMemo(() => ({
    grid: { top: 2, right: 0, bottom: 2, left: 0 },
    xAxis: { type: 'category', show: false, data: indices },
    yAxis: { type: 'value', show: false, min: 0, max },
    series: [{
      type: 'line',
      data,
      smooth: true,
      symbol: 'none',
      lineStyle: { color, width: 1.5 },
      areaStyle: {
        color: {
          type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [
            { offset: 0, color: color + '30' },
            { offset: 1, color: color + '05' },
          ],
        },
      },
    }],
    animation: false,
  }), [data, indices, color, max]);

  const chartRef = useEChart(option, [option]);

  return <div ref={chartRef} style={{ width: '100%', height }} />;
}
