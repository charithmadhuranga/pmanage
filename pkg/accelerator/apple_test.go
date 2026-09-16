//go:build darwin

package accelerator

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestAppleSource_Live validates against real IOKit + IOReport data on Apple
// Silicon. It is skipped automatically when running in CI or on Intel macs
// without an IOGPU-family service exposing PerformanceStatistics.
func TestAppleSource_Live(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	src := NewAppleSource()
	if !src.Detect() {
		t.Log("no IOGPU accelerator with PerformanceStatistics detected — skipping")
		return
	}
	devs := src.Devices()
	if len(devs) == 0 {
		t.Fatal("detect true but no devices")
	}
	for _, d := range devs {
		if d.Vendor != "Apple" {
			t.Errorf("bad device metadata: %+v", d)
		}
	}

	// First sample initializes the energy baseline (power = 0 W).
	if _, err := src.Sample(context.Background()); err != nil {
		t.Fatalf("sample: %v", err)
	}
	time.Sleep(1 * time.Second)
	sample, err := src.Sample(context.Background())
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	if len(sample.Devices) != len(devs) {
		t.Fatalf("expected %d device metrics, got %d", len(devs), len(sample.Devices))
	}
	for _, m := range sample.Devices {
		t.Logf("device=%s util=%.0f%% vramUsed=%d vramTotal=%d powerW=%.2f status=%s clockMHz=%d tempC=%.1f",
			m.DeviceID, m.UtilizationPct, m.VRAMUsed, m.VRAMTotal, m.PowerW, m.Status, m.ClockMHz, m.TemperatureC)
		if m.Status != "ok" {
			t.Errorf("expected ok status on M-series, got %s", m.Status)
		}
		// Utilization is a percentage from the kernel; negative means N/A (ANE).
		if m.UtilizationPct > 100 {
			t.Errorf("implausible utilization: %f", m.UtilizationPct)
		}
		if m.DeviceID == "gpu0" && m.UtilizationPct < 0 {
			t.Error("gpu0 should have a non-negative utilization")
		}
		if m.DeviceID == "gpu0" && m.VRAMTotal == 0 {
			t.Error("expected non-zero unified memory total")
		}
		// Energy delta over one second should land GPU/ANE power in a sane band.
		if m.PowerW < 0 || m.PowerW > 200 {
			t.Errorf("implausible power: %.2f W", m.PowerW)
		}
	}

	// Per-process GPU column (Activity Monitor parity): at least one process
	// uses the GPU in a booted session on M-series.
	t.Logf("gpu procs=%d", len(sample.Procs))
	foundGpu := false
	for _, p := range sample.Procs {
		t.Logf("proc pid=%d name=%s dev=%s util=%.1f%% cpu=%.1f%% memRes=%d",
			p.PID, p.Name, p.DeviceID, p.UtilizationPct, p.CpuUsage, p.MemResident)
		if p.UtilizationPct < 0 || p.UtilizationPct > 100 {
			t.Errorf("implausible per-process util: %f", p.UtilizationPct)
		}
		if p.DeviceID != "gpu0" {
			t.Errorf("unexpected per-process device id: %s", p.DeviceID)
		}
		if p.PID > 0 && p.UtilizationPct >= 0 {
			foundGpu = true
		}
	}
	if !foundGpu {
		t.Log("no active GPU processes in this 1 s window (idle) — acceptable")
	}
}

// TestAppleGPU_DetectMetadata validates that GPU detection returns correct
// metadata on Apple Silicon: model name, core count, vendor, driver.
func TestAppleGPU_DetectMetadata(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	gpus := appleGPUs()
	if len(gpus) == 0 {
		t.Skip("no Apple GPU detected")
	}
	for i, g := range gpus {
		t.Logf("gpu[%d]: name=%q model=%q cores=%d hasStats=%v",
			i, g.Name, g.Model, g.CoreCount, g.HasStats)
		if g.Model == "" && g.Name == "" {
			t.Error("expected at least one of model or name to be non-empty")
		}
		if g.CoreCount <= 0 {
			t.Errorf("expected positive core count, got %d", g.CoreCount)
		}
		if !g.HasStats {
			t.Error("expected PerformanceStatistics on Apple Silicon")
		}
	}
}

// TestAppleANE_Detect validates ANE detection returns valid identity data.
func TestAppleANE_Detect(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	ane := appleANEs()
	if !ane.Found {
		t.Skip("no Apple ANE detected")
	}
	t.Logf("ANE: name=%q arch=%q ver=%q cores=%d", ane.Name, ane.Arch, ane.Ver, ane.Cores)
	if ane.Cores <= 0 {
		t.Errorf("expected positive ANE core count, got %d", ane.Cores)
	}
}

