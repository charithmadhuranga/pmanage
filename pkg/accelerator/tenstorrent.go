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

// TenstorrentSource reads Tenstorrent AI accelerator metrics via the
// tt-kmd kernel driver sysfs/hwmon interface.
//
// Data sources:
//   - /sys/class/hwmon/hwmon*/ — hwmon for temperature, power, fan
//   - /sys/bus/pci/drivers/tenstorrent/ — PCI device detection
//   - /sys/kernel/debug/tenstorrent/ — debugfs for utilization
//
// Supports Blackhole, Wormhole, and Grayskull architectures.
// No external libraries required — all data from sysfs/hwmon/procfs.
type TenstorrentSource struct {
	mu      sync.Mutex
	devices []DeviceInfo
}

func NewTenstorrentSource() *TenstorrentSource { return &TenstorrentSource{} }

func (t *TenstorrentSource) Name() string      { return "tenstorrent" }
func (t *TenstorrentSource) Kind() Kind        { return GPU }
func (t *TenstorrentSource) Capabilities() []Capability {
	return []Capability{CapTemperature, CapPower, CapFans, CapClocks}
}

func (t *TenstorrentSource) Detect() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.devices = t.devices[:0]

	// Check for Tenstorrent PCI devices
	pciDirs, _ := filepath.Glob("/sys/bus/pci/devices/*/vendor")
	for _, vendorFile := range pciDirs {
		data, err := os.ReadFile(vendorFile)
		if err != nil {
			continue
		}
		vendor := strings.TrimSpace(string(data))
		if vendor != "0x1e2e" { // Tenstorrent PCI vendor ID
			continue
		}
		// Extract BDF (Bus:Device.Function)
		bdf := filepath.Base(filepath.Dir(vendorFile))
		deviceDir := filepath.Dir(vendorFile)

		// Try to read device name from class or product
		name := "Tenstorrent Accelerator"
		if cls, err := os.ReadFile(filepath.Join(deviceDir, "class")); err == nil {
			class := strings.TrimSpace(string(cls))
			if strings.HasPrefix(class, "0x030") { // VGA controller
				name = "Tenstorrent GPU"
			}
		}

		t.devices = append(t.devices, DeviceInfo{
			ID:      fmt.Sprintf("tt_%s", bdf),
			Name:    name,
			Vendor:  "Tenstorrent",
			Kind:    GPU,
			Driver:  "tenstorrent",
			BusInfo: bdf,
		})
	}

	// Fallback: check for hwmon with tenstorrent in name
	if len(t.devices) == 0 {
		hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*/name")
		for _, nameFile := range hwmons {
			data, err := os.ReadFile(nameFile)
			if err != nil {
				continue
			}
			name := strings.TrimSpace(string(data))
			if strings.Contains(strings.ToLower(name), "tenstorrent") ||
				strings.Contains(strings.ToLower(name), "tt_") {
				idx := strings.TrimPrefix(filepath.Base(filepath.Dir(nameFile)), "hwmon")
				t.devices = append(t.devices, DeviceInfo{
					ID:     fmt.Sprintf("tt_hwmon%s", idx),
					Name:   name,
					Vendor: "Tenstorrent",
					Kind:   GPU,
					Driver: "tenstorrent",
				})
			}
		}
	}

	return len(t.devices) > 0
}

func (t *TenstorrentSource) Devices() []DeviceInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]DeviceInfo, len(t.devices))
	copy(out, t.devices)
	return out
}

func (t *TenstorrentSource) Sample(_ context.Context) (Sample, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := Sample{Timestamp: unixNow()}
	for _, dev := range t.devices {
		m := DeviceMetrics{
			DeviceID: dev.ID,
			Status:   "ok",
		}

		// Try reading temperature from hwmon
		if temp, ok := readTTHwmonTemp(dev.ID); ok {
			m.TemperatureC = temp
		}

		// Try reading power from hwmon
		if power, ok := readTTHwmonPower(dev.ID); ok {
			m.PowerW = power
		}

		// Try reading fan speed
		if fan, ok := readTTHwmonFan(dev.ID); ok {
			m.FanSpeedPct = fan
		}

		// Try reading clock from debugfs
		if clk, ok := readTTClock(dev.ID); ok {
			m.ClockMHz = clk
		}

		// Try reading utilization from debugfs
		if util, ok := readTTUtil(dev.ID); ok {
			m.UtilizationPct = util
		}

		out.Devices = append(out.Devices, m)
	}
	return out, nil
}

func readTTHwmonTemp(devID string) (float64, bool) {
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, hwmon := range hwmons {
		nameFile := filepath.Join(hwmon, "name")
		data, err := os.ReadFile(nameFile)
		if err != nil {
			continue
		}
		if !strings.Contains(strings.ToLower(string(data)), "tenstorrent") {
			continue
		}
		tempFile := filepath.Join(hwmon, "temp1_input")
		if v, ok := readSysfsFloat(tempFile); ok {
			return v / 1000.0, true
		}
	}
	return 0, false
}

func readTTHwmonPower(devID string) (float64, bool) {
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, hwmon := range hwmons {
		nameFile := filepath.Join(hwmon, "name")
		data, err := os.ReadFile(nameFile)
		if err != nil {
			continue
		}
		if !strings.Contains(strings.ToLower(string(data)), "tenstorrent") {
			continue
		}
		powerFile := filepath.Join(hwmon, "power1_average")
		if v, ok := readSysfsFloat(powerFile); ok {
			return v / 1_000_000.0, true // microwatts → watts
		}
	}
	return 0, false
}

func readTTHwmonFan(devID string) (float64, bool) {
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, hwmon := range hwmons {
		nameFile := filepath.Join(hwmon, "name")
		data, err := os.ReadFile(nameFile)
		if err != nil {
			continue
		}
		if !strings.Contains(strings.ToLower(string(data)), "tenstorrent") {
			continue
		}
		fanFile := filepath.Join(hwmon, "fan1_input")
		if v, ok := readSysfsFloat(fanFile); ok {
			// Convert RPM to percentage (assume max ~6000 RPM)
			if v > 0 {
				return (v / 6000.0) * 100.0, true
			}
		}
	}
	return 0, false
}

func readTTClock(devID string) (uint32, bool) {
	// Try debugfs
	debugPaths := []string{
		"/sys/kernel/debug/tenstorrent/clock",
		fmt.Sprintf("/sys/kernel/debug/tenstorrent/%s/clock", devID),
	}
	for _, p := range debugPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		val := strings.TrimSpace(string(data))
		var clk uint32
		if _, err := fmt.Sscanf(val, "%d", &clk); err == nil {
			if clk > 1000 {
				return clk / 1_000_000, true // Hz → MHz
			}
			return clk, true
		}
	}
	return 0, false
}

func readTTUtil(devID string) (float64, bool) {
	debugPaths := []string{
		"/sys/kernel/debug/tenstorrent/utilization",
		fmt.Sprintf("/sys/kernel/debug/tenstorrent/%s/utilization", devID),
	}
	for _, p := range debugPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		val := strings.TrimSpace(string(data))
		var util float64
		if _, err := fmt.Sscanf(val, "%f", &util); err == nil {
			return util, true
		}
	}
	return 0, false
}
