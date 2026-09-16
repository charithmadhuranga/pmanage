package alerts

import (
	"path/filepath"
	"testing"
	"time"

	"pmanage/pkg/accelerator"
)

func dev(util float64, temp float64, vramTotal uint64, vramUsed uint64, power float64) accelerator.DeviceMetrics {
	return accelerator.DeviceMetrics{
		DeviceID: "gpu0", Status: "ok",
		UtilizationPct: util, TemperatureC: temp, VRAMTotal: vramTotal,
		VRAMUsed: vramUsed, PowerW: power,
	}
}

func TestThermalRuleFiresOncePerCooldown(t *testing.T) {
	e := NewEngineAt(filepath.Join(t.TempDir(), "alerts.json"))
	e.rules = PresetRules()
	e.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	fires := e.Evaluate(accelerator.Sample{Devices: []accelerator.DeviceMetrics{dev(0, 95, 0, 0, 0)}})
	if len(fires) != 1 {
		t.Fatalf("expected 1 thermal fire, got %d", len(fires))
	}
	if fires[0].RuleName != "Thermal warning" {
		t.Errorf("unexpected rule %q", fires[0].RuleName)
	}
	if fires[0].Severity != "warn" {
		t.Errorf("expected warn severity, got %q", fires[0].Severity)
	}

	// same temperature on next tick → cooldown suppresses re-fire
	fires = e.Evaluate(accelerator.Sample{Devices: []accelerator.DeviceMetrics{dev(0, 96, 0, 0, 0)}})
	if len(fires) != 0 {
		t.Fatalf("expected cooldown suppression, got %d fires", len(fires))
	}
}

func TestVRAMOomRule(t *testing.T) {
	e := NewEngineAt(filepath.Join(t.TempDir(), "alerts.json"))
	e.rules = PresetRules()
	e.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	fires := e.Evaluate(accelerator.Sample{Devices: []accelerator.DeviceMetrics{dev(0, 30, 24 << 30, 24 << 30, 200)}})
	found := false
	for _, f := range fires {
		if f.RuleName == "VRAM nearly full" {
			found = true
			if f.Severity != "crit" {
				t.Errorf("expected crit severity, got %q", f.Severity)
			}
		}
	}
	if !found {
		t.Fatal("expected vram oom fire")
	}
}

func TestDeviceSpecificEntity(t *testing.T) {
	e := NewEngineAt(filepath.Join(t.TempDir(), "alerts.json"))
	e.rules = []Rule{{
		ID: "gpu0-hot", Name: "gpu0 hot", Metric: MetricTempC, Entity: "gpu0",
		Operator: OpGte, Threshold: 90, Action: ActionToast, Enabled: true,
		CooldownSec: 60, Message: "too hot",
	}}
	e.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	// npu0 should NOT match
	dm := dev(0, 95, 0, 0, 0)
	dm.DeviceID = "npu0"
	if f := e.Evaluate(accelerator.Sample{Devices: []accelerator.DeviceMetrics{dm}}); len(f) != 0 {
		t.Fatalf("gpu-only rule fired on npu0: %+v", f)
	}
	// gpu0 matches
	dm.DeviceID = "gpu0"
	if f := e.Evaluate(accelerator.Sample{Devices: []accelerator.DeviceMetrics{dm}}); len(f) != 1 {
		t.Fatalf("expected fire on gpu0, got %d", len(f))
	}
}

func TestZombieRuleFiresAfterTicks(t *testing.T) {
	e := NewEngineAt(filepath.Join(t.TempDir(), "alerts.json"))
	e.rules = []Rule{PresetRules()[3]} // preset-zombie
	e.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	// pid 42 holds 4 GiB, 0 util, for ZOMBIE_TICKS ticks → fires once
	var fires []Fire
	for i := 0; i < ZOMBIE_TICKS+2; i++ {
		fires = append(fires, e.Evaluate(accelerator.Sample{
			Devices: []accelerator.DeviceMetrics{dev(0, 30, 0, 0, 0)},
			Procs: []accelerator.ProcUsage{{
				PID: 42, Name: "python3", DeviceID: "gpu0",
				VRAMUsed: 4 << 30, UtilizationPct: 0,
			}},
		})...)
	}

	total := 0
	for _, f := range fires {
		if f.RuleName == "GPU zombie (vram held, 0 util)" {
			total++
		}
	}
	if total != 1 {
		t.Fatalf("expected exactly 1 zombie fire over %d ticks, got %d", ZOMBIE_TICKS+2, total)
	}
}

func TestZombieResetWhenUtilReturns(t *testing.T) {
	e := NewEngineAt(filepath.Join(t.TempDir(), "alerts.json"))
	e.rules = []Rule{PresetRules()[3]}
	e.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	proc := accelerator.ProcUsage{PID: 7, Name: "train", DeviceID: "gpu0", VRAMUsed: 2 << 30, UtilizationPct: 0}
	// 3 zero ticks (below ZOMBIE_TICKS threshold)
	for i := 0; i < 3; i++ {
		e.Evaluate(accelerator.Sample{Devices: []accelerator.DeviceMetrics{dev(0, 30, 0, 0, 0)},
			Procs: []accelerator.ProcUsage{proc}})
	}
	// util returns → counter resets, no fire
	proc.UtilizationPct = 50
	e.Evaluate(accelerator.Sample{Devices: []accelerator.DeviceMetrics{dev(50, 30, 0, 0, 0)},
		Procs: []accelerator.ProcUsage{proc}})
	if len(e.ActiveFires()) != 0 {
		t.Fatalf("unexpected fires after util returned: %+v", e.ActiveFires())
	}
}

func TestRulesPersistAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alerts.json")
	e := NewEngineAt(path)
	_ = e.DeleteRule("preset-powercap")

	reloaded := NewEngineAt(path)
	rules := reloaded.Rules()
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules after delete+reload, got %d", len(rules))
	}
	found := false
	for _, r := range rules {
		if r.ID == "preset-powercap" {
			found = true
		}
	}
	if found {
		t.Fatal("powercap rule should have been deleted")
	}
}

func TestCheckMetricValue(t *testing.T) {
	dm := dev(0, 30, 100, 96, 200)
	if v, ok := metricValue(MetricVRAMPct, dm); !ok || v != 96 {
		t.Errorf("expected 96%% vram, got %f ok=%v", v, ok)
	}
	if _, ok := metricValue(MetricVRAMPct, dev(0, 30, 0, 1, 200)); ok {
		t.Error("expected not-ok for 0 total vram")
	}
}