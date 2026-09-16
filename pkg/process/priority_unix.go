//go:build linux || darwin

package process

import (
	"context"
	"fmt"

	"golang.org/x/sys/unix"
)

func setNice(_ context.Context, pid int32, nice int32) error {
	if nice < -20 || nice > 19 {
		return fmt.Errorf("nice value %d out of range (-20..19)", nice)
	}
	if err := unix.Setpriority(unix.PRIO_PROCESS, int(pid), int(nice)); err != nil {
		return fmt.Errorf("set priority: %w", err)
	}
	return nil
}