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
		t.Logf("device=%s util=%.0f%% vramUsed=%d vramTotal=%d powerW=%.2f status=%s clockMHz=%d",
			m.DeviceID, m.UtilizationPct, m.VRAMUsed, m.VRAMTotal, m.PowerW, m.Status, m.ClockMHz)
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
		t.Logf("proc pid=%d name=%s dev=%s util=%.1f%%", p.PID, p.Name, p.DeviceID, p.UtilizationPct)
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
