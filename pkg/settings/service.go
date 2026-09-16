package settings

import (
	"pmanage/pkg/accelerator"
)

// SourceInfo is the per-source row the Sources view renders.
type SourceInfo struct {
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	Enabled        bool     `json:"enabled"`
	Detected       bool     `json:"detected"`
	DeviceCount    int      `json:"deviceCount"`
	Capabilities   []string `json:"capabilities"`
	DriverVersions []string `json:"driverVersions"`
}

// Service exposes the accelerator registry's source toggles and capability
// matrix to the frontend. It is re-registered as a Wails service in main.go.
type Service struct {
	registry *accelerator.Registry
}

func NewService(registry *accelerator.Registry) *Service {
	return &Service{registry: registry}
}

func (s *Service) ListSources() []SourceInfo {
	if s == nil || s.registry == nil {
		return nil
	}
	matrix := s.registry.Capabilities()
	detected := map[string]bool{}
	for _, src := range s.registry.DetectedSources() {
		detected[src.Name()] = true
	}
	var out []SourceInfo
	for _, src := range s.registry.Sources() {
		devices := src.Devices()
		caps := matrix[src.Name()]
		cs := make([]string, 0, len(caps))
		for _, c := range caps {
			cs = append(cs, string(c))
		}
		out = append(out, SourceInfo{
			Name:         src.Name(),
			Kind:         string(src.Kind()),
			Enabled:      s.registry.IsEnabled(src.Name()),
			Detected:     detected[src.Name()],
			DeviceCount:  len(devices),
			Capabilities: cs,
		})
	}
	return out
}

// SetSourceEnabled toggles a source and re-runs detection so DetectedSources /
// SampleAll reflect the change immediately.
func (s *Service) SetSourceEnabled(name string, enabled bool) error {
	if s == nil || s.registry == nil {
		return nil
	}
	s.registry.Enable(name, enabled)
	s.registry.Detect()
	return nil
}

// SetCapability disabled/enables a capability globally across sources.
func (s *Service) SetCapability(capability string, enabled bool) error {
	if s == nil || s.registry == nil {
		return nil
	}
	s.registry.DisableCapability(accelerator.Capability(capability), !enabled)
	return nil
}
