//go:build linux

package fdinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// drmDevPrefixes are the device nodes whose fd leaks reveal GPU clients.
var drmDevPrefixes = []string{"/dev/dri/", "/dev/accel/"}

// Walker scans /proc for processes holding DRM client fds and aggregates
// per-process clients. Root is overridable for tests.
type Walker struct {
	Root string // typically "/proc"
}

// Result is one sample of all live DRM clients.
type Result struct {
	Clients []ClientInfo `json:"clients"`
}

func (w Walker) Walk() (Result, error) {
	procRoot := filepath.Join(w.Root, "proc")
	entries, err := os.ReadDir(w.Root + "/proc")
	if err != nil {
		entries, err = os.ReadDir(procRoot)
	}
	if err != nil {
		return Result{}, fmt.Errorf("read proc root: %w", err)
	}

	var (
		mu      sync.Mutex
		clients = make(map[int32]*ClientInfo)
		wg      sync.WaitGroup
	)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.ParseInt(e.Name(), 10, 32)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func(pid int32) {
			defer wg.Done()
			row := w.walkPid(pid)
			if len(row) == 0 {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, ci := range row {
				key := pid
				agg, ok := clients[key]
				if !ok {
					agg = &ClientInfo{
						Pid:      pid,
						ClientID: ci.ClientID,
						EngineNS: make(map[string]uint64),
						Memory:   make(map[string]uint64),
					}
					clients[key] = agg
				}
				agg.Merge(&ci)
			}
		}(int32(pid))
	}
	wg.Wait()

	out := make([]ClientInfo, 0, len(clients))
	for _, c := range clients {
		if !c.Empty() {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pid < out[j].Pid })
	return Result{Clients: out}, nil
}

func (w Walker) walkPid(pid int32) []ClientInfo {
	root := w.Root
	if root == "" {
		root = "/proc"
	}
	fdDir := fmt.Sprintf("%s/%d/fd", root, pid)
	fds, err := os.ReadDir(fdDir)
	if err != nil {
		return nil
	}

	var clients []*ClientInfo
	for _, fd := range fds {
		link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
		if err != nil || !isDrmNode(link) {
			continue
		}
		ci, err := readFdInfo(root, pid, fd.Name())
		if err != nil {
			continue
		}
		ci.Pid = pid
		clients = append(clients, ci)
	}
	if len(clients) == 0 {
		return nil
	}

	// Merge fds belonging to the same client id.
	var merged []ClientInfo
	for _, ci := range clients {
		found := false
		for i := range merged {
			if merged[i].ClientID == ci.ClientID {
				merged[i].Merge(ci)
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, ClientInfo{
				Pid:      pid,
				ClientID: ci.ClientID,
				EngineNS: cloneMap(ci.EngineNS),
				Memory:   cloneMap(ci.Memory),
			})
		}
	}
	return merged
}

func isDrmNode(link string) bool {
	for _, p := range drmDevPrefixes {
		if strings.HasPrefix(link, p) {
			return true
		}
	}
	return false
}

func cloneMap(in map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func readFdInfo(root string, pid int32, fd string) (*ClientInfo, error) {
	data, err := os.ReadFile(fmt.Sprintf("%s/%d/fdinfo/%s", root, pid, fd))
	if err != nil {
		return nil, err
	}
	return ParseFile(string(data))
}