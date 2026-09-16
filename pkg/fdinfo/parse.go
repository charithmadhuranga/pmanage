// Package fdinfo parses Linux DRM fdinfo files (/proc/<pid>/fdinfo/<fd>).
// The parsing is pure and cross-platform so it can be unit-tested anywhere;
// only the /proc walker is Linux-gated.
package fdinfo

import (
	"bufio"
	"strconv"
	"strings"
)

const (
	keyClientID  = "drm-client-id"
	keyEngine    = "drm-engine"
	keyMemory    = "drm-memory"
	totalKeysMax = 64
)

// ClientInfo is one aggregated DRM client (pid, client-id) sampled from fds.
type ClientInfo struct {
	Pid      int32             `json:"pid"`
	ClientID uint32            `json:"clientId"`
	EngineNS map[string]uint64 `json:"engineNs"` // engine name -> cumulative ns
	Memory   map[string]uint64 `json:"memory"`   // region -> bytes
}

// ParseFile parses the raw text of one fdinfo file.
func ParseFile(text string) (*ClientInfo, error) {
	info := &ClientInfo{
		EngineNS: make(map[string]uint64),
		Memory:   make(map[string]uint64),
	}

	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch {
		case key == keyClientID:
			id, err := strconv.ParseUint(value, 10, 32)
			if err != nil {
				return nil, err
			}
			info.ClientID = uint32(id)

		case strings.HasPrefix(key, keyEngine+"-"):
			name := strings.TrimPrefix(key, keyEngine+"-")
			// strip trailing client-id suffix, e.g. drm-engine-gfx-7
			name = normalizeEngineName(name)
			ns, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				continue
			}
			info.EngineNS[name] += ns

		case strings.HasPrefix(key, keyMemory+"-"):
			region := strings.TrimPrefix(key, keyMemory+"-")
			region = normalizeRegion(region)
			bytesN, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				continue
			}
			info.Memory[region] = bytesN
		}
	}
	return info, sc.Err()
}

// normalizeEngineName strips a trailing -<num> client-id distinguisher.
// Kernel emits drm-engine-<name> and also drm-engine-<name>-<client> when
// multiple clients share a device in one fd.
func normalizeEngineName(name string) string {
	if i := strings.LastIndexByte(name, '-'); i > 0 {
		if _, err := strconv.ParseUint(name[i+1:], 10, 32); err == nil {
			return name[:i]
		}
	}
	return name
}

func normalizeRegion(region string) string {
	if i := strings.LastIndexByte(region, '-'); i > 0 {
		if _, err := strconv.ParseUint(region[i+1:], 10, 32); err == nil {
			return region[:i]
		}
	}
	return region
}

// Merge folds a parsed fd into an existing aggregate (same pid+client).
func (c *ClientInfo) Merge(other *ClientInfo) {
	for k, v := range other.EngineNS {
		c.EngineNS[k] += v
	}
	for k, v := range other.Memory {
		c.Memory[k] += v
	}
}

func (c *ClientInfo) Empty() bool {
	return c.ClientID == 0 && len(c.EngineNS) == 0 && len(c.Memory) == 0
}