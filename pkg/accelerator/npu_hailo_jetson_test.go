package accelerator

import (
	"strings"
	"testing"
)

func TestHailoDetectFromFixture(t *testing.T) {
	src := NewHailoSource()
	src.cli = &fixtureCLI{script: map[string]string{
		"scan": "Hailo Devices:\n[-] Device: 0000:01:00.0\n",
		"fw-control identify --extended": "" +
			"Identifying board\n" +
			"Control Protocol Version: 2.11\n" +
			"Firmware Version: 4.17.0 (release,app)\n" +
			"Logger Version: 0\n" +
			"Device Architecture: HAILO8\n" +
			"Serial Number: AB12345678\n" +
			"Part Number: HAILO-8M.2\n" +
			"Product Name: Hailo-8\n" +
			"Neural Network Core Clock Rate: 1000MHz\n" +
			"Boot source: PCIE\n" +
			"Device supported features: PCIE, Power Measurement\n",
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
	if devs[0].BusInfo != "0000:01:00.0" {
		t.Errorf("expected bus info 0000:01:00.0, got %q", devs[0].BusInfo)
	}
	if !strings.Contains(devs[0].Version, "4.17.0") {
		t.Errorf("expected firmware version in Version field, got %q", devs[0].Version)
	}
}

func TestHailoDetectArchitectures(t *testing.T) {
	tests := []struct {
		arch     string
		wantName string
	}{
		{"HAILO8", "Hailo-8"},
		{"HAILO8L", "Hailo-8L"},
		{"HAILO10H", "Hailo-10H"},
		{"HAILO15H", "Hailo-15H"},
		{"HAILO15L", "Hailo-15L"},
		{"HAILO15M", "Hailo-15M"},
		{"HAILO12L", "Hailo-12L (Mars)"},
		{"MARS", "Hailo-12L (Mars)"},
	}
	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			name := hailoArchName(tt.arch, "")
			if name != tt.wantName {
				t.Errorf("hailoArchName(%q) = %q, want %q", tt.arch, name, tt.wantName)
			}
		})
	}
}

func TestHailoDetectProductNameOverride(t *testing.T) {
	name := hailoArchName("HAILO8", "My Custom Hailo Device")
	if name != "My Custom Hailo Device" {
		t.Errorf("expected product name override, got %q", name)
	}
}

func TestHailoDetectNoCLI(t *testing.T) {
	src := NewHailoSource()
	src.cli = &fixtureCLI{script: map[string]string{}}
	if src.Detect() {
		t.Fatal("expected detect to fail without hailortcli fixture")
	}
}

func TestHailoDetectFallbackNoExtended(t *testing.T) {
	src := NewHailoSource()
	src.cli = &fixtureCLI{script: map[string]string{
		"scan": "Hailo Devices:\n[-] Device: 0000:01:00.0\n",
		// No --extended variant, only basic identify
		"fw-control identify": "" +
			"Identifying board\n" +
			"Control Protocol Version: 2.11\n" +
			"Firmware Version: 3.10.0 (release,app)\n" +
			"Device Architecture: HAILO8L\n",
	}}
	if !src.Detect() {
		t.Fatal("expected detect to succeed with basic identify fallback")
	}
	devs := src.Devices()
	if len(devs) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devs))
	}
	if !strings.Contains(devs[0].Name, "Hailo-8L") {
		t.Errorf("expected Hailo-8L name, got %q", devs[0].Name)
	}
}

func TestHailoSampleParsesMonitorTable(t *testing.T) {
	src := NewHailoSource()
	src.cli = &fixtureCLI{script: map[string]string{
		"scan": "Hailo Devices:\n[-] Device: 0000:01:00.0\n",
		"fw-control identify --extended": "" +
			"Identifying board\n" +
			"Device Architecture: HAILO10H\n" +
			"Firmware Version: 4.17.0 (release,app)\n" +
			"Neural Network Core Clock Rate: 1200MHz\n",
		"monitor --interval=1000 --no-clear-screen": "" +
			"Device ID            Architecture   NNC Utilization (%)    CPU Utilization (%)    RAM Utilization (%)    RAM Usage (MB)  On Die Temperature (C) On Die Voltage (mV)   \n" +
			"--------------------- --------------- ----------------------- ----------------------- ----------------------- ---------------- ----------------------- ----------------------\n" +
			"hailo0               HAILO10H       53.2                   12.1                   67.5                   512 / 768       64.5                   850.0                 \n",
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
		t.Errorf("expected NNC util ~53.2, got %f", dm.UtilizationPct)
	}
	if dm.TemperatureC != 64.5 {
		t.Errorf("expected temp 64.5, got %f", dm.TemperatureC)
	}
	if dm.VRAMUsed != 512*1024*1024 {
		t.Errorf("expected VRAM used 512MB, got %d", dm.VRAMUsed)
	}
	if dm.VRAMTotal != 768*1024*1024 {
		t.Errorf("expected VRAM total 768MB, got %d", dm.VRAMTotal)
	}
	if dm.ClockMHz != 1200 {
		t.Errorf("expected clock 1200MHz from identify, got %d", dm.ClockMHz)
	}
}

func TestHailoSampleParsesJSONLine(t *testing.T) {
	src := NewHailoSource()
	src.cli = &fixtureCLI{script: map[string]string{
		"scan": "Hailo Devices:\n[-] Device: 0000:01:00.0\n",
		"fw-control identify --extended": "" +
			"Identifying board\n" +
			"Device Architecture: HAILO8\n" +
			"Firmware Version: 3.10.0 (release,app)\n",
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
}

func TestHailoMonitorFailStillSamples(t *testing.T) {
	src := NewHailoSource()
	src.cli = &fixtureCLI{script: map[string]string{
		"scan": "Hailo Devices:\n[-] Device: 0000:01:00.0\n",
		"fw-control identify --extended": "" +
			"Identifying board\n" +
			"Device Architecture: HAILO8\n",
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

func TestHailoMultiDeviceDetect(t *testing.T) {
	src := NewHailoSource()
	src.cli = &fixtureCLI{script: map[string]string{
		"scan": "Hailo Devices:\n[-] Device: 0000:01:00.0\n[-] Device: 0000:02:00.0\n",
		"fw-control identify --extended": "" +
			"Identifying board\n" +
			"Device Architecture: HAILO8\n" +
			"Firmware Version: 4.17.0 (release,app)\n" +
			"Neural Network Core Clock Rate: 1000MHz\n" +
			"Identifying board\n" +
			"Device Architecture: HAILO10H\n" +
			"Firmware Version: 4.17.0 (release,app)\n" +
			"Neural Network Core Clock Rate: 1200MHz\n",
	}}
	if !src.Detect() {
		t.Fatal("detect failed")
	}
	devs := src.Devices()
	if len(devs) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devs))
	}
	if !strings.Contains(devs[0].Name, "Hailo-8") {
		t.Errorf("expected first device to be Hailo-8, got %q", devs[0].Name)
	}
	if !strings.Contains(devs[1].Name, "Hailo-10H") {
		t.Errorf("expected second device to be Hailo-10H, got %q", devs[1].Name)
	}
}

func TestHailoParseClockMHz(t *testing.T) {
	tests := []struct {
		input string
		want  uint32
	}{
		{"1000MHz", 1000},
		{"1000 MHz", 1000},
		{"1200MHz", 1200},
		{"", 0},
		{"invalid", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseClockMHz(tt.input)
			if got != tt.want {
				t.Errorf("parseClockMHz(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
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
