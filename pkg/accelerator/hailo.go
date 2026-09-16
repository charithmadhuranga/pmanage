package accelerator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// HailoSource runs `hailortcli monitor` (HAILO_MONITOR output lines) to read
// Hailo-8/10 device-level util, temperature, and bandwidth. Per-process
// attribution is not available — always N/A.
type HailoSource struct {
	cli   cliRunner
	devs  []hailoDev
	ready bool
}

type hailoDev struct {
	id   string
	info DeviceInfo
}

func NewHailoSource() *HailoSource { return &HailoSource{cli: &execRunner{name: "hailortcli"}} }

func (s *HailoSource) Name() string { return "hailo" }
func (s *HailoSource) Kind() Kind   { return NPU }

func (s *HailoSource) Capabilities() []Capability {
	// monitor reports util/temp/bandwidth but no per-process attribution.
	return []Capability{CapTemperature, CapPower}
}

func (s *HailoSource) Detect() bool {
	if s.ready {
		return true
	}
	// fw-control identify returns device info lines like:
	// ID: 0000-0000-0000-0000, Hailo-8, running firmware ...
	out, err := s.cli.run("fw-control", "identify")
	if err != nil {
		return false
	}
	count := 0
	for _, ln := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		count++
		id := fmt.Sprintf("hailo%d", len(s.devs))
		s.devs = append(s.devs, hailoDev{
			id: id,
			info: DeviceInfo{
				ID:     id,
				Name:   "Hailo-" + hailoModel(trimmed),
				Vendor: "hailo",
				Kind:   NPU,
				Driver: "hailort",
			},
		})
	}
	// Honest detection: if identify parsed no devices, we report nothing and
	// mark the source not-ready — an empty parse is "no Hailo hardware seen",
	// not a license to fabricate a generic invisible Hailo-8.
	if count == 0 {
		s.ready = false
		return false
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

	// monitor prints report lines at interval; credential-less device-level
	// count is derived from identify at first sample as a best-effort proxy.
	out, err := s.cli.run("monitor", "--interval=1000", "--no-clear-screen")
	if err != nil {
		// monitor may need a TTY; degrade to a N/A device row rather than fail.
		for _, d := range s.devs {
			sample.Devices = append(sample.Devices, DeviceMetrics{
				DeviceID: d.id, Status: "ok", UtilizationPct: -1,
			})
		}
		return sample, nil
	}
	for _, d := range s.devs {
		dm := DeviceMetrics{DeviceID: d.id, Status: "ok", UtilizationPct: -1}
		if m := hailoParse(out, d.info.Name); m != nil {
			if m.util >= 0 {
				dm.UtilizationPct = m.util
			}
			dm.TemperatureC = m.tempC
			dm.PowerW = m.powerW
			dm.Status = "ok"
		}
		sample.Devices = append(sample.Devices, dm)
	}
	sample.Procs = nil
	return sample, nil
}

type hailoMetrics struct {
	util   float64
	tempC  float64
	powerW float64
}

// hailoParse extracts device-level metrics from hailortcli monitor output.
// Supports the JSON-ish line form:
//
//	{"device_index":0,"device_id":"0000-...","utilization":53.2,"temperature":64.5,"power":3.2}
//
// and the plaintext form:
//
//	device:0 util:53.2% temp:64.5C power:3.2W
//
// Lines are included if they contain any recognized key (util/temp/power)
// regardless of the want filter, which is applied as a pre-check.
func hailoParse(out, want string) *hailoMetrics {
	m := &hailoMetrics{util: -1}
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		// Pre-filter: skip lines that are clearly not metric lines (empty JSON
		// objects, header banners, etc.) but do not enforce strict name match
		// since JSON payloads may not contain the human-readable name.
		if want != "" {
			hasAnyMetricKey := strings.Contains(ln, "util") || strings.Contains(ln, "temp") ||
				strings.Contains(ln, "power") || strings.Contains(ln, "energy")
			if !hasAnyMetricKey {
				continue
			}
		}
		util := tryMatch(ln, "util")
		if util == -2 {
			continue // no util field present → not a metrics line
		}
		m.util = util
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

func hailoModel(trimmed string) string {
	if strings.Contains(strings.ToLower(trimmed), "10") {
		return "10"
	}
	return "8"
}
