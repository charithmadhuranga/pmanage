//go:build windows

package accelerator

import (
	"context"
	"fmt"
	"runtime"
)

// WindowsIntelSource is a Phase 7 scaffold: Level Zero Sysman device discovery
// behind a build tag. It currently reports zero devices so the platform
// bootstrap stays green until the CGo zes wrapper lands (RESEARCH §4.3).
type WindowsIntelSource struct {
	ready bool
	info  []DeviceInfo
}

func newWindowsIntelSource() *WindowsIntelSource {
	return &WindowsIntelSource{}
}

func (s *WindowsIntelSource) Name() string { return "intel-windows" }
func (s *WindowsIntelSource) Kind() Kind   { return GPU }
func (s *WindowsIntelSource) Detect() bool {
	if s.ready {
		return true
	}
	// Scaffold: Level Zero Sysman requires ZES_ENABLE_SYSMAN=1 and the zes_api
	// cgo shim (not yet wired). Missing driver → honest no-device.
	s.ready = false
	_ = runtime.GOOS
	return false
}
func (s *WindowsIntelSource) Devices() []DeviceInfo { return s.info }
func (s *WindowsIntelSource) Sample(_ context.Context) (Sample, error) {
	if !s.ready {
		return Sample{}, fmt.Errorf("intel-windows scaffold not ready")
	}
	return Sample{}, nil
}
