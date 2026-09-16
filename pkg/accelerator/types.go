package accelerator

import "context"

// clamp01 clamps a percentage-like value into [0, 100]. Negative/nonsense
// readings (e.g. counter resets, N/A sentinels) collapse to a safe floor.
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

type Kind string

const (
	GPU Kind = "gpu"
	NPU Kind = "npu"
	TPU Kind = "tpu"
)

type AcceleratorSource interface {
	Name() string
	Kind() Kind
	Detect() bool
	Devices() []DeviceInfo
	Sample(ctx context.Context) (Sample, error)
}

// Capability is a named capability a source can offer. Sources that do not
// advertise a capability are treated as fully N/A for it by the registry.
type Capability string

const (
	CapPerProcessGPU    Capability = "per-process gpu"
	CapPerProcessVRAM   Capability = "per-process vram"
	CapPower            Capability = "power"
	CapTemperature      Capability = "temperature"
	CapClocks           Capability = "clocks"
	CapFans             Capability = "fans"
	CapComputeMode      Capability = "compute mode"
	CapMPS              Capability = "MPS"
	CapMIG              Capability = "MIG"
	CapReset            Capability = "reset"
	CapPerProcessEngine Capability = "per-process engines"
)

// capabilitySource is implemented by sources that advertise what they can do.
// Kept optional so new sources compile without declaring a matrix.
type capabilitySource interface {
	AcceleratorSource
	Capabilities() []Capability
}

type DeviceInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Vendor  string `json:"vendor"`
	Kind    Kind   `json:"kind"`
	Driver  string `json:"driver"`
	Version string `json:"version,omitempty"`
	BusInfo string `json:"busInfo,omitempty"`
	// MinKernel is the kernel release required for full per-process metric
	// support (Linux sources); empty when no gate applies. Surfaced in UI
	// tooltips so a too-old kernel explains honest N/A cells.
	MinKernel string `json:"minKernel,omitempty"`
}

type DeviceMetrics struct {
	DeviceID       string  `json:"deviceId"`
	UtilizationPct float64 `json:"utilPct"`
	VRAMUsed       uint64  `json:"vramUsed"`
	VRAMTotal      uint64  `json:"vramTotal"`
	TemperatureC   float64 `json:"tempC"`
	PowerW         float64 `json:"powerW"`
	ClockMHz       uint32  `json:"clockMhz"`
	ClockMaxMHz    uint32  `json:"clockMaxMhz"`
	FanSpeedPct    float64 `json:"fanPct"`
	Status         string  `json:"status"`
	// MinKernel mirrors DeviceInfo.MinKernel on samples so the UI can explain a
	// too-old kernel next to honest N/A cells (Linux drm fdinfo gates).
	MinKernel string `json:"minKernel,omitempty"`
}

type ProcUsage struct {
	PID            int32              `json:"pid"`
	Name           string             `json:"name"`
	DeviceID       string             `json:"deviceId"`
	VRAMUsed       uint64             `json:"vramUsed"`
	UtilizationPct float64            `json:"utilPct"`
	Engine         map[string]float64 `json:"engine,omitempty"`
}

type Sample struct {
	Devices   []DeviceMetrics `json:"devices"`
	Procs     []ProcUsage     `json:"procs"`
	Timestamp int64           `json:"ts"`
}
