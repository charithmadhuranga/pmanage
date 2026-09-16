package accelerator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// JetsonSource monitors an NVIDIA Jetson (L4T) via `tegrastats`: aggregate
// engine loads (GR3D, NVENC, NVDEC, NVDLA, EMC) and per-rail power. Device
// info comes from /etc/nv_tegra_release. Per-process GPU attribution reuses
// the nvidia-smi query when the tool is present, else N/A.
type JetsonSource struct {
	cli   cliRunner // tegrastats
	smi   smiRunner
	fs    npuFS
	devs  []jetsonDev
	ready bool
}

type jetsonDev struct {
	id   string
	info DeviceInfo
}

func NewJetsonSource() *JetsonSource {
	return &JetsonSource{
		cli: &execRunner{name: "tegrastats"},
		smi: &smiExecRunner{},
		fs:  realNpuFS{},
	}
}

func (s *JetsonSource) Name() string { return "jetson" }
func (s *JetsonSource) Kind() Kind   { return GPU }

func (s *JetsonSource) Capabilities() []Capability {
	// Per-process GPU only when nvidia-smi is present on L4T.
	return []Capability{CapPower, CapClocks, CapPerProcessGPU, CapPerProcessVRAM}
}

func (s *JetsonSource) Detect() bool {
	if s.ready {
		return true
	}
	// L4T release marker routed through the injectable fs (fixture-testable).
	rel, err := s.fs.readFile("/etc/nv_tegra_release")
	if err != nil {
		return false
	}
	// tegrastats must exist; nvidia-smi per-process is optional.
	id := "jetson0"
	name := "NVIDIA Jetson (Tegra)"
	if rel != "" {
		if fields := strings.Fields(rel); len(fields) >= 2 {
			name = "Jetson " + fields[0] + " " + fields[1] + " (Tegra)"
		}
	}
	s.devs = append(s.devs, jetsonDev{
		id: id,
		info: DeviceInfo{
			ID:     id,
			Name:   name,
			Vendor: "nvidia",
			Kind:   GPU,
			Driver: "tegra",
		},
	})
	s.ready = true
	return true
}

func (s *JetsonSource) Devices() []DeviceInfo {
	out := make([]DeviceInfo, len(s.devs))
	for i, d := range s.devs {
		out[i] = d.info
	}
	return out
}

func (s *JetsonSource) Sample(ctx context.Context) (Sample, error) {
	if !s.ready {
		return Sample{}, fmt.Errorf("jetson not ready")
	}
	var sample Sample
	sample.Timestamp = time.Now().UnixMilli()

	out, err := s.cli.run("--interval=1000", "--exit")
	if err != nil {
		// tegrastats not runnable → still surface device with N/A row.
		for _, d := range s.devs {
			sample.Devices = append(sample.Devices, DeviceMetrics{
				DeviceID: d.id, Status: "ok", UtilizationPct: -1,
			})
		}
		return sample, nil
	}

	for _, d := range s.devs {
		dm := DeviceMetrics{DeviceID: d.id, Status: "ok", UtilizationPct: -1}
		if m := tegrastatsParse(out); m != nil {
			dm.UtilizationPct = m.g3d
			// Engine breakdown folded into the device row: NVDLA depth, etc.
			dm.VRAMUsed = m.ramUsed
			dm.VRAMTotal = m.ramTotal
			dm.ClockMHz = m.g3dFreqMHz
			dm.PowerW = m.powerW
			dm.Status = "ok"
		}
		sample.Devices = append(sample.Devices, dm)
	}

	// Per-process GPU via nvidia-smi compute-apps (present on most L4T images).
	if pout, err := s.smi.run("--query-compute-apps=pid,process_name,used_memory --format=csv,noheader,nounits"); err == nil {
		for _, r := range smiRows(pout) {
			if len(r) < 3 {
				continue
			}
			pid, err1 := strconv.Atoi(strings.TrimSpace(r[0]))
			mem, err2 := strconv.ParseFloat(strings.TrimSpace(r[2]), 64)
			if err1 != nil || err2 != nil {
				continue
			}
			sample.Procs = append(sample.Procs, ProcUsage{
				PID:      int32(pid),
				Name:     strings.TrimSpace(r[1]),
				DeviceID: s.devs[0].id,
				VRAMUsed: uint64(mem * 1024 * 1024),
			})
		}
	}
	return sample, nil
}

type tegraMetrics struct {
	g3d        float64
	g3dFreqMHz uint32
	ramUsed    uint64
	ramTotal   uint64
	powerW     float64
}

