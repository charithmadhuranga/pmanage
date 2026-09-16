//go:build darwin

package accelerator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
)

// AppleSource reads Apple Silicon GPU / ANE metrics using techniques from nvtop:
//   - IOKit PerformanceStatistics for aggregate GPU util + system memory
//   - IOReport energy deltas for GPU/ANE power
//   - AGXDeviceUserClient for per-process GPU time
//   - host_info(HOST_BASIC_INFO) for total unified memory (nvtop technique)
//   - proc_pidinfo for per-process CPU/memory stats (nvtop technique)
//   - Two-bucket process cache for lifecycle tracking (nvtop technique)
//   - Validity bitmask for honest N/A reporting (nvtop technique)
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

	// Total unified memory (cached from host_info).
	totalMem uint64

	// Per-process GPU time deltas (Activity-Monitor GPU column source).
	gpuProc     map[int]uint64 // pid -> cumulative accumulatedGPUTime (ns)
	gpuProcInit bool           // true after first baseline capture

	// Process cache for name/username/CPU stats (nvtop two-bucket pattern).
	procCache *ProcessCache
}

func NewAppleSource() *AppleSource {
	return &AppleSource{
		first:     true,
		procCache: NewProcessCache(),
	}
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

	// Cache total unified memory from host_info (nvtop technique).
	a.totalMem = appleSystemMemory()
	if a.totalMem == 0 {
		// Fallback to gopsutil if host_info fails.
		if vm, err := mem.VirtualMemory(); err == nil {
			a.totalMem = vm.Total
		}
	}

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

	// Use cached total memory (nvtop technique: host_info).
	totalMem := a.totalMem

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

	// --- GPU device metrics (with validity bitmask) ---
	for i := range gpus {
		id := fmt.Sprintf("gpu%d", i)
		m := DeviceMetrics{
			DeviceID:     id,
			Status:       "ok",
			PowerW:       gpuW,
			TemperatureC: -1, // Apple does not expose GPU temp via IOKit
		}
		m.Set(ValidPower)

		if gpus[i].HasStats {
			m.UtilizationPct = gpus[i].DevUtil
			m.Set(ValidUtilization)

			m.VRAMUsed = gpus[i].AllocSysMem
			m.Set(ValidVRAMUsed)

			m.VRAMTotal = totalMem
			m.Set(ValidVRAMTotal)

			// Memory utilization rate (nvtop technique).
			if totalMem > 0 {
				m.MemUtilRate = float64(gpus[i].AllocSysMem) * 100.0 / float64(totalMem)
				m.Set(ValidMemUtilRate)
			}
		} else {
			m.Status = "n/a"
			m.UtilizationPct = -1
		}

		// Temperature: Apple does not expose GPU temp via IOKit.
		// Leave unset (invalid) — frontend shows "N/A".

		// Effective load (nvtop technique): gpu_util * (power / power_max).
		// power_max not available from IOGPU, so skip for now.

		out.Devices = append(out.Devices, m)
	}

	// --- NPU device metrics (with validity bitmask) ---
	if a.hasANE {
		m := DeviceMetrics{
			DeviceID:       "npu0",
			UtilizationPct: -1, // Apple does not expose ANE utilization
			PowerW:         aneW,
			VRAMTotal:      totalMem, // ANE uses unified memory
			TemperatureC:   -1,       // Apple does not expose ANE temp
			Status:         "ok",
		}
		m.Set(ValidPower)
		if totalMem > 0 {
			m.Set(ValidVRAMTotal)
		}
		out.Devices = append(out.Devices, m)
	}

	// Return error only if no devices at all (neither GPU nor ANE).
	if len(gpus) == 0 && !a.hasANE {
		return out, fmt.Errorf("apple: no accelerator available")
	}

	// --- Per-process GPU utilization (nvtop two-bucket cache technique) ---
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

			proc := ProcUsage{
				PID:            int32(p.PID),
				Name:           p.Name,
				DeviceID:       "gpu0",
				UtilizationPct: util,
			}
			proc.Set(ValidUtilization)

			// Resolve process metadata from cache (nvtop technique).
			cacheEntry := a.procCache.Get(int32(p.PID))
			if cacheEntry == nil {
				// New process — resolve name, username, CPU stats.
				fullName := appleProcessCommand(p.PID)
				if fullName == "" {
					fullName = p.Name
				}
				username := appleProcessUsername(p.PID)

				cacheEntry = &ProcessCacheEntry{
					PID:      int32(p.PID),
					Name:     fullName,
					Username: username,
				}
				a.procCache.Put(int32(p.PID), cacheEntry)
			}

			if cacheEntry.Name != "" {
				proc.Name = cacheEntry.Name
			}
			if cacheEntry.Username != "" {
				proc.Username = cacheEntry.Username
			}

			// Per-process CPU/memory stats (nvtop technique: proc_pidinfo).
			if userTime, kernelTime, virtMem, residentMem, ok := appleProcessInfo(p.PID); ok {
				totalCPU := userTime + kernelTime
				if cacheEntry.LastMeasurementTime.IsZero() {
					proc.CpuUsage = 0
				} else {
					elapsed := now.Sub(cacheEntry.LastMeasurementTime).Seconds()
					if elapsed > 0 {
						proc.CpuUsage = (totalCPU - cacheEntry.LastTotalCPUTime) / elapsed * 100.0
						if proc.CpuUsage > 100 {
							proc.CpuUsage = 100
						}
						if proc.CpuUsage < 0 {
							proc.CpuUsage = 0
						}
					}
				}
				proc.MemResident = residentMem
				proc.MemVirtual = virtMem
				proc.Set(ValidCpuUsage)
				proc.Set(ValidMemResident)
				proc.Set(ValidMemVirtual)

				cacheEntry.LastTotalCPUTime = totalCPU
				cacheEntry.LastMeasurementTime = now
			}

			out.Procs = append(out.Procs, proc)
		}
	}

	// Save GPU time baseline for next tick.
	a.gpuProc = make(map[int]uint64, len(procs))
	for _, p := range procs {
		a.gpuProc[p.PID] = p.GPUTimeNS
	}
	a.gpuProcInit = true

	// Advance process cache lifecycle (nvtop technique).
	a.procCache.Swap()

	return out, nil
}
