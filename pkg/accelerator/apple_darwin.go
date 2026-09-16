//go:build darwin

package accelerator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
)

// AppleSource reads Apple Silicon GPU / ANE metrics:
//   - aggregate GPU utilization + system memory from IOKit PerformanceStatistics
//   - GPU power from the IOReport "Energy Model" / "GPU Energy" channel delta
//   - ANE power from the IOReport "PMP" / "ANE" channel delta
//
// All power values are derived from cumulative energy counters and therefore need
// two samples at least one telemetry interval apart; the first sample reports 0 W.
type AppleSource struct {
	mu      sync.Mutex
	devices []DeviceInfo
	hasANE  bool

	// Cached GPU stats from last Detect() — avoids re-enumerating IOKit every tick.
	cachedGPUStats []appleGPUStat

	gpuEnergy *appleEnergySub // "Energy Model" group
	aneEnergy *appleEnergySub // "PMP" group

	lastGPUJ   float64
	lastANEj   float64
	lastSample time.Time
	first      bool

	// Per-process GPU time deltas (Activity-Monitor GPU column source).
	gpuProc     map[int]uint64 // pid -> cumulative accumulatedGPUTime (ns)
	gpuProcInit bool           // true after first baseline capture
}

func NewAppleSource() *AppleSource {
	return &AppleSource{first: true}
}

func (a *AppleSource) Name() string { return "apple" }

func (a *AppleSource) Kind() Kind { return GPU }

// Capabilities advertises what the Apple source observes: per-process GPU time
// (AGXDeviceUserClient AppUsage), IOReport power, aggregate util/vram. No temp,
// no fan, no control verbs (compute mode/MPS/MIG/reset).
func (a *AppleSource) Capabilities() []Capability {
	return []Capability{
		CapPerProcessGPU, CapPower,
	}
}

func (a *AppleSource) Detect() bool {
	gpus := appleGPUs()
	ane := appleANEs()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.devices = a.devices[:0]
	a.hasANE = false
	a.cachedGPUStats = gpus
	for i, g := range gpus {
		name := g.Model
		if name == "" {
			name = friendlyAppleName(g.Name)
		}
		a.devices = append(a.devices, DeviceInfo{
			ID:      fmt.Sprintf("gpu%d", i),
			Name:    name,
			Vendor:  "Apple",
			Kind:    GPU,
			Driver:  "AGX",
			Version: fmt.Sprintf("%d cores", g.CoreCount),
		})
	}
	if ane.Found {
		a.hasANE = true
		name := ane.Name
		if name == "" {
			name = "Apple Neural Engine"
		}
		a.devices = append(a.devices, DeviceInfo{
			ID:     "npu0",
			Name:   name,
			Vendor: "Apple",
			Kind:   NPU,
			Driver: "ANE",
		})
	}
	return len(a.devices) > 0
}

func (a *AppleSource) Devices() []DeviceInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]DeviceInfo, len(a.devices))
	copy(out, a.devices)
	return out
}

func (a *AppleSource) Sample(_ context.Context) (Sample, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.first {
		a.gpuEnergy = openAppleEnergy("Energy Model")
		a.aneEnergy = openAppleEnergy("PMP")
		a.first = false
	}

	// Re-read GPU stats every tick (PerformanceStatistics values change).
	gpus := appleGPUs()
	a.cachedGPUStats = gpus

	now := time.Now()
	out := Sample{Timestamp: now.UnixMilli()}
	totalMem := uint64(0)
	if vm, err := mem.VirtualMemory(); err == nil {
		totalMem = vm.Total
	}

	// Cumulative energy read once per tick; deltas below are J consumed this tick.
	gpuJ, gpuOK := 0.0, false
	aneJ, aneOK := 0.0, false
	if a.gpuEnergy != nil {
		gpuJ, gpuOK = a.gpuEnergy.Joules("GPU Energy")
	}
	if a.aneEnergy != nil {
		aneJ, aneOK = a.aneEnergy.Joules("ANE")
	}

	gpuW := 0.0
	aneW := 0.0
	interval := now.Sub(a.lastSample) // captured before lastSample is reset below
	if a.lastSample.IsZero() {
		a.lastSample = now
		a.lastGPUJ = gpuJ
		a.lastANEj = aneJ
	} else {
		dt := interval.Seconds()
		if dt > 0 {
			if gpuOK && gpuJ >= a.lastGPUJ {
				gpuW = (gpuJ - a.lastGPUJ) / dt
			}
			if aneOK && aneJ >= a.lastANEj {
				aneW = (aneJ - a.lastANEj) / dt
			}
		}
		a.lastSample = now
		a.lastGPUJ = gpuJ
		a.lastANEj = aneJ
	}

	// --- GPU device metrics ---
	for i := range gpus {
		id := fmt.Sprintf("gpu%d", i)
		m := DeviceMetrics{
			DeviceID:     id,
			Status:       "ok",
			PowerW:       gpuW,
			TemperatureC: -1, // Apple does not expose GPU temp via IOKit
		}
		if gpus[i].HasStats {
			m.UtilizationPct = gpus[i].DevUtil
			m.VRAMUsed = gpus[i].AllocSysMem
			m.VRAMTotal = totalMem
		} else {
			m.Status = "n/a"
			m.UtilizationPct = -1
		}
		out.Devices = append(out.Devices, m)
	}

	// --- NPU device metrics ---
	if a.hasANE {
		out.Devices = append(out.Devices, DeviceMetrics{
			DeviceID:       "npu0",
			UtilizationPct: -1, // Apple does not expose ANE utilization
			PowerW:         aneW,
			VRAMTotal:      totalMem, // ANE uses unified memory
			TemperatureC:   -1,       // Apple does not expose ANE temp
			Status:         "ok",
		})
	}

	// Return error only if no devices at all (neither GPU nor ANE).
	if len(gpus) == 0 && !a.hasANE {
		return out, fmt.Errorf("apple: no accelerator available")
	}

	// --- Per-process GPU utilization: delta(accumulatedGPUTime) / wall time ---
	dt := interval.Seconds()
	procs := appleGPUProcesses()
	if a.gpuProcInit && dt > 0 {
		for _, p := range procs {
			prev, ok := a.gpuProc[p.PID]
			if !ok || p.GPUTimeNS < prev {
				continue // new process or counter reset
			}
			util := (float64(p.GPUTimeNS-prev) / 1e9) / dt * 100.0
			if util < 0 {
				util = 0
			}
			if util > 100 {
				util = 100
			}
			out.Procs = append(out.Procs, ProcUsage{
				PID:            int32(p.PID),
				Name:           p.Name,
				DeviceID:       "gpu0",
				UtilizationPct: util,
			})
		}
	}
	// Save baseline for next tick. Initialize even if first sample had no procs
	// so the second sample can compute deltas correctly.
	a.gpuProc = make(map[int]uint64, len(procs))
	for _, p := range procs {
		a.gpuProc[p.PID] = p.GPUTimeNS
	}
	a.gpuProcInit = true

	return out, nil
}
