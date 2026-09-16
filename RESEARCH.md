# PMANAGE — Research Report

One-stop **GPU / NPU / TPU process viewer & manager** (desktop).
Consolidates all research: existing tooling, vendor metric backends, per-process attribution,
process-management primitives, and framework analysis.

> Supersedes the earlier partial `ACCELERATOR_METRICS_RESEARCH.md` (its content is folded in
> here). Header date: 2026-09.

---

## 1. Purpose & Scope

Build a cross-platform desktop app that:

1. **Discovers** every accelerator on the machine (GPU, NPU, TPU across vendors).
2. **Shows live device metrics** — utilization, memory, temperature, power, clocks.
3. **Attributes usage per process** where the hardware/driver allows it.
4. **Manages processes** — kill, suspend/resume, priority, GPU-level controls (compute mode, MPS, MIG).
5. **Goes beyond** — history, alerts, ML-workload classification, optional remote/fleet mode.

---

## 2. Existing Tool Landscape (Inspiration & Gaps)

### 2.1 Open-source tools

| Tool | Vendors | Stack | Per-proc | Kill | GUI | Key idea to steal |
|---|---|---|---|---|---|---|
| **nvitop** (XuehaiPan) | NVIDIA | Python + NVML | ✅ mem+util | ✅ | TUI | Process **tree view**, filter, env vars, 256-color bars, embeddable `ResourceMetricCollector` |
| **nvtop** (Syllo) | NVIDIA, AMD, Intel, Apple, Ascend, Adreno | C | ✅ (fdinfo) | ❌ | TUI | Only cross-vendor *viewer*; per-vendor backends behind ncurses |
| **gpustat** | NVIDIA | Python | ✅ | ❌ | CLI | JSON out, tight layout |
| **amdgpu_top** | AMD | Rust | ✅ fdinfo | ❌ | TUI+GUI | GRBM counters, `-p` per-process, fdinfo walker |
| **intel_gpu_top / gputop** | Intel i915/Xe | C+PMU | ✅ per-client | ❌ | TUI | Per-engine busyness + `-J` JSON client dump |
| **gsmi + radeontop** | NVIDIA/AMD | C/Python | partial | ❌ | CLI/TUI | Register-level detail |
| **asitop / pumas / aneperf / macmon** | Apple GPU + ANE | Python/Rust/Go | GPU only | ❌ | TUI | **aneperf is pure-Go**: ANE via private IOReport/IOKit (purego) |
| **mactop** (metaspartan) | Apple Silicon | Go+CGo | experimental | ❌ | TUI | Closest Go reference for IOKit/SMC/IOReport |
| **jetson-stats / jtop** | Jetson | Python | partial | ❌ | TUI | Engine-level (NVENC/NVDLA) breakdown |
| **nputop** | Huawei Ascend | — | ✅ | ❌ | TUI | HNPU process viewer (nvtop supports Ascend too) |
| **gpu-kill** (treadiehq) | NVIDIA/AMD/Intel/Apple | Rust | ✅ | ✅ | Web+MCP | **Closest to our goal**: monitor + kill + crypto-miner detection + fleet/remote |
| **kill-gpu-zombie** | NVIDIA | Python | ✅ | ✅ | CLI | Zombie detection (mem held / 0% util) + auto-kill |
| **rktop** | Rockchip SoC | Rust | ❌ | ❌ | TUI | Reads debugfs/devfreq NPU load |

### 2.2 Commercial / platform tools

| Tool | Notes |
|---|---|
| NVIDIA **DCGM / DCGM Insights / Nsight** | 500+ field IDs, per-PID via `dcgmGetPidInfo`, host-engine daemon, cluster-oriented |
| **DCGM Exporter + Datadog / New Relic** | Prometheus→agent check; dashboards + alerts |
| **netdata** `go.d.plugin:nvidia_smi` | Loop collector, remote instances, alerting |
| **Windows Task Manager** (Copilot+ PCs) | **NPU column** (Win 11 24H2+); per-process GPU/NPU engine + mem via WDDM2 perf counters |
| **MSI Afterburner + RTSS / ROG GPU Tweak / Adrenalin** | OC + per-GPU OSD monitos |
| **macOS Activity Monitor** | CLI/GPU history windows; per-process GPU via IOReport; `powermetrics` for ANE |

