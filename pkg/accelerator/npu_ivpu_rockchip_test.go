package accelerator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIvpuDetectFromFixture(t *testing.T) {
	src := NewIvpuSource()
	src.fs = newFixtureFS(t, map[string]string{
		"sys/class/accel/accel0/device/driver":                    "ivpu",
		"sys/class/accel/accel0/device/npu_busy_time_us":          "120000000",
		"sys/class/accel/accel0/device/npu_current_frequency_mhz": "900",
		"sys/class/accel/accel0/device/npu_memory_utilization":    "45",
	})
	if !src.Detect() {
		t.Fatal("expected ivpu detect from fixture")
	}
	devs := src.Devices()
	if len(devs) != 1 || devs[0].ID != "npu0" {
		t.Fatalf("unexpected devices: %+v", devs)
	}
	if devs[0].Driver != "ivpu" || devs[0].Vendor != "intel" {
		t.Errorf("unexpected driver/vendor: %+v", devs[0])
	}
}

func TestIvpuNoDriverSkipsNonIvpuAccel(t *testing.T) {
	src := NewIvpuSource()
	// accel1 is amdxdna, not ivpu → must be skipped.
	src.fs = newFixtureFS(t, map[string]string{
		"sys/class/accel/accel0/device/driver": "ivpu",
		"sys/class/accel/accel1/device/driver": "amdxdna",
	})
	if !src.Detect() {
		t.Fatal("expected detect")
	}
	if len(src.Devices()) != 1 || src.Devices()[0].ID != "npu0" {
		t.Fatalf("expected exactly npu0 from ivpu fixture, got %+v", src.Devices())
	}
}

func TestIvpuSampleTwoTickUtil(t *testing.T) {
	src := NewIvpuSource()
	fs := newFixtureFS(t, map[string]string{
		"sys/class/accel/accel0/device/driver":                    "ivpu",
		"sys/class/accel/accel0/device/npu_busy_time_us":          "0",
		"sys/class/accel/accel0/device/npu_current_frequency_mhz": "900",
	})
	src.fs = fs
	if !src.Detect() {
		t.Fatal("detect failed")
	}

	// First tick: baseline, util = 0.
	s1, err := src.Sample(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(s1.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(s1.Devices))
	}
	if s1.Devices[0].UtilizationPct != 0 {
		t.Errorf("expected baseline util 0, got %f", s1.Devices[0].UtilizationPct)
	}
	if s1.Devices[0].ClockMHz != 900 {
		t.Errorf("expected clock 900, got %d", s1.Devices[0].ClockMHz)
	}

	// Second tick: 1 second later (simulated) with +1s of busy time.
	// npu_busy_time_us increments by 1_000_000 over ~1s → ~100%.
	overwriteFixture(t, fs, "sys/class/accel/accel0/device/npu_busy_time_us", "10000000")
	src.tick = time.Now().Add(-10 * time.Second)
	src.prevBusy = map[string]uint64{"npu0": 0}

	s2, err := src.Sample(nil)
	if err != nil {
		t.Fatal(err)
	}
	pct := s2.Devices[0].UtilizationPct
	// 10_000_000 µs / 10 s = 1e6 per s / 1e6 *100 = 100%
	want := 100.0
	if pct < want-1 || pct > want+1 {
		t.Errorf("expected ~100%% util, got %f", pct)
	}
}

func TestRockchipDetectAndSample(t *testing.T) {
	src := NewRockchipSource()
	src.fs = newFixtureFS(t, map[string]string{
		"sys/class/devfreq/fdab0000.npu/cur_freq": "800000000",
		"sys/kernel/debug/rknpu/load":             "35 12 0",
	})
	if !src.Detect() {
		t.Fatal("expected rockchip detect")
	}
	if src.Devices()[0].ID != "npu0" {
		t.Fatalf("unexpected device: %+v", src.Devices())
	}
	s, err := src.Sample(nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Devices[0].ClockMHz != 800 {
		t.Errorf("expected clock 800, got %d", s.Devices[0].ClockMHz)
	}
	if s.Devices[0].UtilizationPct != 35 {
		t.Errorf("expected util 35, got %f", s.Devices[0].UtilizationPct)
	}
}

func TestRockchipNoDebugfsIsNotACrash(t *testing.T) {
	src := NewRockchipSource()
	// devfreq present but debugfs absent (unprivileged) → util stays 0, no error.
	src.fs = newFixtureFS(t, map[string]string{
		"sys/class/devfreq/fdab0000.npu/cur_freq": "800000000",
	})
	if !src.Detect() {
		t.Fatal("expected detect")
	}
	if _, err := src.Sample(nil); err != nil {
		t.Fatalf("sample should succeed without debugfs, got %v", err)
	}
}

func overwriteFixture(t *testing.T, fs *fixtureFS, path, content string) {
	t.Helper()
	first, _ := fs.glob(path)
	if len(first) == 0 {
		t.Fatalf("no fixture file at %s", path)
	}
	if err := os.WriteFile(filepath.Join(fs.root, strings.TrimPrefix(path, "/")), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
