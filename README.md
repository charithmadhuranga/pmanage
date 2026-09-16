<p align="center">
  <img src="frontend/public/pmanage-logo.svg" alt="pmanage logo" width="120" height="120">
</p>

<h1 align="center">pmanage</h1>

<p align="center">
  <strong>Fleet-grade GPU / NPU / TPU process manager</strong><br>
  See every accelerator on every machine, per-process, from one desktop app or one edge box.
</p>

<p align="center">
  <a href="#quick-start"><img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-blue" alt="Platforms"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="License"></a>
  <a href="https://github.com/charithmadhuranga/pmanage/tree/dev"><img src="https://img.shields.io/badge/branch-dev-brightgreen" alt="Branch"></a>
  <a href="#supported-accelerators"><img src="https://img.shields.io/badge/GPU%2F%2FNPU%2F%2FNPU-14%20vendors-purple" alt="Accelerators"></a>
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.25-00ADD8?logo=go" alt="Go"></a>
  <a href="https://wails.io"><img src="https://img.shields.io/badge/Wails-v3-FF1E56" alt="Wails"></a>
  <a href="https://react.dev"><img src="https://img.shields.io/badge/React-19-61DAFB?logo=react" alt="React"></a>
</p>

<p align="center">
  <em>Honest telemetry — no fabricated metrics. When a metric isn't measurable, it renders <strong>N/A</strong> with the reason.</em>
</p>

---

## Table of Contents

