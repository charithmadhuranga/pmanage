package servermode

import (
	"errors"

	"pmanage/pkg/process"
)

// Service is exposed to the frontend so the UI can reflect server-mode
// state: whether we're headless over HTTP, and whether this session may
// issue destructive verbs.
type Service struct {
	cfg      *Config
	proc     *process.Service
	readOnly bool
}

func NewService(cfg *Config, proc *process.Service) *Service {
	return &Service{cfg: cfg, proc: proc, readOnly: cfg != nil && cfg.ReadOnly()}
}

type Info struct {
	Enabled   bool   `json:"enabled"`
	Address   string `json:"address"`
	Auth      bool   `json:"auth"`
	ReadOnly  bool   `json:"readOnly"`
	RemoteAdj bool   `json:"remoteAdj"`
}

func (s *Service) Info() Info {
	if s.cfg == nil {
		return Info{Enabled: false, ReadOnly: false}
	}
	return Info{
		Enabled:   true,
		Address:   s.cfg.ListenAddr(),
		Auth:      s.cfg.Token != "",
		ReadOnly:  s.readOnly || s.proc.ReadOnly(),
		RemoteAdj: !s.cfg.ReadOnly(),
	}
}

// UnlockAdmin toggles the process service out of read-only when the caller
// presents the configured admin token. Mirrors the cookie-style handshake in
// a service call so remote sessions can enable kill from the UI with the
// right secret.
func (s *Service) UnlockAdmin(token string) error {
	if s.cfg == nil {
		return errors.New("server mode not active")
	}
	if !s.cfg.IsAdmin(token) {
		return errors.New("invalid admin token")
	}
	s.proc.SetReadOnly(false)
	s.readOnly = false
	return nil
}

// LockAdmin re-enables read-only gating.
func (s *Service) LockAdmin() {
	s.proc.SetReadOnly(true)
	s.readOnly = true
}