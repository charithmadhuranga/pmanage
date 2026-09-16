package accelerator

import (
	"context"
	"testing"
	"time"
)

func TestMockSource(t *testing.T) {
	src := NewMockSource()
	if !src.Detect() {
		t.Fatal("mock source should always detect")
	}
	if src.Name() != "mock" {
		t.Fatalf("unexpected name %q", src.Name())
	}

	devices := src.Devices()
	if len(devices) != 4 {
		t.Fatalf("expected 4 mock devices, got %d", len(devices))
	}

	sample, err := src.Sample(context.Background())
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	if len(sample.Devices) != 4 {
		t.Fatalf("expected 4 device metrics, got %d", len(sample.Devices))
	}
	if sample.Timestamp == 0 {
		t.Fatal("timestamp should be set")
	}

	// gpu0 should carry full metrics
	g0 := sample.Devices[0]
	if g0.DeviceID != "gpu0" || g0.VRAMTotal == 0 {
		t.Fatalf("gpu0 malformed: %+v", g0)
	}
	if g0.UtilizationPct < 0 || g0.UtilizationPct > 100 {
		t.Fatalf("gpu0 util %v out of range", g0.UtilizationPct)
	}

	// tpu0 should be n/a (honesty rule)
	t0 := sample.Devices[3]
	if t0.DeviceID != "tpu0" || t0.Status != "n/a" {
		t.Fatalf("tpu0 should be n/a: %+v", t0)
	}

	if len(sample.Procs) == 0 {
		t.Fatal("expected mock processes")
	}
}

func TestRegistryAssemblesAllSources(t *testing.T) {
	r := NewRegistry()
	r.AddSource(NewMockSource())

	devices := r.Detect()
	if len(devices) != 4 {
		t.Fatalf("expected 4 detected devices, got %d", len(devices))
	}
	if len(r.DetectedSources()) != 1 {
		t.Fatalf("expected 1 detected source")
	}

	// sample repeatedly to ensure the tick counter advances
	ctx := context.Background()
	s1 := r.SampleAll(ctx)
	time.Sleep(5 * time.Millisecond)
	s2 := r.SampleAll(ctx)
	if s1.Timestamp == s2.Timestamp {
		t.Fatal("timestamps should advance between samples")
	}
}
