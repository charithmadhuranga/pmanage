# pmanage

**Fleet-grade accelerator process manager** — see every GPU / NPU / TPU on every machine,
per-process, from one desktop app or one edge box. Honest about what it can and cannot see.

> **Now with server mode:** run `pmanage` headless on a fleet of edge devices and pull all of
> their telemetry into your browser over SSH-tunnelled WebSocket — no dashboards-box needed.

Logo: `frontend/public/pmanage-logo.svg` (SVG, also rendered as app icon in `build/darwin`/`build/windows`/`build/linux`).

---

## What it is

`pmanage` is a native desktop application (Wails v3 + React + ECharts, Go backend) that reads
**real accelerator telemetry** from real hardware — and never fabricates numbers. When a metric
isn't measurable on a given device it renders an honest **N/A** chip with the reason, instead of
inventing a plausible-looking value. No accelerator present? The app runs a clean empty state
(N/A chips) rather than a demo dashboard.

- **GPU** — NVIDIA (NVML + `nvidia-smi` fallback), AMD (KFD fdinfo + `amdgpu`), Intel iGPU (fdinfo + i915)
- **NPU** — AMD XDNA, Intel IVPU, Apple ANE (power), Hailo, Rockchip, `amdgpu_top`-class, Jetson NVDLA
- **TPU** — Google Coral (honest N/A per-process attribution)

---

## Capabilities by vendor

| Vendor | Detection | Per-process | Utilization | Power | Temps | Kill/suspend |
|---|---|---|---|---|---|---|
| NVIDIA | NVML ✓ | NVSMI ✓ | SM ✓ | ✓ | ✓ | ✓ (nvidia-smi models) |
| AMD GPU | fdinfo ✓ | fdinfo ✓ | ✓ | ✓ | ✓ | ✓ |
| Intel iGPU | fdinfo ✓ | fdinfo ✓ | ✓ | ✓ | ✓ | ✓ |
| Apple ANE | IOKit/IOReport ✓ | N/A (honest) | power only | ✓ | N/A | N/A |
| Hailo | hailortcli ✓ | N/A (honest) | ✓ | ✓ | ✓ | N/A |
| AMD XDNA | sysfs ✓ | ✓ | ✓ | N/A | ✓ | N/A |
| Google Coral | ✓ | N/A (honest) | N/A | N/A | ✓ | N/A |

---

## Quick start

### Desktop (macOS / Windows / Linux)

```sh
# deps
task dev:deps        # go mod tidy, npm ci, tailwind+echarts
# run hot-reload (macOS)
task run:darwin
# build a production binary
task build
```

Prefer the Taskfile — `task run`/`task build`/`task test` wire everything (Go + frontend +
Wails bundling+bindings).

### Server mode (headless fleet)

```sh
task build:server
# config: $HOME/.config/pmanage/server.json (token, host, port) or env PMANAGE_*
./bin/pmanage-server            # headless HTTP + WebSocket broadcast
# from a laptop: ssh -L 8123:host:8123 user@edge   then browse http://127.0.0.1:8123
```

Clients connect with a bearer token; the server is **read-only** for remote clients unless you
grant remote-admin explicitly — kill/suspend/migrate verbs stay gated.

---

## How it stays honest

- **No fake telemetry.** All metric rows come from NVML, `nvidia-smi`, sysfs fdinfo, IOKit,
  IOReport / powermetrics, `hailortcli`, or `amdgpu_top`-class walkers — never from a lookup
  table keyed by device name.
- **`N/A` with a reason.** If a column genuinely can't be attributed per-process (e.g. ANE util,
  VRAM-per-process on Coral), the UI renders `N/A` plus the exact reason, not a fabricated 0.
- **Mock is dev-only.** `pkg/accelerator/mock.go` is compiled only under `-tags mock`; the
  production binary never auto-injects synthetic devices, even when nothing is detected.
- **Empty hardware = empty app, honestly.** No synthetic demo dashboard when you have no
  accelerators — you get the real, empty state.

---

## Project layout

```
main.go                 entrypoint; source registry + service wiring
pkg/accelerator/        NVML/nvidia-smi/fdinfo/powermetrics/IOReport/Hailo/mock sources
pkg/process/            process list + kill/suspend/resume/nice (gopsutil)
pkg/history/            ring-buffer metric history + CSV/JSON export
pkg/alerts/             alert rule engine
pkg/classify/           workload classifier (training / inference / encoding / crypto)
pkg/servermode/         headless server config + token auth + read-only gate
pkg/metrics/            sampling service + telemetry fan-out
pkg/settings/           per-source toggles
frontend/src/           React + Tailwind v4 + ECharts UI
scripts/                build/test/e2e helpers
PLAN.md                 phase-by-phase plan and honest status
```

---

## Development

Requirements: Go 1.25, Node 20+, Task (optional), and one or more real accelerators for
meaningful output. Cross-platform builds for Windows/Linux run best on their own builders;
the Taskfile carves out per-OS packaging (`task package:darwin|windows|linux`).

```sh
task test         # go test ./... + tsc --noEmit + vitest (frontend)
task vet          # go vet ./...
task e2e          # smoke: detect → 1 Hz telemetry → kill a process
task package:dmg  # macOS .app + DMG (signed ad-hoc; full notarization needs Dev ID)
```

---

## Documentation

- `MOCKUP.md` — full UI mockups (shell, views, ECharts recipes, status chips)
- `RESEARCH.md` — hardware feasibility matrix per vendor/OS/API
- `PLAN.md` — phase plan with live, honest checkboxes

---

## Roadmap status

See `PLAN.md` for the phase ledger. Currently **Phase 6/7** are functionally complete
(history + alerts + classifier + workload view + server mode + read-only remote clients);
**Phase 8** hardening (collection tuning, CGO safety, permission matrix UI, CI matrix, docs,
SBOM/signing) is the open tail.

---

## License

Proprietary — © 2026 pmanage. See `LICENSE`.
