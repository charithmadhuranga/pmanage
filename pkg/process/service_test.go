package process

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func startSleep(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn sleep: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	return cmd
}

func getStatus(t *testing.T, s *Service, pid int32) string {
	t.Helper()
	procs, err := s.ListProcesses()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, p := range procs {
		if p.PID == pid {
			return p.Status
		}
	}
	return ""
}

func TestProcessLifecycle(t *testing.T) {
	s := NewService()
	cmd := startSleep(t)
	pid := int32(cmd.Process.Pid)

	if got := getStatus(t, s, pid); got == "" {
		t.Fatalf("child not found in process list")
	}

	if res, err := s.Suspend(pid); err != nil || !res.OK {
		t.Fatalf("suspend: %+v err=%v", res, err)
	}
	if got := getStatus(t, s, pid); got != "T" && got != "T (stopped)" && got != "stop" && got != "stopped" {
		t.Errorf("expected stopped status after suspend, got %q", got)
	}

	if res, err := s.Resume(pid); err != nil || !res.OK {
		t.Fatalf("resume: %+v err=%v", res, err)
	}
	if got := getStatus(t, s, pid); got == "" {
		t.Errorf("child missing after resume")
	}

	if res, err := s.Terminate(pid); err != nil || !res.OK {
		t.Fatalf("terminate: %+v err=%v", res, err)
	}
	reaped := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(reaped)
	}()
	select {
	case <-reaped:
	case <-time.After(3 * time.Second):
		t.Fatal("child not reaped after terminate")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := getStatus(t, s, pid); got == "" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("child still present after terminate")
}

func TestListIncludesTestProcess(t *testing.T) {
	s := NewService()
	procs, err := s.ListProcesses()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(procs) == 0 {
		t.Fatal("no processes enumerated")
	}
	found := false
	for _, p := range procs {
		if p.PID == int32(os.Getpid()) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("current test process not listed")
	}
}

func TestHostInfo(t *testing.T) {
	s := NewService()
	info, err := s.HostInfo()
	if err != nil {
		t.Fatalf("host info: %v", err)
	}
	if info.Hostname == "" || info.OS == "" || info.KernelVer == "" {
		t.Fatalf("incomplete host info: %+v", info)
	}
}