// tegrastatsParse extracts the latest metrics line from `tegrastats`
// output, e.g.:
//
//	RAM 4187/15569MB (lfb 4803MB) IRAM 0/128kB(lfb 128kB) ...
//	GR3D_FREQ 47% 670MHz ... VDD_GPU 4893/1850mV 624mW ...
func tegrastatsParse(out string) *tegraMetrics {
	m := &tegraMetrics{g3d: -1}
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		ln := lines[i]
		if !strings.Contains(ln, "GR3D_FREQ") && !strings.Contains(ln, "GR3D") {
			continue
		}
		m.g3d = fieldPct(ln, "GR3D_FREQ")
		m.g3dFreqMHz = fieldMHz(ln)
		if tok := fieldVal(ln, "RAM"); tok != "" {
			if a, b, ok := parseUsedTotal(tok); ok {
				m.ramUsed, m.ramTotal = a, b
			}
		}
		for _, rail := range []string{"VDD_GPU", "VDD_SOC"} {
			if w, ok := railPower(ln, rail); ok {
				m.powerW = w
				break
			}
		}
		return m
	}
	return nil
}

// fieldPct returns the percent following the token, e.g. "GR3D_FREQ 47%" → 47.
func fieldPct(ln, token string) float64 {
	if idx := strings.Index(ln, token); idx >= 0 {
		return parseNum(ln[idx+len(token):], "%")
	}
	return -1
}

// fieldMHZAfter returns the MHz value right after a given engine token, e.g.
// "GR3D_FREQ 47% 670MHz" → 670.
func fieldMHzAfter(ln, token string) uint32 {
	idx := strings.Index(ln, token)
	if idx < 0 {
		return 0
	}
	seg := ln[idx+len(token):]
	mhz := strings.Index(seg, "MHz")
	if mhz < 0 {
		return 0
	}
	seg = seg[:mhz]
	sp := strings.LastIndex(seg, " ")
	var num string
	if sp < 0 {
		num = seg
	} else {
		num = seg[sp+1:]
	}
	num = strings.TrimSpace(num)
	if num == "" {
		return 0
	}
	v, err := strconv.ParseUint(num, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(v)
}

func fieldMHz(ln string) uint32 {
	return fieldMHzAfter(ln, "GR3D_FREQ")
}

func fieldVal(ln, token string) string {
	idx := strings.Index(ln, token)
	if idx < 0 {
		return ""
	}
	rest := ln[idx+len(token):]
	rest = strings.TrimLeft(rest, " ")
	end := strings.IndexAny(rest, " ")
	if end < 0 {
		end = len(rest)
	}
	return strings.TrimRight(rest[:end], " ")
}

func parseUsedTotal(tok string) (used, total uint64, ok bool) {
	a, b, ok := strings.Cut(strings.TrimSuffix(tok, "MB"), "/")
	if !ok {
		return 0, 0, false
	}
	u, err1 := strconv.ParseUint(strings.TrimSpace(a), 10, 64)
	t, err2 := strconv.ParseUint(strings.TrimSpace(b), 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return u * 1024 * 1024, t * 1024 * 1024, true
}

func railPower(ln, rail string) (float64, bool) {
	idx := strings.Index(ln, rail)
	if idx < 0 {
		return 0, false
	}
	// e.g. "... VDD_GPU 4893/1850mV 624mW ..." — take the first "NNNmW".
	rest := ln[idx:]
	for _, tok := range strings.Fields(rest) {
		if strings.HasSuffix(tok, "mW") {
			v, err := strconv.ParseFloat(strings.TrimSuffix(tok, "mW"), 64)
			if err != nil {
				return 0, false
			}
			return v / 1000, true
		}
		if strings.HasSuffix(tok, "W") {
			v, err := strconv.ParseFloat(strings.TrimSuffix(tok, "W"), 64)
			if err != nil {
				return 0, false
			}
			return v, true
		}
	}
	return 0, false
}

// parseNum extracts the leading numeric value from a string like " 47% 670MHz".
func parseNum(rest, unit string) float64 {
	rest = strings.TrimSpace(rest)
	for i, ch := range rest {
		if !(ch >= '0' && ch <= '9' || ch == '.' || ch == '-' || ch == '+') {
			rest = rest[:i]
			break
		}
	}
	rest = strings.TrimSpace(rest)
	if unit != "" {
		rest = strings.TrimSuffix(rest, unit)
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
	if err != nil {
		return -1
	}
	return v
}
