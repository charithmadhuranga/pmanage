package fdinfo

import (
	"testing"
	"time"
)

func TestEngineUtil(t *testing.T) {
	elapsed := time.Second
	cases := []struct {
		name     string
		prev, cur uint64
		want     float64
	}{
		{"idle", 0, 0, 0},
		{"full", uint64(time.Second), uint64(2 * time.Second), 100},
		{"half", uint64(time.Second), uint64(1500 * time.Millisecond), 50},
		{"first-observation", 0, 1000, 0},
		{"counter-reset", 5000, 100, 0},
		{"below-prev-guard", 1000, 900, 0},
		{"clamp", uint64(time.Second), uint64(5 * time.Second), 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EngineUtil(tc.prev, tc.cur, elapsed)
			if got != tc.want {
				t.Fatalf("EngineUtil(%d→%d) = %v, want %v", tc.prev, tc.cur, got, tc.want)
			}
		})
	}
}

func TestEngineDelta(t *testing.T) {
	prev := map[string]uint64{"gfx": uint64(time.Second), "compute": uint64(50 * time.Millisecond)}
	cur := map[string]uint64{
		"gfx":      uint64(1500 * time.Millisecond),
		"compute":  uint64(150 * time.Millisecond),
		"newengine": 1,
	}
	d := EngineDelta(prev, cur, time.Second)
	if d["gfx"] != 50 {
		t.Fatalf("gfx = %v, want 50", d["gfx"])
	}
	if d["compute"] != 10 {
		t.Fatalf("compute = %v, want 10", d["compute"])
	}
	if d["newengine"] != 0 {
		t.Fatalf("newengine should be 0 on first sight")
	}
	if got := MaxEnginePct(d); got != 50 {
		t.Fatalf("max = %v, want 50", got)
	}
}

func TestMultiGpuAggregationScalability(t *testing.T) {
	// Simulates a busy 4-card rig: each card ~40% busy between two 1s ticks.
	elapsed := time.Second
	prev := uint64(400 * time.Millisecond)
	cur := uint64(800 * time.Millisecond)
	for i := 0; i < 4; i++ {
		u := EngineUtil(prev, cur, elapsed)
		if u < 39 || u > 41 {
			t.Fatalf("card %d util out of range: %v", i, u)
		}
	}
}