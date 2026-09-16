//go:build darwin

package accelerator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// powermetricsSource runs `/usr/bin/powermetrics` as a privileged one-shot
// sample per tick and parses the XML-plist stream for SoC-level metrics.
//
// Enabled only when PMANAGE_POWERMETRICS=1 (the app never spawns sudo
// unilaterally). After initial auth, macOS caches the credential for ~5 min;
// during that window repeated samples run silently.
//
// Limitations (per Apple's docs + real-world testing):
//   - --show-process-gpu is unreliable on Apple Silicon — returns zeroes or is
//     absent from the plist. Per-process GPU remains honest N/A.
//   - Values are *estimates* and should not be used for absolute comparison.

type powermetricsSource struct {
	sampleRateMs int
	count        int
	mu           sync.Mutex
}

// NewPowermetricsSource returns an opt-in source guarded by the
// PMANAGE_POWERMETRICS env var. Returns nil when the env is not set.
func NewPowermetricsSource() AcceleratorSource {
	if !powermetricsEnabled() {
		return nil
	}
	return &powermetricsSource{
		sampleRateMs: 1000,
		count:        1,
	}
}

func powermetricsEnabled() bool {
	return os.Getenv("PMANAGE_POWERMETRICS") == "1"
}

func (s *powermetricsSource) Name() string { return "powermetrics" }
func (s *powermetricsSource) Kind() Kind   { return GPU }

// Capabilities advertises powermetrics' extra power/freq telemetry. Per-process
// GPU on Apple Silicon is unreliable via powermetrics (documented N/A).
func (s *powermetricsSource) Capabilities() []Capability {
	return []Capability{CapPower, CapClocks}
}
func (s *powermetricsSource) Devices() []DeviceInfo {
	return []DeviceInfo{
		{ID: "gpu0-pm", Vendor: "Apple", Name: "Apple GPU (powermetrics)", Driver: "powermetrics", Kind: GPU},
		{ID: "npu0-pm", Vendor: "Apple", Name: "Apple ANE (powermetrics)", Driver: "powermetrics", Kind: NPU},
	}
}

func (s *powermetricsSource) Detect() bool {
	pm, err := runPowermetrics(s.sampleRateMs, s.count)
	if err != nil {
		return false
	}
	defer pm.Close()
	_, err = readOnePlistSample(pm)
	return err == nil
}

func (s *powermetricsSource) Sample(_ context.Context) (Sample, error) {
	pm, err := runPowermetrics(s.sampleRateMs, s.count)
	if err != nil {
		return emptyPMSample(), nil
	}
	defer pm.Close()

	rec, err := readOnePlistSample(pm)
	if err != nil {
		return emptyPMSample(), nil
	}

	return Sample{
		Devices: []DeviceMetrics{
			{
				DeviceID:       "gpu0-pm",
				UtilizationPct: 0, // GPU util not reliable via powermetrics; IOGPU already provides it.
				PowerW:         rec.GPUPowerMW / 1000.0,
				ClockMHz:       uint32(rec.GPUFreqMHz),
				Status:         "ok",
			},
			{
				DeviceID:       "npu0-pm",
				UtilizationPct: -1, // not measurable without root; see ane_read for H/W identity.
				PowerW:         rec.ANEPowerMW / 1000.0,
				Status:         "ok",
			},
		},
	}, nil
}

func emptyPMSample() Sample {
	return Sample{
		Devices: []DeviceMetrics{
			{DeviceID: "gpu0-pm", Status: "n/a"},
			{DeviceID: "npu0-pm", Status: "n/a"},
		},
	}
}

// ---------------------------------------------------------------------------
// powermetrics invocation
// ---------------------------------------------------------------------------

func runPowermetrics(sampleRateMs, count int) (io.ReadCloser, error) {
	args := []string{
		"--samplers", "gpu_power,ane_power",
		"-i", fmt.Sprintf("%d", sampleRateMs),
		"-n", fmt.Sprintf("%d", count),
		"-f", "plist",
	}
	// Try without sudo first (works if running as root or credential cached).
	if rc, err := tryCmd("/usr/bin/powermetrics", args); err == nil {
		return rc, nil
	}
	return runViaAppleScript(args)
}

func tryCmd(bin string, args []string) (io.ReadCloser, error) {
	cmd := exec.Command(bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	rc, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &waitCloser{rc, cmd}, nil
}

type waitCloser struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (w *waitCloser) Close() error {
	w.ReadCloser.Close()
	return w.cmd.Wait()
}

func runViaAppleScript(args []string) (io.ReadCloser, error) {
	shellCmd := "/usr/bin/powermetrics " + shellJoin(args)
	script := fmt.Sprintf(`do shell script %q with administrator privileges`, shellCmd)
	cmd := exec.Command("osascript", "-e", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	rc, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &waitCloser{rc, cmd}, nil
}

func shellJoin(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \"'") {
			out[i] = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		} else {
			out[i] = a
		}
	}
	return strings.Join(out, " ")
}

// ---------------------------------------------------------------------------
// plist parsing: pipe through plutil → JSON → encoding/json
// ---------------------------------------------------------------------------

type pmRecord struct {
	GPUPowerMW float64 `json:"gpu_power"`
	ANEPowerMW float64 `json:"ane_power"`
	GPUFreqMHz float64 `json:"-"`
}

func readOnePlistSample(r io.Reader) (*pmRecord, error) {
	// Read until first NUL byte (one complete plist sample).
	var buf bytes.Buffer
	one := make([]byte, 4096)
	for {
		n, err := r.Read(one)
		for i := 0; i < n; i++ {
			if one[i] == 0 {
				if buf.Len() == 0 {
					continue // skip empty delimiters
				}
				return convertAndParse(buf.Bytes())
			}
			buf.WriteByte(one[i])
		}
		if err != nil {
			break
		}
	}
	if buf.Len() == 0 {
		return nil, errors.New("no plist sample received")
	}
	return convertAndParse(buf.Bytes())
}

// convertAndParse pipes the raw XML plist through plutil to get JSON, then
// extracts the fields we care about.
func convertAndParse(plistData []byte) (*pmRecord, error) {
	cmd := exec.Command("plutil", "-convert", "json", "-o", "-", "-r", "-")
	cmd.Stdin = bytes.NewReader(plistData)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("plutil convert: %w", err)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &root); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	rec := &pmRecord{}

	if proc, ok := root["processor"].(map[string]interface{}); ok {
		if v, ok := proc["gpu_power"].(float64); ok {
			rec.GPUPowerMW = v
		}
		if v, ok := proc["ane_power"].(float64); ok {
			rec.ANEPowerMW = v
		}
		// Fallback: energy fields (mJ over sample window ≈ mW for 1 s).
		if rec.GPUPowerMW == 0 {
			if v, ok := proc["gpu_energy"].(float64); ok {
				rec.GPUPowerMW = v
			}
		}
		if rec.ANEPowerMW == 0 {
			if v, ok := proc["ane_energy"].(float64); ok {
				rec.ANEPowerMW = v
			}
		}
	}
	if gpu, ok := root["gpu"].(map[string]interface{}); ok {
		if v, ok := gpu["freq_hz"].(float64); ok {
			rec.GPUFreqMHz = v / 1e6
		}
	}

	return rec, nil
}
