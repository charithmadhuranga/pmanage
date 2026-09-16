package accelerator

import (
	"context"
	"strings"
	"testing"
)

// fakeControlSource implements both AcceleratorSource and ControlProvider so
// NvidiaControl tests exercise gating + delegation without NVML.
type fakeControlSource struct {
	modes map[string]string
}

func (f *fakeControlSource) Name() string { return "fake-control" }
func (f *fakeControlSource) Kind() Kind   { return GPU }
func (f *fakeControlSource) Detect() bool { return true }
func (*fakeControlSource) Devices() []DeviceInfo {
	return []DeviceInfo{{ID: "gpu0", Name: "Fake GPU", Vendor: "nvidia", Kind: GPU}}
}
func (f *fakeControlSource) Capabilities() []Capability {
	return []Capability{CapComputeMode}
}
func (f *fakeControlSource) SetComputeMode(deviceID, mode string) error {
	f.modes[deviceID] = mode
	return nil
}
func (f *fakeControlSource) GetComputeMode(deviceID string) (string, error) {
	return f.modes[deviceID], nil
}
func (f *fakeControlSource) GetMigMode(string) (string, error) { return "disabled", nil }

func (f *fakeControlSource) Sample(ctx context.Context) (Sample, error) {
	return Sample{Timestamp: 0}, nil
}

func sampleControlSource(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	f := &fakeControlSource{modes: map[string]string{"gpu0": "default"}}
	r.AddSource(f)
	r.Detect()
	return r
}

func TestControlSetComputeModeVerbose(t *testing.T) {
	r := sampleControlSource(t)
	ctl := NewControl(r)

	got, err := ctl.SetComputeModeVerbose("fake-control", "gpu0", "exclusive")
	if err != nil {
		t.Fatalf("set compute mode: %v", err)
	}
	if got != "exclusive" {
		t.Fatalf("expected exclusive, got %q", got)
	}
}

func TestControlCapabilityGate(t *testing.T) {
	r := sampleControlSource(t)
	ctl := NewControl(r)
	r.DisableCapability(CapComputeMode, true)

	if _, err := ctl.SetComputeModeVerbose("fake-control", "gpu0", "exclusive"); err == nil {
		t.Fatal("expected capability-gated error when compute-mode control disabled")
	} else if !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestControlUnknownSource(t *testing.T) {
	r := sampleControlSource(t)
	ctl := NewControl(r)

	if _, err := ctl.SetComputeModeVerbose("no-such-source", "gpu0", "exclusive"); err == nil {
		t.Fatal("expected error for unknown source")
	}
}

func TestControlMPSStub(t *testing.T) {
	r := sampleControlSource(t)
	ctl := NewControl(r)
	if err := ctl.MPSStart(); err == nil {
		t.Fatal("expected MPS stub to error with guidance")
	}
}
