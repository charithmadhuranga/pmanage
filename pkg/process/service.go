package process

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	sysinfo "pmanage/pkg/system"
)

type ProcessInfo struct {
	PID        int32   `json:"pid"`
	PPID       int32   `json:"ppid"`
	Name       string  `json:"name"`
	Username   string  `json:"username"`
	Status     string  `json:"status"`
	CPU        float64 `json:"cpu"`
	MemBytes   uint64  `json:"memBytes"`
	MemPct     float64 `json:"memPct"`
	Threads    int32   `json:"threads"`
	CreateTime int64   `json:"createTime"`
	ElapsedSec int64   `json:"elapsedSec"`
}

type OpError struct {
	PID     int32  `json:"pid"`
	Action  string `json:"action"`
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *OpError) Error() string { return fmt.Sprintf("%s of pid %d: %s", e.Action, e.PID, e.Message) }

type Service struct {
	mu       sync.RWMutex
	readOnly bool
}

func NewService() *Service { return &Service{} }

// SetReadOnly toggles destructive-verb gating. Server mode defaults to
// read-only unless an admin token has been configured and presented.
func (s *Service) SetReadOnly(ro bool) {
	s.mu.Lock()
	s.readOnly = ro
	s.mu.Unlock()
}

// ReadOnly reports the current gating state.
func (s *Service) ReadOnly() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readOnly
}

var ErrReadOnly = errors.New("read-only: server not granted remote admin")

func (s *Service) checkWrite() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.readOnly {
		return ErrReadOnly
	}
	return nil
}

func (s *Service) HostInfo() (sysinfo.HostInfo, error) {
	return sysinfo.Gather()
}

func (s *Service) ListProcesses() ([]ProcessInfo, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("enumerate processes: %w", err)
	}

	now := time.Now().Unix()
	out := make([]ProcessInfo, len(procs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)

	for i, p := range procs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, p *process.Process) {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = describe(p, now)
		}(i, p)
	}
	wg.Wait()

	sort.Slice(out, func(a, b int) bool { return out[a].CPU > out[b].CPU })
	return out, nil
}

func describe(p *process.Process, now int64) ProcessInfo {
	info := ProcessInfo{PID: p.Pid}

	if name, err := p.Name(); err == nil {
		info.Name = name
	}
	if ppid, err := p.Ppid(); err == nil {
		info.PPID = ppid
	}
	if user, err := p.Username(); err == nil {
		info.Username = user
	}
	if statuses, err := p.Status(); err == nil && len(statuses) > 0 {
		info.Status = statuses[0]
	}
	if cpu, err := p.CPUPercent(); err == nil {
		info.CPU = cpu
	}
	if mi, err := p.MemoryInfo(); err == nil {
		info.MemBytes = mi.RSS
	}
	if mp, err := p.MemoryPercent(); err == nil {
		info.MemPct = float64(mp)
	}
	if threads, err := p.NumThreads(); err == nil {
		info.Threads = threads
	}
	if ct, err := p.CreateTime(); err == nil {
		info.CreateTime = ct / 1000
		info.ElapsedSec = now - info.CreateTime
	}
	return info
}

func (s *Service) Suspend(pid int32) (OpError, error) {
	if err := s.checkWrite(); err != nil {
		return OpError{PID: pid, Action: "suspend", Code: "read-only", Message: err.Error()}, err
	}
	return op(pid, "suspend", func() error {
		p, err := process.NewProcess(pid)
		if err != nil {
			return err
		}
		return p.Suspend()
	})
}

func (s *Service) Resume(pid int32) (OpError, error) {
	if err := s.checkWrite(); err != nil {
		return OpError{PID: pid, Action: "resume", Code: "read-only", Message: err.Error()}, err
	}
	return op(pid, "resume", func() error {
		p, err := process.NewProcess(pid)
		if err != nil {
			return err
		}
		return p.Resume()
	})
}

func (s *Service) Terminate(pid int32) (OpError, error) {
	if err := s.checkWrite(); err != nil {
		return OpError{PID: pid, Action: "terminate", Code: "read-only", Message: err.Error()}, err
	}
	return op(pid, "terminate", func() error {
		p, err := process.NewProcess(pid)
		if err != nil {
			return err
		}
		return p.Terminate()
	})
}

func (s *Service) Kill(pid int32) (OpError, error) {
	if err := s.checkWrite(); err != nil {
		return OpError{PID: pid, Action: "kill", Code: "read-only", Message: err.Error()}, err
	}
	return op(pid, "kill", func() error {
		p, err := process.NewProcess(pid)
		if err != nil {
			return err
		}
		return p.Kill()
	})
}

func (s *Service) SetPriority(pid int32, nice int32) (OpError, error) {
	if err := s.checkWrite(); err != nil {
		return OpError{PID: pid, Action: "set-priority", Code: "read-only", Message: err.Error()}, err
	}
	return op(pid, "set-priority", func() error {
		return setNice(context.Background(), pid, nice)
	})
}

type opFunc func() error

func op(pid int32, action string, fn opFunc) (OpError, error) {
	res := OpError{PID: pid, Action: action}
	if err := fn(); err != nil {
		res.OK = false
		res.Code = classify(err)
		res.Message = friendly(err)
		return res, err
	}
	res.OK = true
	res.Message = "ok"
	return res, nil
}

var ErrNotOwner = errors.New("not permitted")

func classify(err error) string {
	if errors.Is(err, process.ErrorProcessNotRunning) {
		return "not-running"
	}
	return "fail"
}

func friendly(err error) string {
	if errors.Is(err, process.ErrorProcessNotRunning) {
		return "process is no longer running"
	}
	return fmt.Sprintf("%v", err)
}