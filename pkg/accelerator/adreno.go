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

// AdrenoSource reads Qualcomm Adreno GPU metrics via the MSM drm driver
// sysfs interface (kernel 6.0+ for fdinfo-based per-process utilization).
//
// Data sources:
//   - /sys/class/drm/card*/device/kgsl-3d0/ — MSM sysfs for utilization, clocks, temp
//   - /sys/class/drm/card*/device/gpu_busy_percentage — GPU busy % (MSM 6.x)
//   - /sys/class/thermal/thermal_zone*/temp — SoC temperature
//
// Limitations: per-process GPU usage requires kernel 6.0+ fdinfo support;
// older kernels report aggregate only.
type AdrenoSource struct {
	mu      sync.Mutex
	devices []DeviceInfo
}

func NewAdrenoSource() *AdrenoSource { return &AdrenoSource{} }

func (a *AdrenoSource) Name() string      { return "adreno" }
func (a *AdrenoSource) Kind() Kind        { return GPU }
func (a *AdrenoSource) Capabilities() []Capability {
	return []Capability{CapTemperature, CapClocks}
}

func (a *AdrenoSource) Detect() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.devices = a.devices[:0]

	cards, _ := filepath.Glob("/sys/class/drm/card*")
	for _, card := range cards {
		driverLink, err := os.Readlink(filepath.Join(card, "device", "driver"))
		if err != nil {
			continue
		}
		if !strings.HasSuffix(filepath.Base(driverLink), "msm") {
			continue
		}
		idx := strings.TrimPrefix(filepath.Base(card), "card")
		a.devices = append(a.devices, DeviceInfo{
			ID:     fmt.Sprintf("adreno%s", idx),
			Name:   detectAdrenoModel(card),
			Vendor: "Qualcomm",
			Kind:   GPU,
			Driver: "msm",
		})
	}
	return len(a.devices) > 0
}

func (a *AdrenoSource) Devices() []DeviceInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]DeviceInfo, len(a.devices))
	copy(out, a.devices)
	return out
}

func (a *AdrenoSource) Sample(_ context.Context) (Sample, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := Sample{Timestamp: unixNow()}
	for _, dev := range a.devices {
		idx := strings.TrimPrefix(dev.ID, "adreno")
		card := fmt.Sprintf("/sys/class/drm/card%s", idx)
		m := DeviceMetrics{
			DeviceID: dev.ID,
			Status:   "ok",
		}

		if v, ok := readSysfsFloat(filepath.Join(card, "device", "gpu_busy_percentage")); ok {
			m.UtilizationPct = v
		} else if v, ok := readSysfsFloat(filepath.Join(card, "device", "kgsl-3d0", "gpu_busy_percentage")); ok {
			m.UtilizationPct = v
		}

		if v, ok := readSysfsUint64(filepath.Join(card, "device", "kgsl-3d0", "gpuclk")); ok {
			m.ClockMHz = uint32(v / 1_000_000)
		}

		if v, ok := readAdrenoTemp(card); ok {
			m.TemperatureC = v
		}

		out.Devices = append(out.Devices, m)
	}
	return out, nil
}

func detectAdrenoModel(card string) string {
	// Try to read model from sysfs uevent or modalias
	uevent, err := os.ReadFile(filepath.Join(card, "device", "uevent"))
	if err != nil {
		return "Qualcomm Adreno"
	}
	for _, line := range strings.Split(string(uevent), "\n") {
		if strings.HasPrefix(line, "PCI_ID=") {
			parts := strings.Split(strings.TrimPrefix(line, "PCI_ID="), ":")
			if len(parts) == 2 {
				return fmt.Sprintf("Adreno %s", parts[1])
			}
		}
	}
	return "Qualcomm Adreno"
}

func readAdrenoTemp(card string) (float64, bool) {
	// Try thermal_zone for this GPU
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	for _, z := range zones {
		typeBytes, _ := os.ReadFile(filepath.Join(z, "type"))
		typeStr := strings.TrimSpace(string(typeBytes))
		if strings.Contains(strings.ToLower(typeStr), "gpu") || strings.Contains(strings.ToLower(typeStr), "adreno") {
			if v, ok := readSysfsFloat(filepath.Join(z, "temp")); ok {
				return v / 1000.0, true // millidegrees → degrees
			}
		}
	}
	return 0, false
}
