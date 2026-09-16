package history

import (
	"strings"
	"testing"
	"time"

	"pmanage/pkg/accelerator"
)

func sample(ts int64, util float64) accelerator.Sample {
	return accelerator.Sample{
		Timestamp: ts,
		Devices: []accelerator.DeviceMetrics{{
			DeviceID: "gpu0", Status: "ok", UtilizationPct: util,
			VRAMUsed: uint64(util) * 1024 * 1024, VRAMTotal: 24 << 30,
			TemperatureC: 40 + util, PowerW: 100 + util, ClockMHz: 1800, FanSpeedPct: 30,
		}},
	}
}

func TestIngestAndRangeRing(t *testing.T) {
	s := NewService(time.Millisecond*100, 1) // 36000 points capacity
	base := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		s.Ingest(sample(base+int64(i)*100, float64(i)))
	}

	if got := s.Devices(); len(got) != 1 || got[0] != "gpu0" {
		t.Fatalf("unexpected devices %v", got)
	}
	pts := s.Range("gpu0", 1)
	if len(pts) != 5 {
		t.Fatalf("expected 5 points in 1s window, got %d", len(pts))
	}
	if pts[0].TS != base || pts[4].UtilPct != 4 {
		t.Errorf("ring order wrong: %+v", pts)
	}
}

func TestCapacityTrimsOldest(t *testing.T) {
	s := NewService(time.Second, 1) // 3600 capacity
	base := time.Now().UnixMilli()
	for i := 0; i < 3610; i++ {
		s.Ingest(sample(base+int64(i)*1000, 1))
	}
	if got := len(s.Range("gpu0", 10000)); got != 3600 {
		t.Fatalf("expected ring bounded at 3600, got %d", got)
	}
	// oldest dropped: first kept ts should be base+10s
	if pts := s.Range("gpu0", 10000); pts[0].TS != base+10*1000 {
		t.Errorf("expected oldest-dropped, got first ts %d", pts[0].TS)
	}
}

func TestRangeWindow(t *testing.T) {
	s := NewService(time.Second, 1)
	base := time.Now().UnixMilli()
	for i := 0; i < 100; i++ {
		s.Ingest(sample(base+int64(i)*1000, float64(i)))
	}
	// last 10 s → 10 points (every 1s)
	if pts := s.Range("gpu0", 10); len(pts) != 10 {
		t.Fatalf("expected 10, got %d", len(pts))
	} else if pts[0].UtilPct != 90 {
		t.Errorf("expected window starting at 90, got %f", pts[0].UtilPct)
	}
}

func TestExportCSV(t *testing.T) {
	s := NewService(time.Second, 1)
	s.Ingest(sample(time.Now().UnixMilli(), 42))
	out, err := s.ExportCSV("gpu0", 60)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header + 1 row, got %d lines", len(lines))
	}
	if !strings.HasPrefix(lines[0], "ts,ts_iso,util_pct") {
		t.Errorf("bad header: %q", lines[0])
	}
	if !strings.Contains(lines[1], ",42,") {
		t.Errorf("expected util 42 in row, got %q", lines[1])
	}
}

func TestExportJSON(t *testing.T) {
	s := NewService(time.Second, 1)
	s.Ingest(sample(time.Now().UnixMilli(), 7))
	out, err := s.ExportJSON("gpu0", 60)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"utilPct":7`) {
		t.Errorf("unexpected json: %s", out)
	}
}

func TestExportUnknownDevice(t *testing.T) {
	s := NewService(time.Second, 1)
	if _, err := s.ExportCSV("nope", 60); err == nil {
		t.Fatal("expected error for unknown device")
	}
	if _, err := s.ExportJSON("nope", 60); err == nil {
		t.Fatal("expected error for unknown device")
	}
}