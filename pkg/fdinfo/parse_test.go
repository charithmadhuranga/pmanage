package fdinfo

import "testing"

// amdgpuFile is a captured k6.9 fdinfo tree for an AMD Radeon client.
const amdgpuFile = `pos:	0
flags:	02000000
mnt_id:	30
ino:	345
drm-client-id:	7
drm-total-memory-busy-0:	1048576
drm-engine-gfx:	784986021477
drm-engine-compute:	0
drm-engine-enc-0:	0
drm-engine-dec-0:	0
drm-engine-copy:	0
drm-memory-vram:	2147483648
drm-memory-gtt:	1048576
drm-memory-0:	1073741824`

// intelFile is a k6.7 i915 tree for a compute + media client.
const intelFile = `pos:	0
flags:	02004000
mnt_id:	41
ino:	1028
drm-client-id:	3
drm-engine-rendercopy:	15237893321
drm-engine-blitter:	1685412
drm-engine-video:	12998838
drm-engine-video-enhance:	0
drm-engine-balanced:	0
drm-memory-region-0:	67108864
drm-memory-region-1:	262144`

// xdnaFile is a k6.10 amdxdna NPU client.
const xdnaFile = `pos:	0
flags:	02000000
mnt_id:	52
ino:	17
drm-client-id:	11
drm-engine-amdxdna:	4891234567
drm-memory-0:	104857600
drm-memory-1:	0`

// partialFile is the k5.15 era format: no client-id, engine time only.
const partialFile = `pos:	0
flags:	02000002
mnt_id:	21
ino:	733
drm-engine-gfx:	123456789
drm-memory-vram:	536870912`

func TestParseAmdgpu(t *testing.T) {
	ci, err := ParseFile(amdgpuFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ci.ClientID != 7 {
		t.Fatalf("client id: got %d want 7", ci.ClientID)
	}
	if ci.EngineNS["gfx"] != 784986021477 {
		t.Fatalf("gfx engine: got %d", ci.EngineNS["gfx"])
	}
	if ci.Memory["vram"] != 2147483648 {
		t.Fatalf("vram: got %d", ci.Memory["vram"])
	}
	if ci.Memory["gtt"] != 1048576 {
		t.Fatalf("gtt: got %d", ci.Memory["gtt"])
	}
}

func TestParseIntel(t *testing.T) {
	ci, err := ParseFile(intelFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ci.EngineNS["rendercopy"] != 15237893321 {
		t.Fatalf("rendercopy: got %d", ci.EngineNS["rendercopy"])
	}
	if ci.EngineNS["video"] != 12998838 {
		t.Fatalf("video: got %d", ci.EngineNS["video"])
	}
}

func TestParseXdna(t *testing.T) {
	ci, err := ParseFile(xdnaFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ci.EngineNS["amdxdna"] != 4891234567 {
		t.Fatalf("amdxdna: got %d", ci.EngineNS["amdxdna"])
	}
	if ci.ClientID != 11 {
		t.Fatalf("client id: got %d", ci.ClientID)
	}
}

func TestParsePartialFormat(t *testing.T) {
	// k5.15: no drm-client-id, still must surface engine/memory.
	ci, err := ParseFile(partialFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ci.EngineNS["gfx"] != 123456789 {
		t.Fatalf("gfx: got %d", ci.EngineNS["gfx"])
	}
	if ci.Memory["vram"] != 536870912 {
		t.Fatalf("vram: got %d", ci.Memory["vram"])
	}
	if ci.ClientID != 0 {
		t.Fatalf("expected 0 client id on partial format")
	}
}

func TestParseSuffixNormalization(t *testing.T) {
	// drm-engine-* and drm-memory-* with trailing client-id indexes aggregate.
	text := `drm-client-id:	7
drm-engine-gfx-7:	1000
drm-engine-compute-7:	2000
drm-memory-vram-7:	4096
drm-memory-gtt-7:	512
`
	ci, err := ParseFile(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ci.EngineNS["gfx"] != 1000 {
		t.Fatalf("gfx: got %d", ci.EngineNS["gfx"])
	}
	if ci.EngineNS["compute"] != 2000 {
		t.Fatalf("compute: got %d", ci.EngineNS["compute"])
	}
	if ci.Memory["vram"] != 4096 || ci.Memory["gtt"] != 512 {
		t.Fatalf("mem: %+v", ci.Memory)
	}
}

func TestParseIgnoresNonDrm(t *testing.T) {
	ci, err := ParseFile("pid:\t123\nfd:\t4\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !ci.Empty() {
		t.Fatalf("expected empty parse: %+v", ci)
	}
}

func TestParseMerge(t *testing.T) {
	a, err := ParseFile(amdgpuFile)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseFile(`drm-client-id:	7
drm-engine-gfx:	500000000
drm-memory-gtt:	1024
`)
	if err != nil {
		t.Fatal(err)
	}
	a.Merge(b)
	if a.EngineNS["gfx"] != 784986021477+500000000 {
		t.Fatalf("merge gfx: got %d", a.EngineNS["gfx"])
	}
	if a.Memory["gtt"] != 1048576+1024 {
		t.Fatalf("merge gtt: got %d", a.Memory["gtt"])
	}
}