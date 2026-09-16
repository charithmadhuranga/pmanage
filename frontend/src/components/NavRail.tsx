import { useState } from 'react';

export type RailMode = 'expanded' | 'collapsed' | 'hidden';

interface Props {
  active: string;
  onNav: (id: string) => void;
  mode: RailMode;
  onToggleMode: () => void;
}

const NAV_ITEMS = [
  { id: 'overview',  label: 'Overview',  icon: 'M4 6h16M4 12h16M4 18h16' },
  { id: 'devices',   label: 'Devices',   icon: 'M9 3v2m6-2v2M9 19v2m6-2v2M3 9h2m-2 6h2m14-6h2m-2 6h2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z' },
  { id: 'processes', label: 'Processes', icon: 'M4 6h16M4 10h16M4 14h10M4 18h7' },
  { id: 'workloads', label: 'Workloads', icon: 'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z' },
  { id: 'alerts',    label: 'Alerts',    icon: 'M12 9v4m0 4h.01M5 19a9 9 0 1114 0H5z' },
  { id: 'history',   label: 'History',   icon: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z' },
  { id: 'sources',   label: 'Sources',   icon: 'M4 7h16M4 12h10M4 17h16M16 12l3 3 5-5' },
];

export default function NavRail({ active, onNav, mode, onToggleMode }: Props) {
  const [hovered, setHovered] = useState(false);

  const isExpanded = mode === 'expanded' || (mode === 'collapsed' && hovered);
  const isHidden = mode === 'hidden' && !hovered;

  const width = isHidden ? 0 : isExpanded ? 'w-44' : 'w-12';

  return (
    <nav
      onMouseEnter={() => mode !== 'expanded' && setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className={`relative shrink-0 bg-surface border-r border-hairline transition-all duration-150 flex flex-col
        ${width} ${isHidden ? 'overflow-visible' : 'overflow-hidden'}`}
    >
      {/* Hidden mode: show edge handle */}
      {mode === 'hidden' && !hovered && (
        <button
          onClick={onToggleMode}
          className="absolute -left-0 top-1/2 -translate-y-1/2 w-1.5 h-12 bg-hairline/60 hover:bg-brand/60 rounded-r cursor-pointer transition-colors z-10"
          title="Show navigation"
        />
      )}

      {/* Logo */}
      <div className="h-9 flex items-center justify-between px-2 shrink-0 border-b border-hairline/60">
        <div className="flex items-center gap-2">
          <div className="w-7 h-7 rounded-md bg-brand/15 flex items-center justify-center shrink-0">
            <svg viewBox="0 0 24 24" className="w-4 h-4" fill="none" stroke="#22d3ee" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <path d="M12 2L2 7l10 5 10-5-10-5z" />
              <path d="M2 17l10 5 10-5" />
              <path d="M2 12l10 5 10-5" />
            </svg>
          </div>
          {isExpanded && <span className="text-sm font-semibold text-zinc-100 whitespace-nowrap">pmanage</span>}
        </div>
        {isExpanded && (
          <button
            onClick={onToggleMode}
            className="w-6 h-6 flex items-center justify-center rounded text-faint hover:text-zinc-200 hover:bg-white/5"
            title="Collapse rail (⌘\\)"
          >
            <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><path d="M15 18l-6-6 6-6"/></svg>
          </button>
        )}
      </div>

      {/* Nav items */}
      <div className="flex-1 flex flex-col items-center gap-1 pt-3 px-1.5">
        {NAV_ITEMS.map(item => (
          <button
            key={item.id}
            onClick={() => onNav(item.id)}
            title={item.label}
            className={`w-9 h-9 flex items-center justify-center rounded-md transition-colors
              ${active === item.id
                ? 'bg-brand/10 text-brand'
                : 'text-faint hover:bg-white/5 hover:text-zinc-200'}`}
          >
            <svg viewBox="0 0 24 24" className="w-[18px] h-[18px]" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <path d={item.icon} />
            </svg>
          </button>
        ))}
      </div>

      {/* Bottom: show expand button when collapsed */}
      {!isExpanded && mode !== 'hidden' && (
        <button
          onClick={onToggleMode}
          className="mx-auto mb-3 w-8 h-8 flex items-center justify-center rounded-md text-faint hover:text-zinc-200 hover:bg-white/5"
          title="Expand rail (⌘\\)"
        >
          <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><path d="M9 18l6-6-6-6"/></svg>
        </button>
      )}
    </nav>
  );
}