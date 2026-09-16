package accelerator

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// smiRunner abstracts `nvidia-smi` invocation so the source is testable with
// captured fixtures. The real runner uses exec.LookPath + exec.Command.
type smiRunner interface {
	run(args ...string) (string, error)
}

type smiExecRunner struct {
	path string
}

func (r *smiExecRunner) run(args ...string) (string, error) {
	if r.path == "" {
		p, err := exec.LookPath("nvidia-smi")
		if err != nil {
			return "", err
		}
		r.path = p
	}
	cmd := exec.Command(r.path, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// SmiSource is a pure-Go fallback that reads NVIDIA device + per-process VRAM
// metrics by shelling out to nvidia-smi. Used when NVML fails to load (e.g.
// no driver library, restricted root, container). Lower fidelity than NVML:
// per-process utilization is not available via nvidia-smi (needs accounting
// mode), so it stays 0 / N/A unless pmon provides it.
type SmiSource struct {
	mu     sync.Mutex
	runner smiRunner
	info   []DeviceInfo
	ready  bool
}

func NewNvidiaSmiSource() *SmiSource {
	return &SmiSource{runner: &smiExecRunner{}}
}

func (s *SmiSource) Name() string { return "nvidia-smi" }
func (s *SmiSource) Kind() Kind   { return GPU }

// Capabilities advertises what the nvidia-smi CLI can observe. Per-process
// utilization is intentionally absent (not available via command line).
func (s *SmiSource) Capabilities() []Capability {
	return []Capability{
		CapPerProcessVRAM,
		CapPower, CapTemperature, CapClocks, CapFans,
	}
}

// gpuQueryFields are the --query-gpu columns emitted by Detect and Sample, in
// index order.
const gpuQueryFields = "index,name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw,clocks.gr,clocks.max.gr,fan.speed"

func (s *SmiSource) Detect() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ready {
		return true
	}

	out, err := s.runner.run("--query-gpu=" + gpuQueryFields + " --format=csv,noheader,nounits")
	if err != nil {
		return false
	}
	rows := smiRows(out)
	if len(rows) == 0 {
		return false
	}
	for i, r := range rows {
		idx := r.gpuIndex(i)
		s.info = append(s.info, DeviceInfo{
			ID:     fmt.Sprintf("gpu%d", idx),
			Name:   r[1],
			Vendor: "nvidia",
			Kind:   GPU,
			Driver: "nvidia-smi",
		})
	}
	s.ready = len(s.info) > 0
	return s.ready
}

func (s *SmiSource) Devices() []DeviceInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]DeviceInfo, len(s.info))
	copy(out, s.info)
	return out
}

func (s *SmiSource) Sample(ctx context.Context) (Sample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready {
		return Sample{}, fmt.Errorf("nvidia-smi source not ready")
	}

	out, err := s.runner.run("--query-gpu=" + gpuQueryFields + " --format=csv,noheader,nounits")
	if err != nil {
		return Sample{}, err
	}
	rows := smiRows(out)

	var sample Sample
	for _, d := range s.info {
		dm := DeviceMetrics{DeviceID: d.ID, Status: "ok"}
		for _, r := range rows {
			if r.gpuIndex(len(rows)) != gpuID(d.ID) {
				continue
			}
			// r = [0]index [1]name [2]util [3]mem.used [4]mem.total
			//     [5]temp [6]power [7]clock [8]maxclock [9]fan
			if v, ok := smiFloat(r[2]); ok {
				dm.UtilizationPct = v
			}
			if v, ok := smiUint(r[3]); ok {
				dm.VRAMUsed = v * 1024 * 1024 // MiB -> bytes
			}
			if v, ok := smiUint(r[4]); ok {
				dm.VRAMTotal = v * 1024 * 1024
			}
			if v, ok := smiFloat(r[5]); ok {
				dm.TemperatureC = v
			}
			if v, ok := smiFloat(r[6]); ok {
				dm.PowerW = v
			}
			if v, ok := smiUint(r[7]); ok {
				dm.ClockMHz = uint32(v)
			}
			if v, ok := smiUint(r[8]); ok {
				dm.ClockMaxMHz = uint32(v)
			}
			if v, ok := smiFloat(r[9]); ok {
				dm.FanSpeedPct = v
			}
			break
		}
		sample.Devices = append(sample.Devices, dm)
	}

	// Per-process VRAM via --query-compute-apps (pid, process_name, used_memory).
	if pout, err := s.runner.run("--query-compute-apps=pid,process_name,used_memory --format=csv,noheader,nounits"); err == nil {
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
				DeviceID: s.info[0].ID,
				VRAMUsed: uint64(mem * 1024 * 1024),
			})
		}
	}
	return sample, nil
}

// smiRow is a parsed nvidia-smi CSV record.
type smiRow []string

// gpuIndex parses the record's leading index column.
func (r smiRow) gpuIndex(fallback int) int {
	if len(r) > 0 {
		if v, err := strconv.Atoi(strings.TrimSpace(r[0])); err == nil {
			return v
		}
	}
	return fallback
}

func gpuID(id string) int {
	v, _ := strconv.Atoi(strings.TrimPrefix(id, "gpu"))
	return v
}

func smiRows(out string) []smiRow {
	var rows []smiRow
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "[") {
			continue
		}
		parts := strings.Split(ln, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		rows = append(rows, smiRow(parts))
	}
	return rows
}

func smiFloat(v string) (float64, bool) {
	v = strings.TrimSpace(v)
	if v == "" || v == "[N/A]" || strings.HasPrefix(v, "[") {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil
}

func smiUint(v string) (uint64, bool) {
	f, ok := smiFloat(v)
	if !ok || f < 0 {
		return 0, false
	}
	return uint64(f), true
}
