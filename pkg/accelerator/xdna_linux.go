//go:build linux

package accelerator

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"pmanage/pkg/fdinfo"
)

type xdnaDevice struct {
	accelPath string
	id        string
	info      DeviceInfo
}

type XdnaSource struct {
	devs  []xdnaDevice
	ready bool
	tick  time.Time
}

func NewXdnaSource() *XdnaSource { return &XdnaSource{} }

func (s *XdnaSource) Name() string { return "xdna" }
func (s *XdnaSource) Kind() Kind   { return NPU }

const amdxdnaDriver = "amdxdna"

func (s *XdnaSource) Detect() bool {
	if s.ready {
		return true
	}
	devices, _ := accelDevices()
	for _, dev := range devices {
		// check the driver symlink matches amdxdna
		driverLink := dev + "/device/driver"
		target, err := filepath.EvalSymlinks(driverLink)
		if err != nil {
			continue
		}
		if filepath.Base(target) != amdxdnaDriver {
			continue
		}
		id := fmt.Sprintf("npu%d", len(s.devs))
		s.devs = append(s.devs, xdnaDevice{
			accelPath: dev,
			id:        id,
			info: DeviceInfo{
				ID:        id,
				Name:      "AMD NPU (XDNA)",
				Vendor:    "amd",
				Kind:      NPU,
				Driver:    "amdxdna",
				Version:   "XDNA",
				MinKernel: "6.7.0",
			},
		})
	}
	s.ready = len(s.devs) > 0
	return s.ready
}

func (s *XdnaSource) Devices() []DeviceInfo {
	out := make([]DeviceInfo, len(s.devs))
	for i, d := range s.devs {
		out[i] = d.info
	}
	return out
}

func (s *XdnaSource) Sample(ctx context.Context) (Sample, error) {
	if !s.ready {
		return Sample{}, fmt.Errorf("xdna not ready")
	}
	now := time.Now()
	var sample Sample
	sample.Timestamp = now.UnixMilli()

	for _, d := range s.devs {
		dm := DeviceMetrics{DeviceID: d.id, Status: "ok", MinKernel: d.info.MinKernel}

		// npu_busy_time_us → percentage over elapsed interval
		// For now report raw — frontend can show N/A for compute util
		dm.UtilizationPct = 0 // will compute on second tick
		if freq, err := sysfsUint64(d.accelPath, "device/npu_current_frequency_mhz"); err == nil {
			dm.ClockMHz = uint32(freq)
		}
		if memUtil, err := sysfsUint64(d.accelPath, "device/npu_memory_utilization"); err == nil {
			dm.VRAMUsed = memUtil * 1024 * 1024 // percentage interpreted as "used MB" placeholder
		}
		if power, err := sysfsUint64(d.accelPath, "device/npu_power_w"); err == nil {
			dm.PowerW = float64(power)
		}
		dm.Status = "ok"
		sample.Devices = append(sample.Devices, dm)

		// fdinfo walker: xdna engine (amdxdna)
		walker := fdinfo.Walker{Root: "/proc"}
		res, _ := walker.Walk()
		for _, c := range res.Clients {
			if c.Pid == 0 {
				continue
			}
			var totalMem uint64
			for _, v := range c.Memory {
				totalMem += v
			}
			pu := ProcUsage{
				PID:      c.Pid,
				Name:     fmt.Sprintf("pid:%d", c.Pid),
				DeviceID: d.id,
				VRAMUsed: totalMem,
			}
			if ns, ok := c.EngineNS["amdxdna"]; ok && ns > 0 {
				pu.Engine = map[string]float64{"amdxdna": float64(ns) / 1e6}
			}
			sample.Procs = append(sample.Procs, pu)
		}
	}
	s.tick = now
	return sample, nil
}
