# PMANAGE — Build Plan (PLAN.md)

Phase-by-phase, systematic build plan for the one-stop **GPU / NPU / TPU process viewer & manager**.

Context docs:
- `RESEARCH.md` — vendor backends, per-process attribution, feasibility matrix, framework analysis.
- `MOCKUP.md` — UI/UX spec: Tailwind v4 theme, app shell, views, ECharts recipes, data wiring.

Status legend: ⬜ not started · 🟡 in progress · ✅ done.

---

## 0. Strategy & Guiding Principles

1. **NVIDIA first, then the DRM-fdinfo multiplies.** NVIDIA (NVML) is the only backend with
   everything: device + per-process + control. It validates the full pipeline. The DRM fdinfo
   engine then unlocks AMD + Intel + AMD-NPU from one walker. NPUs/TPU/Apple come after the core
   pipeline is proven.
2. **UI driven by MockCollector.** Frontend is built and tuned against mock data in lock-step
   with real backends, never blocked on hardware.
3. **Honesty as a feature.** Every metric-availability gap renders as an explicit `N/A` chip with
   a vendor reason, per MOCKUP §4.8.
4. **Permission-aware controls.** Kill/suspend/control surfaces must surface permission failures
   inline, never silently fail.
5. **Tests with fixtures.** fdinfo/NVML/powermetrics outputs are captured as fixtures; collectors
   are pure enough to unit-test without hardware.
6. **Every phase ends shippable.** Each phase has a demonstrable slice of the real app.

---

## 1. Milestone Map

| # | Phase | Outcome | Target effort |
|---|---|---|---|
| 0 | Scaffold & contracts | renamed module, Tailwind+ECharts shell, collector contract, MockCollector dash | 1 wk |
| 1 | App shell & process layer | nav rail + header + status bar, device registry, gopsutil process table + kill/suspend/nice | 1-2 wk |
| 2 | NVIDIA backend | full NVIDIA device+process+control parity (nvitop-class) | 2-3 wk |
| 3 | DRM fdinfo engine | AMD + Intel + AMD-NPU per-process views | 2-3 wk |
| 4 | Apple Silicon | GPU + ANE dashboards (Activity-Monitor-class GPU column) | 2-3 wk |
| 5 | NPU & edge | Intel ivpu, Rockchip, Hailo, Jetson (device-level + N/A handling) | 2 wk |
| 6 | Beyond: history/alerts/workloads | ring buffers, alert rules, ML-workload classifier | 2-3 wk |
| 7 | Remote & packaging | `-tags server` fleet mode; DMG/NSIS/MSIX/AppImage/DEB/RPM | 2 wk |
| 8 | Hardening & v1.0 | perf, CGO safety, tests coverage, signing, release | 2-3 wk |

Total ≈ 4-5 months part-time; compressible to ~2.5 at full-time with hardware available.

---

## 2. Phase 0 — Scaffold, Contracts, Dev-Skeleton (1 wk)

**Goal:** repo is a clean, runnable skeleton with the accelerator contract and a mock dashboard.

### Tasks
- [ ] Rename Go module `changeme` → `pmanage`; update `go.mod`, imports, generated binding path
  (`frontend/bindings/changeme` → `bindings/pmanage`).
- [ ] Update Wails identity in `build/config.yml` + `build/darwin/Info.plist` (bundle IDs
  `com.<org>.pmanage`, version, company).
- [ ] Frontend: adopt Tailwind v4 + ECharts (MOCKUP §10):
  `npm i tailwindcss @tailwindcss/vite echarts`; add plugin to `vite.config.ts`; replace
  `style.css` body with the `@theme` block from MOCKUP §2.
- [ ] Define Go contracts in `pkg/accelerator/` (interfaces from RESEARCH §8):
  `AcceleratorSource`, `Sample`, `DeviceMetrics`, `ProcUsage`, `Kind`.
- [ ] Implement `MockSource` + `mock; //go:build mock` driver emitting realistic synthetic
  devices (RTX 4090, RX 7900, core iGPU, Hailo-8, Coral) with simulated load curves.
- [ ] `MetricService` (`ServiceStartup` goroutine, 1 Hz ticker) emitting one batched
  `telemetry` event — the exact Wails pattern from RESEARCH §7.1.
- [ ] Minimal Overview (MOCKUP §6.1) with KpiCards + ECharts sparklines fed by MockSource.
- [ ] Window: 1280×800 min, `h-9` header / 48px rail / `h-6` status bar per MOCKUP §3.

