package fdinfo

import "time"

// EngineUtil converts a delta of cumulative engine nanoseconds between two
// ticks into a percent, guarding against counter resets. Returns 0 on reset.
func EngineUtil(prevNS, curNS uint64, elapsed time.Duration) float64 {
	if prevNS == 0 || curNS < prevNS || curNS == prevNS {
		return 0
	}
	delta := float64(curNS - prevNS)
	elapsedNs := float64(elapsed.Nanoseconds())
	if elapsedNs <= 0 {
		return 0
	}
	pct := delta / elapsedNs * 100.0
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// EngineDelta computes per-engine utilization between two aggregate samples.
func EngineDelta(prev, cur map[string]uint64, elapsed time.Duration) map[string]float64 {
	out := make(map[string]float64, len(cur))
	for name, curNS := range cur {
		prevNS, ok := prev[name]
		if !ok {
			// New engine — first observation, no baseline yet.
			out[name] = 0
			continue
		}
		out[name] = EngineUtil(prevNS, curNS, elapsed)
	}
	return out
}

// MaxEnginePct returns the highest utilization across engines (the headline
// "device busyness" that amdgpu_top/gputop style tools report).
func MaxEnginePct(delta map[string]float64) float64 {
	var max float64
	for _, v := range delta {
		if v > max {
			max = v
		}
	}
	return max
}