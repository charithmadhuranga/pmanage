//go:build linux

package accelerator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// VideoCoreSource reads Broadcom VideoCore GPU metrics on Raspberry Pi
// and other Broadcom SoC boards.
//
// Data sources:
//   - /sys/class/drm/card*/device/ — DRM card for driver identification
//   - /sys/kernel/debug/v3d/ — V3D debugfs for utilization (RPi OS)
//   - /dev/vcio — VideoCore mailbox interface (requires kernel 6.12+)
//   - /sys/class/thermal/thermal_zone*/temp — SoC temperature
//
// The VideoCore GPU on Raspberry Pi 4/5 shares memory with the CPU
// (unified memory architecture), so there is no separate VRAM.
type VideoCoreSource struct {
	mu      sync.Mutex
	devices []DeviceInfo
}

func NewVideoCoreSource() *VideoCoreSource { return &VideoCoreSource{} }

func (v *VideoCoreSource) Name() string      { return "videocore" }
func (v *VideoCoreSource) Kind() Kind        { return GPU }
func (v *VideoCoreSource) Capabilities() []Capability {
	return []Capability{CapTemperature, CapClocks}
}

func (v *VideoCoreSource) Detect() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.devices = v.devices[:0]

	// Check for V3D driver via DRM
	cards, _ := filepath.Glob("/sys/class/drm/card*")
	for _, card := range cards {
		driverLink, err := os.Readlink(filepath.Join(card, "device", "driver"))
		if err != nil {
			continue
		}
		driverName := filepath.Base(driverLink)
		if driverName == "v3d" || driverName == "vc4" {
			idx := strings.TrimPrefix(filepath.Base(card), "card")
			v.devices = append(v.devices, DeviceInfo{
				ID:     fmt.Sprintf("vc%s", idx),
				Name:   "Broadcom VideoCore GPU",
				Vendor: "Broadcom",
				Kind:   GPU,
				Driver: driverName,
			})
		}
	}

	// Fallback: check for /dev/vcio (VideoCore mailbox)
	if len(v.devices) == 0 {
		if _, err := os.Stat("/dev/vcio"); err == nil {
			v.devices = append(v.devices, DeviceInfo{
				ID:     "vc0",
				Name:   "Broadcom VideoCore GPU",
				Vendor: "Broadcom",
				Kind:   GPU,
				Driver: "vcio",
			})
		}
	}

	return len(v.devices) > 0
}

func (v *VideoCoreSource) Devices() []DeviceInfo {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]DeviceInfo, len(v.devices))
	copy(out, v.devices)
	return out
}

func (v *VideoCoreSource) Sample(_ context.Context) (Sample, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	out := Sample{Timestamp: unixNow()}
	for _, dev := range v.devices {
		m := DeviceMetrics{
			DeviceID: dev.ID,
			Status:   "ok",
		}

		// Try reading V3D utilization from debugfs
		if v, ok := readVCUtilization(); ok {
			m.UtilizationPct = v
		}

		// Try reading temperature
		if temp, ok := readVCTemp(); ok {
			m.TemperatureC = temp
		}

		// Try reading clock frequency
		if clk, ok := readVCClock(); ok {
			m.ClockMHz = clk
		}

		out.Devices = append(out.Devices, m)
	}
	return out, nil
}

func readVCUtilization() (float64, bool) {
	// Try V3D debugfs (RPi OS with V3D driver loaded)
	debugPaths := []string{
		"/sys/kernel/debug/v3d/attrs",
		"/sys/kernel/debug/dri/0/v3d_busy",
	}
	for _, p := range debugPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if strings.Contains(strings.ToLower(key), "utilization") ||
				strings.Contains(strings.ToLower(key), "busy") {
				if v, err := strconv.ParseFloat(val, 64); err == nil {
					return v, true
				}
			}
		}
	}

	// Try generic GPU busy from drm
	if v, ok := readSysfsFloat("/sys/class/drm/card0/device/gpu_busy_percentage"); ok {
		return v, true
	}

	return 0, false
}

func readVCTemp() (float64, bool) {
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	for _, z := range zones {
		typeBytes, _ := os.ReadFile(filepath.Join(z, "type"))
		typeStr := strings.TrimSpace(string(typeBytes))
		if strings.Contains(strings.ToLower(typeStr), "gpu") ||
			strings.Contains(strings.ToLower(typeStr), "cpu") {
			if v, ok := readSysfsFloat(filepath.Join(z, "temp")); ok {
				return v / 1000.0, true
			}
		}
	}
	return 0, false
}

func readVCClock() (uint32, bool) {
	// Try reading GPU clock from sysfs
	paths := []string{
		"/sys/class/drm/card0/device/devfreq/cur_freq",
		"/sys/class/drm/card0/device/gpu_clock",
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		s := strings.TrimSpace(string(data))
		var v uint64
		if _, err := fmt.Sscanf(s, "%d", &v); err == nil {
			if v > 1_000_000 {
				return uint32(v / 1_000_000), true // Hz → MHz
			}
			return uint32(v), true
		}
	}
	return 0, false
}
