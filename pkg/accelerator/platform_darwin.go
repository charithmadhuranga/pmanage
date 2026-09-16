//go:build darwin

package accelerator

// PlatformSources returns the Apple Silicon accelerator sources on darwin.
// The powermetrics source is opt-in (PMANAGE_POWERMETRICS=1) and returns nil
// when disabled.
func PlatformSources() []AcceleratorSource {
	sources := []AcceleratorSource{NewAppleSource()}
	if pm := NewPowermetricsSource(); pm != nil {
		sources = append(sources, pm)
	}
	return sources
}
