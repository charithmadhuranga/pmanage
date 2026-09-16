# PMANAGE — UI/UX Mockup (v0.1)

One-stop GPU / NPU / TPU process manager.
Stack: **Wails v3 (Go)** + **React 18 / TypeScript** + **Tailwind CSS v4** + **ECharts**.

This document is the single source of truth for look & feel. It defines the theme, the app
shell, the shared component inventory, every view, the ECharts recipes, and the data wiring.

---

## 1. Design Principles

1. **Dense, not cramped.** This is a pro tool (Task Manager + nvitop + Grafana). Rows are
   28-32px, table fonts 12px, numbers `tabular-nums`. Every pixel that isn't data is quiet.
2. **Dark by default, high contrast.** `zinc-950` canvas, faint hairline borders, one strong
   accent. Data pops; chrome recedes.
3. **Honesty about gaps.** No per-process NPU? Show a `N/A` chip with an explanatory tooltip —
   never a fake number. ("TPU detected — metrics not exposed by vendor")
4. **Glanceable → drillable.** Every card/page starts as a glanceable summary; one click drills
   into detail. Nothing demands reading before acting.
5. **Keyboard-first.** htop/nvitop users expect keys. Global shortcuts for every primary action.
6. **Live but calm.** 1 Hz telemetry, animated sparklines, no strobing, `prefers-reduced-motion`
   respected.
7. **Consistent typography.** Inter (or system-ui on macOS). Uppercase micro-labels (`text-[10px]
   tracking-widest text-zinc-500`) for every metric group. Numbers always `tabular-nums`.

---

## 2. Design Tokens (Tailwind v4 `@theme`)

Tailwind v4 config lives in CSS. Replace `frontend/src/style.css` with:

```css
@import "tailwindcss";

@theme {
  /* Surfaces */
  --color-canvas:   #09090b;   /* app background                    */
  --color-surface:  #121214;   /* cards / panels                    */
  --color-raised:   #18181b;   /* elevated (menus, toolbars)        */
  --color-inset:    #0d0d0f;   /* wells, chart backgrounds          */
  --color-hairline: rgb(255 255 255 / 0.07);
  --color-hairline-strong: rgb(255 255 255 / 0.12);

  /* Brand + status */
  --color-brand:    #22d3ee;   /* cyan — primary accent             */
  --color-ok:       #34d399;
  --color-warn:     #fbbf24;
  --color-crit:     #f87171;
  --color-info:     #60a5fa;
  --color-muted:    #a1a1aa;   /* secondary text (zinc-400)         */
  --color-faint:    #71717a;   /* tertiary text (zinc-500)          */

  /* Vendor accents (badges / dots only — never status) */
  --color-nvidia:   #76b900;
  --color-amd:      #e8744a;
  --color-intel:    #38bdf8;
  --color-apple:    #d4d4d8;
  --color-hailo:    #facc15;
  --color-rockchip: #93c5fd;
  --color-google:   #818cf8;

  /* Typography + radii */
  --font-sans: "Inter", ui-sans-serif, system-ui, -apple-system, sans-serif;
  --font-mono: "JetBrains Mono", ui-monospace, "SF Mono", menlo, monospace;
  --radius-btn: 6px;
  --radius-card: 8px;
  --radius-chip: 999px;
}

:root {
  font-feature-settings: "cv11";          /* Inter tabular-nums variant */
  color-scheme: dark;
}
.num { font-variant-numeric: tabular-nums; }   /* alias utility for all numbers */
```

### Typography scale (compact)

| Role                  | Class                        | Notes                          |
| --------------------- | ---------------------------- | ------------------------------ |
| App title / headings  | `text-sm font-semibold`      | UI keeps ≤ 14px titles         |
| Section label         | `text-[10px] uppercase tracking-[0.12em] text-faint font-semibold` | every card header |
| KPI values            | `text-2xl font-semibold num` | the headline number            |
| Device card big util  | `text-3xl font-bold num`     | ring-gauge center              |
| Table data            | `text-xs text-zinc-300`      | all table rows                 |
| Table numeric cells   | `text-xs num tabular-nums`   | alignment-critical             |
| Micro metadata        | `text-[11px] text-faint`     | PIDs, times, units             |

### Status → color map (apply to util %, temp, power, mem load bars)

