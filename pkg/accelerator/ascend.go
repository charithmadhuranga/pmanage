//go:build linux

package accelerator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// AscendSource reads Huawei Ascend NPU metrics via the DCMI (Device
// Control and Management Interface) sysfs interface.
//
// Data sources:
//   - /dev/davinci* — Ascend device nodes (kernel module)
//   - /sys/class/davinci_manager/ — device manager sysfs
//   - /usr/local/Ascend/driver/tools/dcmi — DCMI CLI (fallback)
//
// The Ascend 910B exposes utilization, temperature, and power through
// the dcmi kernel interface. Per-process metrics require root and the
// Ascend driver stack.
type AscendSource struct {
	mu      sync.Mutex
	devices []DeviceInfo
}

func NewAscendSource() *AscendSource { return &AscendSource{} }

func (a *AscendSource) Name() string      { return "ascend" }
func (a *AscendSource) Kind() Kind        { return NPU }
func (a *AscendSource) Capabilities() []Capability {
	return []Capability{CapTemperature, CapPower}
}

func (a *AscendSource) Detect() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.devices = a.devices[:0]

	// Check for davinci device nodes
	devs, _ := filepath.Glob("/dev/davinci*")
	for _, dev := range devs {
		base := filepath.Base(dev)
		a.devices = append(a.devices, DeviceInfo{
			ID:     base,
			Name:   "Huawei Ascend NPU",
			Vendor: "Huawei",
			Kind:   NPU,
			Driver: "davinci",
		})
	}

	// Fallback: check for davinci_manager sysfs
	if len(a.devices) == 0 {
		if _, err := os.Stat("/sys/class/davinci_manager"); err == nil {
			a.devices = append(a.devices, DeviceInfo{
				ID:     "ascend0",
				Name:   "Huawei Ascend NPU",
				Vendor: "Huawei",
				Kind:   NPU,
				Driver: "davinci",
			})
		}
	}

	return len(a.devices) > 0
}

func (a *AscendSource) Devices() []DeviceInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]DeviceInfo, len(a.devices))
	copy(out, a.devices)
	return out
}

func (a *AscendSource) Sample(_ context.Context) (Sample, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := Sample{Timestamp: unixNow()}
	for _, dev := range a.devices {
		m := DeviceMetrics{
			DeviceID: dev.ID,
			Status:   "ok",
		}

		// Try reading utilization from sysfs
		if v, ok := readAscendUtil(dev.ID); ok {
			m.UtilizationPct = v
		}

		if v, ok := readAscendTemp(dev.ID); ok {
			m.TemperatureC = v
		}

		if v, ok := readAscendPower(dev.ID); ok {
			m.PowerW = v
		}

		out.Devices = append(out.Devices, m)
	}
	return out, nil
}

func readAscendUtil(devID string) (float64, bool) {
	paths := []string{
		fmt.Sprintf("/sys/class/davinci_manager/%s/utilization", devID),
		fmt.Sprintf("/sys/class/davinci_manager/device/%s/utilization", devID),
	}
	for _, p := range paths {
		if v, ok := readSysfsFloat(p); ok {
			return v, true
		}
	}
	return 0, false
}

func readAscendTemp(devID string) (float64, bool) {
	// Try thermal zones
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	for _, z := range zones {
		typeBytes, _ := os.ReadFile(filepath.Join(z, "type"))
		typeStr := strings.TrimSpace(string(typeBytes))
		if strings.Contains(strings.ToLower(typeStr), "npu") ||
			strings.Contains(strings.ToLower(typeStr), "ascend") ||
			strings.Contains(strings.ToLower(typeStr), "davinci") {
			if v, ok := readSysfsFloat(filepath.Join(z, "temp")); ok {
				return v / 1000.0, true
			}
		}
	}
	return 0, false
}

func readAscendPower(devID string) (float64, bool) {
	paths := []string{
		fmt.Sprintf("/sys/class/davinci_manager/%s/power", devID),
		fmt.Sprintf("/sys/class/davinci_manager/device/%s/power", devID),
	}
	for _, p := range paths {
		if v, ok := readSysfsFloat(p); ok {
			return v, true
		}
	}
	return 0, false
}
