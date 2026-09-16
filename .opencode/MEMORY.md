# pmanage — Shared Memory (persists across sessions)

Project: Wails v3 desktop app for unified GPU/NPU/TPU process monitoring (`pmanage`).
Root: /Users/charith/Desktop/GO-PROJECTS/Projects/pmanage. Module `pmanage`,
Go 1.25, go-nvml v0.13.4 (cgo), Wails v3.0.0-beta.22, TS frontend.
Authoritative plan: PLAN.md (Phases 0-8; "Phase 7 = Remote/Server Mode & Packaging").

## Phase 7 status (verified on this darwin machine)
- Server mode DONE + smoke-tested 7/7 locally (scripts/e2e_smoke.sh):
  `-tags server` headless build → /health ok, token gates IPC (401 without token),
  read-only gate verified (kill disabled unless remote-admin granted).
  Server details: token/env config ~/.config/pmanage/server.json, WebSocket IPC
  (WebSocketBroadcaster) fan-out to N browser clients, SSH-tunnel guidance.
- macOS packaging DONE + verified: bin/pmanage.app (ad-hoc codesign + hardened
  runtime) + bin/pmanage.dmg contains .app + Applications symlink. build/config.yml
  at v0.2.0, appicon/version/metadata finalized.
- Windows (NSIS+MSIX), Linux (deb/rpm/AppImage/AUR), Docker server cross-build —
  BLOCKED on this Mac (no cross-toolchains/Docker daemon/EV cert). build:server,
  build:docker, run:server, run:docker Tasks scaffolded, ready on a Linux builder.
- E2E smoke script per-OS on CI matrix — scaffolded (scripts/e2e_smoke.sh), only
  server-mode path runnable locally; Windows/Linux need CI hosts.

## Commands / verification baseline
- `go build ./...` and `go build -tags server -o bin/pmanage-server .` exit 0
  (cgo nvml deprecation warnings are noise). `go vet ./pkg/...` clean,
  `go test ./pkg/...` all pass.
- Frontend: `tsc --noEmit` clean, `npm run build` succeeds.
- darwin smoke: `scripts/e2e_smoke.sh` (PMANAGE_SMOKE_SERVER_ONLY=1 for headless).

## Architecture notes
- servermode gate: pkg/servermode/{config,middleware,service}.go; pkg/process
  Service has SetReadOnly/checkWrite; desktop builds pass nil (no gate).
- Server runtime uses WailsServer with token middleware + read-only for remote
  clients; local desktop keeps full kill control.
