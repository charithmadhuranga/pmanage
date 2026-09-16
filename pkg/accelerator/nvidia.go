package accelerator

import (
	"context"
	"fmt"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// nvmlAPI is the index-based subset of NVML that NvidiaSource needs.
// int indices keep it unit-testable with a fake (nvml.Device is a 280-method
// interface that cannot be constructed outside the go-nvml package).
type nvmlAPI interface {
	Init() nvml.Return
	Shutdown() nvml.Return
	SystemGetDriverVersion() (string, nvml.Return)
	SystemGetProcessName(pid int) (string, nvml.Return)
	DeviceGetCount() (int, nvml.Return)
	DeviceGetName(index int) (string, nvml.Return)
	DeviceGetMinorNumber(index int) (int, nvml.Return)
	DeviceGetCudaComputeCapability(index int) (int, int, nvml.Return)
	DeviceGetUtilizationRates(index int) (nvml.Utilization, nvml.Return)
	DeviceGetMemoryInfo(index int) (nvml.Memory, nvml.Return)
	DeviceGetTemperature(index int, sensor nvml.TemperatureSensors) (uint32, nvml.Return)
	DeviceGetPowerUsage(index int) (uint32, nvml.Return)
	DeviceGetClockInfo(index int, cType nvml.ClockType) (uint32, nvml.Return)
	DeviceGetMaxClockInfo(index int, cType nvml.ClockType) (uint32, nvml.Return)
	DeviceGetFanSpeed(index int) (uint32, nvml.Return)
	DeviceGetComputeRunningProcesses(index int) ([]nvml.ProcessInfo, nvml.Return)
	DeviceGetGraphicsRunningProcesses(index int) ([]nvml.ProcessInfo, nvml.Return)
}

// nvmlAdapter adapts the real go-nvml Interface (handle-based) to the
// index-based nvmlAPI.
type nvmlAdapter struct {
	inner nvml.Interface
}

func (a *nvmlAdapter) Init() nvml.Return { return a.inner.Init() }
func (a *nvmlAdapter) Shutdown() nvml.Return {
	return a.inner.Shutdown()
}
func (a *nvmlAdapter) SystemGetDriverVersion() (string, nvml.Return) {
	return a.inner.SystemGetDriverVersion()
}
func (a *nvmlAdapter) SystemGetProcessName(pid int) (string, nvml.Return) {
	return a.inner.SystemGetProcessName(pid)
}
func (a *nvmlAdapter) DeviceGetCount() (int, nvml.Return) { return a.inner.DeviceGetCount() }

func (a *nvmlAdapter) handle(index int) (nvml.Device, nvml.Return) {
	return a.inner.DeviceGetHandleByIndex(index)
}
func (a *nvmlAdapter) DeviceGetName(index int) (string, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return "", ret
	}
	return a.inner.DeviceGetName(h)
}
func (a *nvmlAdapter) DeviceGetMinorNumber(index int) (int, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return 0, ret
	}
	return a.inner.DeviceGetMinorNumber(h)
}
func (a *nvmlAdapter) DeviceGetCudaComputeCapability(index int) (int, int, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return 0, 0, ret
	}
	return a.inner.DeviceGetCudaComputeCapability(h)
}
func (a *nvmlAdapter) DeviceGetUtilizationRates(index int) (nvml.Utilization, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return nvml.Utilization{}, ret
	}
	return a.inner.DeviceGetUtilizationRates(h)
}
func (a *nvmlAdapter) DeviceGetMemoryInfo(index int) (nvml.Memory, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return nvml.Memory{}, ret
	}
	return a.inner.DeviceGetMemoryInfo(h)
}
func (a *nvmlAdapter) DeviceGetTemperature(index int, sensor nvml.TemperatureSensors) (uint32, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return 0, ret
	}
	return a.inner.DeviceGetTemperature(h, sensor)
}
func (a *nvmlAdapter) DeviceGetPowerUsage(index int) (uint32, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return 0, ret
	}
	return a.inner.DeviceGetPowerUsage(h)
}
func (a *nvmlAdapter) DeviceGetClockInfo(index int, cType nvml.ClockType) (uint32, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return 0, ret
	}
	return a.inner.DeviceGetClockInfo(h, cType)
}
func (a *nvmlAdapter) DeviceGetMaxClockInfo(index int, cType nvml.ClockType) (uint32, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return 0, ret
	}
	return a.inner.DeviceGetMaxClockInfo(h, cType)
}
func (a *nvmlAdapter) DeviceGetFanSpeed(index int) (uint32, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return 0, ret
	}
	return a.inner.DeviceGetFanSpeed(h)
}
func (a *nvmlAdapter) DeviceGetComputeRunningProcesses(index int) ([]nvml.ProcessInfo, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return nil, ret
	}
	return a.inner.DeviceGetComputeRunningProcesses(h)
}
func (a *nvmlAdapter) DeviceGetGraphicsRunningProcesses(index int) ([]nvml.ProcessInfo, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return nil, ret
	}
	return a.inner.DeviceGetGraphicsRunningProcesses(h)
}

