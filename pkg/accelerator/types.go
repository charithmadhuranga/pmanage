package accelerator

import (
	"context"
	"sync"
	"time"
)

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

// ============================================================================
// Validity bitmask system (inspired by nvtop's extract_gpuinfo_common.h)
//
// Each metric field has a corresponding valid bit. Sources only set fields
// they can actually provide. The frontend checks IsValid() before rendering.
// This eliminates ambiguity between "value is 0" and "value is not available".
// ============================================================================

type ValidField uint32

const (
	ValidUtilization ValidField = iota
	ValidVRAMUsed
	ValidVRAMTotal
	ValidTemperature
	ValidPower
	ValidClock
	ValidClockMax
	ValidFanSpeed
	ValidMemUtilRate
	ValidEncoderRate
	ValidDecoderRate
	ValidEffectiveLoad
	ValidPowerMax
	ValidECCCorrected
	ValidECCUncorrected
	ValidPCIeGen
	ValidPCIeWidth
	ValidPCIeRXTX
	ValidCpuUsage
	ValidMemResident
	ValidMemVirtual
	ValidTotal
)

type ValidMask [ValidTotal]uint32

func (v *ValidMask) Set(f ValidField) {
	v[f/32] |= 1 << (f % 32)
}

func (v *ValidMask) Reset(f ValidField) {
	v[f/32] &^= 1 << (f % 32)
}

func (v *ValidMask) IsValid(f ValidField) bool {
	return v[f/32]&(1<<(f%32)) != 0
}

func (v *ValidMask) Clear() {
	*v = ValidMask{}
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
	MinKernel    string  `json:"minKernel,omitempty"`
	MemUtilRate  float64 `json:"memUtilRate,omitempty"`
	EncoderRate  float64 `json:"encoderRate,omitempty"`
	DecoderRate  float64 `json:"decoderRate,omitempty"`
	PowerMaxW    float64 `json:"powerMaxW,omitempty"`
	EffectiveLoad float64 `json:"effectiveLoad,omitempty"`

	// Validity bitmask — which fields are actually populated by the source.
	Valid ValidMask `json:"-"`
}

// IsValid returns true if the given field was populated by the source.
func (m *DeviceMetrics) IsValid(f ValidField) bool {
	return m.Valid.IsValid(f)
}

// Set marks a field as valid in the bitmask.
func (m *DeviceMetrics) Set(f ValidField) {
	m.Valid.Set(f)
}

// ProcUsage represents per-process accelerator usage.
type ProcUsage struct {
	PID            int32              `json:"pid"`
	Name           string             `json:"name"`
	Username       string             `json:"username,omitempty"`
	DeviceID       string             `json:"deviceId"`
	VRAMUsed       uint64             `json:"vramUsed"`
	UtilizationPct float64            `json:"utilPct"`
	Engine         map[string]float64 `json:"engine,omitempty"`
	CpuUsage       float64            `json:"cpuUsage,omitempty"`
	MemResident    uint64             `json:"memResident,omitempty"`
	MemVirtual     uint64             `json:"memVirtual,omitempty"`
	Type           string             `json:"type,omitempty"` // "graphical", "compute", "graphical+compute"

	// Validity bitmask for per-process fields.
	Valid ValidMask `json:"-"`
}

func (p *ProcUsage) IsValid(f ValidField) bool {
	return p.Valid.IsValid(f)
}

func (p *ProcUsage) Set(f ValidField) {
	p.Valid.Set(f)
}

type Sample struct {
	Devices   []DeviceMetrics `json:"devices"`
	Procs     []ProcUsage     `json:"procs"`
	Timestamp int64           `json:"ts"`
}

// ============================================================================
// Process Cache (inspired nvtop's two-bucket hash table pattern)
//
// Maintains a cache of process metadata (name, username, CPU stats) between
// telemetry ticks. This avoids re-resolving process names every second and
// enables correct lifecycle tracking (new/dead processes).
// ============================================================================

type ProcessCacheEntry struct {
	PID                 int32
	Name                string
	Username            string
	LastTotalCPUTime    float64
	LastMeasurementTime time.Time
}

type ProcessCache struct {
	mu      sync.Mutex
	cached  map[int32]*ProcessCacheEntry // previous tick
	updated map[int32]*ProcessCacheEntry // current tick
}

func NewProcessCache() *ProcessCache {
	return &ProcessCache{
		cached:  make(map[int32]*ProcessCacheEntry),
		updated: make(map[int32]*ProcessCacheEntry),
	}
}

// Get returns the cached entry for a PID, or nil if not seen before.
func (pc *ProcessCache) Get(pid int32) *ProcessCacheEntry {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if e, ok := pc.cached[pid]; ok {
		delete(pc.cached, pid)
		pc.updated[pid] = e
		return e
	}
	if e, ok := pc.updated[pid]; ok {
		return e
	}
	return nil
}

// Put adds or updates an entry in the current tick's cache.
func (pc *ProcessCache) Put(pid int32, entry *ProcessCacheEntry) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.updated[pid] = entry
}

// Swap advances to the next tick. Entries remaining in cached are dead processes.
func (pc *ProcessCache) Swap() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	// Clear old cached entries (dead processes)
	pc.cached = pc.updated
	pc.updated = make(map[int32]*ProcessCacheEntry)
}

// Clear removes all cached entries.
func (pc *ProcessCache) Clear() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.cached = make(map[int32]*ProcessCacheEntry)
	pc.updated = make(map[int32]*ProcessCacheEntry)
}

// EffectiveLoad computes a combined metric: gpu_util * (power / power_max).
// Returns 0 if insufficient data.
func EffectiveLoad(utilPct, powerW, powerMaxW float64) float64 {
	if powerMaxW <= 0 || utilPct < 0 {
		return 0
	}
	load := utilPct * (powerW / powerMaxW)
	if load > 100 {
		load = 100
	}
	if load < 0 {
		load = 0
	}
	return load
}
