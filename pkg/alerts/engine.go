package alerts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/adrg/xdg"
)

// Metric is a measurable dimension an alert rule can target.
type Metric string

const (
	MetricUtilPct  Metric = "util_pct"
	MetricTempC    Metric = "temp_c"
	MetricPowerW   Metric = "power_w"
	MetricVRAMPct  Metric = "vram_pct"
	MetricVRAMUsed Metric = "vram_used" // bytes
)

// Action is what an alert does when it fires.
type Action string

const (
	ActionToast         Action = "toast"           // in-app toast (frontend renders)
	ActionNotify        Action = "notify"          // desktop notification
	ActionAutoSuspend   Action = "auto-suspend"    // suspend offender (zombie)
	ActionAutoTerminate Action = "auto-terminate"  // kill offender (zombie, double-confirm in future)
)

// Operator compares sample value against Threshold.
type Operator string

const (
	OpGt  Operator = ">"
	OpGte Operator = ">="
	OpLt  Operator = "<"
	OpLte Operator = "<="
)

// Rule is a persisted alert rule: entity × metric × threshold × action.
// Entity mirrors DeviceInfo.ID ("gpu0") or "*" for all devices of a source
// kind (e.g. "gpu0"*, "npu0"*, "*").
type Rule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Metric      Metric   `json:"metric"`
	Entity      string   `json:"entity"` // device id suffix match; "*" = any; "proc" = per-process zombie rule
	Operator    Operator `json:"operator"`
	Threshold   float64  `json:"threshold"`
	Action      Action   `json:"action"`
	Enabled     bool     `json:"enabled"`
	CooldownSec int      `json:"cooldownSec"` // min seconds between fires
	Message     string   `json:"message,omitempty"`
}

// Fire is an emitted alert (surfaces as toast/notification, kept in ActiveList).
type Fire struct {
	ID        string `json:"id"`
	RuleName  string `json:"ruleName"`
	Message   string `json:"message"`
	DeviceID  string `json:"deviceId"`
	Metric    Metric `json:"metric"`
	Value     float64 `json:"value"`
	Severity  string `json:"severity"` // info | warn | crit
	Action    Action `json:"action"`
	FiredAt   int64  `json:"firedAt"`
}

// Engine evaluates rules against each Sample. It survives UI restarts: rules
// are persisted to a JSON store, evaluation runs in Go.
type Engine struct {
	mu         sync.Mutex
	rules      []Rule
	active     []Fire
	lastFire   map[string]time.Time // ruleID:entity → last fire time
	zombieSeen map[int32]int        // pid → consecutive ticks with 0 util + vram held
	zombiePid  map[int32]string     // pid → device id
	configPath string
	now        func() time.Time
}

// NewEngine loads rules from the user config dir (xdg); missing file → presets.
func NewEngine() *Engine {
	path := filepath.Join(xdg.ConfigHome, "pmanage", "alerts.json")
	e := &Engine{
		lastFire:   map[string]time.Time{},
		zombieSeen: map[int32]int{},
		zombiePid:  map[int32]string{},
		configPath: path,
		now:        time.Now,
	}
	if err := e.load(); err != nil || len(e.rules) == 0 {
		e.rules = PresetRules()
		_ = e.save()
	}
	return e
}

// NewEngineAt is for tests: rules load/save from an explicit path.
func NewEngineAt(path string) *Engine {
	e := NewEngine()
	e.configPath = path
	_ = e.load()
	return e
}

// RulePreset builders.
func PresetRules() []Rule {
	return []Rule{
		{ID: "preset-thermal", Name: "Thermal warning", Metric: MetricTempC,
			Entity: "*", Operator: OpGte, Threshold: 85, Action: ActionToast,
			Enabled: true, CooldownSec: 300,
			Message: "temperature above 85°C"},
		{ID: "preset-powercap", Name: "Power cap", Metric: MetricPowerW,
			Entity: "*", Operator: OpGte, Threshold: 300, Action: ActionNotify,
			Enabled: false, CooldownSec: 300,
			Message: "power draw above 300W"},
		{ID: "preset-vramoom", Name: "VRAM nearly full", Metric: MetricVRAMPct,
			Entity: "*", Operator: OpGte, Threshold: 95, Action: ActionNotify,
			Enabled: true, CooldownSec: 600,
			Message: "VRAM utilization above 95% — OOM risk"},
		{ID: "preset-zombie", Name: "GPU zombie (vram held, 0 util)", Metric: MetricUtilPct,
			Entity: "proc", Operator: OpLte, Threshold: 1, Action: ActionToast,
			Enabled: true, CooldownSec: 900,
			Message: "process holds VRAM but reports 0% accelerator utilization"},
	}
}

func (e *Engine) Rules() []Rule {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Rule, len(e.rules))
	copy(out, e.rules)
	return out
}

func (e *Engine) UpsertRule(r Rule) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r.ID == "" {
		r.ID = fmt.Sprintf("rule-%d", e.now().UnixNano())
	}
	for i := range e.rules {
		if e.rules[i].ID == r.ID {
			e.rules[i] = r
			return e.saveLocked()
		}
	}
	e.rules = append(e.rules, r)
	return e.saveLocked()
}

func (e *Engine) DeleteRule(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.rules {
		if e.rules[i].ID == id {
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			return e.saveLocked()
		}
	}
	return nil
}

func (e *Engine) SetRuleEnabled(id string, on bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.rules {
		if e.rules[i].ID == id {
			e.rules[i].Enabled = on
			return e.saveLocked()
		}
	}
	return nil
}

func (e *Engine) ActiveFires() []Fire {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Fire, len(e.active))
	copy(out, e.active)
	return out
}

func (e *Engine) AcknowledgeAll() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.active = nil
}

func (e *Engine) load() error {
	data, err := os.ReadFile(e.configPath)
	if err != nil {
		return err
	}
	_ = json.Unmarshal(data, &e.rules)
	return nil
}

func (e *Engine) save() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.saveLocked()
}

func (e *Engine) saveLocked() error {
	if e.configPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(e.configPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(e.rules, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(e.configPath, data, 0o644)
}