- [What it is](#what-it-is)
- [Supported Accelerators](#supported-accelerators)
- [Quick Start](#quick-start)
- [Server Mode](#server-mode)
- [How It Stays Honest](#how-it-stays-honest)
- [Project Layout](#project-layout)
- [Development](#development)
- [Documentation](#documentation)
- [Roadmap](#roadmap)
- [License](#license)

---

## What it is

`pmanage` is a native desktop application (Wails v3 + React + ECharts, Go backend) that reads **real accelerator telemetry** from real hardware — and never fabricates numbers. When a metric isn't measurable on a given device it renders an honest **N/A** chip with the reason, instead of inventing a plausible-looking value.

**Key features:**

- Real-time GPU/NPU/TPU utilization, temperature, power, memory, and per-process attribution
- Fleet monitoring via headless server mode (HTTP + WebSocket, token-authenticated)
- Process management: kill, suspend, resume, renice — with safety gates
- Workload classification: training, inference, encoding, crypto
- Alert engine with threshold-based notifications
- History ring buffer with CSV/JSON export

---

## Supported Accelerators

### NVIDIA GPUs

| Device Family | Detection | Per-Process GPU | Utilization | Power | Temperature | Clocks | Memory (VRAM) | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|---|---|
| GeForce RTX 40/30/20 series | NVML ✓ | ✓ | SM % | ✓ | ✓ | ✓ | ✓ | ✓ | Linux, Windows, macOS |
| GeForce GTX 16/10 series | NVML ✓ | ✓ | SM % | ✓ | ✓ | ✓ | ✓ | ✓ | Linux, Windows |
| Tesla / A100 / H100 / L40 | NVML ✓ | ✓ | SM % | ✓ | ✓ | ✓ | ✓ | ✓ | Linux |
| Jetson (Orin, Xavier) | NVML ✓ | ✓ | SM % | ✓ | ✓ | ✓ | ✓ | ✓ | Linux (L4T) |
| Fallback (`nvidia-smi`) | ✓ | ✓ | SM % | ✓ | ✓ | — | ✓ | ✓ | Linux, Windows |

**Data source:** NVML (`libnvidia-ml.so`) with `nvidia-smi` fallback. Per-process GPU time via `nvmlDeviceGetComputeRunningProcesses` / `nvmlDeviceGetGraphicsRunningProcesses`.

---

### AMD GPUs

| Device Family | Detection | Per-Process GPU | Utilization | Power | Temperature | Clocks | Memory (VRAM) | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|---|---|
| Radeon RX 7000/6000 series | amdgpu ✓ | fdinfo ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | Linux |
| Radeon Pro / Instinct | amdgpu ✓ | fdinfo ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | Linux |

**Data source:** `/sys/class/drm/card*/device/gpu_busy_percentage` + fdinfo for per-process. Requires kernel 5.14+ for per-process fdinfo.

---

### Intel GPUs

| Device Family | Detection | Per-Process GPU | Utilization | Power | Temperature | Clocks | Memory | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|---|---|
| Arc A-series (Alchemist) | i915/xe ✓ | fdinfo ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | Linux |
| UHD / Iris (integrated) | i915 ✓ | fdinfo ✓ | ✓ | ✓ | ✓ | ✓ | — | ✓ | Linux |

**Data source:** `/sys/class/drm/card*/device/` via i915/xe driver. Requires kernel 5.19+ for per-process fdinfo. Intel GPU requires `CAP_PERFMON` for memory access.

---

### Apple Silicon

| Device | Detection | Per-Process GPU | Utilization | Power | Temperature | Clocks | Memory | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|---|---|
| Apple M1 / M1 Pro / M1 Max / M1 Ultra | IOKit ✓ | ✓ (AGXDeviceUserClient) | Device + Renderer + Tiler % | ✓ (IOReport) | — | — | Alloc/InUse | — | macOS 13+ |
| Apple M2 / M2 Pro / M2 Max / M2 Ultra | IOKit ✓ | ✓ (AGXDeviceUserClient) | Device + Renderer + Tiler % | ✓ (IOReport) | — | — | Alloc/InUse | — | macOS 13+ |
| Apple M3 / M3 Pro / M3 Max | IOKit ✓ | ✓ (AGXDeviceUserClient) | Device + Renderer + Tiler % | ✓ (IOReport) | — | — | Alloc/InUse | — | macOS 13+ |
| Apple M4 / M4 Pro / M4 Max | IOKit ✓ | ✓ (AGXDeviceUserClient) | Device + Renderer + Tiler % | ✓ (IOReport) | — | — | Alloc/InUse | — | macOS 13+ |
| Apple ANE (Neural Engine) | IOKit ✓ | N/A (honest) | — | ✓ (IOReport PMP) | — | — | — | — | macOS 13+ |
| Powermetrics (opt-in) | powermetrics ✓ | N/A (honest) | — | ✓ | ✓ | ✓ | — | — | macOS (sudo) |

**Data source:** IOKit `IOGPU` `PerformanceStatistics` (CGo), IOReport energy channels (`Energy Model` → GPU, `PMP` → ANE), AGXDeviceUserClient for per-process GPU time. GPU model/core count from `ioreg -r -c AGXAccelerator`.

---

### Hailo NPUs

| Device | Detection | Per-Process | NNC Utilization | Power | Temperature | Clocks | RAM | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|---|---|
| Hailo-8 | hailortcli ✓ | N/A (honest) | ✓ | ✓ (voltage) | ✓ | ✓ (from identify) | ✓ (used/total) | — | Linux, Windows |
| Hailo-8L | hailortcli ✓ | N/A (honest) | ✓ | ✓ | ✓ | ✓ | ✓ | — | Linux, Windows |
| Hailo-10H | hailortcli ✓ | N/A (honest) | ✓ | ✓ | ✓ | ✓ | ✓ | — | Linux |
| Hailo-15H (SoC) | hailortcli ✓ | N/A (honest) | ✓ | ✓ | ✓ | ✓ | ✓ | — | Linux |
| Hailo-12L (Mars) | hailortcli ✓ | N/A (honest) | ✓ | ✓ | ✓ | ✓ | ✓ | — | Linux |

**Data source:** `hailortcli fw-control identify --extended` (architecture, firmware, clock rate, serial), `hailortcli monitor` (NNC/CPU/RAM utilization, on-die temperature, on-die voltage). Per-process attribution not available on Hailo hardware.

---

### AMD XDNA (Ryzen AI)

| Device | Detection | Per-Process | Utilization | Power | Temperature | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|
| Ryzen AI 700/800 series | sysfs ✓ | fdinfo ✓ | ✓ | — | ✓ | — | Linux |

**Data source:** `/sys/class/accel/accel*/` via `amdxdna` driver.

---

### Intel NPU (Meteor Lake+)

| Device | Detection | Per-Process | Utilization | Power | Temperature | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|
| Intel AI Boost (Meteor Lake) | sysfs ✓ | fdinfo ✓ | ✓ | — | ✓ | — | Linux |

**Data source:** `/sys/class/accel/accel*/` via `intel_vpu` driver.

---

### Qualcomm Adreno

| Device | Detection | Per-Process | Utilization | Power | Temperature | Clocks | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|---|
| Adreno 7xx/6xx (Snapdragon) | msm sysfs ✓ | N/A | ✓ (gpu_busy) | — | ✓ | ✓ | — | Linux 6.0+ |

**Data source:** `/sys/class/drm/card*/device/kgsl-3d0/` via MSM driver. Per-process requires kernel 6.0+ fdinfo.

---

### Huawei Ascend

| Device | Detection | Per-Process | Utilization | Power | Temperature | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|
| Ascend 910B/910C | davinci sysfs ✓ | N/A | ✓ | ✓ | ✓ | — | Linux |

**Data source:** `/dev/davinci*` device nodes + `/sys/class/davinci_manager/` sysfs.

---

### NVIDIA Jetson (Tegra)

| Device | Detection | Per-Process GPU | Utilization | Power | Temperature | Clocks | Memory | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|---|---|
| Jetson Orin AGX/NX | tegrastats ✓ | nvidia-smi ✓ | GR3D % | VDD_GPU ✓ | ✓ | ✓ | RAM ✓ | ✓ | Linux (L4T) |
| Jetson Xavier AGX/NX | tegrastats ✓ | nvidia-smi ✓ | GR3D % | VDD_GPU ✓ | ✓ | ✓ | RAM ✓ | ✓ | Linux (L4T) |

**Data source:** `tegrastats` for GPU util/clocks/power, `nvidia-smi` for per-process GPU memory.

---

### Broadcom VideoCore

| Device | Detection | Per-Process | Utilization | Power | Temperature | Clocks | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|---|
| VideoCore VI (RPi 4) | v3d/vc4 ✓ | N/A | ✓ (debugfs) | — | ✓ | ✓ | — | Linux |
| VideoCore VII (RPi 5) | v3d/vc4 ✓ | N/A | ✓ (debugfs) | — | ✓ | ✓ | — | Linux 6.12+ |

**Data source:** `/sys/class/drm/card*/device/` via v3d/vc4 driver, `/dev/vcio` mailbox interface.

---

### Tenstorrent

| Device | Detection | Per-Process | Utilization | Power | Temperature | Fans | Clocks | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|---|---|
| Wormhole / Blackhole | PCI hwmon ✓ | N/A | ✓ (debugfs) | ✓ | ✓ | ✓ | ✓ | — | Linux |
| Grayskull | PCI hwmon ✓ | N/A | ✓ (debugfs) | ✓ | ✓ | ✓ | ✓ | — | Linux |

**Data source:** PCI vendor `0x1e2e`, `/sys/class/hwmon/hwmon*/` for temp/power/fan, debugfs for clock/utilization.

---

### Rockchip NPU

| Device | Detection | Per-Process | Utilization | Power | Temperature | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|
| RK3588 NPU | sysfs ✓ | N/A | ✓ (devfreq) | — | ✓ | — | Linux |
| RK3566/RK3568 NPU | sysfs ✓ | N/A | ✓ (devfreq) | — | ✓ | — | Linux |

**Data source:** `/sys/class/devfreq/` for NPU utilization.

---

### Google Coral TPU

| Device | Detection | Per-Process | Utilization | Power | Temperature | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|
| Coral USB / M.2 TPU | ✓ | N/A (honest) | N/A | — | ✓ | — | Linux |

**Data source:** `libedgetpu` (archived). Per-process attribution not available.

---

### Hailo via sysfs (Linux native)

| Device | Detection | Per-Process | Utilization | Power | Temperature | Kill/Suspend | Platform |
|---|---|---|---|---|---|---|---|
| Hailo-8 (PCIe, Linux) | sysfs ✓ | N/A | ✓ | ✓ | ✓ | — | Linux |

**Data source:** `/sys/class/accel/accel*/` via `hailort` kernel driver (when `hailortcli` is not installed).

---

## Summary: Feature Matrix

| Vendor | Detection | Per-Process GPU | Utilization | Power | Temperature | Clocks | Memory | Kill/Suspend |
|---|---|---|---|---|---|---|---|---|
| **NVIDIA** | NVML | ✓ (NVML) | SM % | ✓ | ✓ | ✓ | VRAM | ✓ |
| **AMD** | amdgpu | ✓ (fdinfo) | ✓ | ✓ | ✓ | ✓ | VRAM | ✓ |
| **Intel** | i915/xe | ✓ (fdinfo) | ✓ | ✓ | ✓ | ✓ | — | ✓ |
| **Apple** | IOKit | ✓ (AGXClient) | Dev/Ren/Til % | ✓ | — | — | Alloc/InUse | — |
| **Hailo** | hailortcli | N/A | NNC % | ✓ | ✓ | ✓ | RAM | — |
| **AMD XDNA** | sysfs | ✓ (fdinfo) | ✓ | — | ✓ | — | — | — |
| **Intel NPU** | sysfs | ✓ (fdinfo) | ✓ | — | ✓ | — | — | — |
| **Adreno** | msm sysfs | N/A | ✓ | — | ✓ | ✓ | — | — |
| **Ascend** | davinci | N/A | ✓ | ✓ | ✓ | — | — | — |
| **Jetson** | tegrastats | ✓ (nvidia-smi) | GR3D % | ✓ | ✓ | ✓ | RAM | ✓ |
| **VideoCore** | v3d/vc4 | N/A | ✓ | — | ✓ | ✓ | — | — |
| **Tenstorrent** | PCI/hwmon | N/A | ✓ | ✓ | ✓ | ✓ | — | — |
| **Rockchip** | sysfs | N/A | ✓ | — | ✓ | — | — | — |
| **Coral** | libedgetpu | N/A | N/A | — | ✓ | — | — | — |

---

## Quick Start

### Desktop (macOS / Windows / Linux)

```sh
# dependencies
task dev:deps        # go mod tidy, npm ci, tailwind+echarts
# run hot-reload (macOS)
task run:darwin
# build a production binary
task build
```

### Server Mode (headless fleet)

```sh
task build:server
# config: $HOME/.config/pmanage/server.json (token, host, port) or env PMANAGE_*
./bin/pmanage-server            # headless HTTP + WebSocket broadcast
# from a laptop: ssh -L 8123:host:8123 user@edge   then browse http://127.0.0.1:8123
```

Clients connect with a bearer token; the server is **read-only** for remote clients unless you grant remote-admin explicitly — kill/suspend/migrate verbs stay gated.

---

## How It Stays Honest

- **No fake telemetry.** All metric rows come from NVML, `nvidia-smi`, sysfs fdinfo, IOKit, IOReport / powermetrics, `hailortcli`, or MSM driver — never from a lookup table keyed by device name.
- **`N/A` with a reason.** If a column genuinely can't be attributed per-process (e.g. ANE util, VRAM-per-process on Coral), the UI renders `N/A` plus the exact reason.
- **Mock is dev-only.** `pkg/accelerator/mock.go` is compiled only under `-tags mock`; the production binary never auto-injects synthetic devices.
- **Empty hardware = empty app, honestly.** No synthetic demo dashboard when you have no accelerators.

---

## Project Layout

```
main.go                    entrypoint; source registry + service wiring
pkg/accelerator/           NVML/nvidia-smi/fdinfo/powermetrics/IOReport/Hailo/sources
  apple_io.go              IOKit CGo: GPU util, memory, per-process GPU time, ANE
  apple_darwin.go          Apple Silicon source (util, power, processes)
  powermetrics_darwin.go   opt-in powermetrics source (thermal, power)
  nvidia.go                NVIDIA NVML source
  nvidia_smi.go            NVIDIA nvidia-smi fallback source
  amdgpu_linux.go          AMD GPU (amdgpu) source
  intel_linux.go           Intel GPU (i915/xe) source
  hailo.go                 Hailo NPU source (hailortcli)
  ivpu.go                  Intel NPU (VPU) source
  xdna_linux.go            AMD XDNA (Ryzen AI) source
  rockchip.go              Rockchip NPU source
  jetson.go                NVIDIA Jetson (Tegra) source
  adreno.go                Qualcomm Adreno GPU source
  ascend.go                Huawei Ascend NPU source
  videocore.go             Broadcom VideoCore GPU source
  tenstorrent.go           Tenstorrent AI accelerator source
  registry.go              source registry + detect/sample orchestration
pkg/process/               process list + kill/suspend/resume/nice (gopsutil)
pkg/history/               ring-buffer metric history + CSV/JSON export
pkg/alerts/                alert rule engine
pkg/classify/              workload classifier (training / inference / encoding / crypto)
pkg/servermode/            headless server config + token auth + read-only gate
pkg/metrics/               sampling service + telemetry fan-out
pkg/settings/              per-source toggles
frontend/src/              React + Tailwind v4 + ECharts UI
scripts/                   build/test/e2e helpers
```

---

## Development

Requirements: Go 1.25, Node 20+, Task (optional), and one or more real accelerators for meaningful output.

```sh
task test         # go test ./... + tsc --noEmit + vitest (frontend)
task vet          # go vet ./...
task e2e          # smoke: detect → 1 Hz telemetry → kill a process
task package:dmg  # macOS .app + DMG (signed ad-hoc)
```

---

## Documentation

- `MOCKUP.md` — full UI mockups (shell, views, ECharts recipes, status chips)
- `RESEARCH.md` — hardware feasibility matrix per vendor/OS/API
- `PLAN.md` — phase plan with live, honest checkboxes

---

## Roadmap

See `PLAN.md` for the phase ledger. Currently **Phase 6/7** are functionally complete (history + alerts + classifier + workload view + server mode + read-only remote clients); **Phase 8** hardening (collection tuning, CGO safety, permission matrix UI, CI matrix, docs, SBOM/signing) is the open tail.

---

## License

[MIT](LICENSE) -- © 2026 pmanage contributors
