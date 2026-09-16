//go:build !linux && !darwin

package accelerator

// PlatformSources is empty on platforms without platform-specific accelerators.
func PlatformSources() []AcceleratorSource {
	return nil
}
