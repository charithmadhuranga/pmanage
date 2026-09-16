import { useEffect, useMemo, useState } from 'react';

import * as Engine from '../../bindings/pmanage/pkg/alerts/engine';
import * as AlertsModels from '../../bindings/pmanage/pkg/alerts';

import type { AlertRule, AlertFire } from '../lib/types';

interface Props {
  onToast: (msg: string, kind?: 'ok' | 'error' | 'info') => void;
}

const SEVERITY_STYLE: Record<string, { color: string; bg: string }> = {
  info: { color: '#38bdf8', bg: '#38bdf81a' },
  warn: { color: '#facc15', bg: '#facc151a' },
  crit: { color: '#f87171', bg: '#f871711a' },
};

const METRIC_LABEL: Record<string, string> = {
  util_pct: 'Utilization %',
  temp_c: 'Temp °C',
  power_w: 'Power W',
  vram_pct: 'VRAM %',
  vram_used: 'VRAM Used',
};

const DEFAULT_RULE: AlertRule = {
  id: '',
  name: '',
  metric: 'temp_c',
  entity: '*',
  operator: '>=',
  threshold: 80,
  action: 'toast',
  enabled: true,
  cooldownSec: 300,
};

export default function AlertsView({ onToast }: Props) {
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [fires, setFires] = useState<AlertFire[]>([]);
  const [draft, setDraft] = useState<AlertRule | null>(null);
  const [editing, setEditing] = useState(false);

  const load = useMemo(() => async () => {
    try {
      const [r, f] = await Promise.all([
        Engine.Rules() as Promise<AlertRule[]>,
        Engine.ActiveFires() as Promise<AlertFire[]>,
      ]);
      setRules(r ?? []);
      setFires(f ?? []);
    } catch { /* bindings not ready */ }
  }, []);

  useEffect(() => {
    load();
    const iv = setInterval(load, 5000);
    return () => clearInterval(iv);
  }, [load]);

  const upsert = async (rule: AlertRule) => {
    try {
      await Engine.UpsertRule(rule as AlertsModels.Rule);
      onToast(`Rule "${rule.name || 'untitled'}" saved`, 'ok');
      setDraft(null); setEditing(false);
      load();
    } catch (err: any) {
      onToast(`Save failed: ${err?.message ?? err}`, 'error');
    }
  };

  const toggle = async (rule: AlertRule) => {
    try {
      await Engine.SetRuleEnabled(rule.id, !rule.enabled);
      load();
    } catch (err: any) {
      onToast(`Toggle failed: ${err?.message ?? err}`, 'error');
    }
  };

  const remove = async (id: string) => {
    try {
      await Engine.DeleteRule(id);
      onToast('Rule deleted', 'ok');
      load();
    } catch (err: any) {
      onToast(`Delete failed: ${err?.message ?? err}`, 'error');
    }
  };

  const fmtVal = (r: AlertRule) =>
    r.metric === 'vram_used' ? `${(r.threshold / 1e9).toFixed(0)}GB` : String(r.threshold);

  return (
    <div className="space-y-4">
      {/* Active fires */}
      <section>
        <h2 className="text-[10px] uppercase tracking-[0.12em] text-faint font-semibold mb-2">
          Active Alerts {fires.length > 0 && <span className="text-crit">{fires.length}</span>}
        </h2>
        <div className="rounded-lg bg-surface ring-1 ring-hairline overflow-hidden">
          {fires.length === 0 && (
            <div className="px-3 py-4 text-center text-faint text-xs">No active alerts</div>
          )}
          {fires.map(f => {
            const st = SEVERITY_STYLE[f.severity] ?? SEVERITY_STYLE.info;
            return (
              <div key={f.id + f.firedAt} className="flex items-center gap-3 px-3 py-2 border-b border-hairline/50 last:border-0">
                <span className="w-2 h-2 rounded-full shrink-0" style={{ background: st.color }} />
                <div className="flex-1 min-w-0">
                  <div className="text-xs font-medium text-zinc-100 truncate">{f.message}</div>
                  <div className="text-[10px] text-faint">
                    {f.ruleName} · {f.deviceId} · {METRIC_LABEL[f.metric] ?? f.metric} = {f.value.toFixed(1)} · {new Date(f.firedAt * 1000).toLocaleTimeString()}
                  </div>
                </div>
                <span className="text-[10px] font-semibold px-1.5 py-0.5 rounded" style={{ color: st.color, background: st.bg }}>
                  {f.severity.toUpperCase()}
                </span>
              </div>
            );
          })}
        </div>
      </section>

      {/* Rules */}
      <section>
        <div className="flex items-center justify-between mb-2">
          <h2 className="text-[10px] uppercase tracking-[0.12em] text-faint font-semibold">Alert Rules</h2>
          <button
            onClick={() => { setDraft({ ...DEFAULT_RULE, id: crypto.randomUUID() }); setEditing(true); }}
            className="px-2 py-1 rounded text-[10px] font-medium bg-brand/15 text-brand border border-brand/30 hover:bg-brand/25"
          >
            + New rule
          </button>
        </div>

        {editing && draft && (
          <div className="rounded-lg bg-canvas/60 ring-1 ring-hairline p-3 mb-3 grid gap-3" style={{ gridTemplateColumns: 'repeat(auto-fit,minmax(140px,1fr))' }}>
            <label className="flex flex-col gap-1 text-[10px] text-faint">
              Name
              <input value={draft.name} onChange={e => setDraft({ ...draft, name: e.target.value })}
                placeholder="e.g. GPU hot" className="bg-surface border border-hairline rounded px-2 py-1 text-xs text-zinc-100 focus:border-brand/50 outline-none" />
            </label>
            <label className="flex flex-col gap-1 text-[10px] text-faint">
              Metric
              <select value={draft.metric} onChange={e => setDraft({ ...draft, metric: e.target.value as AlertRule['metric'] })}
                className="bg-surface border border-hairline rounded px-2 py-1 text-xs text-zinc-100 focus:border-brand/50 outline-none">
                {Object.entries(METRIC_LABEL).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
              </select>
            </label>
            <label className="flex flex-col gap-1 text-[10px] text-faint">
              Operator
              <select value={draft.operator} onChange={e => setDraft({ ...draft, operator: e.target.value as AlertRule['operator'] })}
                className="bg-surface border border-hairline rounded px-2 py-1 text-xs text-zinc-100 focus:border-brand/50 outline-none">
                <option value=">">above</option>
                <option value=">=">at or above</option>
                <option value="<">below</option>
                <option value="<=">at or below</option>
              </select>
            </label>
            <label className="flex flex-col gap-1 text-[10px] text-faint">
              Threshold
              <input type="number" value={draft.threshold} onChange={e => setDraft({ ...draft, threshold: Number(e.target.value) })}
                className="bg-surface border border-hairline rounded px-2 py-1 text-xs text-zinc-100 focus:border-brand/50 outline-none" />
            </label>
            <label className="flex flex-col gap-1 text-[10px] text-faint">
              Entity (device / *)
              <input value={draft.entity} onChange={e => setDraft({ ...draft, entity: e.target.value })} placeholder="gpu0 / npu / *"
                className="bg-surface border border-hairline rounded px-2 py-1 text-xs text-zinc-100 focus:border-brand/50 outline-none" />
            </label>
            <label className="flex flex-col gap-1 text-[10px] text-faint">
              Action
              <select value={draft.action} onChange={e => setDraft({ ...draft, action: e.target.value as AlertRule['action'] })}
                className="bg-surface border border-hairline rounded px-2 py-1 text-xs text-zinc-100 focus:border-brand/50 outline-none">
                <option value="toast">toast</option>
                <option value="notify">desktop notify</option>
                <option value="auto-suspend">auto-suspend</option>
                <option value="auto-terminate">auto-terminate</option>
              </select>
            </label>
            <label className="flex flex-col gap-1 text-[10px] text-faint">
              Cooldown (s)
              <input type="number" value={draft.cooldownSec} onChange={e => setDraft({ ...draft, cooldownSec: Number(e.target.value) })}
                className="bg-surface border border-hairline rounded px-2 py-1 text-xs text-zinc-100 focus:border-brand/50 outline-none" />
            </label>
            <div className="flex items-end gap-2">
              <button onClick={() => upsert(draft)}
                className="px-3 py-1.5 rounded text-[11px] font-medium bg-brand/20 text-brand border border-brand/40 hover:bg-brand/30">
                Save
              </button>
              <button onClick={() => { setDraft(null); setEditing(false); }}
                className="px-3 py-1.5 rounded text-[11px] text-faint border border-hairline hover:text-zinc-200">
                Cancel
              </button>
            </div>
          </div>
        )}

        <div className="rounded-lg bg-surface ring-1 ring-hairline overflow-hidden">
          {rules.length === 0 && (
            <div className="px-3 py-4 text-center text-faint text-xs">No rules yet — add one above</div>
          )}
          {rules.map(r => (
            <div key={r.id} className="flex items-center gap-3 px-3 py-2 border-b border-hairline/50 last:border-0">
              {/* toggle */}
              <button
                onClick={() => toggle(r)}
                className={`relative w-8 h-4.5 h-[18px] rounded-full transition-colors ${r.enabled ? 'bg-ok/70' : 'bg-zinc-700'}`}
                title={r.enabled ? 'Disable' : 'Enable'}
              >
                <span className="absolute top-[2px] w-3.5 h-3.5 rounded-full bg-zinc-100 transition-all"
                  style={{ left: r.enabled ? '14px' : '2px' }} />
              </button>
              <div className="flex-1 min-w-0">
                <div className="text-xs font-medium text-zinc-100">{r.name}</div>
                <div className="text-[10px] text-faint">
                  {METRIC_LABEL[r.metric]} {r.operator} {fmtVal(r)} · entity <code className="text-zinc-300">{r.entity}</code> · {r.cooldownSec}s cooldown
                </div>
              </div>
              <span className={`text-[10px] font-semibold px-1.5 py-0.5 rounded ${r.enabled ? 'text-ok bg-ok/10' : 'text-faint bg-white/5'}`}>
                {r.enabled ? 'ON' : 'OFF'}
              </span>
              <button onClick={() => { setDraft({ ...r }); setEditing(true); }}
                className="text-[10px] text-faint hover:text-zinc-200 px-1.5 py-0.5 rounded hover:bg-white/5">Edit</button>
              <button onClick={() => remove(r.id)}
                className="text-[10px] text-faint hover:text-crit px-1.5 py-0.5 rounded hover:bg-crit/10">Delete</button>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}