// nvmlControlAPI is the optional NVML subset used by the control verbs. It is
// separate from nvmlAPI so unit fakes do not have to implement every verb.
type nvmlControlAPI interface {
	DeviceGetComputeMode(index int) (nvml.ComputeMode, nvml.Return)
	DeviceSetComputeMode(index int, mode nvml.ComputeMode) nvml.Return
	DeviceGetMigMode(index int) (int, int, nvml.Return)
}

func (a *nvmlAdapter) DeviceGetComputeMode(index int) (nvml.ComputeMode, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return nvml.COMPUTEMODE_DEFAULT, ret
	}
	return a.inner.DeviceGetComputeMode(h)
}
func (a *nvmlAdapter) DeviceSetComputeMode(index int, mode nvml.ComputeMode) nvml.Return {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return ret
	}
	return a.inner.DeviceSetComputeMode(h, mode)
}
func (a *nvmlAdapter) DeviceGetMigMode(index int) (int, int, nvml.Return) {
	h, ret := a.handle(index)
	if ret != nvml.SUCCESS {
		return 0, 0, ret
	}
	return a.inner.DeviceGetMigMode(h)
}

type NvidiaSource struct {
	mu     sync.Mutex
	nv     nvmlAPI
	info   []DeviceInfo
	ready  bool
	driver string
}

func NewNvidiaSource() *NvidiaSource {
	return &NvidiaSource{nv: &nvmlAdapter{inner: nvml.New()}}
}

func (n *NvidiaSource) Name() string { return "nvidia" }
func (n *NvidiaSource) Kind() Kind   { return GPU }

// Capabilities advertises what nvml can observe/control (subject to the
// global registry overrides). Per-process utilization needs "compute mode =
// Exclusive"/accounting to be reliable; the source reports honest N/A then.
func (n *NvidiaSource) Capabilities() []Capability {
	return []Capability{
		CapPerProcessGPU, CapPerProcessVRAM, CapPerProcessEngine,
		CapPower, CapTemperature, CapClocks, CapFans,
		CapComputeMode, CapMPS, CapMIG, CapReset,
	}
}

func (n *NvidiaSource) Detect() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ready {
		return true
	}

	if ret := n.nv.Init(); ret != nvml.SUCCESS {
		return false
	}

	driver, _ := n.nv.SystemGetDriverVersion()
	n.driver = driver

	count, ret := n.nv.DeviceGetCount()
	if ret != nvml.SUCCESS || count == 0 {
		n.nv.Shutdown()
		return false
	}

	for i := 0; i < count; i++ {
		name, _ := n.nv.DeviceGetName(i)
		minor, _ := n.nv.DeviceGetMinorNumber(i)
		major, minorCC, _ := n.nv.DeviceGetCudaComputeCapability(i)

		n.info = append(n.info, DeviceInfo{
			ID:      fmt.Sprintf("gpu%d", i),
			Name:    name,
			Vendor:  "nvidia",
			Kind:    GPU,
			Driver:  fmt.Sprintf("NVIDIA %s", driver),
			Version: fmt.Sprintf("CUDA %d.%d · minor %d", major, minorCC, minor),
		})
	}
	n.ready = len(n.info) > 0
	if !n.ready {
		n.nv.Shutdown()
	}
	return n.ready
}

