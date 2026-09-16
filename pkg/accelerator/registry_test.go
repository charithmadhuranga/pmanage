package accelerator

import (
	"context"
	"testing"
)

func TestRegistryEnableToggle(t *testing.T) {
	r := NewRegistry()
	r.AddSource(NewMockSource())
	r.AddSource(NewMockSourceNamed("mock2"))

	devices := r.Detect()
	if len(devices) != 6 {
		t.Fatalf("expected 6 devices from 2 sources, got %d", len(devices))
	}
	if len(r.DetectedSources()) != 2 {
		t.Fatalf("expected 2 detected sources, got %d", len(r.DetectedSources()))
	}

	r.Enable("mock", false)
	if r.IsEnabled("mock") {
		t.Fatal("mock should be disabled")
	}
	if !r.IsEnabled("mock2") {
		t.Fatal("mock2 should remain enabled")
	}

	devices = r.Detect()
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices after disabling mock, got %d", len(devices))
	}
	if len(r.DetectedSources()) != 1 {
		t.Fatalf("expected 1 detected source, got %d", len(r.DetectedSources()))
	}

	s := r.SampleAll(context.Background())
	if len(s.Devices) != 2 {
		t.Fatalf("expected 2 devices in sample, got %d", len(s.Devices))
	}

	r.Enable("mock", true)
	devices = r.Detect()
	if len(devices) != 6 {
		t.Fatalf("expected 6 devices after re-enabling, got %d", len(devices))
	}

	if got := r.Source("no-such-source"); got != nil {
		t.Fatalf("expected nil for unknown source, got %v", got)
	}
	if got := r.Source("mock"); got == nil {
		t.Fatal("expected mock source to be findable")
	}
}

func TestRegistryCapabilityMatrix(t *testing.T) {
	r := NewRegistry()
	r.AddSource(NewMockSource())
	r.AddSource(NewMockSourceNamed("mock2"))

	// mockSource does not implement capabilitySource -> empty matrix, and the
	// registry must not panic.
	matrix := r.Capabilities()
	for _, name := range []string{"mock", "mock2"} {
		if _, ok := matrix[name]; !ok {
			t.Fatalf("capability matrix missing source %q", name)
		}
	}

	// Capability overrides mask across all sources.
	if !r.CapabilityEnabled(CapPower) {
		t.Fatal("capability should be enabled by default")
	}
	r.DisableCapability(CapPower, true)
	if r.CapabilityEnabled(CapPower) {
		t.Fatal("capability should be disabled after override")
	}
}
