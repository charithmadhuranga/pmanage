package accelerator

import (
	"strings"
	"testing"
)

type fixtureRunner struct {
	script map[string]string // exact key match on joined args -> output
}

func (f *fixtureRunner) run(args ...string) (string, error) {
	key := strings.Join(args, " ")
	if out, ok := f.script[key]; ok {
		return out, nil
	}
	return "", &fixtureErr{key: key}
}

type fixtureErr struct{ key string }

func (e *fixtureErr) Error() string { return "no fixture for: " + e.key }

func fixtureSMIScript() map[string]string {
	return map[string]string{
		"--query-gpu=index,name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw,clocks.gr,clocks.max.gr,fan.speed --format=csv,noheader,nounits": "" +
			"0, NVIDIA GeForce RTX 4090, 42, 12345, 24564, 61, 195.43, 2520, 2520, 45\n" +
			"1, NVIDIA RTX A6000, 5, 4096, 49140, 48, 55.10, 1440, 1860, 0\n",
		"--query-compute-apps=pid,process_name,used_memory --format=csv,noheader,nounits": "" +
			"4210, python3, 8123\n" +
			"9183, ffmpeg, 0\n",
	}
}

func TestSmiSourceDetectFromFixture(t *testing.T) {
	src := NewNvidiaSmiSource()
	src.runner = &fixtureRunner{script: fixtureSMIScript()}

	if !src.Detect() {
		t.Fatal("expected fixture-based detect to succeed")
	}
	devs := src.Devices()
	if len(devs) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devs))
	}
	if devs[0].ID != "gpu0" || devs[1].ID != "gpu1" {
		t.Errorf("unexpected device ids: %q, %q", devs[0].ID, devs[1].ID)
	}
	if devs[0].Name != "NVIDIA GeForce RTX 4090" {
		t.Errorf("unexpected name: %q", devs[0].Name)
	}
}

func TestSmiSourceSampleFromFixture(t *testing.T) {
	src := NewNvidiaSmiSource()
	src.runner = &fixtureRunner{script: fixtureSMIScript()}
	if !src.Detect() {
		t.Fatal("detect failed")
	}

	sample, err := src.Sample(nil)
	if err != nil {
		t.Fatalf("sample failed: %v", err)
	}
	if len(sample.Devices) != 2 {
		t.Fatalf("expected 2 device samples, got %d", len(sample.Devices))
	}

	gpu0 := sample.Devices[0]
	if gpu0.UtilizationPct != 42 {
		t.Errorf("expected util 42, got %f", gpu0.UtilizationPct)
	}
	if gpu0.VRAMUsed != 12345*1024*1024 {
		t.Errorf("expected vram 12345MiB, got %d", gpu0.VRAMUsed)
	}
	if gpu0.PowerW != 195.43 {
		t.Errorf("expected power 195.43, got %f", gpu0.PowerW)
	}
	if gpu0.ClockMHz != 2520 {
		t.Errorf("expected clock 2520, got %d", gpu0.ClockMHz)
	}
	if gpu0.FanSpeedPct != 45 {
		t.Errorf("expected fan 45, got %f", gpu0.FanSpeedPct)
	}

	gpu1 := sample.Devices[1]
	if gpu1.UtilizationPct != 5 {
		t.Errorf("expected gpu1 util 5, got %f", gpu1.UtilizationPct)
	}

	if len(sample.Procs) != 2 {
		t.Fatalf("expected 2 procs, got %d", len(sample.Procs))
	}
	if sample.Procs[0].PID != 4210 || sample.Procs[0].Name != "python3" {
		t.Errorf("unexpected proc[0]: %+v", sample.Procs[0])
	}
	if sample.Procs[0].VRAMUsed != 8123*1024*1024 {
		t.Errorf("expected proc vram 8123MiB, got %d", sample.Procs[0].VRAMUsed)
	}
}

func TestSmiSourceMissingBinary(t *testing.T) {
	src := NewNvidiaSmiSource()
	src.runner = &fixtureRunner{script: map[string]string{}}
	if src.Detect() {
		t.Fatal("expected detect to fail without a fixture")
	}
}

func TestSmiSourceNAValues(t *testing.T) {
	src := NewNvidiaSmiSource()
	src.runner = &fixtureRunner{script: map[string]string{
		"--query-gpu=index,name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw,clocks.gr,clocks.max.gr,fan.speed --format=csv,noheader,nounits": "" +
			"0, NVIDIA GeForce RTX 4090, [N/A], [N/A], 24564, [N/A], [N/A], [N/A], 2520, [N/A]\n",
		"--query-compute-apps=pid,process_name,used_memory --format=csv,noheader,nounits": "",
	}}
	if !src.Detect() {
		t.Fatal("detect failed")
	}
	sample, err := src.Sample(nil)
	if err != nil {
		t.Fatal(err)
	}
	if sample.Devices[0].UtilizationPct != 0 {
		t.Errorf("expected 0 util for [N/A], got %f", sample.Devices[0].UtilizationPct)
	}
	if sample.Devices[0].Status != "ok" {
		t.Errorf("expected status ok, got %q", sample.Devices[0].Status)
	}
}
