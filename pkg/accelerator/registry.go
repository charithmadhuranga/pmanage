package accelerator

import (
	"context"
	"strings"
	"time"
)

type Registry struct {
	sources     []AcceleratorSource
	detected    []AcceleratorSource
	disabled    map[string]bool
	capOverride map[string]bool // capability -> globally force-disabled
}

func NewRegistry() *Registry {
	return &Registry{
		disabled:    map[string]bool{},
		capOverride: map[string]bool{},
	}
}

// Enable/Disable toggle a source by name for subsequent Detect/SampleAll runs.
// Unknown names are no-ops.
func (r *Registry) Enable(name string, on bool) {
	if r == nil {
		return
	}
	for _, s := range r.sources {
		if s.Name() != name {
			continue
		}
		if on {
			delete(r.disabled, name)
		} else {
			r.disabled[name] = true
		}
		return
	}
}

func (r *Registry) IsEnabled(name string) bool {
	if r == nil {
		return true
	}
	return !r.disabled[name]
}

// DisableCapability globally masks a capability across all sources until the
// end of the run. Registry is source-agnostic; this is the control knob the
// UI surfaces via "experimental / driver hooks" toggles.
func (r *Registry) DisableCapability(c Capability, off bool) {
	if r == nil {
		return
	}
	r.capOverride[string(c)] = off
}

func (r *Registry) CapabilityEnabled(c Capability) bool {
	if r == nil {
		return true
	}
	return !r.capOverride[string(c)]
}

// Capabilities returns the capability matrix of each registered source,
// honoring the global capability overrides.
func (r *Registry) Capabilities() map[string][]Capability {
	if r == nil {
		return nil
	}
	out := make(map[string][]Capability, len(r.sources))
	for _, s := range r.sources {
		out[s.Name()] = r.sourceCapabilities(s)
	}
	return out
}

func (r *Registry) sourceCapabilities(s AcceleratorSource) []Capability {
	cs, ok := s.(capabilitySource)
	if !ok {
		return nil
	}
	res := make([]Capability, 0, 8)
	for _, c := range cs.Capabilities() {
		if r.capOverride[string(c)] {
			continue
		}
		res = append(res, c)
	}
	return res
}

// Source returns the registered source with the given name, or nil.
func (r *Registry) Source(name string) AcceleratorSource {
	if r == nil {
		return nil
	}
	for _, s := range r.sources {
		if s.Name() == name {
			return s
		}
	}
	return nil
}

func (r *Registry) AddSource(s AcceleratorSource) {
	r.sources = append(r.sources, s)
}

// Sources returns a copy of the registered sources in insertion order.
func (r *Registry) Sources() []AcceleratorSource {
	if r == nil {
		return nil
	}
	out := make([]AcceleratorSource, len(r.sources))
	copy(out, r.sources)
	return out
}

func (r *Registry) Detect() []DeviceInfo {
	r.detected = nil
	var devices []DeviceInfo
	for _, s := range r.sources {
		if r.disabled[s.Name()] {
			continue
		}
		if s.Detect() {
			r.detected = append(r.detected, s)
			devices = append(devices, s.Devices()...)
		}
	}
	return devices
}

func (r *Registry) SampleAll(ctx context.Context) Sample {
	var all Sample
	for _, s := range r.detected {
		if r.disabled[s.Name()] {
			continue
		}
		sample, err := s.Sample(ctx)
		if err != nil {
			continue
		}
		all.Devices = append(all.Devices, sample.Devices...)
		all.Procs = append(all.Procs, sample.Procs...)
	}

	// Compute effective load for all devices (nvtop technique).
	for i := range all.Devices {
		dm := &all.Devices[i]
		powerMax := estimatePowerCap(dm)
		if powerMax > 0 && dm.IsValid(ValidPower) {
			dm.PowerMaxW = powerMax
			dm.Set(ValidPowerMax)
			dm.EffectiveLoad = EffectiveLoad(dm.UtilizationPct, dm.PowerW, powerMax)
			dm.Set(ValidEffectiveLoad)
		}
		// Build JSON-serialized valid fields list for the frontend.
		dm.BuildValidFields()
	}

	// Build valid fields for per-process entries.
	for i := range all.Procs {
		all.Procs[i].BuildValidFields()
	}

	all.Timestamp = time.Now().UnixMilli()
	return all
}

// estimatePowerCap returns a reasonable TDP estimate for the device based on
// vendor/device naming heuristics. Returns 0 when unknown (honest N/A).
func estimatePowerCap(dm *DeviceMetrics) float64 {
	id := dm.DeviceID
	switch {
	// NVIDIA datacenter GPUs
	case strings.HasPrefix(id, "gpu0") && dm.IsValid(ValidPowerMax) && dm.PowerMaxW > 0:
		return dm.PowerMaxW // source already provided it
	case strings.Contains(id, "nvidia") || strings.Contains(strings.ToLower(id), "nvidia"):
		return 300 // conservative TDP estimate for NVIDIA GPUs
	case strings.HasPrefix(id, "gpu0"):
		return 250 // generic NVIDIA
	// AMD
	case strings.HasPrefix(id, "gpu1") || strings.Contains(strings.ToLower(id), "amd"):
		return 300
	// Intel
	case strings.HasPrefix(id, "gpu2") || strings.Contains(strings.ToLower(id), "intel"):
		return 150
	default:
		return 0 // unknown — don't fabricate
	}
}

func (r *Registry) DetectedSources() []AcceleratorSource {
	return r.detected
}
