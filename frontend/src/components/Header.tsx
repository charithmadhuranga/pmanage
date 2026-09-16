interface ServerInfo {
  enabled: boolean;
  address: string;
  auth: boolean;
  readOnly: boolean;
  remoteAdj: boolean;
}

interface Props {
  view: string;
  paused: boolean;
  onPause: () => void;
  onOpenPalette: () => void;
  server?: ServerInfo | null;
}

const VIEW_NAMES: Record<string, string> = {
  overview: 'Overview',
  devices: 'Devices',
  processes: 'Processes',
  workloads: 'Workloads',
  alerts: 'Alerts',
  history: 'History',
  sources: 'Sources',
};

export default function Header({ view, paused, onPause, onOpenPalette, server }: Props) {
  return (
    <header className="h-9 flex items-center justify-between px-4 bg-surface/80 backdrop-blur-sm border-b border-hairline shrink-0">
      {/* Left: breadcrumb */}
      <div className="flex items-center gap-2 text-sm">
        <span className="text-zinc-100 font-medium">{VIEW_NAMES[view] || 'Overview'}</span>
        {server?.enabled && (
          <span
            className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[10px] font-medium"
            style={server.readOnly
              ? { color: '#facc15', background: '#facc1518', border: '1px solid #facc1533' }
              : { color: '#34d399', background: '#34d39918', border: '1px solid #34d39933' }}
            title={`Remote server · ${server.address}`}
          >
            <span className="w-1.5 h-1.5 rounded-full" style={{ background: server.readOnly ? '#facc15' : '#34d399' }} />
            Server {server.readOnly ? '✓ read-only' : 'remote-admin'}
          </span>
        )}
      </div>

      {/* Right: controls */}
      <div className="flex items-center gap-3">
        {/* Palette trigger */}
        <button
          onClick={onOpenPalette}
          className="flex items-center gap-1.5 px-2 py-1 rounded bg-canvas/60 border border-hairline text-faint hover:text-zinc-200 hover:border-brand/40 transition-colors"
          title="Search (⌘K)"
        >
          <svg className="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/></svg>
          <span className="text-[10px]">Search…</span>
          <kbd className="text-[9px] px-1 rounded bg-surface/80 border border-hairline text-faint">⌘K</kbd>
        </button>

        {/* Pause/Resume */}
        <button
          onClick={onPause}
          className={`flex items-center gap-1.5 px-2 py-1 rounded text-[10px] font-medium transition-colors
            ${paused
              ? 'bg-warn/10 text-warn border border-warn/30 hover:bg-warn/20'
              : 'bg-ok/10 text-ok border border-ok/30 hover:bg-ok/20'}`}
          title={paused ? 'Resume sampling' : 'Pause sampling'}
        >
          {paused ? (
            <svg className="w-3 h-3" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>
          ) : (
            <svg className="w-3 h-3" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>
          )}
          {paused ? 'Paused' : 'Live 1 Hz'}
        </button>
      </div>
    </header>
  );
}