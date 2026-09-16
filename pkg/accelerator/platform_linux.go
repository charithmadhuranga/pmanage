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
