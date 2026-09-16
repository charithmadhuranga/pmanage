package alerts

import (
	"fmt"
	"time"

	"pmanage/pkg/accelerator"
)

// evaluate runs one sample through every enabled rule. Device rules compare
// device metrics; the zombie rule (entity "proc") tracks processes that hold
// VRAM but report 0 util for ZOMBIE_TICKS consecutive ticks.
const ZOMBIE_TICKS = 5

// Evaluate processes a Sample and returns any fires it produced (also stored
// in ActiveFires). Concurrency-safe; call from the metrics sink goroutine.
func (e *Engine) Evaluate(sample accelerator.Sample) []Fire {
	var fires []Fire
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, dm := range sample.Devices {
		for _, r := range e.rules {
			if !r.Enabled || r.Entity == "proc" {
				continue
			}
			if r.Entity != "*" && !entityMatches(r.Entity, dm.DeviceID) {
				continue
			}
			switch r.Metric {
			case MetricTempC, MetricPowerW, MetricUtilPct, MetricVRAMPct, MetricVRAMUsed:
			default:
				continue
			}
			value, ok := metricValue(r.Metric, dm)
			if !ok {
				continue
			}
			if !compare(r.Operator, value, r.Threshold) {
				continue
			}
			if f, ok := e.fireLocked(r, dm.DeviceID, value); ok {
				fires = append(fires, f)
			}
		}
	}

	// Zombie detection is entity "proc": per-process, needs the sample's procs.
	for _, r := range e.rules {
		if !r.Enabled || r.Entity != "proc" {
			continue
		}
		if r.Metric != MetricUtilPct {
			continue
		}
		e.trackZombies(r, sample)
		e.zombieTicks(r, sample, &fires)
	}
	return fires
}

// trackZombies increments/decrements per-pid counters: pid holds VRAM and has
// util ≤ threshold → ++, otherwise reset.
func (e *Engine) trackZombies(r Rule, sample accelerator.Sample) {
	seen := map[int32]bool{}
	for _, p := range sample.Procs {
		seen[p.PID] = true
		if p.VRAMUsed == 0 {
			continue
		}
		// Zombie = holds memory but no reported accelerator utilization.
		// For per-process rows our util is the aggregate device util proxy only
		// when the sample carries it; treat missing as 0 → too noisy, so only
		// count when VRAMUsed is meaningful (≥ 1 MiB).
		if compare(r.Operator, p.UtilizationPct, r.Threshold) {
			e.zombieSeen[p.PID]++
			e.zombiePid[p.PID] = p.DeviceID
		} else {
			delete(e.zombieSeen, p.PID)
		}
	}
	// Drop PIDs no longer on the device.
	for pid := range e.zombieSeen {
		if !seen[pid] {
			delete(e.zombieSeen, pid)
			delete(e.zombiePid, pid)
		}
	}
}

// zombieTicks fires the zombie rule once a pid has stayed at 0 util ≥ ZOMBIE_TICKS.
func (e *Engine) zombieTicks(r Rule, sample accelerator.Sample, fires *[]Fire) {
	for pid, ticks := range e.zombieSeen {
		if ticks != ZOMBIE_TICKS {
			continue
		}
		devID := e.zombiePid[pid]
		if _, ok := e.lastFire["zombie:"+r.ID+":"+devID]; ok {
			continue
		}
		f := Fire{
			ID:       fmt.Sprintf("%s-%d", r.ID, e.now().UnixMilli()),
			RuleName: r.Name,
			Message:  fmt.Sprintf("pid %d holds accelerator memory with 0%% utilization (%d consecutive samples)", pid, ZOMBIE_TICKS),
			DeviceID: devID,
			Metric:   r.Metric,
			Value:    0,
			Action:   r.Action,
			FiredAt:  e.now().UnixMilli(),
		}
		f.Severity = severityOf(r)
		e.active = append(e.active, f)
		e.lastFire["zombie:"+r.ID+":"+devID] = e.now()
		*fires = append(*fires, f)
		// keep counting so we only fire once per cooldown; reset after to avoid
		// immediate re-fire loop below via lastFire gate.
		e.zombieSeen[pid] = 0
	}
}

// fireLocked emits a Fire for the given rule/entity if cooldown has elapsed.
func (e *Engine) fireLocked(r Rule, deviceID string, value float64) (Fire, bool) {
	key := r.ID + ":" + deviceID
	last, ok := e.lastFire[key]
	if ok {
		cd := time.Duration(r.CooldownSec) * time.Second
		if cd > 0 && e.now().Sub(last) < cd {
			return Fire{}, false
		}
	}
	if !ok || e.now().Sub(last) >= time.Duration(r.CooldownSec)*time.Second {
		f := Fire{
			ID:       fmt.Sprintf("%s-%d", r.ID, e.now().UnixMilli()),
			RuleName: r.Name,
			Message:  fmt.Sprintf("%s (%s = %.1f)", r.Message, r.Metric, value),
			DeviceID: deviceID,
			Metric:   r.Metric,
			Value:    value,
			Action:   r.Action,
			FiredAt:  e.now().UnixMilli(),
		}
		f.Severity = severityOf(r)
		e.active = append(e.active, f)
		e.lastFire[key] = e.now()
		return f, true
	}
	return Fire{}, false
}

func severityOf(r Rule) string {
	switch r.Metric {
	case MetricTempC:
		if r.Threshold >= 100 {
			return "crit"
		}
		return "warn"
	case MetricVRAMPct, MetricVRAMUsed:
		if r.Threshold >= 95 {
			return "crit"
		}
		return "warn"
	default:
		return "warn"
	}
}

func entityMatches(entity, deviceID string) bool {
	if entity == "*" {
		return true
	}
	// allow prefix match: entity "gpu0" matches exactly; "gpu" matches any gpuN
	if entity == deviceID {
		return true
	}
	// treat entity as prefix if it's a source prefix like "gpu"/"npu"/"tpu"
	for _, prefix := range []string{"gpu", "npu", "tpu", "intel", "amd"} {
		if entity == prefix && len(deviceID) > len(prefix) && deviceID[:len(prefix)] == entity {
			return true
		}
	}
	return false
}

func metricValue(m Metric, dm accelerator.DeviceMetrics) (float64, bool) {
	switch m {
	case MetricTempC:
		return dm.TemperatureC, dm.TemperatureC > 0
	case MetricPowerW:
		return dm.PowerW, dm.PowerW >= 0
	case MetricUtilPct:
		return dm.UtilizationPct, dm.UtilizationPct >= 0
	case MetricVRAMPct:
		if dm.VRAMTotal == 0 {
			return 0, false
		}
		return float64(dm.VRAMUsed) / float64(dm.VRAMTotal) * 100, true
	case MetricVRAMUsed:
		return float64(dm.VRAMUsed), true
	default:
		return 0, false
	}
}

func compare(op Operator, value, threshold float64) bool {
	switch op {
	case OpGt:
		return value > threshold
	case OpGte:
		return value >= threshold
	case OpLt:
		return value < threshold
	case OpLte:
		return value <= threshold
	default:
		return false
	}
}