| State      | Hue     | Threshold example                                |
| ---------- | ------- | ------------------------------------------------ |
| idle       | `ok`    | util < 30%, temp < 70°C, VRAM < 60%              |
| active     | `brand` | util 30-80%                                      |
| busy/hot   | `warn`  | util > 80% or temp 80-92°C                       |
| critical   | `crit`  | temp > 92°C, ECC errors, OOM, process hang       |

---

## 3. App Shell

```
┌───┬──────────────────────────────────────────────────────────────┐
│   │  HEADER (h-9)                                                │
│ 🛰 │  ▸ Overview        ⌘K Search…               ● Live 1 Hz ⢐  │
├───┼──────────────────────────────────────────────────────────────┤
│ N │                                                              │
│ A │  VIEW CONTENT (flex-1, overflow-y-auto, p-3)                 │
│ V │                                                              │
│ │ │                                                              │
│ R │                                                              │
│ A │                                                              │
│ I │                                                              │
│ L │                                                              │
├───┼──────────────────────────────────────────────────────────────┤
│   │  STATUS BAR (h-6)                                            │
│   │  ● 6 accelerators │ GPU avg 64% │ NPU avg 12% │ 1 Hz │ ⎇ v0.1.0 │
└───┴──────────────────────────────────────────────────────────────┘
```

### 3.1 Navigation rail — 3 states

- **Collapsed:** 48px icon rail (default at ≥1000px window).
- **Expanded:** 176px labeled rail (`⌘⇧B`).
- **Hidden:** full 100%-content mode (`⌘B`) — one-click return via header hamburger.

| # | Item           | Icon        | Badge                     |
| - | -------------- | ----------- | ------------------------- |
| 1 | Overview       | grid        | aggregate util            |
| 2 | Devices        | cpu         | count of detected devices |
| 3 | Processes      | list        | # processes using accel.  |
| 4 | Workloads      | layers      | # ML workloads classified |
| 5 | Alerts         | bell        | active alert count (crit red) |
| 6 | History        | chart-line  | —                         |
| 7 | Settings       | gear        | —                         |

Rail item: `flex items-center gap-2.5 px-3 h-9 rounded-md text-xs text-muted hover:bg-white/5
hover:text-zinc-100`. Active: `bg-brand/10 text-brand ring-1 ring-brand/20`.

Also pinned to the rail **top** (always visible, like Activity Monitor): a compact per-vendor
mini-strip showing combined util — micro 3px segmented bars. This gives the whole "fleet" pulse
without leaving any view.

### 3.2 Header (h-9, `bg-surface/80 backdrop-blur border-b border-hairline`)

Left: breadcrumb `Overview` (current view). Center-right:

- **Global search** `⌘K` → Command Palette (Linear/Raycast style): jump to device, filter
  processes by PID/name, run actions (`kill <pid>`, `toggle compute mode`).
- **Live pill:** `● Live` (sync indicator) — green when sample ≤ 1.5s old, amber when 1.5-5s (slow
  collector/source), red when stale. `⎇ 1 Hz` drops down to choose 0.5 / 1 / 2 / 5 Hz.
- **Right cluster:** pause/resume sampling (`␣`), alert count chip, settings shortcut.

### 3.3 Status bar (h-6, `border-t border-hairline text-[11px] text-faint font-mono`)

`● 6 accelerators │ GPU 64% · NPU 12% · TPU — │ collecting… │ v0.1.0` — left-justified fleet
summary, right-justified version + driver/library status icons.

---

## 4. Shared Component Inventory

All cards share this skeleton:

```html
<section class="rounded-lg bg-surface ring-1 ring-hairline overflow-hidden">
  <header class="flex items-center justify-between px-3 h-8 border-b border-hairline">
    <h2 class="text-[10px] uppercase tracking-[0.12em] text-faint font-semibold">Section</h2>
    <div class="flex items-center gap-2">…actions…</div>
  </header>
  <div class="p-3">…</div>
</section>
```

### 4.1 KpiCard

```
┌──────────────────────────────┐
│ GPUS         2 ▼  ▲ ⟳        │  ← section label + expand/collapse
│ ───────────────────────────  │
│  64%  ▓▓▓▓▓▓▓▓▓░░   │ 12/24GB│  ← big num w/ inline bar
│  48°C  240W  ──◉──  │  ●live │  ← metadata row
│        ▁▂▄▇▅▂▁ spark         │  ← ECharts mini area  → 4.5
└──────────────────────────────┘
```