### 2.3 The gap (why PMANAGE is worth building)

1. **No unified cross-vendor UI** — nvtop views only; gpu-kill is Rust CLI/dashboard-first; NPUs stay siloed per vendor.
2. **No tool pairs per-process attribution + process management (kill/tree) + history/alerts** for *all* accelerator families.
3. **Per-process NPU attribution** is nonexistent everywhere (ANE/XDNA/ivpu/Hailo).
4. No **ML-workload classification** (LLM training / inference / codec / mining groups).
5. No polished **desktop app** (matrix/TUI gaps) that non-experts can use.

---

## 3. Per-Process Feasibility Matrix (the hard truth)

✅ = vendor/OS gives per-PID attribution. ◐ = partial/aggregate. ❌ = not exposed.

| Accelerator | Device metrics | **Per-process** | Mechanism | Permissions | Notes |
|---|---|---|---|---|---|
| NVIDIA GPU (desktop+Jetson) | ✅ rich | ✅ **mem + util** | NVML `Get*RunningProcesses`, accounting mode | user/root | WDDM on Windows returns N/A for mem — use WDDM counters instead |
| AMD GPU (Linux) | ✅ | ✅ **VRAM + engine** | DRM **fdinfo** (k≥6.7), KFD procfs | fd owner/root | `amdgpu_top`-style fdinfo walker |
| Intel GPU (Linux) | ✅ | ✅ **per-client** | i915 fdinfo (k≥5.15) + `gputop -J` | root / `perf_event_paranoid` |
| Intel GPU (Windows) | ✅ | ✅ | Level Zero Sysman `zesDeviceProcessesGetState` |
| Apple Silicon GPU | ✅ | ◐ **experimental** | IOReport/IOKit `accumulatedGPUTime`; VRAM needs SIP off | some sudo |
| Apple **ANE** | ◐ (power-derived) | ❌ | IOReport channels (aneperf pattern) | none |
| AMD **NPU (XDNA)** | ✅ | ✅ **engine** | `amdxdna` DRM fdinfo (k≥6.5) | fd owner |
| Intel **NPU (ivpu)** | ✅ | ❌ | sysfs `npu_busy_time_us`, `npu_memory_utilization` | root |
| **Hailo-8 / -10** | ◐ | ❌ | `hailortcli monitor` (needs `HAILO_MONITOR=1`) | root |
| **Rockchip NPU** | ◐ | ❌ | debugfs `/sys/kernel/debug/rknpu`, devfreq | root |
| **Qualcomm Hexagon** | minimal | ❌ | Snapdragon Profiler (OEM/NDA) |
| **Huawei Ascend** | ✅ | ✅ | DCMI API (nvtop/nputop) |
| **Google Coral / Cloud TPU** | ❌ ¹ | ❌ | — | Coral exposes **no util API**; Cloud TPU → Cloud Monitoring/LibTPU |

¹ Coral Edge TPU: hardware has no observability surface. **Design decision:** enumerate the device
(USB VID/PID 1a6e / `/dev/apex_0`) and label it "TPU detected — metrics not exposed by vendor."

### Windows cross-vendor note

On Windows the **WDDM 2.0+ performance counter** path (`\GPU Engine(*)\Utilization`,
`\GPU Process Memory(*)\Dedicated Usage`) covers *all* vendors with one mechanism — same data
source Windows Task Manager uses. Wrap with `golang.org/x/sys/windows` + perf counter API.

---

## 4. Vendor Backend Deep-Dive

### 4.1 NVIDIA

**Primary: `github.com/NVIDIA/go-nvml`** — official Go bindings, Apache-2.0, Linux-only, CGo+`dlopen`
(no compile-time driver needed).

- `DeviceGetUtilizationRates`, `DeviceGetMemoryInfo`, temperature, power, clocks, ECC, MIG.
- **Per-process:** `DeviceGetComputeRunningProcesses_v2`, `DeviceGetGraphicsRunningProcesses_v2`,
  `DeviceGetMPSComputeRunningProcesses`; accounting (`DeviceGetAccountingPids/Stats`) for lifecycle
  history; `vGPU` per-process for MIG/vGPU.
