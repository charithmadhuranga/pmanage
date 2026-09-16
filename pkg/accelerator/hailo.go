package accelerator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// HailoSource monitors Hailo AI accelerators via hailortcli.
//
// Supported devices (from hailort fw_control_command.cpp architecture map):
//   - Hailo-8   (HAILO8)   — 26 TOPS PCIe M.2 accelerator
//   - Hailo-8L  (HAILO8L)  — lower-power variant
//   - Hailo-10H (HAILO10H) — newer generation PCIe accelerator
//   - Hailo-15H (HAILO15H) — vision SoC (integrated, not PCIe)
//   - Hailo-15L (HAILO15L) — vision SoC variant
//   - Hailo-15M (HAILO15M) — vision SoC variant
//   - Hailo-12L (MARS)     — next-gen codename "Mars"
//
// Data sources (from hailort accelerator_monitor.cpp):
//   - `hailortcli fw-control identify --extended` — architecture, firmware version,
//     NNC clock rate, serial/part/product, supported features, boot source
//   - `hailortcli monitor --interval=1000` — NNC utilization, CPU utilization,
//     RAM utilization (used/total), on-die temperature, on-die voltage
//   - `hailortcli scan` — enumerate PCIe device IDs
//   - PCIe sysfs — bus address, driver info
//
// Per-process GPU attribution is not available on Hailo devices (always N/A).
type HailoSource struct {
	cli   cliRunner
	devs  []hailoDev
	ready bool
}

type hailoDev struct {
	id      string
	info    DeviceInfo
	arch    string // e.g. "HAILO8", "HAILO10H"
	fwVer   string // e.g. "4.17.0 (release,app)"
	clockMHz uint32
	serial  string
}

func NewHailoSource() *HailoSource {
	return &HailoSource{cli: &execRunner{name: "hailortcli"}}
}

func (s *HailoSource) Name() string { return "hailo" }
func (s *HailoSource) Kind() Kind   { return NPU }

// Capabilities: temperature, power, and clocks (NNC clock rate from extended identify).
// Per-process attribution is not available — always N/A.
func (s *HailoSource) Capabilities() []Capability {
	return []Capability{CapTemperature, CapPower, CapClocks}
}

func (s *HailoSource) Detect() bool {
	if s.ready {
		return true
	}
	s.devs = s.devs[:0]

	// Phase 1: enumerate devices via scan ( PCIe device IDs )
	deviceIDs := s.scanDevices()

	// Phase 2: identify each device for architecture/firmware/clock
	out, err := s.cli.run("fw-control", "identify", "--extended")
	if err != nil {
		// Fallback: try without --extended
		out, err = s.cli.run("fw-control", "identify")
		if err != nil {
			return false
		}
	}

	// Parse identify output — each device block starts with "Identifying board"
	// and contains Architecture, Firmware Version, etc.
	s.parseIdentifyOutput(out, deviceIDs)

	// Phase 3: for devices not found via identify, add them with best-effort info
	if len(s.devs) == 0 && len(deviceIDs) > 0 {
		for i, did := range deviceIDs {
			id := fmt.Sprintf("hailo%d", i)
			s.devs = append(s.devs, hailoDev{
				id: id,
				info: DeviceInfo{
					ID:      id,
					Name:    "Hailo NPU",
					Vendor:  "hailo",
					Kind:    NPU,
					Driver:  "hailort",
					BusInfo: did,
				},
			})
		}
	}

	s.ready = len(s.devs) > 0
	return s.ready
}

func (s *HailoSource) Devices() []DeviceInfo {
	out := make([]DeviceInfo, len(s.devs))
	for i, d := range s.devs {
		out[i] = d.info
	}
	return out
}