### Deliverables / Acceptance
- `task dev` boots a dashboard showing animated mock GPU/NPU/TPU KPIs and sparklines.
- Contracts compile with zero vendor dependencies.
- Wails events measured < 8KB/tick (verify with logs).

### Dependencies
None (research + MOCKUP done).

---

## 3. Phase 1 — App Shell & System Process Layer (1-2 wk)

**Goal:** the app *is* a usable "Task Manager for accelerators" even before real accel collectors.

### Tasks
- [ ] Full shell: 3-state nav rail (collapsed/expanded/hidden), header (breadcrumb, `⌘K` palette
  stub, live pill, Hz selector, pause), status bar — per MOCKUP §3.
- [ ] `ProcessService` (gopsutil/v4): process list, tree, CPU/RAM/IO, kill/suspend/resume/nice,
  permission-error tooltips (MOCKUP §4.8).
- [ ] `DeviceRegistry`: run every source's `Detect()`, build capability matrix; per-source
  enable toggles; driver/kernel version readouts.
- [ ] Processes view: dense ProcessTable (MOCKUP §4.4) — sort, filter (`name|pid:`), group by
  user/state, batch actions bar. Accel columns render `N/A` until Phase 2+.
- [ ] Devices view + DeviceCard grid (MOCKUP §6.2, §4.2) from registry.
- [ ] Command palette: jump-to-view + device lookup + process search (kill/suspend verbs wired to
  ProcessService).
- [ ] Test: `processservice` + `registry` unit tests with gopsutil mocks.

### Deliverables / Acceptance
- Kill / suspend / resume / priority work on real processes (with permission handling).
- All views navigable; status bar shows heartbeat; pause/resume sampling works.
- **No accel metric is fake** — everything not yet collected shows `N/A` chips.

### Dependencies
Phase 0.

---

## 4. Phase 2 — NVIDIA Backend (2-3 wk)

**Goal:** production-equivalent NVIDIA coverage (device + per-process + control). Sets the UI bar.

### Tasks
- [x] `nvmlSource` on `github.com/NVIDIA/go-nvml`:
  - Device: util (SM/mem expect), VRAM, temp, power, clocks, fan, PCIe info; MIG mode read.
  - Per-process: `DeviceGetComputeRunningProcesses` + `GraphicsRunningProcesses` for VRAM;
    SM% utilization only in EXCLUSIVE_PROCESS mode; accounting-mode history deferred (see tradeoff).
  - Graceful failures: `nvml.Init()` error → yellow banner (MOCKUP §4.8) with the dlopen error.
- [x] Process-accurate per-process **utilization** (nvitop parity): documented in RESEARCH §4.1 —
  real SM% in EXCLUSIVE_PROCESS, honest `N/A`/0 in DEFAULT mode (accounting mode required).
- [x] Fallback `nvidia-smi` source for machines where NVML loading fails (parse
  `--query-gpu/--query-compute-apps`, `pmon` for util) — `nvidia_smi.go`, fixture-tested.
- [x] ControlService NVIDIA verbs (gated by permission & capability):
  - compute mode cycle DEFAULT ↔ EXCLUSIVE_PROCESS ✓ (registry-capability gated)
  - MPS daemon start/stop (stub → clear guidance); MIG mode list; driver reset N/A (no NVML reset API).
- [ ] Views: Devices shows NVIDIA tiles; Device Detail (Utilization/Memory/Processes/Faults tabs,
  MOCKUP §6.3) with ECharts recipes (§5 of MOCKUP) — gauge, engine breakdown, VRAM timeline,
  per-process bar (deferred to UI pass).
- [x] Tests: golden-format fixtures from `nvidia-smi` CSV (nvidia_smi_test.go) + NVML mock samples
  (nvidia_mock_test.go); control verb tests via fake control provider.

### Deliverables / Acceptance
- On an NVIDIA box: per-GPU metrics at 1 Hz, per-process mem + SM% accurate to
  nvidia-smi.
- Kill/suspend/MPS/compute-mode work with correct validation & confirmation UX.
- Zero regressions on machines without NVIDIA (still green UI + N/A).

### Dependencies
Phase 1. Hardware: any NVIDIA GPU (desktop) for dev, A100/H100 optional for MIG.

---

## 5. Phase 3 — Linux DRM fdinfo Engine (2-3 wk)

**Goal:** one walker powers AMD GPU + Intel GPU + AMD-NPU per-process on Linux (kernels ≥6.7).

### Tasks
- [x] `fdinfoWalker` (RESEARCH §5.2): iterate `/proc/<pid>/fd`, resolve `/dev/dri/*` links, parse
  `drm-client-id`, `drm-engine-*` (ns), `drm-memory-*` (bytes); aggregate fds→client rows; diff
  engine time between ticks → %.