- **Fallbacks:** `nvidia-smi --query-gpu=... ,--query-compute-apps=... --format=csv`; `pmon` live;
  JSON via `fffaraz/nvidia-smi-json`.
- **DCGM** (Apache-2.0, server-tier): 500+ fields, `dcgmGetPidInfo`, needs `libdcgm.so`.
- **Management:** compute mode (`-c EXCLUSIVE_PROCESS/DEFAULT`), MPS daemon, MIG partitioning,
  `nvidia-smi -r` GPU reset; Windows caveat: WDDM → use WDDM counters for mem.
- **Per-process utilization tradeoff (nvitop parity):** NVML exposes **running-process
  utilization only when the device is in `COMPUTE_MODE_EXCLUSIVE_PROCESS`** (`ProcessManager`
  sampling API). In the common DEFAULT mode, `Get*RunningProcesses` reports per-process **memory
  but no SM%**; historical per-process SM% after exit needs **accounting mode**
  (`nvmlDeviceSetAccountingMode`), which adds per-process overhead and resets on driver reload.
  pmanage therefore reports per-process `utilPct` honestly:
  - NVML in EXCLUSIVE_PROCESS → real SM% per process (nvitop parity).
  - NVML in DEFAULT mode → per-process **memory only**; the GPU column shows the process lives on
    the device with `utilPct` left at 0 and a `N/A`-style tooltip explaining accounting-mode is
    required (never print a fake SM%).
  This is the documented tradeoff: no root/sysadmin intervention is required for VRAM + kill, and
  SM% make *sense* the rest of the time, matching `nvitop`'s behavior on shared-mode systems.
- **Build caveats:** gold linker segfault (use `--weak-unresolved-symbols` / `-z,lazy`), Go 1.21+
  `-rdynamic` fixed upstream (`--export-dynamic`).

### 4.2 AMD GPU

- **Sysfs (pure Go, no CGo):** `/sys/class/drm/card*/device/{gpu_busy_percent, mem_info_vram_used,
  mem_info_vram_total, hwmon/hwmon*/temp1_input, pp_dpm_sclk, pp_dpm_mclk, power/amdgpu/*}`.
- **Per-process (k≥6.7, the standard DRM fdinfo format):**
  `/proc/<pid>/fdinfo/<fd>` → `drm-client-id`, `drm-engine-gfx` (ns), `drm-memory-vram/resident/
  active/shared`, `drm-maxfreq-gfx`. Diff engine time between ticks → %.
- **KFD:** `/sys/class/kfd/…`, process apertures ioctl for ROCm per-proc memory.
- **AMD SMI (`libamdsmi`, MIT):** rich device metrics (`get_gpu_activity` GFX/MM/MEM%, power,
  clocks, gpu_metrics table); replaced rocm-smi-lib; repo moved Rocm/amdsmi → ROCm/rocm-systems.
- **CLI fallback:** `rocm-smi --showuse --showmeminfo --showpids`; `amd-smi`.

### 4.3 Intel GPU (i915 / Xe)

- **Per-process via fdinfo** (i915 k≥5.15): `drm-engine-render/copy/video` (ns), `drm-memory-local`,
  `drm-client-id`. New **Xe/`gputop`** for Battlemage+ (intel_gpu_top is i915-only).
- **`intel_gpu_top -J`** (igt-gpu-tools, MIT): JSON with per-client `{pid, name, busy, rt}` —
  the simplest reliable per-process source. Requires perf PMU access.
- **Level Zero Sysman (`zesDeviceProcessesGetState`)** — Linux + Windows, needs
  `ZES_ENABLE_SYSMAN=1`; CGo wrap of `zes_api.h`.
- **RAPL** power; sysfs `gt/gt*/freq_cur`.

### 4.4 Apple Silicon (macOS)

