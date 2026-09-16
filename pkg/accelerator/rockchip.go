package accelerator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RockchipSource reads the rknpu via devfreq (cur_freq, max_freq) and the
// debugfs load node (/sys/kernel/debug/rknpu/load, root-only). Device-level
// only; per-process NPU attribution is not available.
type RockchipSource struct {
	fs    npuFS
	devs  []rkdev
	ready bool
}

type rkdev struct {
	devfreqPath string
	id          string
	info        DeviceInfo
}

func NewRockchipSource() *RockchipSource { return &RockchipSource{fs: realNpuFS{}} }

func (s *RockchipSource) Name() string { return "rockchip" }
func (s *RockchipSource) Kind() Kind   { return NPU }

func (s *RockchipSource) Capabilities() []Capability {
	// rknpu load needs debugfs mount (root); when unreadable it's N/A.
	return []Capability{CapClocks}
}

func (s *RockchipSource) Detect() bool {
	if s.ready {
		return true
	}
	// devfreq nodes ending in "npu" (e.g. fdab0000.npu on RK3588).
	paths, err := s.fs.glob("/sys/class/devfreq/*npu*")
	if err != nil {
		return false
	}
	for _, p := range paths {
		id := fmt.Sprintf("npu%d", len(s.devs))
		name := strings.TrimPrefix(p, "/sys/class/devfreq/")
		s.devs = append(s.devs, rkdev{
			devfreqPath: p,
			id:          id,
			info: DeviceInfo{
				ID:      id,
				Name:    "Rockchip NPU (" + name + ")",
				Vendor:  "rockchip",
				Kind:    NPU,
				Driver:  "rknpu",
				BusInfo: name,
			},
		})
	}
	s.ready = len(s.devs) > 0
	return s.ready
}

func (s *RockchipSource) Devices() []DeviceInfo {
	out := make([]DeviceInfo, len(s.devs))
	for i, d := range s.devs {
		out[i] = d.info
	}
	return out
}

func (s *RockchipSource) Sample(ctx context.Context) (Sample, error) {
	if !s.ready {
		return Sample{}, fmt.Errorf("rockchip not ready")
	}
	var sample Sample
	sample.Timestamp = time.Now().UnixMilli()

	for _, d := range s.devs {
		dm := DeviceMetrics{DeviceID: d.id, Status: "ok"}

		// devfreq cur_freq is in Hz.
		if cf, err := s.fs.readFile(d.devfreqPath + "/cur_freq"); err == nil {
			if v, perr := strconv.ParseUint(cf, 10, 64); perr == nil {
				dm.ClockMHz = uint32(v / 1_000_000)
			}
		}
		// debugfs load is root-only; report N/A (util 0 + no dev-fmt) when absent.
		if load, err := s.fs.readFile("/sys/kernel/debug/rknpu/load"); err == nil {
			if v, ok := rknpuLoadPct(load); ok {
				dm.UtilizationPct = v
			}
		}
		sample.Devices = append(sample.Devices, dm)
	}
	sample.Procs = nil
	return sample, nil
}

// rknpuLoadPct parses the debugfs "load" node, e.g. "20 10 0" (per-core %).
func rknpuLoadPct(load string) (float64, bool) {
	fields := strings.Fields(strings.TrimSpace(load))
	if len(fields) == 0 {
		return 0, false
	}
	var max float64
	for _, f := range fields {
		if v, err := strconv.ParseFloat(f, 64); err == nil && v > max {
			max = v
		}
	}
	return max, true
}