func (s *HailoSource) Sample(ctx context.Context) (Sample, error) {
	if !s.ready {
		return Sample{}, fmt.Errorf("hailo not ready")
	}
	var sample Sample
	sample.Timestamp = time.Now().UnixMilli()

	// Run monitor to get live metrics
	out, err := s.cli.run("monitor", "--interval=1000", "--no-clear-screen")
	if err != nil {
		// monitor may need a TTY or may not be supported on this device family;
		// degrade to N/A device rows rather than fail.
		for _, d := range s.devs {
			sample.Devices = append(sample.Devices, DeviceMetrics{
				DeviceID: d.id, Status: "ok", UtilizationPct: -1,
			})
		}
		return sample, nil
	}

	for _, d := range s.devs {
		dm := DeviceMetrics{DeviceID: d.id, Status: "ok", UtilizationPct: -1}

		if m := hailoParseMonitor(out, d); m != nil {
			// NNC utilization is the primary GPU-like metric
			if m.nncUtil >= 0 {
				dm.UtilizationPct = m.nncUtil
			}
			dm.TemperatureC = m.tempC
			dm.PowerW = m.powerW
			dm.VRAMUsed = m.ramUsedBytes
			dm.VRAMTotal = m.ramTotalBytes
			dm.ClockMHz = d.clockMHz // from identify --extended
			dm.Status = "ok"
		}
		sample.Devices = append(sample.Devices, dm)
	}

	sample.Procs = nil // per-process attribution not available on Hailo
	return sample, nil
}

// ---------------------------------------------------------------------------
// Device enumeration
// ---------------------------------------------------------------------------

// scanDevices runs `hailortcli scan` and extracts device IDs.
// Output format: "Hailo Devices:\n[-] Device: <id>\n"
func (s *HailoSource) scanDevices() []string {
	out, err := s.cli.run("scan")
	if err != nil {
		return nil
	}
	var ids []string
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "[-] Device:") {
			id := strings.TrimSpace(strings.TrimPrefix(ln, "[-] Device:"))
			if id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// ---------------------------------------------------------------------------
// Identify parsing
// ---------------------------------------------------------------------------

// parseIdentifyOutput extracts architecture, firmware version, clock rate,
// serial number, and other device info from `hailortcli fw-control identify[--extended]`.
//
// Output format (from fw_control_command.cpp):
//
//	Identifying board
//	Control Protocol Version: 2.11
//	Firmware Version: 4.17.0 (release,app)
//	Logger Version: 0
//	Device Architecture: HAILO8
//	Serial Number: <hex>
//	Part Number: <string>
//	Product Name: <string>
//	Neural Network Core Clock Rate: 1000MHz     (with --extended)
//	Boot source: PCIE                            (with --extended)
//	Device supported features: PCIE, Power Measurement (with --extended)
func (s *HailoSource) parseIdentifyOutput(out string, deviceIDs []string) {
	lines := strings.Split(out, "\n")
	devIdx := 0

	for i := 0; i < len(lines); i++ {
		ln := strings.TrimSpace(lines[i])

		if ln == "Identifying board" {
			// Start of a new device block
			id := fmt.Sprintf("hailo%d", devIdx)
			busInfo := ""
			if devIdx < len(deviceIDs) {
				busInfo = deviceIDs[devIdx]
			}

			arch := ""
			fwVer := ""
			var clockMHz uint32
			serial := ""
			productName := ""

			// Parse subsequent lines until next "Identifying board" or end
			for j := i + 1; j < len(lines); j++ {
				fieldLn := strings.TrimSpace(lines[j])
				if fieldLn == "Identifying board" || fieldLn == "" && j > i+1 {
					// Check if this is a blank line followed by more content or end
					if fieldLn == "Identifying board" {
						break
					}
				}

				if strings.HasPrefix(fieldLn, "Device Architecture:") {
					arch = strings.TrimSpace(strings.TrimPrefix(fieldLn, "Device Architecture:"))
				} else if strings.HasPrefix(fieldLn, "Firmware Version:") {
					fwVer = strings.TrimSpace(strings.TrimPrefix(fieldLn, "Firmware Version:"))
				} else if strings.HasPrefix(fieldLn, "Neural Network Core Clock Rate:") {
					clockStr := strings.TrimSpace(strings.TrimPrefix(fieldLn, "Neural Network Core Clock Rate:"))
					clockMHz = parseClockMHz(clockStr)
				} else if strings.HasPrefix(fieldLn, "Serial Number:") {
					serial = strings.TrimSpace(strings.TrimPrefix(fieldLn, "Serial Number:"))
				} else if strings.HasPrefix(fieldLn, "Product Name:") {
					productName = strings.TrimSpace(strings.TrimPrefix(fieldLn, "Product Name:"))
				}
			}

			name := hailoArchName(arch, productName)
			s.devs = append(s.devs, hailoDev{
				id:       id,
				arch:     arch,
				fwVer:    fwVer,
				clockMHz: clockMHz,
				serial:   serial,
				info: DeviceInfo{
					ID:      id,
					Name:    name,
					Vendor:  "hailo",
					Kind:    NPU,
					Driver:  "hailort",
					Version: fmt.Sprintf("FW %s | %s", fwVer, arch),
					BusInfo: busInfo,
				},
			})
			devIdx++
		}
	}
}

// parseClockMHz extracts MHz from strings like "1000MHz" or "1000 MHz".
func parseClockMHz(s string) uint32 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "MHz")
	s = strings.TrimSuffix(s, " MHz")
	s = strings.TrimSpace(s)
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(v)
}