- **IOReport/IOKit** (private): GPU engine utilization + `accumulatedGPUTime` (per-process),
  ANE channels. **Go references:** `mactop` (CGo), `aneperf` (purego). Private-API breakage risk
  between macOS releases — wrap & degrade.
- **`powermetrics`** (sudo): GPU util/freq/power, ANE util+power, **`--show-process-gpu`** per-PID.
- **IORegistry:** `ioreg -c IOGPU` → `PerformanceStatistics` (aggregate).
- **VRAM per process:** `task_vm_info.graphics_footprint`, needs SIP off.
- IOKit basic util typically needs no sudo; IOReport partial.

### 4.5 NVIDIA Jetson

- **`go-nvml`** works on L4T (per-process GPU). **`tegrastats`** engine breakdown (GR3D, NVENC,
  NVDEC, NVDLA, EMC, per-rail power). **jtop/jetson-stats** is Python.
- sysfs: `/sys/devices/gpu.0`, bpmp debug.

### 4.6 NPUs

| NPU | Backend | Metrics |
|---|---|---|
| **Hailo-8/-10** | `hailortcli monitor` + libhailort C API (MIT) | VPU util, temperature, bandwidth; **no per-process** |
| **Rockchip** | debugfs `/sys/kernel/debug/rknpu`, devfreq `/sys/class/devfreq/fdab0000.npu` | load + freq; **no per-process** |
| **Intel ivpu** | sysfs `/sys/class/accel/accel*/{npu_busy_time_us, npu_memory_utilization, npu_*_frequency_mhz}`; Level Zero | util + mem (device-level) |
| **AMD XDNA** | `amdxdna` DRM fdinfo (`drm-engine-amdxdna`), kernel ≥6.5; `xrt-smi` | **per-process engine time** |
| **Qualcomm Hexagon** | OEM Snapdragon Profiler | minimal |

### 4.7 TPU

- **Coral Edge TPU:** no util API (libedgetpu archived). Enumerate only.
- **Google Cloud TPU:** Cloud Monitoring auto-telemetry, LibTPU runtime telemetry, AI Telemetry
  Collector (Prometheus). Only relevant for remote/server mode.

---

## 5. Per-Process Attribution — The Unified Mechanism

### 5.1 Driver-provided (accurate, vendor-specific)

- NVIDIA NVML, Intel fdinfo/i915, Level Zero Sysman, AMD XDNA fdinfo, Apple IOReport.

### 5.2 DRM fdinfo — the Linux breakthrough

Standardized per-client engine time + memory (k ≥ 6.7; partial 5.15+). One walker handles
**amdgpu, i915, Xe, amdxdna** (and registers drivers adopting it: msm/Adreno, panfrost/panthor,
nouveau). Algorithm:

```text
for each PID:
  for each fd in /proc/<pid>/fd:
    resolve symlink → starts with /dev/dri/ ?
      read /proc/<pid>/fdinfo/<fd>
      parse drm-client-id, drm-engine-* (ns), drm-memory-* (bytes)
  aggregate fds with the same drm-client-id → one process row
```

Kernel/driver variance is the risk: version-gate features, normalize into the `ProcUsage`
contract, and unit-test with captured fdinfo fixtures.

### 5.3 OS scheduler / estimation (when nothing else)

- **Windows:** WDDM2 perf counters (per-PID, per-engine) — production-grade, all vendors.
- **Apple:** `accumulatedGPUTime` (per-process, experimental) — fallback to power-weighted ANE
  (never per-process).

> **Attribution ladder:** driver per-PID → DRM fdinfo → WDDM/perf counters → power/estimation →
> N/A (honest chip).

---

## 6. Process Management Primitives (Go)

Use **`github.com/shirou/gopsutil/v4`** (BSD-3, Linux/macOS/Windows):

| Op | Linux | macOS | Windows |
|---|---|---|---|
| Kill | `SIGKILL` | same | `TerminateProcess` |
| Terminate | `SIGTERM` | same | `TerminateProcess(1)` |
| Suspend/Resume | `SIGSTOP`/`SIGCONT` | same | `NtSuspendProcess`/`ResumeThread` |
| Priority | `setpriority` (-20..19) | `setpriority` (-20..20) | `SetPriorityClass` (6 classes) |
| List/inspect | `process.Processes()`, CPU%, mem, IO counters |

