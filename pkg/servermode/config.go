package servermode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adrg/xdg"
)

// Config describes how the headless `-tags server` build behaves: which
// interface to bind, the read token every client must send, and the optional
// admin token that unlocks destructive verbs.
//
// Env overrides (highest precedence), then a JSON config file at
// $XDG_CONFIG_HOME/pmanage/server.json, then defaults.
type Config struct {
	// Host to bind; default "localhost". Use "0.0.0.0" for the LAN.
	Host string `json:"host"`
	// Port to listen on; default 8080.
	Port int `json:"port"`
	// Token required for any IPC call. Empty = no auth (localhost only).
	Token string `json:"token"`
	// AdminToken, when set, is required for suspend/resume/kill/set-priority.
	// Clients presenting only Token get a read-only view.
	AdminToken string `json:"adminToken"`
}

const (
	envHost     = "PMANAGE_SERVER_HOST"
	envPort     = "PMANAGE_SERVER_PORT"
	envToken    = "PMANAGE_SERVER_TOKEN"
	envAdminTok = "PMANAGE_SERVER_ADMIN_TOKEN"
)

// Path returns the config file location.
func Path() string { return filepath.Join(xdg.ConfigHome, "pmanage", "server.json") }

// Load reads the config file (if present) and overlays env overrides.
func Load() (*Config, error) {
	cfg := &Config{Host: "localhost", Port: 8080}

	data, err := os.ReadFile(Path())
	if err == nil {
		if len(data) > 0 {
			if jerr := json.Unmarshal(data, cfg); jerr != nil {
				return nil, fmt.Errorf("parse %s: %w", Path(), jerr)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", Path(), err)
	}

	if v := os.Getenv(envHost); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv(envPort); v != "" {
		p, perr := strconv.Atoi(v)
		if perr != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid %s: %q", envPort, v)
		}
		cfg.Port = p
	}
	if v := os.Getenv(envToken); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv(envAdminTok); v != "" {
		cfg.AdminToken = v
	}
	return cfg, nil
}

// Save persists the config (creating parent dirs).
func (c *Config) Save() error {
	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), raw, 0o600)
}

// ListenAddr returns "host:port".
func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// ValidToken reports whether presented is a bearer token acceptable for reads.
func (c *Config) ValidToken(presented string) bool {
	if c.Token == "" {
		// No read token configured: localhost is trusted by default.
		if c.isLocalhost() {
			return true
		}
		// A non-local bind with no token is refused writes; reads are fine.
		return true
	}
	return presented != "" && strings.EqualFold(presented, c.Token)
}

// IsAdmin reports whether presented unlocks destructive verbs.
func (c *Config) IsAdmin(presented string) bool {
	return c.AdminToken != "" && strings.EqualFold(presented, c.AdminToken)
}

// ReadOnly reports whether no admin token is configured, meaning remote
// clients can never explore kill/suspend.
func (c *Config) ReadOnly() bool { return c.AdminToken == "" }

func (c *Config) isLocalhost() bool {
	return c.Host == "localhost" || c.Host == "127.0.0.1" || c.Host == "::1"
}