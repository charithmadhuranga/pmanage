//go:build linux

package accelerator

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"pmanage/pkg/fdinfo"
)

type intelDevice struct {
	cardPath   string
	devicePath string
	id         string
	info       DeviceInfo
}

type IntelSource struct {
	devs       []intelDevice
	ready      bool
	prevTick   time.Time
	prevEngine map[int32]map[string]uint64
}

func NewIntelSource() *IntelSource { return &IntelSource{} }

func (s *IntelSource) Name() string { return "intel" }
func (s *IntelSource) Kind() Kind   { return GPU }

const intelVendor = "0x8086"

func (s *IntelSource) Detect() bool {
	if s.ready {
		return true
	}
	cards, _ := drmCards()
	for _, card := range cards {
		vendor, _ := sysfsString(card, "device/vendor")
		if vendor != intelVendor {
			continue
		}
		id := fmt.Sprintf("intel%d", len(s.devs))
		s.devs = append(s.devs, intelDevice{
			cardPath:   card,
			devicePath: card,
			id:         id,
			info: DeviceInfo{
				ID:        id,
				Name:      sysfsStringDefault(card, "device/uevent", "Intel GPU"),
				Vendor:    "intel",
				Kind:      GPU,
				Driver:    "i915",
				Version:   sysfsStringDefault(card, "device/driver/module/version", "unknown"),
				MinKernel: "5.15.0",
			},
		})
	}
	s.ready = len(s.devs) > 0
	return s.ready
}

func (s *IntelSource) Devices() []DeviceInfo {
	out := make([]DeviceInfo, len(s.devs))
	for i, d := range s.devs {
		out[i] = d.info
	}
	return out
}

func (s *IntelSource) Sample(ctx context.Context) (Sample, error) {
	if !s.ready {
		return Sample{}, fmt.Errorf("intel not ready")
	}
	now := time.Now()
	var sample Sample
	sample.Timestamp = now.UnixMilli()

	var curEngine map[int32]map[string]uint64
	if !s.prevTick.IsZero() {
		curEngine = make(map[int32]map[string]uint64)
	}

	for _, d := range s.devs {
		dm := DeviceMetrics{DeviceID: d.id, Status: "ok", MinKernel: d.info.MinKernel}

		// i915: engine_busy_ns for device util (delta needed, first tick uses 0 baseline)
		if total, err := sysfsUint64(d.devicePath, "device/engine_busy_ns"); err == nil && !s.prevTick.IsZero() {
			// use overall: proportional based on elapsed; first tick stays 0
			dm.UtilizationPct = 0 // computed from delta later if needed; use per-engine max instead
		}

		// mem info (Xe kernels ≥6.1)
		if vramTotal, err := sysfsUint64(d.devicePath, "device/mem_total"); err == nil {
			dm.VRAMTotal = vramTotal
		}
		if vramUsed, err := sysfsUint64(d.devicePath, "device/mem_used"); err == nil {
			dm.VRAMUsed = vramUsed
		}
		// RAPL power (if available)
		hwmonGlob := d.devicePath + "/device/hwmon/hwmon*"
		if hwmonPaths, _ := filepath.Glob(hwmonGlob); len(hwmonPaths) > 0 {
			hwmon := hwmonPaths[0]
			if t, err := sysfsFloat64(hwmon, "temp1_input"); err == nil {
				dm.TemperatureC = t / 1000.0
			}
			if p, err := sysfsUint64(hwmon, "power1_average"); err == nil {
				dm.PowerW = float64(p) / 1e6
			}
		}

		sample.Devices = append(sample.Devices, dm)

		// fdinfo walker: per-process
		walker := fdinfo.Walker{Root: "/proc"}
		res, _ := walker.Walk()
		for _, c := range res.Clients {
			if c.Pid == 0 {
				continue
			}
			var totalVram uint64
			for _, v := range c.Memory {
				totalVram += v
			}
			pu := ProcUsage{
				PID:      c.Pid,
				Name:     fmt.Sprintf("pid:%d", c.Pid),
				DeviceID: d.id,
				VRAMUsed: totalVram,
				Engine:   make(map[string]float64),
			}
			if curEngine != nil {
				if curEngine[c.Pid] == nil {
					curEngine[c.Pid] = make(map[string]uint64)
				}
				for eng, ns := range c.EngineNS {
					curEngine[c.Pid][eng] = ns
				}
			}
			sample.Procs = append(sample.Procs, pu)
		}
	}

	// overlay per-process engine % on subsequent ticks
	if !s.prevTick.IsZero() {
		elapsed := now.Sub(s.prevTick)
		for i := range sample.Procs {
			p := &sample.Procs[i]
			prevEngines := s.prevEngine[p.PID]
			if prevEngines == nil {
				continue
			}
			maxPct := 0.0
			for eng, ns := range curEngine[p.PID] {
				pct := fdinfo.EngineUtil(prevEngines[eng], ns, elapsed)
				p.Engine[eng] = pct
				if pct > maxPct {
					maxPct = pct
				}
			}
			if maxPct > 0 {
				// use max engine pct as device util proxy for this client
				dm := &sample.Devices[0]
				if maxPct > dm.UtilizationPct {
					dm.UtilizationPct = maxPct
				}
			}
		}
	}

	s.prevEngine = curEngine
	s.prevTick = now
	return sample, nil
}
