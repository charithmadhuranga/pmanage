//go:build linux

package accelerator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// kernelVersion returns the running kernel release (e.g. "6.7.9").
func kernelVersion() string {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

// kernelAtLeast reports whether the running kernel meets the minimum release
// (e.g. "6.7.0") used for full per-process fdinfo support. Only the first two
// numeric components are compared.
func kernelAtLeast(minRelease string) bool {
	cur := strings.Fields(kernelVersion())
	if len(cur) == 0 {
		return false
	}
	a := atoiTok(cur[0], 2)
	b := atoiTok(minRelease, 2)
	for i := range b {
		if a[i] > b[i] {
			return true
		}
		if a[i] < b[i] {
			return false
		}
	}
	return true
}

// atoiTok splits "6.7.9-arch1" into at most n leading numeric tokens.
func atoiTok(s string, n int) []int {
	var out []int
	for _, p := range strings.Split(s, ".") {
		if len(out) >= n {
			break
		}
		v, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		out = append(out, v)
	}
	return out
}

func sysfsString(classPath, filename string) (string, error) {
	data, err := os.ReadFile(filepath.Join(classPath, filename))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func sysfsUint64(classPath, filename string) (uint64, error) {
	s, err := sysfsString(classPath, filename)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(s, 10, 64)
}

func sysfsFloat64(classPath, filename string) (float64, error) {
	s, err := sysfsString(classPath, filename)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s/%s: %w", classPath, filename, err)
	}
	return v, nil
}

func drmCards() ([]string, error) {
	matches, err := filepath.Glob("/sys/class/drm/card[0-9]*")
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

func accelDevices() ([]string, error) {
	matches, err := filepath.Glob("/sys/class/accel/accel[0-9]*")
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

func drmDevicePath(cardPath string) (string, error) {
	// Prefer the renderD* device node (not the card* control node).
	renderGlob := strings.Replace(cardPath, "/sys/class/drm/card", "/dev/dri/renderD", 1)
	if _, err := os.Stat(renderGlob); err == nil {
		return renderGlob, nil
	}
	// Fallback: card device node.
	cardDev := strings.Replace(cardPath, "/sys/class/drm/card", "/dev/dri/card", 1)
	if _, err := os.Stat(cardDev); err == nil {
		return cardDev, nil
	}
	return "", fmt.Errorf("no device node for %s", cardPath)
}