// hailoArchName returns a human-readable device name from the architecture string.
// Architecture values from fw_control_command.cpp identity_arch_string():
//
//	HAILO8, HAILO8L, HAILO10H, HAILO15H, HAILO15L, HAILO15M, HAILO12L (MARS)
func hailoArchName(arch, productName string) string {
	if productName != "" {
		return productName
	}
	switch arch {
	case "HAILO8":
		return "Hailo-8"
	case "HAILO8L":
		return "Hailo-8L"
	case "HAILO10H":
		return "Hailo-10H"
	case "HAILO15H":
		return "Hailo-15H"
	case "HAILO15L":
		return "Hailo-15L"
	case "HAILO15M":
		return "Hailo-15M"
	case "HAILO12L", "MARS":
		return "Hailo-12L (Mars)"
	default:
		if arch != "" {
			return "Hailo " + arch
		}
		return "Hailo NPU"
	}
}

// ---------------------------------------------------------------------------
// Monitor output parsing
// ---------------------------------------------------------------------------

// hailoMetrics holds parsed metrics from `hailortcli monitor`.
type hailoMetrics struct {
	nncUtil     float64  // NNC utilization % (-1 = N/A)
	cpuUtil     float64  // CPU utilization % (-1 = N/A)
	ramUtil     float64  // RAM utilization % (-1 = N/A)
	ramUsedBytes uint64  // RAM used in bytes
	ramTotalBytes uint64 // RAM total in bytes
	tempC       float64  // on-die temperature °C
	powerW      float64  // power watts
	voltageMV   float64  // on-die voltage mV
}