Used as: **GPU / NPU / TPU fleet cards** (per-family aggregate) and small `summary` variants
(no sparkline) for header widgets. Total width fixed `w-56`, min-width `w-48`.

### 4.2 DeviceCard (grid tile, `h-40`)

```
┌──────────────────────────────┐
│ 🟢  GPU 0 · RTX 4090   [⟳][x]│   vendor dot + name + chips
│  ╭───╮   83%           ┌──┐  │
│  │   │   Util           │EN│  │   EN = enc/dec mini gauges
│  ╰───╯   ████ 76%      └──┘  │   ring gauge center = util
│  18.6/24 GB · 61°C · 330W    │
│  ▁▂▄▇▅▂▁▃  sparkline    5 procs│
└──────────────────────────────┘
```

Left: **ECharts radial gauge** (72px) — util in center, gradient arc, warn/crit color zones.
Right column: EN/DEC/V/MEM ring micro-gauges (28px). Bottom: dense metric line + sparkline +
click target → opens Device Detail.

### 4.3 MetricBar (inline utilization bar)

`w-24 h-1.5 rounded-full bg-white/10` + segmented fill. Used inside table cells, KPI rows,
process rows whenever a % must inline. Color follows status map.

### 4.4 ProcessTable (the heart of the app)

Dense table, `text-xs`, row `h-7`, alternating rows only on hover (`hover:bg-white/[0.03]`).

```
H:  PROC │  PID │ USER │ ACCEL ⬍  │ GPU% │ vMEM  │ NPU% │ CPU% │ HOST MEM │  STATUS │ ACTIONS
R1: python·train  4210  root  NVIDIA▸ 83% ██  12.4G▐  —     █    3.2%▐  8.1G▐ R▸ ▰ ⨯
R2: ffmpeg         9183  uid0  Intel▸     47% █   1.1G▐  —       1.1%▐  64M▐  S ⵿ ⨯   ← selected (row highlight)
```

- **Accel column:** vendor chip (colored dot + short name), `▸` expands a drill-down sub-row
  showing that process's per-device engine breakdown (render/copy/video / SM/mem) — nvitop-style.
- **Percent cells:** `80% ████████░░` (MetricBar + number), aligned `text-right`.
- **Status:** `R` running, `S` stopped, `D` uninterruptible, `Z` zombie (red), `T` traced.
- **Actions:** per-row secondary actions revealed on hover — ⏸ suspend, ▶ resume, ⤒ raise priority,
  ⨯ kill (confirm). Context menu (right-click) for batch: *Suspend selected*, *Kill selected*,
  *Copy PIDs*, *Filter children*.
- **Sorting:** click headers; default sort = GPU% desc.
- **Grouping toggle:** flat / by device / by workload / by user.
- **Filter bar** above table: `⌘F` focus; live `name|pid:` syntax; chips for accel family, status,
  state (running/zombie), device id.
- **Column density:** settings-driven → `compact (h-7)` / `comfortable (h-9)`, and hide/show
  columns (GPU%, vMEM, NPU%, ...).

### 4.5 WorkloadCard / WorkloadRow (classification)

```
┌──────────────────────────────────────────────┐
│ TRAINING       3 workloads    148 GB vMem  ⬍ │
│ ┌───────────┬─────┬──────┬──────┬──────────┐ │
│ │ WORKLOAD  │ GPU%│ vMEM │ HOST │ PROCS    │ │
│ │ llama-7b   │ 97% │ 84GB │ 120G │ 3 / 15   │ │   expand → group of PIDs
│ │ whisper    │ 41% │  9GB │  32G │ 1 / 6    │ │
└─┴───────────┴─────┴──────┴──────┴──────────┘ ┘
```

Cards: **Training / Inference / Media codec / Crypto-mining / Unknown Ops**. Rule-based
classifier (exec path, open libs, CUDA/ONNX hints). Click → filtered ProcessTable.

### 4.6 AlertItem + AlertToast

- AlertItem: `⛔ crit  GPU0 thermal 94°C > 92°C  2m ago [ack] [silence]` — severity icon + color,
  entity chip, message, relative time.
- Toast (top-right, slides down): transient alerts for crit only. Lifecycle managed in Go
  (`app.Event.Emit("alert", …)`) or frontend timer — 6s auto-dismiss, click to open Alerts view.

### 4.7 CommandPalette (`⌘K`)

