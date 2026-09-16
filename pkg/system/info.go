package system

import (
	"fmt"
	"runtime"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

type HostInfo struct {
	OS          string  `json:"os"`
	Platform    string  `json:"platform"`
	PlatformVer string  `json:"platformVer"`
	KernelVer   string  `json:"kernelVer"`
	Arch        string  `json:"arch"`
	Hostname    string  `json:"hostname"`
	UptimeSec   uint64  `json:"uptimeSec"`
	NumCPU      int     `json:"numCpu"`
	MemTotal    uint64  `json:"memTotal"`
	MemUsed     uint64  `json:"memUsed"`
	MemPct      float64 `json:"memPct"`
}

func Gather() (HostInfo, error) {
	var info HostInfo
	info.OS = runtime.GOOS
	info.Arch = runtime.GOARCH

	if hi, err := host.Info(); err == nil {
		info.Platform = hi.Platform
		info.PlatformVer = hi.PlatformVersion
		info.KernelVer = hi.KernelVersion
		info.Hostname = hi.Hostname
		info.UptimeSec = hi.Uptime
	} else {
		return info, fmt.Errorf("host info: %w", err)
	}

	if n, err := cpu.Counts(true); err == nil {
		info.NumCPU = n
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		info.MemTotal = vm.Total
		info.MemUsed = vm.Used
		info.MemPct = vm.UsedPercent
	}
	return info, nil
}