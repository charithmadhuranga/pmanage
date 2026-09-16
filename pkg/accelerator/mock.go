package accelerator

import (
	"context"
	"math"
	"sync"
	"time"
)

type mockSource struct {
	label   string
	tick    int
	mu      sync.Mutex
	devices []DeviceInfo
}

func NewMockSource() AcceleratorSource {
	return &mockSource{
		devices: []DeviceInfo{
			{ID: "gpu0", Name: "NVIDIA GeForce RTX 4090", Vendor: "nvidia", Kind: GPU, Driver: "nvidia-smi 560.35.03"},
			{ID: "gpu1", Name: "AMD Radeon RX 7900 XTX", Vendor: "amd", Kind: GPU, Driver: "amdgpu 6.7.0"},
			{ID: "gpu2", Name: "Intel UHD Graphics 770", Vendor: "intel", Kind: GPU, Driver: "i915 1.28"},
			{ID: "tpu0", Name: "Google Coral TPU", Vendor: "google", Kind: TPU, Driver: "libedgetpu 1.0 (archived)"},
		},
	}
}

// NewMockSourceNamed returns a mock source with a distinct registered name so
// tests can verify registry enable/disable toggles per source.
func NewMockSourceNamed(name string) AcceleratorSource {
	return &mockSource{
		label: name,
		devices: []DeviceInfo{
			{ID: "gpu10", Name: "Mock GPU B", Vendor: "mock", Kind: GPU, Driver: "mock"},
			{ID: "gpu11", Name: "Mock GPU C", Vendor: "mock", Kind: GPU, Driver: "mock"},
		},
	}
}

func (m *mockSource) Name() string {
	if m.label == "" {
		return "mock"
	}
	return m.label
}
func (m *mockSource) Kind() Kind            { return GPU }
func (m *mockSource) Detect() bool          { return true }
func (m *mockSource) Devices() []DeviceInfo { return m.devices }

func (m *mockSource) Sample(_ context.Context) (Sample, error) {
	m.mu.Lock()
	m.tick++
	t := float64(m.tick)
	m.mu.Unlock()

	devs := make([]DeviceMetrics, len(m.devices))
	for i, d := range m.devices {
		switch d.ID {
		case "gpu0":
			devs[i] = mockGPU0(t)
		case "gpu1":
			devs[i] = mockGPU1(t)
		case "gpu2":
			devs[i] = mockGPU2(t)
		case "tpu0":
			devs[i] = mockTPU0()
		default:
			devs[i] = DeviceMetrics{
				DeviceID: d.ID, UtilizationPct: clamp(30+10*math.Sin(t*0.1), 0, 100),
				VRAMTotal: 8_000_000_000, VRAMUsed: 1_000_000_000,
				TemperatureC: 45, PowerW: 60, ClockMHz: 900, Status: "ok",
			}
		}
	}

	return Sample{
		Timestamp: time.Now().UnixMilli(),
		Devices:   devs,
		Procs:     mockProcs(t),
	}, nil
}

func mockGPU0(t float64) DeviceMetrics {
	util := 55 + 30*math.Sin(t*0.07) + 8*math.Sin(t*0.31)
	vram := uint64(12_000_000_000 + 4_000_000_000*math.Sin(t*0.04))
	temp := 58 + 15*math.Sin(t*0.09)
	power := 200 + 100*math.Sin(t*0.06)
	clock := uint32(2100 + 300*math.Sin(t*0.05))
	return DeviceMetrics{
		DeviceID: "gpu0", UtilizationPct: clamp(util, 0, 100),
		VRAMUsed: vram, VRAMTotal: 24_000_000_000,
		TemperatureC: clamp(temp, 30, 95), PowerW: clamp(power, 50, 450),
		ClockMHz: clock, ClockMaxMHz: 2520, FanSpeedPct: clamp(40+25*math.Sin(t*0.08), 0, 100),
		Status: statusFromTemp(temp),
	}
}

func mockGPU1(t float64) DeviceMetrics {
	util := 35 + 25*math.Sin(t*0.09+1.2) + 10*math.Sin(t*0.37)
	vram := uint64(6_000_000_000 + 3_000_000_000*math.Sin(t*0.05))
	temp := 48 + 12*math.Sin(t*0.11)
	power := 120 + 80*math.Sin(t*0.07)
	clock := uint32(1800 + 200*math.Sin(t*0.06))
	return DeviceMetrics{
		DeviceID: "gpu1", UtilizationPct: clamp(util, 0, 100),
		VRAMUsed: vram, VRAMTotal: 24_000_000_000,
		TemperatureC: clamp(temp, 30, 95), PowerW: clamp(power, 30, 350),
		ClockMHz: clock, ClockMaxMHz: 2400, FanSpeedPct: clamp(30+20*math.Sin(t*0.10), 0, 100),
		Status: statusFromTemp(temp),
	}
}

func mockGPU2(t float64) DeviceMetrics {
	util := 12 + 10*math.Sin(t*0.13+2.4)
	temp := 42 + 6*math.Sin(t*0.15)
	power := 25 + 15*math.Sin(t*0.10)
	return DeviceMetrics{
		DeviceID: "gpu2", UtilizationPct: clamp(util, 0, 100),
		VRAMUsed: 0, VRAMTotal: 0,
		TemperatureC: clamp(temp, 30, 95), PowerW: clamp(power, 5, 65),
		ClockMHz: 0, ClockMaxMHz: 0, FanSpeedPct: 0,
		Status: statusFromTemp(temp),
	}
}

func mockTPU0() DeviceMetrics {
	return DeviceMetrics{DeviceID: "tpu0", Status: "n/a"}
}

func mockProcs(t float64) []ProcUsage {
	util1 := 80 + 15*math.Sin(t*0.12)
	util2 := 40 + 20*math.Sin(t*0.18+1.5)
	util3 := 8 + 5*math.Sin(t*0.25)
	return []ProcUsage{
		{PID: 4210, Name: "python3", DeviceID: "gpu0", VRAMUsed: 15_200_000_000, UtilizationPct: clamp(util1, 0, 100),
			Engine: map[string]float64{"sm": clamp(util1, 0, 100), "enc": clamp(util1*0.1, 0, 100), "dec": 0}},
		{PID: 9183, Name: "ffmpeg", DeviceID: "gpu1", VRAMUsed: 1_050_000_000, UtilizationPct: clamp(util2, 0, 100),
			Engine: map[string]float64{"render": clamp(util2, 0, 100), "copy": 2.1}},
		{PID: 10512, Name: "chrome", DeviceID: "gpu2", VRAMUsed: 340_000_000, UtilizationPct: clamp(util3, 0, 100)},
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func statusFromTemp(temp float64) string {
	switch {
	case temp >= 92:
		return "critical"
	case temp >= 80:
		return "busy"
	default:
		return "ok"
	}
}