- [x] `amdgpuSource`: device via sysfs (`gpu_busy_percent`, `mem_info_vram_*`, hwmon temp, clock,
  power/amdgpu) + KFD aperture info; process rows from walker. Align output with `amdgpu_top`.
- [x] `intelSource`: i915/Xe via walker + engine-class mapping; RAPL power; per-engine busyness.
- [x] `xdnaSource` (AMD NPU): walker on `/dev/dri/accel*` (`drm-engine-amdxdna`) → per-process
  NPU engine time.
- [x] Kernel-version gates surfaced in UI: `DeviceInfo.MinKernel` / `DeviceMetrics.MinKernel`
  (`≥6.7.0` amdgpu+amdxdna, `≥5.15` i915) rendered as a tooltip chip on DeviceCard; helper
  `kernelAtLeast()` reads `/proc/sys/kernel/osrelease`.
- [x] Fixture-based unit tests: nvidia_smi_test.go CSV fixtures; registry + control + apple/
  powermetrics fixture suites (fdinfo walker fixtures in pkg/fdinfo).
- [x] Windows Intel path note (Level Zero Sysman) scaffolded behind build tags for Phase 7 —
  `intel_windows.go` stub + `platform_windows.go` bootstrap.

### Deliverables / Acceptance
- On an AMD box: per-process VRAM + GFX% match `amdgpu_top`.
- On an Intel box: per-client busyness matches `gputop -J`.
- AMD NPU engine time per process where `amdxdna` present.
- Tested against k6.7+ and k5.15 partial-format fixtures.

### Dependencies
Phase 1. Hardware: AMD Radeon Linux box, Intel iGPU Linux box, AMD (Ryzen AI / Strix Point)
laptop or dev board.

---

## 6. Phase 4 — Apple Silicon (2-3 wk)

**Goal:** macOS GPU + ANE dashboards, Activity-Monitor-class per-process GPU column.

### Tasks
- [x] `appleSource` (CGO, macOS-only build tag):
  - [x] IOKit: IOGPU `PerformanceStatistics` (aggregate GPU util: Device/Renderer/Tiler %, GPU system memory).
  - [x] IOReport subscription (modern API — no legend API on macOS 13+; matched *by group*):
    `Energy Model`/"GPU Energy" → GPU power W; `PMP`/"ANE" → ANE power W (energy-delta over tick).
  - [x] Per-process GPU: `accumulatedGPUTime` mapping via AGXDeviceUserClient `AppUsage`
    (user-level, no root; validated live: gpuload Metal workload → 93.6% util under load).
    Same source Activity Monitor's GPU column uses. Experimental badge per MOCKUP §4.4.
  - [x] IORegistry enumeration for device identity (GPU + ANE: cores/arch/ver from H11ANEIn); SMC temp N/A.
- [ ] ANE utilization % (needs aneperf-style private regs) — N/A for now, power only.
- [x] `powermetricsSource` (opt-in via `PMANAGE_POWERMETRICS=1`; parses XML plist → JSON via plutil): GPU freq/power + ANE power per tick. Per-process GPU column is unreliable on Apple Silicon (documented N/A).
- [x] Version-gate: dlsym probe for IOReport symbols before first call; graceful fallback to N/A on macOS <13.
- [ ] VRAM-per-process via `task_vm_info.graphics_footprint` behind SIP-off detection (unreliable without root — deferred).
- [x] Packaging hooks (`build/darwin/Taskfile.yml`): `package`, `package:dmg` (styled DMG), `sign`, `sign:notarize` placeholders + Info.plist/bundle resources.

### Deliverables / Acceptance
- GPU util history + per-process GPU column (experimental) on an M-series Mac.
- ANE util/power shown when running CoreML workloads; clear `N/A` when not measurable.
- No sudo required for the default experience.

### Dependencies
Phase 1. Hardware: Apple Silicon Mac (M1+). Risk gate: verify IOReport API version → §10.

---

## 7. Phase 5 — NPU & Edge Platforms (2 wk)

**Goal:** every attainable NPU shows live device metrics; honesty where per-process is impossible.

### Tasks
- [x] `ivpuSource` (Intel NPU): sysfs `/sys/class/accel/accel*/device/{npu_busy_time_us,
  npu_memory_utilization, npu_current_frequency_mhz, sched_mode}`; busy-µs delta → util%;
  driver-symlink gate so shared `/sys/class/accel` is claimed only by ivpu.
  Optional Level Zero for process state (deferred, same tradeoff as RESEARCH §4.1).