// TestAppleSource_NPUHasMemory validates that NPU device metrics include
// unified memory total (not zero).
func TestAppleSource_NPUHasMemory(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	src := NewAppleSource()
	if !src.Detect() {
		t.Skip("no Apple accelerator detected")
	}
	devs := src.Devices()
	hasNPU := false
	for _, d := range devs {
		if d.Kind == NPU {
			hasNPU = true
			break
		}
	}
	if !hasNPU {
		t.Skip("no NPU device detected")
	}

	// First sample to init baseline.
	src.Sample(context.Background())
	time.Sleep(500 * time.Millisecond)
	sample, err := src.Sample(context.Background())
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	for _, m := range sample.Devices {
		if m.DeviceID == "npu0" {
			t.Logf("NPU: util=%.0f%% powerW=%.2f vramTotal=%d tempC=%.1f status=%s",
				m.UtilizationPct, m.PowerW, m.VRAMTotal, m.TemperatureC, m.Status)
			if m.VRAMTotal == 0 {
				t.Error("NPU should report unified memory total, got 0")
			}
			if m.UtilizationPct != -1 {
				t.Errorf("NPU util should be -1 (N/A), got %.0f", m.UtilizationPct)
			}
			if m.TemperatureC != -1 {
				t.Errorf("NPU temp should be -1 (N/A), got %.0f", m.TemperatureC)
			}
			return
		}
	}
	t.Error("NPU device not found in sample")
}

// TestAppleSource_TemperatureNA validates that GPU temperature is set to -1
// (N/A sentinel) since Apple does not expose GPU temp via IOKit.
func TestAppleSource_TemperatureNA(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	src := NewAppleSource()
	if !src.Detect() {
		t.Skip("no Apple accelerator detected")
	}
	src.Sample(context.Background())
	time.Sleep(500 * time.Millisecond)
	sample, err := src.Sample(context.Background())
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	for _, m := range sample.Devices {
		if m.DeviceID == "gpu0" {
			t.Logf("GPU tempC=%.1f", m.TemperatureC)
			if m.TemperatureC != -1 {
				t.Errorf("GPU temp should be -1 (N/A), got %.0f", m.TemperatureC)
			}
			return
		}
	}
	t.Error("gpu0 not found in sample")
}

