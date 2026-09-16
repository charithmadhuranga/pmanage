//go:build windows

package accelerator

// PlatformSources returns platform-specific sources. On Windows, NVIDIA NVML
// is primary and the Intel path is scaffolded behind a build tag for Phase 7
// (Level Zero Sysman, ZES_ENABLE_SYSMAN=1). See RESEARCH §4.3.
func PlatformSources() []AcceleratorSource {
	var out []AcceleratorSource
	if s := newWindowsIntelSource(); s.Detect() {
		out = append(out, s)
	}
	return out
}