- [x] `rockchipSource`: devfreq `/sys/class/devfreq/*npu*` cur_freq + debugfs rknpu load (root);
  typology matches `rktop` (per-core % → max). Debugfs absent → honest N/A (no crash).
- [x] `hailoSource`: `hailortcli fw-control identify` + `monitor` subprocess (exec-runner,
  fixture-tested on JSON + plaintext lines); device info from identify; per-process → `N/A`.
- [x] `jetsonSource`: L4T detection via `/etc/nv_tegra_release` (no privileged read); `tegrastats`
  engines (GR3D% → util, VDD_GPU power, RAM) + per-process GPU/VRAM via `nvidia-smi`
  compute-apps when present.
- [x] Unified NPU device card & detail (freq, load%, temp) — existing DeviceCard renders NPU rows;
  standard N/A handling (util -1 → chip).
- [x] Detection order/priority rules so shared sysfs (`/sys/class/accel`) is claimed once:
  amdxdna vs ivpu matched by real driver symlink; devfreq/devfreq + debugfs + CLI sources
  register in platform_linux.go and Detect() no-ops off-host.
- [x] Tests: all four NPU sources fixture-tested on darwin (no Linux-only build tags) via injectable
  `npuFS` (virtual sysfs) + `cliRunner` (script map).

### Deliverables / Acceptance
- Each NPU vendor (as hardware is available) renders a live device card.
- Per-process column consistently `N/A` with accurate tooltip reasons; no fake numbers.
- Passion test: run a real inference workload and watch util move on Hailo/ivpu.

### Dependencies
Phase 1 (Phase 3 walker where it overlaps). Hardware: Core Ultra laptop, RK3588 SBC, Hailo-8
PCIe, Jetson Orin/Nano.

---

## 8. Phase 6 — Beyond: History, Alerts, ML Workloads (2-3 wk)

**Goal:** the differentiating features (RESEARCH §2.3 items 3-4 + commercial bar).

### Tasks
- [x] `HistoryService`: in-memory ring buffer (configurable hours), `Range(device, window)`
  binding for backfill, CSV/JSON export, History view graphs (MOCKUP §6.x).
- [x] `AlertEngine` in Go (survives UI restarts) + Rules store (config JSON, edited in
  Settings→Alerts): entity × metric × threshold × action (toast / desktop notify / auto-suspend).
  Preset rules: thermal, power cap, VRAM OOM growth, zombie detect (mem held + 0% util, cf.
  `kill-gpu-zombie`).
- [x] `ClassifierService`: group processes into **Training / Inference / Media codec /
  Crypto-mining / Unknown** from exec paths + open libs (libcuda, onnxruntime, rknn, libhailort,
  ffmpeg heuristics) + env (CUDA_VISIBLE_DEVICES etc.). Score + rule provenance per workload.
- [x] Workloads view (MOCKUP §6.5): cards with per-workload GPU% sparkline, expand → scoped
  ProcessTable. Crypto-mining detection policy hooks available later (like gpu-kill guard mode).
- [x] Alerts view (active + rules), desktop notifications, toast path.

### Deliverables / Acceptance
- [x] Kill a zombie process before review; alert fires; workload grouping matches hand-labeled data
  on a sample workload batch.
- [x] History survives pause/resume; export file opens in spreadsheet.

### Dependencies
Phase 2 (needs real accel data); loops back cleanly to Phase 3+ sources.

---

## 9. Phase 7 — Remote/Server Mode & Packaging (2 wk)

**Goal:** fleet visibility + distributable installers.

### Tasks
- [x] Wails `-tags server` headless build: HTTP + WebSocket IPC (`WebSocketBroadcaster`),
  telemetry events fan out to N browser clients; SSH-tunnel guidance; device config / per-host
  allow-list.
- [x] Health/auth for server mode (token), read-only mode for remote clients (kill disabled
  unless remote-admin granted).
- [ ] Packaging per platform (Taskfile):
  - macOS: `.app` + DMG, signing + notarization (gon), Apple Silicon universal.
  - Windows: NSIS + MSIX (artifacts from existing scaffold), EV-cert notes.
  - Linux: AppImage + DEB + RPM (+ AUR), Docker cross-build via provided Dockerfile.
- [ ] `build/config.yml` metadata / icon / version finalized; appicon design.
- [ ] E2E smoke script on each OS: fresh install → detect → 1 Hz telemetry → kill a process.

### Deliverables / Acceptance
- [x] macOS: `.app` bundle + DMG verified on this machine (`build:darwin:package` + `package:dmg`),
  ad-hoc codesign + hardened runtime on `.app`; gon/notary path documented for real identity.
