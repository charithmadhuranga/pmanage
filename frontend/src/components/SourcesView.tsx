import { useEffect, useState } from 'react';

import { Service as SettingsService } from '../../bindings/pmanage/pkg/settings';
import type { SourceInfo } from '../lib/types';

interface Props {
  notify: (msg: string, kind?: 'ok' | 'error' | 'info') => void;
}

const KIND_BADGES: Record<string, string> = {
  gpu: 'bg-brand/10 text-brand border-brand/30',
  npu: 'bg-purple-400/10 text-purple-300 border-purple-400/30',
  tpu: 'bg-green-400/10 text-green-300 border-green-400/30',
};

export default function SourcesView({ notify }: Props) {
  const [sources, setSources] = useState<SourceInfo[]>([]);
  const [loading, setLoading] = useState(true);

  const refresh = () => {
    SettingsService.ListSources()
      .then(s => setSources(s ?? []))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    refresh();
    const iv = setInterval(refresh, 2000);
    return () => clearInterval(iv);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const toggle = (name: string, on: boolean) => {
    SettingsService.SetSourceEnabled(name, on)
      .then(() => {
        notify(`${name} ${on ? 'enabled' : 'disabled'}`, 'ok');
        refresh();
      })
      .catch(e => notify(`failed to ${on ? 'enable' : 'disable'} ${name}: ${String(e)}`, 'error'));
  };

  return (
    <div className="p-4 flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold text-zinc-100">Telemetry sources</h2>
        <span className="text-[10px] text-faint">Re-detected on toggle · 2 s refresh</span>
      </div>

      {loading ? (
        <div className="text-sm text-faint animate-pulse">Loading sources…</div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
          {sources.map(s => (
            <div key={s.name} className="rounded-lg border border-hairline bg-surface/60 flex flex-col gap-3 p-3">
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-zinc-100">{s.name}</span>
                  <span className={`text-[9px] px-1.5 py-0.5 rounded border ${KIND_BADGES[s.kind] || 'border-hairline text-faint'}`}>
                    {s.kind.toUpperCase()}
                  </span>
                </div>
                <button
                  onClick={() => toggle(s.name, !s.enabled)}
                  className={`w-8 h-4 rounded-full transition-colors relative ${
                    s.enabled ? 'bg-brand/80' : 'bg-hairline'
                  }`}
                  title={s.enabled ? 'Disable source' : 'Enable source'}
                  aria-pressed={s.enabled}
                >
                  <span
                    className={`absolute top-0.5 w-3 h-3 rounded-full bg-white transition-all ${
                      s.enabled ? 'left-[18px]' : 'left-0.5'
                    }`}
                  />
                </button>
              </div>

              <dl className="text-[11px] flex flex-col gap-1">
                <div className="flex justify-between">
                  <dt className="text-faint">Detected</dt>
                  <dd className={s.detected ? 'text-ok' : 'text-faint'}>
                    {s.detected ? 'yes' : 'no'}
                  </dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-faint">Devices</dt>
                  <dd className="text-zinc-200">{s.deviceCount}</dd>
                </div>
              </dl>

              <div className="flex flex-wrap gap-1">
                {(s.capabilities?.length ?? 0) === 0 && (
                  <span className="text-[9px] text-faint">no capabilities advertised</span>
                )}
                {s.capabilities?.map(c => (
                  <span key={c} className="text-[9px] px-1.5 py-0.5 rounded bg-canvas/60 border border-hairline text-zinc-300">
                    {c}
                  </span>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}