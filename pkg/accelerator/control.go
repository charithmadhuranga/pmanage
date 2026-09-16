package accelerator

import (
	"errors"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// ControlProvider is implemented by sources that support mutating control
// verbs (compute mode, MIG inspection). Sources that cannot (everything but
// NVML today) simply do not implement it; the ControlService reports a clear
// "capability unavailable" error instead of guessing.
type ControlProvider interface {
	AcceleratorSource
	SetComputeMode(deviceID string, mode string) error
	GetComputeMode(deviceID string) (string, error)
	GetMigMode(deviceID string) (string, error)
}

// NvidiaControl leases NVML control verbs to whichever source owns the device.
// It is capability-gated: toggling a capability off via the registry blocks
// the verb corpus-wide (used by the experimental-driver-hooks toggle).
type NvidiaControl struct {
	registry *Registry
}

func NewControl(registry *Registry) *NvidiaControl {
	return &NvidiaControl{registry: registry}
}

// computeModes maps the canonical names exposed to the UI.
var computeModeNames = map[string]nvml.ComputeMode{
	"default":          nvml.COMPUTEMODE_DEFAULT,
	"exclusive":        nvml.COMPUTEMODE_EXCLUSIVE_PROCESS,
	"prohibited":       nvml.COMPUTEMODE_PROHIBITED,
	"exclusive-thread": nvml.COMPUTEMODE_EXCLUSIVE_THREAD,
}

func (c *NvidiaControl) provider(source string) (ControlProvider, error) {
	cp, ok := c.registry.Source(source).(ControlProvider)
	if !ok {
		return nil, fmt.Errorf("source %q does not support control verbs", source)
	}
	return cp, nil
}

func (c *NvidiaControl) SetComputeMode(source, deviceID, mode string) error {
	if !c.registry.CapabilityEnabled(CapComputeMode) {
		return errors.New("compute-mode control is disabled (experimental hooks off)")
	}
	p, err := c.provider(source)
	if err != nil {
		return err
	}
	return p.SetComputeMode(deviceID, mode)
}

// SetComputeModeVerbose applies a compute-mode change and returns the new mode.
func (c *NvidiaControl) SetComputeModeVerbose(source, deviceID, mode string) (string, error) {
	if err := c.SetComputeMode(source, deviceID, mode); err != nil {
		return "", err
	}
	p, err := c.provider(source)
	if err != nil {
		return "", err
	}
	return p.GetComputeMode(deviceID)
}

func (n *NvidiaSource) ctlIndex(deviceID string) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i, d := range n.info {
		if d.ID == deviceID {
			return i, nil
		}
	}
	return 0, fmt.Errorf("device %q not found", deviceID)
}

func (n *NvidiaSource) SetComputeMode(deviceID, mode string) error {
	ctl, ok := n.nv.(nvmlControlAPI)
	if !ok {
		return errors.New("nvml does not implement compute-mode control")
	}
	idx, err := n.ctlIndex(deviceID)
	if err != nil {
		return err
	}
	target, ok := computeModeNames[mode]
	if !ok {
		return fmt.Errorf("unknown compute mode %q", mode)
	}
	if r := ctl.DeviceSetComputeMode(idx, target); r != nvml.SUCCESS {
		return fmt.Errorf("nvml set compute mode: %s", r)
	}
	return nil
}

func (n *NvidiaSource) GetComputeMode(deviceID string) (string, error) {
	ctl, ok := n.nv.(nvmlControlAPI)
	if !ok {
		return "n/a", nil
	}
	idx, err := n.ctlIndex(deviceID)
	if err != nil {
		return "", err
	}
	mode, r := ctl.DeviceGetComputeMode(idx)
	if r != nvml.SUCCESS {
		return "", fmt.Errorf("nvml get compute mode: %s", r)
	}
	for name, m := range computeModeNames {
		if m == mode {
			return name, nil
		}
	}
	return fmt.Sprintf("mode-%d", mode), nil
}

func (n *NvidiaSource) GetMigMode(deviceID string) (string, error) {
	ctl, ok := n.nv.(nvmlControlAPI)
	if !ok {
		return "n/a", nil
	}
	idx, err := n.ctlIndex(deviceID)
	if err != nil {
		return "", err
	}
	mode, pending, r := ctl.DeviceGetMigMode(idx)
	if r != nvml.SUCCESS {
		return "", fmt.Errorf("nvml get MIG mode: %s", r)
	}
	s := "disabled"
	if mode == 1 {
		s = "enabled"
	}
	if pending == 1 {
		s += " (pending)"
	}
	return s, nil
}

// MPS verbs are driver-daemon operations, not per-device NVML calls; they are
// surfaced as a stub until nvidia-cuda-mps-control management lands.
func (c *NvidiaControl) MPSStart() error {
	return errors.New("MPS daemon control is not yet wired; use nvidia-cuda-mps-control directly")
}
func (c *NvidiaControl) MPSStop() error {
	return c.MPSStart()
}