**Permissions reality:** Linux needs same UID or root/CAP_KILL; **macOS SIP blocks killing
system processes even as root**; Windows needs admin token for system procs. Surface permission
denial as an inline tooltip, never a silent no-op.

**GPU-level controls (vendor wrappers):** NVIDIA compute mode cycle, MPS start/stop, MIG
list/enable, `nvidia-smi -r`; AMD KFD eviction on reset.

---

## 7. Framework Analysis

### 7.1 Wails v3 (chosen — current repo scaffold)

- **Architecture:** OS-native webview (WebView2 / WKWebView / WebKitGTK), Go services over
  sub-ms IPC, static-analysis-generated TS bindings. Services have `ServiceStartup`/`ServiceShutdown`.
- **High-frequency telemetry:** one batched event per tick, small payloads (<8KB) → zero
  retention/drop path (Wails PR #5930, #5934). 1-2 Hz × devices is trivial; polled bindings also fine.
- **Native libs:** CGO required for desktop anyway; `go-nvml` dlopens at runtime; Wails itself
  already uses `dlsym`. Recover blocks around CGO calls (same-process = a C crash kills the app).
- **Charts:** uPlot / ECharts render fine on WebView2 + WKWebView; WebKitGTK may need
  software-rendering fallback for NVIDIA DRM perm issues.
- **Distribution:** single binary + `//go:embed`; cross-compile via Docker/Zig; `-tags server`
  headless + WebSocket for remote mode. Prior art: Observer, HWnow, NVSmiBar (all Wails v2).

### 7.2 Alternatives compared

| Dimension | Wails v3 | Tauri 2 | Electron | Flutter | Qt |
|---|---|---|---|---|---|
| Backend | Go | Rust | JS | Dart | C++ |
| Size / RAM | 10-30MB / 25-80MB | 10-40MB | 150-300MB | 30-80MB | 30-80MB |
| Native/driver access | CGO+dlopen | libloading | node-gyp | ffm | direct C++ |
| IPC | <1ms binding | 1-10ms | 1-10ms | channels | direct |

**Decision:** stay on Wails v3 (Go + CGO + dlopen + gopsutil + go-nvml) — lowest friction for the
vendor-SDK matrix, tiny footprint, and matches the existing scaffold.

---

## 8. Recommended Architecture

```
┌──────────────────────────────────────────────────────────┐
│  React + TS + Tailwind v4 + ECharts (Wails webview)     │
└───────────────▲─────────────────────────────┬───────────┘
         1Hz batched events          method bindings (RPC)
┌───────────────┴─────────────────────────────┴───────────┐
│  GO CORE                                                │
│  DeviceRegistry · MetricService (ring buffer)           │
│  ProcessService (gopsutil kill/suspend/nice)            │
│  ControlService (compute mode / MPS / reset)            │
│  AlertEngine · Workload Classifier · HistoryService     │
│  ───────── AcceleratorSource interface ────────────────│
│  nvidia │ amd │ intel │ apple │ xdna │ ivpu │ hailo    │
│  (nvml) (fdinfo)(fdinfo)(ioReport)(fdinfo)(sysfs)(cli) │
└─────────────────────────────────────────────────────────┘
```

```go
type AcceleratorSource interface {
    Name() string
    Kind() Kind                       // GPU | NPU | TPU
    Detect() bool                     // hardware present?
    Devices() []DeviceInfo
    Sample(ctx context.Context) (Sample, error)
}
type Sample struct {
    Devices []DeviceMetrics            // util, mem, temp, power, clocks
    Procs   []ProcUsage                // PID → devID, mem, util, engine breakdown
}
```

Open questions resolved by an honest "tiers" strategy (below).

---

## 9. Technology Decision Summary

| Concern | Choice | Why |
|---|---|---|
| App shell | **Wails v3** (existing scaffold) | Go + embedded frontend, sub-ms IPC, server mode |
| UI | **React 18 + TS + Tailwind v4** (`@tailwindcss/vite`, Vite 8-ok) | compact pro-tool density, fast iteration |
| Charts | **ECharts** | rich recipes, streaming via `setOption(…,{notMerge:false})` |
| Process mgmt | `gopsutil/v4` | cross-platform kill/suspend/nice |
| NVIDIA / Jetson | `go-nvml` (+ nvidia-smi fallback, DCGM server-tier) | official, per-process, dlopen |
| AMD GPU | sysfs + DRM fdinfo (pure Go) → AMD SMI (CGo) | no CGo needed for the basics |
| Intel GPU | fdinfo + `gputop -J`; Level Zero Sysman on Windows | per-client JSON |
| Apple GPU/ANE | IOReport/IOKit (mactop/aneperf pattern) + powermetrics | sudoless ANE + per-proc GPU |
| AMD NPU | `amdxdna` DRM fdinfo | per-process engine time |
| Intel NPU | ivpu sysfs | device-level |
| Hailo | `hailortcli monitor` subprocess | MIT, CLI integration |
| Coral TPU | enumerate only + honest N/A | no vendor API |

### Source tiers (graceful degradation)

1. Native Go library (go-nvml) — best.
2. CGo wrap of vendor C API (AMD SMI, Level Zero).
3. Pure-Go sysfs/fdinfo reads — no CGo.
4. Subprocess + parse (`gputop -J`, `powermetrics`, `hailortcli`, `tegrastats`, `nvidia-smi`).
5. Enumerate-only → honest `N/A` (Coral), or power-derived aggregate (ANE).

---

## 10. Risk Register

| Risk | Severity | Mitigation |
|---|---|---|
| Wails v3 is beta (β.22) | Med | pin version; isolated spike; software-render fallback for WebKitGTK |
| Apple IOReport private-API churn | High | version-gate + degrade to powermetrics |
| fdinfo variance across kernels/drivers | Med | version-gate; fixture-based unit tests |
| go-nvml linker issues (gold, Go 1.21+) | Low | use documented flags; upstream fixed |
| NPU per-process = mostly impossible | **Known limit** | honest N/A chips; device-level only; auto-kill zombies where detectable |
| macOS SIP blocks killing system procs even as root | Med | surface permission errors inline |
| Co-shipping many vendor SDKs (binary size) | Low | dlopen at runtime; no static linking |

---

## 11. Build Order (from the plan doc)

Phase 2 **NVIDIA first** (proves collect→aggregate→push→render→manage) → Phase 1 system/process
layer → Phase 3 DRM fdinfo engine (AMD/Intel/XDNA at once) → Phase 4 Apple → Phase 5 NPUs (ivpu,
Rockchip, Hailo, Jetson) → Phase 6 history/alerts/classification → Phase 7 server mode + packaging
→ Phase 8 hardening. See the build-plan section of this repo's planning docs / MOCKUP.md for UI.

---

## 12. Key References

- nvitop — https://github.com/XuehaiPan/nvitop · nvtop — https://github.com/Syllo/nvtop
- gpu-kill — https://github.com/treadiehq/gpu-kill · amdgpu_top — https://crates.io/crates/amdgpu_top
- igt-gpu-tools (intel_gpu_top/gputop) — https://gitlab.freedesktop.org/drm/igt-gpu-tools
- aneperf — https://github.com/tmc/aneperf · mactop — https://pkg.go.dev/github.com/metaspartan/mactop/v2 · macmon — https://github.com/vladkens/macmon
- go-nvml — https://github.com/NVIDIA/go-nvml · DCGM — https://github.com/NVIDIA/DCGM
- AMD SMI — https://rocm.docs.amd.com/projects/amdsmi · amdxdna — https://kernel.org/doc/html/latest/accel/amdxdna/
- Intel linux-npu-driver — https://github.com/intel/linux-npu-driver · HailoRT — https://github.com/hailo-ai/hailort
- libedgetpu — https://github.com/google-coral/libedgetpu · jetson-stats — https://github.com/rbonghi/jetson_stats
- gopsutil — https://github.com/shirou/gopsutil · Wails v3 — https://v3.wails.io
- Linux DRM client usage stats — kernel docs (drm-usage-stats.rst)