Floating centered panel `w-[560px] max-h-[420px] rounded-xl bg-raised ring-1 ring-hairline
shadow-2xl`. Sections: **Jump to…** (views), **Devices…**, **Processes** (search as you type,
show PID+accel row), **Actions** (`kill`, `suspend`, `resume`, `set priority`, `compute mode`,
`gpu reset`). Arrow keys + Enter. Type-ahead hint row of recent actions.

### 4.8 Empty / Error / Unavailable states

- **Empty (no devices):** centered illustration + `No accelerators detected` + button `Run
  detection` + link to vendor-driver checklist.
- **Unavailable metric:** `—` with dotted underline + tooltip. Never fake a 0. Chip variant:
  `N/A` with reason tooltip: *"Coral TPU: vendor exposes no utilization API"*.
- **Collector error (e.g., NVML init failed):** yellow banner in Devices view with the exact
  dlopen error + docs link.
- **Kill permission denied:** inline row tooltip *"SIGKILL failed: permission denied (root/sudo
  needed)"* + suggests raising privileges, doesn't turn red and die.

---

## 5. ECharts Recipes

Add `echarts` (npm `echarts`) and a tiny hook — init once, throttle renders, never animate
polling updates:

```tsx
function useEChart(ref: RefObject<HTMLDivElement>, option: EChartsOption, deps: any[]) {
  const chart = useRef<ECharts>();
  useEffect(() => {
    if (!ref.current) return;
    chart.current ??= echarts.init(ref.current, "dark", { renderer: "canvas" });
    chart.current.setOption(option, { notMerge: false });   // merge for streaming
    const onResize = () => chart.current?.resize();
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  return chart;
}
```

Streaming rule: build option **once**, then on each 1 Hz sample mutate data via
`setOption({ series: [{ data: lastN }] }, { notMerge: false })`. Keep `animation: false` for all
live series. Chart backgrounds use `transparent` + canvas `bg-color: transparent`.

### Chart inventory

| Chart | ECharts type | Where | Height |
|---|---|---|---|
| Radial util gauge | `gauge` (arc min/max in `startAngle/endAngle`) | DeviceCard, Device Detail header | 72px / 120px |
| Time-series utilization | `line` (smooth, symbol none) | Device Detail, History | 160-220px |
| VRAM timeline | `line` stacked (used / shared) | Device Detail | 140px |
| Engine breakdown | `bar` stacked (render/copy/video or SM/enc/dec) | Device Detail, process▸ expansion | 120px |
| Per-process memory | `bar` horizontal | Device Detail → Processes tab | 160px |
| Mini sparkline | `line` w/ `areaStyle` gradient | KpiCard, DeviceCard | 24-28px |
| Fleet heat grid | `heatmap` (device × time) | Overview | 140px |
| Workload balance | `pie` donut (by accel family) | Overview | 160px |

ECharts light/dark: register `dark` theme once for explicit chart colors; reference
`--color-brand` etc. via `echarts.init(..., "dark")` + explicit hex in `option.color`.

---

## 6. View-by-View

### 6.1 Overview

```
┌──────────────────────────────────────────────────────────────────┐
│  OVERVIEW                                                         │
│ ┌ HIS ────────────────┬ HI ───────────┬ H ──────────────┬──┐     │
│ │ GPU 64%             │ NPU 12%        │ TPU N/A         │  3 │   │  KpiCard row
│ │ 4 devices · 42G     │ 2 devices      │ 1 device (Coral)│ procs│  │  (right = alerts)
│ │ sparkline           │ sparkline      │ sparkline       │ active│
│ └─────────────────────┴────────────────┴─────────────────┴──┘     │
│                                                                   │
│ ┌ SPARKLINE PANEL ───────────────────┐ ┌ TOP ACCELERATORS ──┐    │
│ │ Fleet util 60m (area, 3 families)  │ │ RTX4090  ████ 83%  │    │
│ │ ── GPU ── NPU ── TPU               │ │ RX 7900  ██  47%  │    │
│ └────────────────────────────────────┘ │ iGPU     █   12%  │    │
│                                        └────────────────────┘    │
│ ┌ WORKLOAD SUMMARY ──────────────────────────────┐ ┌ ISTORY ┐    │
│ │ Training 2 ● 144GB    Inference 5 ● 23GB       │ │5d ago  │    │
│ │ Codec 1 ● 2GB        Mining 0                  │ │ 3d ago │    │
│ └────────────────────────────────────────────────┘ │ today  │    │
│                                                    └────────┘    │
│ ┌ TOP PROCESSES (compact table, 6 rows, click → Processes) ────┐ │
└──────────────────────────────────────────────────────────────────┘
```

