//go:build linux

package accelerator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"pmanage/pkg/fdinfo"
)

type amdDevice struct {
	cardPath   string
	devicePath string
	renderPath string
	id         string
	info       DeviceInfo
}

type AmdgpuSource struct {
	devs       []amdDevice
	ready      bool
	prevTick   time.Time
	prevEngine map[int32]map[string]uint64 // pid → engine name → cumulative ns
}

func NewAmdgpuSource() *AmdgpuSource { return &AmdgpuSource{} }

func (s *AmdgpuSource) Name() string { return "amdgpu" }
func (s *AmdgpuSource) Kind() Kind   { return GPU }

const amdVendor = "0x1002"

func (s *AmdgpuSource) Detect() bool {
	if s.ready {
		return true
	}
	cards, _ := drmCards()
	for _, card := range cards {
		vendor, _ := sysfsString(card, "device/vendor")
		if vendor != amdVendor {
			continue
		}
		devPath, _ := drmDevicePath(card)
		if devPath == "" {
			continue
		}
		id := fmt.Sprintf("amd%d", len(s.devs))
		name := sysfsStringDefault(card, "device/uevent", "AMD GPU")
		s.devs = append(s.devs, amdDevice{
			cardPath:   card,
			devicePath: card,
			renderPath: devPath,
			id:         id,
			info: DeviceInfo{ID: id, Name: name, Vendor: "amd", Kind: GPU, Driver: "amdgpu",
				MinKernel: "6.7.0", Version: "fdinfo per-process"},
		})
	}
	s.ready = len(s.devs) > 0
	return s.ready
}

func (s *AmdgpuSource) Devices() []DeviceInfo {
	out := make([]DeviceInfo, len(s.devs))
	for i, d := range s.devs {
		out[i] = d.info
	}
	return out
}

func (s *AmdgpuSource) Sample(ctx context.Context) (Sample, error) {
	if !s.ready {
		return Sample{}, fmt.Errorf("amdgpu not ready")
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

		if busy, err := sysfsUint64(d.devicePath, "device/gpu_busy_percent"); err == nil {
			dm.UtilizationPct = float64(busy)
		}
		if vramTotal, err := sysfsUint64(d.devicePath, "device/mem_info_vram_total"); err == nil {
			dm.VRAMTotal = vramTotal
		}
		if vramUsed, err := sysfsUint64(d.devicePath, "device/mem_info_vram_used"); err == nil {
			dm.VRAMUsed = vramUsed
		}
		hwmonGlob := d.devicePath + "/device/hwmon/hwmon*"
		if hwmonPaths, _ := filepath.Glob(hwmonGlob); len(hwmonPaths) > 0 {
			hwmon := hwmonPaths[0]
			if t, err := sysfsFloat64(hwmon, "temp1_input"); err == nil {
				dm.TemperatureC = t / 1000.0
			}
			if p, err := sysfsUint64(hwmon, "power1_average"); err == nil {
				dm.PowerW = float64(p) / 1e6
			}
			if f, err := sysfsUint64(hwmon, "fan1_input"); err == nil {
				dm.FanSpeedPct = float64(f) / 100.0
			}
		}
		if clk := parseSclk(d.devicePath); clk > 0 {
			dm.ClockMHz = clk
		}
		sample.Devices = append(sample.Devices, dm)

		// fdinfo walker: per-process attribution
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
			// store cumulative ns for delta
			if curEngine != nil {
				if curEngine[c.Pid] == nil {
					curEngine[c.Pid] = make(map[string]uint64)
				}
				for eng, ns := range c.EngineNS {
					curEngine[c.Pid][eng] = ns
				}
			}
			// on first tick (no prevTick), Engine stays empty (no baseline)
			sample.Procs = append(sample.Procs, pu)
		}
	}

	// on subsequent ticks, overlay per-process % onto previous tick's procs in sample
	if !s.prevTick.IsZero() && len(s.prevEngine) > 0 {
		elapsed := now.Sub(s.prevTick)
		for i := range sample.Procs {
			p := &sample.Procs[i]
			prevEngines := s.prevEngine[p.PID]
			if prevEngines == nil {
				continue
			}
			maxPct := 0.0
			for eng, ns := range curEngine[p.PID] {
				prevNS := prevEngines[eng]
				pct := fdinfo.EngineUtil(prevNS, ns, elapsed)
				p.Engine[eng] = pct
				if pct > maxPct {
					maxPct = pct
				}
			}
		}
	}

	s.prevEngine = curEngine
	s.prevTick = now
	return sample, nil
}

func parseSclk(cardPath string) uint32 {
	data, err := os.ReadFile(cardPath + "/device/pp_dpm_sclk")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "*") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		s := strings.TrimSuffix(parts[1], "Mhz")
		if v, err := strconv.ParseUint(s, 10, 32); err == nil {
			return uint32(v)
		}
	}
	return 0
}

func sysfsStringDefault(classPath, filename, fallback string) string {
	s, _ := sysfsString(classPath, filename)
	if s == "" {
		return fallback
	}
	return s
}