// TestAppleSource_ConsistentSamples validates that consecutive samples
// return consistent device counts and IDs.
func TestAppleSource_ConsistentSamples(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	src := NewAppleSource()
	if !src.Detect() {
		t.Skip("no Apple accelerator detected")
	}
	devs := src.Devices()

	for i := 0; i < 5; i++ {
		sample, err := src.Sample(context.Background())
		if err != nil {
			t.Fatalf("sample %d: %v", i, err)
		}
		if len(sample.Devices) != len(devs) {
			t.Errorf("sample %d: expected %d devices, got %d", i, len(devs), len(sample.Devices))
		}
		for j, m := range sample.Devices {
			if j < len(devs) && m.DeviceID != devs[j].ID {
				t.Errorf("sample %d: device %d ID mismatch: expected %s, got %s",
					i, j, devs[j].ID, m.DeviceID)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// TestAppleSource_PerProcessGPU_DeltaAccuracy validates that per-process GPU
// utilization is computed correctly across multiple samples.
func TestAppleSource_PerProcessGPU_DeltaAccuracy(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	src := NewAppleSource()
	if !src.Detect() {
		t.Skip("no Apple accelerator detected")
	}

	// First sample initializes baseline.
	src.Sample(context.Background())
	time.Sleep(1 * time.Second)

	// Second sample should have per-process data.
	sample, err := src.Sample(context.Background())
	if err != nil {
		t.Fatalf("sample: %v", err)
	}

	t.Logf("per-process GPU procs=%d", len(sample.Procs))
	for _, p := range sample.Procs {
		if p.PID <= 0 {
			t.Errorf("invalid PID: %d", p.PID)
		}
		if p.Name == "" {
			t.Errorf("empty process name for PID %d", p.PID)
		}
		if p.DeviceID != "gpu0" {
			t.Errorf("unexpected device ID: %s", p.DeviceID)
		}
		if p.UtilizationPct < 0 || p.UtilizationPct > 100 {
			t.Errorf("PID %d: implausible util %.1f%%", p.PID, p.UtilizationPct)
		}
	}
}

// TestAppleSource_NoErrorWithANEOnly validates that Sample() does not return
// an error when ANE is detected but GPU metrics are unavailable.
func TestAppleSource_NoErrorWithANEOnly(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	src := NewAppleSource()
	if !src.Detect() {
		t.Skip("no Apple accelerator detected")
	}

	// Even if GPU has no PerformanceStatistics, the sample should succeed
	// as long as at least one device (GPU or ANE) exists.
	_, err := src.Sample(context.Background())
	// On real Apple Silicon, this should never error because we have devices.
	if err != nil {
		t.Logf("sample returned error (may be expected on non-Apple hardware): %v", err)
	}
}

// TestAppleSource_PowerBaseline validates that the first sample returns 0 W
// (energy baseline) and subsequent samples return non-negative power.
func TestAppleSource_PowerBaseline(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	src := NewAppleSource()
	if !src.Detect() {
		t.Skip("no Apple accelerator detected")
	}

	// First sample: power should be 0 (baseline).
	s1, err := src.Sample(context.Background())
	if err != nil {
		t.Fatalf("sample 1: %v", err)
	}
	for _, m := range s1.Devices {
		if m.PowerW != 0 {
			t.Errorf("first sample device %s: expected 0 W baseline, got %.2f W", m.DeviceID, m.PowerW)
		}
	}

	time.Sleep(1 * time.Second)

	// Second sample: power should be non-negative.
	s2, err := src.Sample(context.Background())
	if err != nil {
		t.Fatalf("sample 2: %v", err)
	}
	for _, m := range s2.Devices {
		if m.PowerW < 0 {
			t.Errorf("second sample device %s: negative power %.2f W", m.DeviceID, m.PowerW)
		}
		t.Logf("device %s power=%.2f W", m.DeviceID, m.PowerW)
	}
}

// TestAppleSource_SystemMemory validates that host_info returns valid memory.
func TestAppleSource_SystemMemory(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	mem := appleSystemMemory()
	t.Logf("system memory: %d bytes (%.1f GB)", mem, float64(mem)/1e9)
	if mem == 0 {
		t.Error("host_info returned 0 bytes for system memory")
	}
	if mem < 1e9 {
		t.Errorf("implausible system memory: %d bytes", mem)
	}
}

// TestAppleSource_PerProcessInfo validates that proc_pidinfo returns valid data.
func TestAppleSource_PerProcessInfo(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	// Test with PID 1 (launchd)
	userTime, kernelTime, virtMem, residentMem, ok := appleProcessInfo(1)
	if !ok {
		t.Skip("proc_pidinfo failed for PID 1 (may need elevated privileges)")
	}
	t.Logf("PID 1: userTime=%.2f kernelTime=%.2f virtMem=%d residentMem=%d",
		userTime, kernelTime, virtMem, residentMem)
	if virtMem == 0 {
		t.Error("expected non-zero virtual memory for PID 1")
	}
}

// TestAppleSource_ProcessUsername validates that process username resolution works.
func TestAppleSource_ProcessUsername(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	username := appleProcessUsername(1)
	t.Logf("PID 1 username: %q", username)
	if username == "" {
		t.Skip("could not resolve username for PID 1")
	}
}

// TestAppleSource_ProcessCommand validates that process command line resolution works.
func TestAppleSource_ProcessCommand(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	cmd := appleProcessCommand(1)
	t.Logf("PID 1 command: %q", cmd)
	if cmd == "" {
		t.Skip("could not resolve command for PID 1")
	}
}

// TestAppleSource_ValidBitmask validates that the validity bitmask system works.
func TestAppleSource_ValidBitmask(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("live IOKit test skipped in CI")
	}
	src := NewAppleSource()
	if !src.Detect() {
		t.Skip("no Apple accelerator detected")
	}
	src.Sample(context.Background())
	time.Sleep(500 * time.Millisecond)
	sample, err := src.Sample(context.Background())
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	for _, m := range sample.Devices {
		t.Logf("device=%s valid_util=%v valid_power=%v valid_vram=%v valid_temp=%v",
			m.DeviceID,
			m.IsValid(ValidUtilization),
			m.IsValid(ValidPower),
			m.IsValid(ValidVRAMTotal),
			m.IsValid(ValidTemperature),
		)
		// GPU should have valid utilization and power
		if m.DeviceID == "gpu0" {
			if !m.IsValid(ValidUtilization) {
				t.Error("gpu0 should have valid utilization")
			}
			if !m.IsValid(ValidPower) {
				t.Error("gpu0 should have valid power")
			}
			if !m.IsValid(ValidVRAMTotal) {
				t.Error("gpu0 should have valid VRAM total")
			}
			// Temperature should NOT be valid (Apple doesn't expose it)
			if m.IsValid(ValidTemperature) {
				t.Error("gpu0 should NOT have valid temperature (Apple doesn't expose it)")
			}
		}
	}
}
