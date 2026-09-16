package accelerator

import (
	"strings"
	"testing"
)

func TestHailoDetectFromFixture(t *testing.T) {
	src := NewHailoSource()
	src.cli = &fixtureCLI{script: map[string]string{
		"fw-control identify": "ID: 0000-0000-0000-0000, Hailo-8, running firmware...",
	}}
	if !src.Detect() {
		t.Fatal("expected hailo detect from fixture")
	}
	devs := src.Devices()
	if len(devs) != 1 || devs[0].ID != "hailo0" {
		t.Fatalf("unexpected devices: %+v", devs)
	}
	if !strings.Contains(devs[0].Name, "Hailo-8") {
		t.Errorf("expected Hailo-8 name, got %q", devs[0].Name)
	}
}

func TestHailoDetectNoCLI(t *testing.T) {
	src := NewHailoSource()
	src.cli = &fixtureCLI{script: map[string]string{}}
	if src.Detect() {
		t.Fatal("expected detect to fail without hailortcli fixture")
	}
}

func TestHailoSampleParsesJSONLine(t *testing.T) {
	src := NewHailoSource()
	src.cli = &fixtureCLI{script: map[string]string{
		"fw-control identify": "Hailo-8",
		"monitor --interval=1000 --no-clear-screen": "" +
			`{"device_index":0,"device_id":"0000-0000-0000-0000","utilization":53.2,"temperature":64.5,"power":3.2}` + "\n",
	}}
	if !src.Detect() {
		t.Fatal("detect failed")
	}
	s, err := src.Sample(nil)
	if err != nil {
		t.Fatal(err)
	}
	dm := s.Devices[0]
	if dm.UtilizationPct < 53 || dm.UtilizationPct > 54 {
		t.Errorf("expected util ~53.2, got %f", dm.UtilizationPct)
	}
	if dm.TemperatureC != 64.5 {
		t.Errorf("expected temp 64.5, got %f", dm.TemperatureC)
	}
	if dm.PowerW != 3.2 {
		t.Errorf("expected power 3.2, got %f", dm.PowerW)
	}
}

func TestHailoMonitorFailStillSamples(t *testing.T) {
	src := NewHailoSource()
	src.cli = &fixtureCLI{script: map[string]string{
		"fw-control identify": "Hailo-8",
		// monitor never provided → subprocess error path.
	}}
	if !src.Detect() {
		t.Fatal("detect failed")
	}
	s, err := src.Sample(nil)
	if err != nil {
		t.Fatalf("sample should degrade to N/A rather than error: %v", err)
	}
	if s.Devices[0].UtilizationPct != -1 {
		t.Errorf("expected N/A (util -1) on monitor failure, got %f", s.Devices[0].UtilizationPct)
	}
}

func TestJetsonDetectNeedsL4T(t *testing.T) {
	src := NewJetsonSource()
	src.fs = newFixtureFS(t, map[string]string{})
	src.cli = &fixtureCLI{script: map[string]string{}}
	src.smi = &fixtureRunner{script: map[string]string{}}
	if src.Detect() {
		t.Fatal("expected detect to fail without /etc/nv_tegra_release")
	}
}

func TestJetsonDetectAndSample(t *testing.T) {
	src := NewJetsonSource()
	src.fs = newFixtureFS(t, map[string]string{
		"etc/nv_tegra_release": "R36 .4.0 release (PG905B), 36.4.0, 3.14.1, Thu May  9 17:03:56 UTC 2024",
	})
	src.cli = &fixtureCLI{script: map[string]string{
		"--interval=1000 --exit": "" +
			"RAM 4187/15569MB (lfb 4803MB) IRAM 0/128kB(lfb 128kB) SWAP  0/15568MB(cached 0MB) CPU [8/8] \"@2000MHz\" .... " +
			"VDD_CPU 952/1850mV 712mW VDD_GPU 4893/1850mV 624mW VDD_SOC 3014/1850mV 5109mW...  EMC_FREQ 100% 2133MHz " +
			"GR3D_FREQ 47% 670MHz @600MHz NVENC_FREQ 0%@702MHz NVDEC_FREQ 0%@676MHz SE_FREQ 0%@600MHz",
	}}
	src.smi = &fixtureRunner{script: map[string]string{
		"--query-compute-apps=pid,process_name,used_memory --format=csv,noheader,nounits": "4210, python3, 8123\n",
	}}
	if !src.Detect() {
		t.Fatal("expected jetson detect from L4T fixture")
	}
	s, err := src.Sample(nil)
	if err != nil {
		t.Fatal(err)
	}
	dm := s.Devices[0]
	if dm.UtilizationPct < 46 || dm.UtilizationPct > 48 {
		t.Errorf("expected util ~47 (GR3D_FREQ), got %f", dm.UtilizationPct)
	}
	if dm.ClockMHz != 670 {
		t.Errorf("expected clock 670, got %d", dm.ClockMHz)
	}
	if dm.VRAMUsed != 4187*1024*1024 {
		t.Errorf("expected vram used 4187MiB, got %d", dm.VRAMUsed)
	}
	if dm.PowerW < 0.6 || dm.PowerW > 0.7 {
		t.Errorf("expected power ~0.624 (VDD_GPU), got %f", dm.PowerW)
	}
	if len(s.Procs) != 1 {
		t.Fatalf("expected 1 proc from nvidia-smi fixture, got %d", len(s.Procs))
	}
	if s.Procs[0].PID != 4210 {
		t.Errorf("unexpected proc pid %d", s.Procs[0].PID)
	}
}

func TestJetsonTegrastatsMissingDegrades(t *testing.T) {
	src := NewJetsonSource()
	src.fs = newFixtureFS(t, map[string]string{
		"etc/nv_tegra_release": "R36 .4.0 release (PG905B), 36.4.0",
	})
	src.cli = &fixtureCLI{script: map[string]string{}}
	src.smi = &fixtureRunner{script: map[string]string{}}
	if !src.Detect() {
		t.Fatal("expected jetson detect (L4T present, tegrastats optional)")
	}
	s, err := src.Sample(nil)
	if err != nil {
		t.Fatalf("expected sample to succeed with missing tegrastats, got %v", err)
	}
	if s.Devices[0].UtilizationPct != -1 {
		t.Errorf("expected N/A util when tegrastats missing, got %f", s.Devices[0].UtilizationPct)
	}
}