- [ ] Windows (NSIS + MSIX + EV notes), Linux (deb/rpm/AppImage/AUR + Dockerfile.server cross-build)
  — blockers: cross-toolchains absent on this Mac; `build:server`, `build:docker`,
  `run:server`, `run:docker` tasks scaffolded and ready to run on a Linux builder.
- [ ] E2E smoke script (`scripts/e2e_smoke.sh`) run headless on the CI matrix per OS; boot verification
  (health + token + read-only gate) done locally in server mode.

### Dependencies
Phase 6 (feature-complete core). Hardware: one target box per OS at least for packaging CI.

---

## 10. Phase 8 — Hardening & v1.0 Release (2-3 wk)

**Goal:** production quality, safe native code, release artifacts.

### Tasks
- [ ] Collection tuning: 1-2 Hz cadence, batched events <8KB, frontend
  `requestAnimationFrame` drainage; chart `animation:false`; backpressure pause.
- [ ] CGO safety: recover wrappers around every native call; segfault regression drill (RESEARCH §7.1).
- [ ] Permission matrix UI: Settings→Hardware diagnostic showing each source's read/permission
  status and exact fix (add user to `render`/`video`, sudo helper, SIP note).
- [ ] Test coverage: collector fixture suites (NVML/fdinfo/powermetrics), service mocks, E2E
  flows; CI (GitHub Actions) matrix: macOS arm64, Linux amd64, Windows amd64.
- [ ] Crash reporting opt-in (non-PII, event name only); app telemetry toggle.
- [ ] Docs: user guide (per-vendor capability table), admin guide, troubleshooting; README badges.
- [ ] Version pins + SBOM; license compliance pass (go-licenses).
- [ ] Release: tag v1.0, changelog, signed artifacts.

### Deliverables / Acceptance
- All acceptance criteria of Phases 0-7 pass on the demo-priority matrix.
- Empty-state (no accelerators) app still useful (system processes + clear guidance).
- `N/A` honesty principle verified on Coral + at least one NPU.

---

## 11. Cross-Cutting: Testing & QA Strategy

| Layer | Approach |
|---|---|
| Contracts | interface + golden-format unit tests (no hardware) |
| Collectors | captured fixtures (fdinfo dumps, `nvidia-smi` CSV, `powermetrics` text, `gputop -J`) |
| Services | gopsutil mocks; Wails event-count assertions |
| UI | React Testing Library for table/filter/batch logic; Playwright E2E on WebView (dev mode) |
| Hardware matrix | per-vendor manual acceptance checklist (this repo's `docs/hw-test-matrix.md`) |

## 12. Demo Priority Matrix (who sees what first)

| Hardware | Phase reached | Must-show |
|---|---|---|
| NVIDIA desktop | 2 | per-proc SM%/VRAM + kill + MPS/compute mode |
| macOS M-series | 4 | GPU per-proc column + ANE spikes on CoreML |
| AMD Radeon Linux | 3 | per-proc VRAM/GFX% vs amdgpu_top |
| Intel iGPU Linux | 3 | per-client engines vs gputop |
| Core Ultra (ivpu) | 5 | NPU busy% during ONNX/OpenVINO |
| Hailo-8 | 5 | VPU util during inference |
| RK3588 | 5 | NPU freq/load, all-N/A process column honesty |
| Jetson | 5 | NVENC/NVDLA + per-proc GPU |
| Coral TPU | any | enumerate + honest N/A chip |

## 13. Risk Gates (checkpoints before proceeding)

- **Gate A (end of Ph 2):** Wails v3 IPC + ECharts sustain 1 Hz on WebKitGTK (Linux GPU box) or
  fall back to software rendering. If Wails v3 proves unstable → reassess (research §7.2).
- **Gate B (end of Ph 4):** Apple IOReport API version confirmed on current macOS; degradation
  path exercised. If private APIs break → powermetrics-only GPU column.
- **Gate C (end of Ph 5):** all attainable NPU sources render; honesty principle accepted in demo.
- **Gate D (end of Ph 6):** alert/classifier accuracy acceptable with real workloads.

## 14. Tooling Recap (decided)

- Go 1.25 · Wails v3 (β.22) · React 18 + TS · **Tailwind v4** (`@tailwindcss/vite`) · **ECharts**
- `gopsutil/v4` · `go-nvml` · CGo (Apple IOKit/IOReport; AMD SMI/L0 optional) · sysfs/fdinfo pure-Go
- Taskfile build/package automation (existing scaffold) · GitHub Actions CI