//go:build linux

package accelerator

// PlatformSources returns sources available on this Linux host, in detection
// priority order. The DRM-fdinfo sources claim /sys/class/accel entries by
// driver (amdxdna vs ivpu) so shared sysfs space is never double-claimed;
// sysfs/devfreq/CLI NPU sources are portable and registered here too.
func PlatformSources() []AcceleratorSource {
	return []AcceleratorSource{
		NewAmdgpuSource(),
		NewIntelSource(),
		NewXdnaSource(),
		NewIvpuSource(),
		NewRockchipSource(),
		NewJetsonSource(),
		NewHailoSource(),
	}
}

// OptionalSources returns detect-gated optional accelerators that may or may
// not be present on a given Linux host. Each is registered only when Detect()
// succeeds, avoiding noisy empty sources on machines without the hardware.
func OptionalSources() []AcceleratorSource {
	return []AcceleratorSource{
		NewAdrenoSource(),
		NewAscendSource(),
		NewVideoCoreSource(),
		NewTenstorrentSource(),
	}
}
