//go:build !linux && !darwin && !windows

package accelerator

// PlatformSources is empty on platforms without platform-specific accelerators.
func PlatformSources() []AcceleratorSource {
	return nil
}

// OptionalSources returns detect-gated optional accelerators on other platforms.
func OptionalSources() []AcceleratorSource {
	return nil
}
