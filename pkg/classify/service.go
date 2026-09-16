package classify

import (
	"fmt"
	"path/filepath"

	"github.com/shirou/gopsutil/v4/process"
)

// Service binds process→workload classification for the frontend. Signal
// gathering reads live process state via gopsutil; Classify itself is pure.
type Service struct{}

func NewService() *Service { return &Service{} }

// SignalsFor gathers observable evidence for a pid. Best-effort: failures on
// individual fields leave them empty rather than failing.
func SignalsFor(pid int32) Signals {
	s := Signals{PID: pid}
	p, err := process.NewProcess(pid)
	if err != nil {
		return s
	}
	if name, err := p.Name(); err == nil {
		s.Name = name
	}
	if exe, err := p.Exe(); err == nil {
		s.Exe = exe
	}
	if cmd, err := p.Cmdline(); err == nil {
		s.Cmdline = cmd
	}
	if env, err := p.Environ(); err == nil {
		s.Env = env
	}
	if files, err := p.OpenFiles(); err == nil {
		for _, f := range files {
			if f.Path == "" {
				continue
			}
			base := filepath.Base(f.Path)
			// only shared objects and interpreter binaries carry lib signals
			if filepath.Ext(base) == ".so" || filepath.Ext(base) == ".dylib" || base == "python3" || base == "node" {
				s.Libs = append(s.Libs, base)
			}
		}
	}
	return s
}

// Classify pid by first gathering live signals, then running the pure rules.
func (s *Service) ClassifyPID(pid int32) (Classified, error) {
	sig := SignalsFor(pid)
	if sig.Name == "" && sig.Cmdline == "" && sig.Exe == "" {
		return Classified{PID: pid, Kind: KindUnknown, Score: 0, Hits: nil},
			fmt.Errorf("process %d: no observable signals (not running?)", pid)
	}
	return Classify(sig), nil
}

// SignalsPlusClassify is a test hook that classifies supplied signals.
func ClassifySignals(sig Signals) Classified { return Classify(sig) }