Layout: `grid grid-cols-1 xl:grid-cols-12 gap-3`. KPIs `xl:col-span-3` ×3 + alerts `xl:col-span-3`.
Main rows: fleet chart `xl:col-span-8`, top accels `xl:col-span-4`, workload summary `xl:col-span-7`,
snapshot history `xl:col-span-5`, top processes full-width.

### 6.2 Devices

Card grid `grid-cols-2 md:grid-cols-3 2xl:grid-cols-4 gap-3` of DeviceCards, grouped by family
with section headers (`GPU`, `NPU`, `TPU`). Detecting overlay when `Detect()` runs at startup.
Empty state per §4.8.

### 6.3 Device Detail

Header row: big radial gauge + device metadata grid (model, driver, kernel/interface, compute
interface, bar1 bar2 per vendor) + control toolbar. Tabs below:

```
┌─ UTILIZATION ─┬─ MEMORY ─┬─ PROCESSES ─┬─ FAULTS ─┬──┐
│ ┌──────────────┐ ┌ 60s live ├ ┌ 60m ├    │
│ │ SM / mem /   │ │ ┌───┐ ...              │
│ │ enc / dec    │ │ ...                    │
│ └──────────────┘ └ 120px bar ┘            │
```

- **Utilization:** stacked `line` SM/GPU + MEM + ENC + DEC (60s live auto-scroll, 60m history
  toggle). Power/temp on secondary axis as thin `area`.
- **Memory:** VRAM used + shared + total threshold band. Colored past warn/crit.
- **Processes:** ProcessTable pre-filtered to this device (`?device=GPU0`), grouped by engine.
- **Faults:** ECC error counter, Xid errors (NVIDIA), throttle reasons per vendor where exposed.
- **Control toolbar:** (vendor ∩ permission aware) Compute mode cycle (DEFAULT ↔ EXCLUSIVE_PROCESS),
  MPS start/stop, MIG list (with enable hint), GPU reset (double-confirm). Buttons disabled +
  tooltip when not supported: *"Reset not exposed by vendor driver"*.

### 6.4 Processes

Full-screen ProcessTable (§4.4) + filter bar. Default view groups by **device**. Contains every
process that touches ANY accelerator plus those without accel usage only when "show all" toggled.
Batch actions bar when rows selected (`⏸ Suspend 3 · ⨯ Kill 3`).

### 6.5 Workloads

Sectioned list of WorkloadCards (§4.5) with per-card sparkline of aggregate GPU% and a
classification confidence note (``rule uv 0.92``). Click opens ProcessTable scoped to that
workload.

### 6.6 Alerts

Two columns: **Active** (unacked, grouped by severity) and **Rules** (threshold table, enable /
edit / add, saved to Go config JSON). Rule editor = small form: entity selector, metric, operator,
threshold, action (toast, desktop notify, auto-suspend).

### 6.7 Settings

Left mini-nav (User / Hardware / Collectors / Alerts / Appearance / About). Appearance: density
(compact/comfortable), font mono/UI, accent overrides, chart palette; a **live preview panel**
rendering one DeviceCard with current tokens. Hardware: per-source enable toggles + permissions
diagnostic (which sources can actually read, with the exact reason). Collectors: sample rate,
run as privileged helper checkbox (macOS `powermetrics`, Linux setuid).

---

## 7. Data Flow & Refresh

```
Go MetricService (1 Hz ticker)
   ├─ collect(): for each source → Sample{devices[], procs[]}
   ├─ aggregate/classify in Go (workloads, alerts evaluated)
   └─ app.Event.Emit("telemetry", Sample)          ← single batched event < 8KB
                          │
Frontend — Events.On('telemetry', …) → store (zustand or reducer)
   └─ components consume slices: KpiCards, heat grid, table, charts
```