// hailoParseMonitor extracts device-level metrics from hailortcli monitor output.
//
// The accelerator_monitor.cpp outputs a table with columns:
//
//	Device ID | Architecture | NNC Utilization (%) | CPU Utilization (%) |
//	RAM Utilization (%) | RAM Usage (MB) | On Die Temperature (C) | On Die Voltage (mV)
//
// We also support the older JSON-ish and plaintext formats for backward compatibility.
func hailoParseMonitor(out string, dev hailoDev) *hailoMetrics {
	lines := strings.Split(out, "\n")

	// Strategy 1: Parse the structured table format (Hailo-10 accelerator monitor)
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "Device ID") || strings.HasPrefix(ln, "---") {
			continue
		}
		// Table row: split by whitespace columns
		fields := strings.Fields(ln)
		if len(fields) < 8 {
			continue
		}
		// Check if first field matches our device ID or architecture
		if !strings.Contains(fields[0], "hailo") && !strings.Contains(strings.ToLower(fields[0]), "hailo") {
			// Also check by architecture field
			if len(fields) > 1 && !strings.Contains(fields[1], dev.arch) {
				continue
			}
		}

		m := &hailoMetrics{nncUtil: -1}

		// Parse NNC Utilization (field 2)
		if v, err := strconv.ParseFloat(fields[2], 64); err == nil {
			m.nncUtil = v
		}
		// Parse CPU Utilization (field 3)
		if v, err := strconv.ParseFloat(fields[3], 64); err == nil {
			m.cpuUtil = v
		}
		// Parse RAM Utilization (field 4)
		if v, err := strconv.ParseFloat(fields[4], 64); err == nil {
			m.ramUtil = v
		}
		// Parse RAM Usage "used / total" — fields may be split by whitespace
		// e.g. ["512", "/", "768"] or ["512/768"]
		if len(fields) > 5 {
			ramStr := fields[5]
			// Check if next field is "/" (space-separated format)
			if len(fields) > 6 && fields[6] == "/" && len(fields) > 7 {
				ramStr = fields[5] + fields[6] + fields[7]
			}
			ramParts := strings.Split(ramStr, "/")
			if len(ramParts) == 2 {
				if used, err := strconv.ParseFloat(strings.TrimSpace(ramParts[0]), 64); err == nil {
					m.ramUsedBytes = uint64(used * 1024 * 1024) // MB → bytes
				}
				if total, err := strconv.ParseFloat(strings.TrimSpace(ramParts[1]), 64); err == nil {
					m.ramTotalBytes = uint64(total * 1024 * 1024)
				}
			}
		}

		// Parse Temperature — find the field after RAM usage that looks like a temperature
		// The fields after RAM usage are: temp, voltage
		tempIdx := -1
		voltIdx := -1
		for k := 5; k < len(fields); k++ {
			// Skip the RAM usage fields (numbers, "/", numbers)
			if k == 5 || (k == 6 && fields[k] == "/") || k == 7 {
				continue
			}
			if tempIdx == -1 {
				tempIdx = k
			} else if voltIdx == -1 {
				voltIdx = k
				break
			}
		}

		if tempIdx >= 0 && tempIdx < len(fields) {
			if v, err := strconv.ParseFloat(fields[tempIdx], 64); err == nil {
				m.tempC = v
			}
		}
		if voltIdx >= 0 && voltIdx < len(fields) {
			if v, err := strconv.ParseFloat(fields[voltIdx], 64); err == nil {
				m.voltageMV = v
				m.powerW = v / 1000.0 // rough approximation for display
			}
		}

		return m
	}

	// Strategy 2: Parse JSON-ish format (older hailortcli versions)
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		hasAnyMetricKey := strings.Contains(ln, "util") || strings.Contains(ln, "temp") ||
			strings.Contains(ln, "power") || strings.Contains(ln, "energy")
		if !hasAnyMetricKey {
			continue
		}

		m := &hailoMetrics{nncUtil: -1}
		m.nncUtil = tryMatch(ln, "util")
		if m.nncUtil == -2 {
			continue
		}
		m.tempC = tryMatch(ln, "temp")
		m.powerW = tryMatch(ln, "power")
		return m
	}

	return nil
}

// tryMatch scans key:number pairs like `util:53.2`, `utilization:53.2`,
// `temp:64.5`, `temperature:64.5`, `power:3.2`. Returns -1 when the key is
// missing from the line, -2 when no recognized key appears at all.
func tryMatch(ln, key string) float64 {
	aliases := map[string][]string{
		"util":  {"util", "utilization"},
		"temp":  {"temp", "temperature"},
		"power": {"power"},
	}
	for _, a := range aliases[key] {
		idx := strings.Index(ln, a)
		if idx < 0 {
			continue
		}
		rest := ln[idx+len(a):]
		rest = strings.TrimLeft(rest, `: ="'`)
		numEnd := len(rest)
		for i := 0; i < len(rest); i++ {
			c := rest[i]
			if !(c >= '0' && c <= '9' || c == '.' || c == '-') {
				numEnd = i
				break
			}
		}
		v, err := strconv.ParseFloat(rest[:numEnd], 64)
		if err == nil {
			return v
		}
	}
	for _, a := range aliases[key] {
		if strings.Contains(ln, a) {
			return -1
		}
	}
	return -2
}
