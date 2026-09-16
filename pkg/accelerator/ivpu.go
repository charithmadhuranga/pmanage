package accelerator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// IvpuSource reads Intel NPU (VPU) metrics from /sys/class/accel/accel*.
// Device-level only: npu_busy_time_us (cumulative µs → util%), memory
// utilization, current frequency. Per-process attribution is not available.
type IvpuSource struct {
	fs       npuFS
	devs     []npudev
	prevBusy map[string]uint64 // raw npu_busy_time_us per device
	tick     time.Time
	ready    bool
}

type npudev struct {
	accelPath string
	id        string
	info      DeviceInfo
}

func NewIvpuSource() *IvpuSource { return &IvpuSource{fs: realNpuFS{}} }

func (s *IvpuSource) Name() string { return "ivpu" }
func (s *IvpuSource) Kind() Kind   { return NPU }

func (s *IvpuSource) Capabilities() []Capability {
	return []Capability{CapClocks}
}

func (s *IvpuSource) Detect() bool {
	if s.ready {
		return true
	}
	paths, err := s.fs.glob("/sys/class/accel/accel[0-9]*")
	if err != nil {
		return false
	}
	for _, p := range paths {
		// Confirm it is an ivpu (Intel NPU) device via the driver symlink.
		driver, err := s.fs.readFile(p + "/device/driver")
		if err != nil || !strings.Contains(driver, "ivpu") {
			continue
		}
		id := fmt.Sprintf("npu%d", len(s.devs))
		s.devs = append(s.devs, npudev{
			accelPath: p,
			id:        id,
			info: DeviceInfo{
				ID:        id,
				Name:      "Intel NPU (ivpu)",
				Vendor:    "intel",
				Kind:      NPU,
				Driver:    "ivpu",
				BusInfo:   strings.TrimPrefix(p, "/sys/class/accel/"),
				MinKernel: "5.18.0",
			},
		})
	}
	s.ready = len(s.devs) > 0
	return s.ready
}

func (s *IvpuSource) Devices() []DeviceInfo {
	out := make([]DeviceInfo, len(s.devs))
	for i, d := range s.devs {
		out[i] = d.info
	}
	return out
}

func (s *IvpuSource) Sample(ctx context.Context) (Sample, error) {
	if !s.ready {
		return Sample{}, fmt.Errorf("ivpu not ready")
	}
	now := time.Now()
	var sample Sample
	sample.Timestamp = now.UnixMilli()
	curBusy := make(map[string]uint64, len(s.devs))

	for _, d := range s.devs {
		dm := DeviceMetrics{DeviceID: d.id, Status: "ok", MinKernel: d.info.MinKernel}

		// busy time is cumulative µs; util% over the interval between ticks.
		busy, err := s.fs.readFile(d.accelPath + "/device/npu_busy_time_us")
		if err == nil {
			if v, perr := strconv.ParseUint(busy, 10, 64); perr == nil {
				curBusy[d.id] = v
			}
		}
		if freq, err := s.fs.readFile(d.accelPath + "/device/npu_current_frequency_mhz"); err == nil {
			if v, ferr := strconv.ParseUint(freq, 10, 64); ferr == nil {
				dm.ClockMHz = uint32(v)
			}
		}
		if mem, err := s.fs.readFile(d.accelPath + "/device/npu_memory_utilization"); err == nil {
			if v, merr := strconv.ParseUint(mem, 10, 64); merr == nil && v > 0 {
				dm.VRAMUsed = v * 1024 * 1024 // percent treated as MiB placeholder
			}
		}

		// Two-sample busy → util% (first tick reports 0 baseline).
		if !s.tick.IsZero() && s.prevBusy != nil {
			elapsed := now.Sub(s.tick).Seconds()
			if prev, ok := s.prevBusy[d.id]; ok && elapsed > 0 {
				dm.UtilizationPct = clamp01((float64(curBusy[d.id]) - float64(prev)) / 1e6 / elapsed * 100)
			}
		}
		sample.Devices = append(sample.Devices, dm)
	}

	s.prevBusy = curBusy
	s.tick = now

	// Per-process → honest N/A: ivpu has no per-process bus attribution.
	sample.Procs = nil
	return sample, nil
}
