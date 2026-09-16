package history

import (
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"pmanage/pkg/accelerator"
)

// Point is one sampled snapshot for a single device.
type Point struct {
	TS         int64   `json:"ts"`
	UtilPct    float64 `json:"utilPct"`
	VRAMUsed   uint64  `json:"vramUsed"`
	VRAMTotal  uint64  `json:"vramTotal"`
	TempC      float64 `json:"tempC"`
	PowerW     float64 `json:"powerW"`
	ClockMHz   uint32  `json:"clockMhz"`
	FanPct     float64 `json:"fanPct"`
}

// Service keeps an in-memory ring buffer of samples per device. It is fed from
// the metrics.Service via AddSink and queried through Range/Export bindings.
type Service struct {
	mu       sync.Mutex
	rate     time.Duration
	capacity int // max points retained per device
	series   map[string][]Point
	order    []string // device ids in first-seen order (stable UI iteration)
}

// NewService creates a history buffer sized to hold `hours` of samples at the
// given sampling `rate`.
func NewService(rate time.Duration, hours int) *Service {
	perHour := int(time.Hour / rate)
	if perHour < 1 {
		perHour = 1
	}
	return &Service{
		rate:     rate,
		capacity: perHour * hours,
		series:   map[string][]Point{},
	}
}

// Bounds returns the retention window in seconds.
func (s *Service) Bounds() int {
	return int(time.Duration(s.capacity) * s.rate / time.Second)
}

// Ingest records one Sample into the per-device series and drops the oldest
// point when the ring exceeds capacity.
func (s *Service) Ingest(sample accelerator.Sample) {
	if sample.Timestamp == 0 {
		sample.Timestamp = time.Now().UnixMilli()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, dm := range sample.Devices {
		if _, ok := s.series[dm.DeviceID]; !ok {
			s.order = append(s.order, dm.DeviceID)
		}
		series := append(s.series[dm.DeviceID], Point{
			TS:        sample.Timestamp,
			UtilPct:   dm.UtilizationPct,
			VRAMUsed:  dm.VRAMUsed,
			VRAMTotal: dm.VRAMTotal,
			TempC:     dm.TemperatureC,
			PowerW:    dm.PowerW,
			ClockMHz:  dm.ClockMHz,
			FanPct:    dm.FanSpeedPct,
		})
		if excess := len(series) - s.capacity; excess > 0 {
			series = series[excess:]
		}
		s.series[dm.DeviceID] = series
	}
}

// Devices returns the device IDs that have at least one sample, in first-seen
// order.
func (s *Service) Devices() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.order))
	for _, id := range s.order {
		if len(s.series[id]) > 0 {
			out = append(out, id)
		}
	}
	return out
}

// Range returns the most recent `seconds` of points for a device (empty slice
// when the device is unknown).
func (s *Service) Range(device string, seconds int) []Point {
	if seconds <= 0 {
		seconds = s.Bounds()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rangeLocked(device, seconds)
}

func (s *Service) rangeLocked(device string, seconds int) []Point {
	series, ok := s.series[device]
	if !ok {
		return nil
	}
	keep := int(time.Duration(seconds) * time.Second / s.rate)
	if keep > len(series) {
		keep = len(series)
	}
	out := make([]Point, keep)
	copy(out, series[len(series)-keep:])
	return out
}

// ExportCSV renders a device series (or the last `seconds`) as CSV with a
// header row suitable for spreadsheets.
func (s *Service) ExportCSV(device string, seconds int) (string, error) {
	if seconds <= 0 {
		seconds = s.Bounds()
	}
	s.mu.Lock()
	rows := s.rangeLocked(device, seconds)
	s.mu.Unlock()
	if len(rows) == 0 {
		return "", fmt.Errorf("no history for %q", device)
	}

	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"ts", "ts_iso", "util_pct", "vram_used", "vram_total", "temp_c", "power_w", "clock_mhz", "fan_pct"})
	for _, p := range rows {
		_ = w.Write([]string{
			fmt.Sprintf("%d", p.TS),
			time.UnixMilli(p.TS).UTC().Format(time.RFC3339),
			formatFloat(p.UtilPct),
			fmt.Sprintf("%d", p.VRAMUsed),
			fmt.Sprintf("%d", p.VRAMTotal),
			formatFloat(p.TempC),
			formatFloat(p.PowerW),
			fmt.Sprintf("%d", p.ClockMHz),
			formatFloat(p.FanPct),
		})
	}
	w.Flush()
	return b.String(), nil
}

// ExportJSON returns the last `seconds` of points for a device as a JSON array
// (stable ordering by timestamp).
func (s *Service) ExportJSON(device string, seconds int) (string, error) {
	s.mu.Lock()
	rows := s.rangeLocked(device, seconds)
	s.mu.Unlock()
	if len(rows) == 0 {
		return "", fmt.Errorf("no history for %q", device)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].TS < rows[j].TS })
	var b strings.Builder
	b.WriteByte('[')
	for i, p := range rows {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(fmt.Sprintf(
			`{"ts":%d,"utilPct":%s,"vramUsed":%d,"vramTotal":%d,"tempC":%s,"powerW":%s,"clockMhz":%d,"fanPct":%s}`,
			p.TS, formatFloat(p.UtilPct), p.VRAMUsed, p.VRAMTotal,
			formatFloat(p.TempC), formatFloat(p.PowerW), p.ClockMHz, formatFloat(p.FanPct)))
	}
	b.WriteByte(']')
	return b.String(), nil
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}