func (n *NvidiaSource) Devices() []DeviceInfo {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]DeviceInfo, len(n.info))
	copy(out, n.info)
	return out
}

func (n *NvidiaSource) Sample(ctx context.Context) (Sample, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.ready {
		return Sample{}, fmt.Errorf("nvidia source not ready")
	}

	var sample Sample
	for i, d := range n.info {
		dm := DeviceMetrics{DeviceID: d.ID, Status: "ok"}

		if u, ret := n.nv.DeviceGetUtilizationRates(i); ret == nvml.SUCCESS {
			dm.UtilizationPct = float64(u.Gpu)
			dm.Set(ValidUtilization)
		}
		if mem, ret := n.nv.DeviceGetMemoryInfo(i); ret == nvml.SUCCESS {
			dm.VRAMUsed = mem.Used
			dm.VRAMTotal = mem.Total
			dm.Set(ValidVRAMUsed)
			dm.Set(ValidVRAMTotal)
			if mem.Total > 0 {
				dm.MemUtilRate = float64(mem.Used) * 100.0 / float64(mem.Total)
				dm.Set(ValidMemUtilRate)
			}
		}
		if t, ret := n.nv.DeviceGetTemperature(i, nvml.TEMPERATURE_GPU); ret == nvml.SUCCESS {
			dm.TemperatureC = float64(t)
			dm.Set(ValidTemperature)
		}
		if p, ret := n.nv.DeviceGetPowerUsage(i); ret == nvml.SUCCESS {
			dm.PowerW = float64(p) / 1000.0
			dm.Set(ValidPower)
		}
		if c, ret := n.nv.DeviceGetClockInfo(i, nvml.CLOCK_GRAPHICS); ret == nvml.SUCCESS {
			dm.ClockMHz = c
			dm.Set(ValidClock)
		}
		if c, ret := n.nv.DeviceGetMaxClockInfo(i, nvml.CLOCK_GRAPHICS); ret == nvml.SUCCESS {
			dm.ClockMaxMHz = c
			dm.Set(ValidClockMax)
		}
		if f, ret := n.nv.DeviceGetFanSpeed(i); ret == nvml.SUCCESS {
			dm.FanSpeedPct = float64(f)
			dm.Set(ValidFanSpeed)
		}

		// EffectiveLoad computed in SampleAll via registry (power cap heuristic).

		sample.Devices = append(sample.Devices, dm)

		if procs, ret := n.nv.DeviceGetComputeRunningProcesses(i); ret == nvml.SUCCESS {
			for _, p := range procs {
				pu := ProcUsage{
					PID:      int32(p.Pid),
					Name:     n.resolveName(int(p.Pid)),
					DeviceID: d.ID,
					VRAMUsed: p.UsedGpuMemory,
				}
				pu.Set(ValidVRAMUsed)
				sample.Procs = append(sample.Procs, pu)
			}
		}
		if procs, ret := n.nv.DeviceGetGraphicsRunningProcesses(i); ret == nvml.SUCCESS {
			for _, p := range procs {
				pu := ProcUsage{
					PID:      int32(p.Pid),
					Name:     n.resolveName(int(p.Pid)),
					DeviceID: d.ID,
					VRAMUsed: p.UsedGpuMemory,
				}
				pu.Set(ValidVRAMUsed)
				sample.Procs = append(sample.Procs, pu)
			}
		}
	}
	return sample, nil
}

func (n *NvidiaSource) resolveName(pid int) string {
	if name, ret := n.nv.SystemGetProcessName(pid); ret == nvml.SUCCESS && name != "" {
		return name
	}
	return fmt.Sprintf("pid:%d", pid)
}

func (n *NvidiaSource) Shutdown() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ready {
		n.nv.Shutdown()
		n.ready = false
	}
}