- **One event per tick, not one per device** — keeps payload under Wails' 8KB inline threshold
  (PR #5930 / #5934 zero-retention path).
- At 50+ devices, switch to a polled `GetTelemetry()` binding or batch chunking.
- History: Go ring buffer (last N samples) + `HistoryService.Range(device, range)` binding for
  range backfill; CSV/JSON export on History view.
- Alert evaluation in Go (single source of truth, survives UI restarts); frontend only renders.
- RPC for commands: `ProcessService.Kill(pids)`, `ProcessService.Suspend`, `ControlService.*`.
  Confirm dialogs client-side; permission errors surfaced per §4.8.

---

## 8. Keyboard Shortcuts (nvitop/htop-inspired)

| Key | Action |
| --- | ------ |
| `⌘K` / `␣-t` | Command palette |
| `⌘⇧B` / `⌘B` | Nav rail expand / hide |
| `q` / `f` | Filter / `qf` clear filters |
| `t` | Toggle process tree view |
| `e` | Kill selected process |
| `k` / `␣` | Suspend / resume selected |
| `n` / `p` | Lower / raise priority |
| `← →` / `f` | Prev / next device in detail |
| `g` | Cycle grouping (flat/device/workload/user) |
| `1 2 3` | Goto Overview / Devices / Processes |
| `␣` | Pause / resume sampling |
| `?` | Shortcut help overlay |

Rebindable in Settings → User.

---

## 9. Motion & Accessibility

- Transitions: `duration-150 ease-out` for hover/active, `200ms` for panels; no bounce.
- Sparklines/gauge: static updates at 1 Hz (no tween) — `animation: false` on live series.
- Toast/shortcut overlay: `300ms cubic-bezier(0.2,0.8,0.2,1)` slide+fade.
- `prefers-reduced-motion: reduce` → zero animation, instant toasts, no crossfades.
- Focus visible: `focus-visible:ring-2 ring-brand/60` on every interactive element.
- Contrast: faint `#71717a` only for tertiary copy ≥ 11px; body copy ≥ `text-zinc-300`.
- Table a11y: `aria-sort` on headers, row `aria-selected`, actions `aria-label` only (icons
  behind sr-only text). Native `<button>`/`<select>` everywhere.
- `role="status"` aria-live on live pill changes; alerts list `role="log"`.

---

## 10. Tech Adoption (what changes in the repo)

```bash
cd frontend
npm i tailwindcss @tailwindcss/vite echarts
# @tailwindcss/vite@^4.2.2 supports Vite 8 (npm 4.2.2+)
```

- `vite.config.ts`: add `tailwindcss()` plugin next to `react()`.
- `frontend/src/style.css`: delete the neon-night sheet body → `@import "tailwindcss";` + the
  `@theme` block from §2. Keep the few global behaviors (user-select none, `--wails-draggable`
  opt-outs for interactive chrome) as small `@layer base` rules.
- `frontend/index.html`: title → **pmanage**; keep `viewport-fit=cover`.
- Components tree (proposed):

```
frontend/src/
  app.tsx                 shell: rail + header + view switch
  components/ui/          chip · card · icon · metricbar · button · kbd · tab · modal · tooltip
  components/charts/      useechart.ts · gauge.tsx · sparkline.tsx · timeseries.tsx · heat.tsx
  components/device/      devicecard.tsx · devicedetail.tsx (devicedetail.tabs.tsx)
  components/process/     processtable.tsx · processrow.tsx · filterbar.tsx · batchnbar.tsx
  components/views/       overview.tsx · devices.tsx · processes.tsx · workloads.tsx
                          alerts.tsx · history.tsx · settings.tsx
  lib/                    telemetrystore.ts (subscription store) · fmt.ts (bytes, %, reltime)
                          vendor.ts (colors, labels) · shortcuts.ts
  bindings/pmanage/       generated (renamed from changeme)
```

**Decision record:** icon set — inline SVG (Lucide-style paths, no dependency) for the ~24 icons
needed, over an icon-font package, to keep the bundle lean and the Wails webview SSG-friendly.

---

## 11. Open Questions (blocking v0.1 look-finalization)

1. **Accent color:** default cyan `#22d3ee` — keep, or brand color (e.g. violet)? Easy to flip via
   `--color-brand`.
2. **Header height / density:** 36px (this mockup) vs 32px ultra-compact — cluster demo will
   decide.
3. **Process table columns:** include host CPU/mem by default (adds width) or hide behind column
   toggles until a host-resource tab is needed?
4. **Confirm-on-kill policy:** instant kill with Undo toast (Linear-style) vs modal confirm
   (Task-Manager-style)? Undo requires keeping process metadata post-kill; modal is simpler.