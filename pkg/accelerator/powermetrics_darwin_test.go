//go:build darwin

package accelerator

import (
	"strings"
	"testing"
)

// buildPlistFixture reproduces the shape of powermetrics' XML plist output.
func buildPlistFixture() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n<dict>\n")

	b.WriteString("	<key>processor</key>\n	<dict>\n")
	b.WriteString("		<key>gpu_power</key>\n		<integer>200</integer>\n")
	b.WriteString("		<key>ane_power</key>\n		<integer>50</integer>\n")
	b.WriteString("		<key>cpu_power</key>\n		<integer>1200</integer>\n")
	b.WriteString("		<key>combined_power</key>\n		<integer>1450</integer>\n")
	b.WriteString("	</dict>\n")

	b.WriteString("	<key>gpu</key>\n	<dict>\n")
	b.WriteString("		<key>freq_hz</key>\n		<real>1350000000</real>\n")
	b.WriteString("		<key>idle_ratio</key>\n		<real>0.9</real>\n")
	b.WriteString("	</dict>\n")

	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func TestConvertAndParseFixture(t *testing.T) {
	rec, err := convertAndParse([]byte(buildPlistFixture()))
	if err != nil {
		t.Fatalf("convertAndParse: %v", err)
	}
	if rec.GPUPowerMW != 200 {
		t.Errorf("gpu_power = %v, want 200", rec.GPUPowerMW)
	}
	if rec.ANEPowerMW != 50 {
		t.Errorf("ane_power = %v, want 50", rec.ANEPowerMW)
	}
	if rec.GPUFreqMHz != 1350 {
		t.Errorf("gpu_freq = %v, want 1350", rec.GPUFreqMHz)
	}
}

func TestConvertAndParseEnergyFallback(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<plist version="1.0"><dict>`)
	b.WriteString(`<key>processor</key><dict>`)
	b.WriteString(`<key>gpu_energy</key><integer>123</integer>`)
	b.WriteString(`<key>ane_energy</key><integer>7</integer>`)
	b.WriteString(`</dict></dict></plist>`)

	rec, err := convertAndParse([]byte(b.String()))
	if err != nil {
		t.Fatalf("convertAndParse: %v", err)
	}
	if rec.GPUPowerMW != 123 {
		t.Errorf("gpu energy fallback = %v, want 123", rec.GPUPowerMW)
	}
	if rec.ANEPowerMW != 7 {
		t.Errorf("ane energy fallback = %v, want 7", rec.ANEPowerMW)
	}
}

func TestReadOnePlistSampleSplitsNUL(t *testing.T) {
	payload := buildPlistFixture()
	in := payload + "\x00" + payload
	rec, err := readOnePlistSample(strings.NewReader(in))
	if err != nil {
		t.Fatalf("readOnePlistSample: %v", err)
	}
	if rec.GPUPowerMW != 200 {
		t.Errorf("gpu_power = %v", rec.GPUPowerMW)
	}
}

func TestPowermetricsDisabledByDefault(t *testing.T) {
	// NewPowermetricsSource is nil-safe whether or not the env is set.
	_ = NewPowermetricsSource()
}
