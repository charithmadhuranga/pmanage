package accelerator

import (
	"context"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// fakeNvml implements nvmlAPI with canned data for N identical GPUs.
type fakeNvml struct {
	initFail bool
	count    int
	compute  []nvml.ProcessInfo
	graphics []nvml.ProcessInfo
}

func (f *fakeNvml) Init() nvml.Return {
	if f.initFail {
		return nvml.ERROR_LIBRARY_NOT_FOUND
	}
	return nvml.SUCCESS
}
func (f *fakeNvml) Shutdown() nvml.Return { return nvml.SUCCESS }
func (f *fakeNvml) SystemGetDriverVersion() (string, nvml.Return) {
	return "570.124.06", nvml.SUCCESS
}
func (f *fakeNvml) SystemGetProcessName(pid int) (string, nvml.Return) {
	switch pid {
	case 42:
		return "python", nvml.SUCCESS
	case 43:
		return "nvidia-smi", nvml.SUCCESS
	}
	return "", nvml.ERROR_NOT_FOUND
}
func (f *fakeNvml) DeviceGetCount() (int, nvml.Return) { return f.count, nvml.SUCCESS }
func (f *fakeNvml) DeviceGetName(int) (string, nvml.Return) {
	return "NVIDIA GeForce RTX 4090", nvml.SUCCESS
}
func (f *fakeNvml) DeviceGetMinorNumber(int) (int, nvml.Return) { return 0, nvml.SUCCESS }
func (f *fakeNvml) DeviceGetCudaComputeCapability(int) (int, int, nvml.Return) {
	return 8, 9, nvml.SUCCESS
}
func (f *fakeNvml) DeviceGetUtilizationRates(int) (nvml.Utilization, nvml.Return) {
	return nvml.Utilization{Gpu: 63, Memory: 41}, nvml.SUCCESS
}
func (f *fakeNvml) DeviceGetMemoryInfo(int) (nvml.Memory, nvml.Return) {
	return nvml.Memory{Total: 24 * 1024 * 1024 * 1024, Used: 6 * 1024 * 1024 * 1024}, nvml.SUCCESS
}
func (f *fakeNvml) DeviceGetTemperature(int, nvml.TemperatureSensors) (uint32, nvml.Return) {
	return 62, nvml.SUCCESS
}
func (f *fakeNvml) DeviceGetPowerUsage(int) (uint32, nvml.Return) { return 245000, nvml.SUCCESS }
func (f *fakeNvml) DeviceGetClockInfo(int, nvml.ClockType) (uint32, nvml.Return) {
	return 1845, nvml.SUCCESS
}
func (f *fakeNvml) DeviceGetMaxClockInfo(int, nvml.ClockType) (uint32, nvml.Return) {
	return 2520, nvml.SUCCESS
}
func (f *fakeNvml) DeviceGetFanSpeed(int) (uint32, nvml.Return) { return 35, nvml.SUCCESS }
func (f *fakeNvml) DeviceGetComputeRunningProcesses(int) ([]nvml.ProcessInfo, nvml.Return) {
	return f.compute, nvml.SUCCESS
}
func (f *fakeNvml) DeviceGetGraphicsRunningProcesses(int) ([]nvml.ProcessInfo, nvml.Return) {
	return f.graphics, nvml.SUCCESS
}

func TestNvidiaSource_DetectAndSample(t *testing.T) {
	fake := &fakeNvml{count: 1,
		compute:  []nvml.ProcessInfo{{Pid: 42, UsedGpuMemory: 3 * 1024 * 1024 * 1024}},
		graphics: []nvml.ProcessInfo{{Pid: 43, UsedGpuMemory: 512 * 1024 * 1024}},
	}
	src := &NvidiaSource{nv: fake}

	if !src.Detect() {
		t.Fatal("detect should succeed with fake NVML")
	}
	devs := src.Devices()
	if len(devs) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devs))
	}
	d := devs[0]
	if d.ID != "gpu0" || d.Vendor != "nvidia" || d.Kind != GPU {
		t.Fatalf("unexpected device: %+v", d)
	}
	if d.Name != "NVIDIA GeForce RTX 4090" || d.Driver == "" || d.Version == "" {
		t.Fatalf("unexpected name/driver/version: %+v", d)
	}

	sample, err := src.Sample(context.Background())
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	if len(sample.Devices) != 1 {
		t.Fatalf("expected 1 device sample")
	}
	dm := sample.Devices[0]
	if dm.UtilizationPct != 63 || dm.PowerW != 245 || dm.FanSpeedPct != 35 || dm.TemperatureC != 62 {
		t.Fatalf("unexpected metrics: %+v", dm)
	}
	if dm.ClockMHz != 1845 || dm.ClockMaxMHz != 2520 {
		t.Fatalf("unexpected clocks: %+v", dm)
	}
	if dm.VRAMTotal != 24*1024*1024*1024 || dm.VRAMUsed != 6*1024*1024*1024 {
		t.Fatalf("unexpected vram: %+v", dm)
	}

	if len(sample.Procs) != 2 {
		t.Fatalf("expected 2 procs, got %d", len(sample.Procs))
	}
	if sample.Procs[0].Name != "python" || sample.Procs[0].DeviceID != "gpu0" {
		t.Fatalf("unexpected proc: %+v", sample.Procs[0])
	}
	if sample.Procs[0].VRAMUsed != 3*1024*1024*1024 {
		t.Fatalf("unexpected proc vram: %+v", sample.Procs[0])
	}
	if sample.Procs[1].Name != "nvidia-smi" {
		t.Fatalf("unexpected second proc: %+v", sample.Procs[1])
	}

	src.Shutdown()
}

func TestNvidiaSource_InitFailure(t *testing.T) {
	src := &NvidiaSource{nv: &fakeNvml{initFail: true}}
	if src.Detect() {
		t.Fatal("detect should fail when NVML init fails")
	}
	if len(src.Devices()) != 0 {
		t.Fatal("no devices expected")
	}
	if _, err := src.Sample(context.Background()); err == nil {
		t.Fatal("sample should error when not ready")
	}
}

func TestNvidiaSource_MultiGpuIndices(t *testing.T) {
	src := &NvidiaSource{nv: &fakeNvml{count: 2}}
	if !src.Detect() {
		t.Fatal("detect should succeed")
	}
	if len(src.Devices()) != 2 {
		t.Fatalf("expected 2 devices")
	}
	if src.Devices()[1].ID != "gpu1" {
		t.Fatalf("expected gpu1, got %+v", src.Devices()[1])
	}
}

func TestNvidiaSource_ResolveNameFallback(t *testing.T) {
	fake := &fakeNvml{count: 1, compute: []nvml.ProcessInfo{{Pid: 9999, UsedGpuMemory: 1}}}
	src := &NvidiaSource{nv: fake}
	src.Detect()
	sample, err := src.Sample(context.Background())
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	if len(sample.Procs) != 1 {
		t.Fatalf("expected 1 proc")
	}
	if sample.Procs[0].Name != "pid:9999" {
		t.Fatalf("expected fallback name, got %q", sample.Procs[0].Name)
	}
}
