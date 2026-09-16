//go:build windows

package process

import (
	"context"
	"fmt"

	syswindows "golang.org/x/sys/windows"
)

var priorityClasses = map[int32]uint32{
	-19: syswindows.IDLE_PRIORITY_CLASS,
	0:    syswindows.NORMAL_PRIORITY_CLASS,
	10:   syswindows.HIGH_PRIORITY_CLASS,
	19:   syswindows.REALTIME_PRIORITY_CLASS,
}

func setNice(_ context.Context, pid int32, nice int32) error {
	cls, ok := priorityClasses[nice]
	if !ok {
		return fmt.Errorf("unsupported nice value %d on windows (use -19, 0, 10, 19)", nice)
	}
	handle, err := syswindows.OpenProcess(syswindows.PROCESS_SET_INFORMATION, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("open process: %w", err)
	}
	defer syswindows.CloseHandle(handle)
	if err := syswindows.SetPriorityClass(handle, cls); err != nil {
		return fmt.Errorf("set priority class: %w", err)
	}
	return nil
}