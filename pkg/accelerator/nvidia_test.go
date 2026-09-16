package accelerator

import (
	"context"
	"runtime"
	"testing"
)

func TestNvidiaSource_NoDriver(t *testing.T) {
	src := NewNvidiaSource()

	detected := src.Detect()
	if runtime.GOOS == "darwin" {
		// macOS never has NVIDIA (post-2019) — detect must return false without crashing
		if detected {
			t.Fatal("nvidia detect should be false on macOS")
		}
		if len(src.Devices()) != 0 {
			t.Fatal("devices should be empty when not detected")
		}
		if _, err := src.Sample(context.Background()); err == nil {
			t.Fatal("sample should error when not detected")
		}
		return
	}

	// On linux: if no NVIDIA hardware, detect should still return false gracefully
	if detected {
		t.Log("NVIDIA hardware detected on linux — running full sample")
		sample, err := src.Sample(context.Background())
		if err != nil {
			t.Fatalf("sample: %v", err)
		}
		if len(sample.Devices) == 0 {
			t.Fatal("expected at least one device sample")
		}
		for _, dm := range sample.Devices {
			if dm.DeviceID == "" || dm.Status == "" {
				t.Fatalf("incomplete device metrics: %+v", dm)
			}
			t.Logf("device %s: util=%.0f%% vram=%d/%d temp=%.0f°C power=%.1fW",
				dm.DeviceID, dm.UtilizationPct, dm.VRAMUsed, dm.VRAMTotal, dm.TemperatureC, dm.PowerW)
		}
		t.Logf("processes: %d", len(sample.Procs))
	} else {
		t.Log("no NVIDIA hardware — graceful fallback confirmed")
		if len(src.Devices()) != 0 {
			t.Fatal("devices should be empty when not detected")
		}
		if _, err := src.Sample(context.Background()); err == nil {
			t.Fatal("sample should error when not detected")
		}
	}

	src.Shutdown()
}

func TestNvidiaSource_InterfaceSatisfaction(t *testing.T) {
	src := NewNvidiaSource()
	var _ AcceleratorSource = src
}
