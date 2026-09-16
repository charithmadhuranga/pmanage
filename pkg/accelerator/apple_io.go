//go:build darwin

/*
Package-level C glue for Apple Silicon:
  - IOKit IOGPU PerformanceStatistics (aggregate GPU util + system memory).
  - Private IOReport subscriptions (modern API, present on macOS 13+; matched BY
    GROUP because channel names vary across M1/M2/M3):
  - "Energy Model"  -> "GPU Energy" (nJ)
  - "PMP"           -> "ANE" / "GPU" (mJ)
  - ANE identity via the H1xANELoadBalancer / H11ANEIn registry entries.

IOReport prototypes mirror macmon/oshi reverse-engineered exports (no legend API
any more); we dlopen/link -lIOReport which exists in the dyld shared cache.
*/
package accelerator

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation -lIOReport
#include <IOKit/IOKitLib.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

typedef struct __IOReportSubscription *IOReportSubscriptionRef;

#define MAX_ACCEL 8

typedef struct {
	char name[256];
	char model[256];
	int coreCount;
	double devUtil;
	double renUtil;
	double tilUtil;
	unsigned long long allocSysMem;
	unsigned long long inUseSysMem;
	int hasPerfStats;
} agpu_stat;

typedef struct {
	IOReportSubscriptionRef sub;
	CFMutableDictionaryRef subbed;
	int valid;
} agreport_sub;

static int cfNumberToSInt64(CFTypeRef v, long long *out) {
	if (!v || CFGetTypeID(v) != CFNumberGetTypeID()) return -1;
	// Try SInt64 first; fall back to SInt32 for values stored as int32
	// (common for ANE core count, small counters, etc.).
	if (CFNumberGetValue((CFNumberRef)v, kCFNumberSInt64Type, out)) return 0;
	int32_t i32 = 0;
	if (CFNumberGetValue((CFNumberRef)v, kCFNumberSInt32Type, &i32)) {
		*out = (long long)i32;
		return 0;
	}
	return -1;
}

static int cfDictionarySInt64(CFDictionaryRef dict, const char *key, long long *out) {
	CFStringRef k = CFStringCreateWithCString(kCFAllocatorDefault, key, kCFStringEncodingUTF8);
	if (!k) return -1;
	CFTypeRef v = CFDictionaryGetValue(dict, k);
	CFRelease(k);
	return cfNumberToSInt64(v, out);
}

static int agpu_count(void) {
	int n = 0;
	io_iterator_t iter;
	if (IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("IOGPU"), &iter) != KERN_SUCCESS)
		return 0;
	io_service_t svc;
	while ((svc = IOIteratorNext(iter)) != IO_OBJECT_NULL) {
		CFTypeRef ps = IORegistryEntryCreateCFProperty(svc, CFSTR("PerformanceStatistics"), kCFAllocatorDefault, 0);
		if (ps) { CFRelease(ps); n++; }
		IOObjectRelease(svc);
		if (n >= MAX_ACCEL) break;
	}
	IOObjectRelease(iter);
	return n;
}

static int agpu_read(int idx, agpu_stat *out) {
	memset(out, 0, sizeof(*out));
	io_iterator_t iter;
	if (IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("IOGPU"), &iter) != KERN_SUCCESS)
		return -1;
	io_service_t svc;
	int cur = 0;
	int ret = -1;
	while ((svc = IOIteratorNext(iter)) != IO_OBJECT_NULL) {
		if (cur == idx) {
			// Read model name
			CFTypeRef modelProp = IORegistryEntryCreateCFProperty(svc, CFSTR("model"), kCFAllocatorDefault, 0);
			if (modelProp && CFGetTypeID(modelProp) == CFStringGetTypeID()) {
				CFStringGetCString((CFStringRef)modelProp, out->model, sizeof(out->model)-1, kCFStringEncodingUTF8);
			}
			if (modelProp) CFRelease(modelProp);

			// Read gpu-core-count
			CFTypeRef coreProp = IORegistryEntryCreateCFProperty(svc, CFSTR("gpu-core-count"), kCFAllocatorDefault, 0);
			if (coreProp && CFGetTypeID(coreProp) == CFNumberGetTypeID()) {
				long long cc;
				if (CFNumberGetValue((CFNumberRef)coreProp, kCFNumberSInt64Type, &cc))
					out->coreCount = (int)cc;
			}
			if (coreProp) CFRelease(coreProp);

			CFTypeRef ps = IORegistryEntryCreateCFProperty(svc, CFSTR("PerformanceStatistics"), kCFAllocatorDefault, 0);
			if (ps && CFGetTypeID(ps) == CFDictionaryGetTypeID()) {
				out->hasPerfStats = 1;
				long long v;
				if (cfDictionarySInt64((CFDictionaryRef)ps, "Device Utilization %", &v) == 0) out->devUtil = (double)v;
				if (cfDictionarySInt64((CFDictionaryRef)ps, "Renderer Utilization %", &v) == 0) out->renUtil = (double)v;
				if (cfDictionarySInt64((CFDictionaryRef)ps, "Tiler Utilization %", &v) == 0) out->tilUtil = (double)v;
				if (cfDictionarySInt64((CFDictionaryRef)ps, "Alloc system memory", &v) == 0) out->allocSysMem = (unsigned long long)v;
				if (cfDictionarySInt64((CFDictionaryRef)ps, "In use system memory", &v) == 0) out->inUseSysMem = (unsigned long long)v;
				ret = 0;
			}
			if (ps) CFRelease(ps);
			char nm[256];
			if (IORegistryEntryGetName(svc, nm) == KERN_SUCCESS) {
				strncpy(out->name, nm, sizeof(out->name)-1);
			}
			IOObjectRelease(svc);
			break;
		}
		IOObjectRelease(svc);
		cur++;
	}
	IOObjectRelease(iter);
	return ret;
}

// ---- modern IOReport API (macOS 13+ shared-cache library) ----
// Guard every call behind a one-time dlsym check so the binary still
// loads on macOS <13 (or if the dylib is absent) without crashing.

#include <dlfcn.h>

extern CFMutableDictionaryRef IOReportCopyChannelsInGroup(CFStringRef group, CFStringRef subgroup, uint64_t a, uint64_t b, uint64_t c);
extern IOReportSubscriptionRef IOReportCreateSubscription(void *a, CFMutableDictionaryRef desiredChannels, CFMutableDictionaryRef *subbed, uint64_t b, CFTypeRef c);
extern CFDictionaryRef IOReportCreateSamples(IOReportSubscriptionRef s, CFMutableDictionaryRef ch, CFTypeRef a);
extern CFStringRef IOReportChannelGetChannelName(CFDictionaryRef item);
extern CFStringRef IOReportChannelGetUnitLabel(CFDictionaryRef item);
extern int64_t IOReportSimpleGetIntegerValue(CFDictionaryRef item, int ch);

static int ioReportAvailable(void) {
	static int checked = 0, ok = 0;
	if (!checked) {
		checked = 1;
		ok = dlsym(RTLD_DEFAULT, "IOReportCopyChannelsInGroup")  != NULL &&
		     dlsym(RTLD_DEFAULT, "IOReportCreateSubscription")   != NULL &&
		     dlsym(RTLD_DEFAULT, "IOReportCreateSamples")        != NULL &&
		     dlsym(RTLD_DEFAULT, "IOReportSimpleGetIntegerValue") != NULL;
	}
	return ok;
}

static agreport_sub agreport_open_group(const char *group) {
	agreport_sub r; memset(&r, 0, sizeof(r));
	if (!ioReportAvailable()) return r;
	CFStringRef g = CFStringCreateWithCString(kCFAllocatorDefault, group, kCFStringEncodingUTF8);
	if (!g) return r;
	CFMutableDictionaryRef chans = IOReportCopyChannelsInGroup(g, NULL, 0, 0, 0);
	CFRelease(g);
	if (!chans) return r;
	IOReportSubscriptionRef sub = IOReportCreateSubscription(NULL, chans, &r.subbed, 0, NULL);
	CFRelease(chans);
	if (!sub || !r.subbed) { if (sub) CFRelease(sub); memset(&r,0,sizeof(r)); return r; }
	r.sub = sub; r.valid = 1;
	return r;
}

static void agreport_close(agreport_sub *r) {
	if (r->valid) {
		if (r->sub) CFRelease(r->sub);
		if (r->subbed) CFRelease(r->subbed);
		memset(r, 0, sizeof(*r));
	}
}

// agreport_energy reads the FIRST channel whose exact name matches want and
// stores its value normalized to joules. Returns 0 found, -1 invalid sub,
// -2 channel not exposed.
static int agreport_energy(agreport_sub *r, const char *want, double *joules) {
	if (!r->valid) return -1;
	CFDictionaryRef samples = IOReportCreateSamples(r->sub, r->subbed, NULL);
	if (!samples) return -1;
	CFTypeRef arr = CFDictionaryGetValue(samples, CFSTR("IOReportChannels"));
	int rc = -2;
	if (arr && CFGetTypeID(arr) == CFArrayGetTypeID()) {
		CFIndex n = CFArrayGetCount((CFArrayRef)arr);
		for (CFIndex i = 0; i < n; i++) {
			CFDictionaryRef it = CFArrayGetValueAtIndex((CFArrayRef)arr, i);
			if (!it || CFGetTypeID(it) != CFDictionaryGetTypeID()) continue;
			CFStringRef name = IOReportChannelGetChannelName(it);
			if (!name) continue;
			char buf[128] = {0};
			if (!CFStringGetCString(name, buf, sizeof(buf), kCFStringEncodingUTF8)) continue;
			if (strcmp(buf, want) != 0) continue;
			double scale = 1.0;
			CFStringRef unit = IOReportChannelGetUnitLabel(it);
			if (unit) {
				char u[16] = {0};
				if (CFStringGetCString(unit, u, sizeof(u), kCFStringEncodingUTF8)) {
					if (strcmp(u, "nJ") == 0) scale = 1e-9;
					else if (strcmp(u, "uJ") == 0) scale = 1e-6;
					else if (strcmp(u, "mJ") == 0) scale = 1e-3;
				}
			}
			int64_t v = IOReportSimpleGetIntegerValue(it, 0);
			*joules = (double)v * scale;
			rc = 0;
			break;
		}
	}
	CFRelease(samples);
	return rc;
}

// ---- ANE identity ----

typedef struct {
	char name[128];
	char arch[64];
	char ver[64];
	int cores;
	int found;
} ane_stat;

// Locates the ANE power manager in the IOReport PLUGIN registry: H1xANELoadBalancer
// or H11ANEIn with DeviceProperties ANEDeviceProperty*.
// Tries both entries independently — some M1 variants only expose one.
static int ane_read(ane_stat *out) {
	memset(out, 0, sizeof(*out));
	int found = 0;

	// Try H1xANELoadBalancer first (ANE presence indicator).
	io_iterator_t iter = 0;
	if (IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("H1xANELoadBalancer"), &iter) == KERN_SUCCESS) {
		io_service_t svc;
		while ((svc = IOIteratorNext(iter)) != IO_OBJECT_NULL) {
			IOObjectRelease(svc);
			found = 1;
			break;
		}
		IOObjectRelease(iter);
	}

	// Try H11ANEIn for device properties (architecture, cores, version).
	if (IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("H11ANEIn"), &iter) == KERN_SUCCESS) {
		io_service_t svc;
		while ((svc = IOIteratorNext(iter)) != IO_OBJECT_NULL) {
			CFTypeRef props = IORegistryEntryCreateCFProperty(svc, CFSTR("DeviceProperties"), kCFAllocatorDefault, 0);
			if (props && CFGetTypeID(props) == CFDictionaryGetTypeID()) {
				long long v;
				if (cfDictionarySInt64((CFDictionaryRef)props, "ANEDevicePropertyNumANECores", &v) == 0) out->cores = (int)v;
				CFTypeRef arch = CFDictionaryGetValue((CFDictionaryRef)props, CFSTR("ANEDevicePropertyTypeANEArchitectureTypeStr"));
				CFTypeRef ver = CFDictionaryGetValue((CFDictionaryRef)props, CFSTR("ANEDevicePropertyANEVersion"));
				if (arch && CFGetTypeID(arch) == CFStringGetTypeID())
					CFStringGetCString((CFStringRef)arch, out->arch, sizeof(out->arch), kCFStringEncodingUTF8);
				if (ver && CFGetTypeID(ver) == CFNumberGetTypeID()) {
					long long vnum;
					if (CFNumberGetValue((CFNumberRef)ver, kCFNumberSInt64Type, &vnum))
						snprintf(out->ver, sizeof(out->ver), "%lld", vnum);
				} else if (ver && CFGetTypeID(ver) == CFStringGetTypeID()) {
					CFStringGetCString((CFStringRef)ver, out->ver, sizeof(out->ver), kCFStringEncodingUTF8);
				}
			}
			if (props) CFRelease(props);
			char nm[128];
			if (IORegistryEntryGetName(svc, nm) == KERN_SUCCESS) strncpy(out->name, nm, sizeof(out->name)-1);
			IOObjectRelease(svc);
			found = 1;
			break;
		}
		IOObjectRelease(iter);
	}

	// Fallback: check "AppleANEDevice" which some M1 variants expose.
	if (!found && IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("AppleANEDevice"), &iter) == KERN_SUCCESS) {
		io_service_t svc;
		while ((svc = IOIteratorNext(iter)) != IO_OBJECT_NULL) {
			IOObjectRelease(svc);
			found = 1;
			break;
		}
		IOObjectRelease(iter);
	}

	out->found = found;
	return found ? 0 : -1;
}

// ---- system memory (host_info) ----
// nvtop uses host_info(HOST_BASIC_INFO) for total unified memory on Apple Silicon.
// This is more accurate than gopsutil for GPU unified memory purposes.

#include <mach/mach_host.h>
#include <mach/mach_time.h>
#include <sys/sysctl.h>
#include <libproc.h>
#include <pwd.h>

typedef struct {
	unsigned long long totalBytes;
	int ok;
} apple_mem_info;

static apple_mem_info apple_system_memory(void) {
	apple_mem_info mem;
	memset(&mem, 0, sizeof(mem));
	mach_msg_type_number_t host_size = HOST_BASIC_INFO_COUNT;
	host_basic_info_data_t info;
	kern_return_t kr = host_info(mach_host_self(), HOST_BASIC_INFO, (host_info_t)&info, &host_size);
	if (kr == KERN_SUCCESS) {
		mem.totalBytes = info.max_mem;
		mem.ok = 1;
	}
	return mem;
}

// ---- per-process CPU/memory via proc_pidinfo ----
// nvtop uses proc_pidinfo(PROC_PIDTASKINFO) for CPU time and memory per process.

typedef struct {
	double totalUserTime;    // seconds
	double totalKernelTime;  // seconds
	unsigned long virtualMemory;
	unsigned long residentMemory;
	unsigned int cpuUsage;   // percentage
	int ok;
} apple_proc_info;

static apple_proc_info apple_process_info(int pid) {
	apple_proc_info info;
	memset(&info, 0, sizeof(info));
	struct proc_taskinfo task;
	int st = proc_pidinfo(pid, PROC_PIDTASKINFO, 0, &task, PROC_PIDTASKINFO_SIZE);
	if (st != PROC_PIDTASKINFO_SIZE) return info;

	// Convert Mach absolute time to seconds
	mach_timebase_info_data_t timebase;
	mach_timebase_info(&timebase);
	double ns_per_tick = (double)timebase.numer / (double)timebase.denom;

	info.totalUserTime = (task.pti_total_user * ns_per_tick) / 1e9;
	info.totalKernelTime = (task.pti_total_system * ns_per_tick) / 1e9;
	info.virtualMemory = task.pti_virtual_size;
	info.residentMemory = task.pti_resident_size;
	info.ok = 1;
	return info;
}

// ---- process username via proc_pidinfo ----
// nvtop uses proc_pidinfo(PROC_PIDT_SHORTBSDINFO) + getpwuid() for username.

static int apple_process_username(int pid, char *buf, int bufsize) {
	struct proc_bsdshortinfo bsd;
	int st = proc_pidinfo(pid, PROC_PIDT_SHORTBSDINFO, 0, &bsd, PROC_PIDT_SHORTBSDINFO_SIZE);
	if (st != PROC_PIDT_SHORTBSDINFO_SIZE) return -1;
	struct passwd *pw = getpwuid(bsd.pbsi_uid);
	if (!pw) return -1;
	strncpy(buf, pw->pw_name, bufsize - 1);
	buf[bufsize - 1] = '\0';
	return 0;
}

// ---- process command line via sysctl(KERN_PROCARGS2) ----
// nvtop uses sysctl(KERN_PROCARGS2) for full argument vector.

static int apple_process_command(int pid, char *buf, int bufsize) {
	int mib[3] = {CTL_KERN, KERN_PROCARGS2, pid};
	size_t argmax = 0;
	// First call to get size
	if (sysctl(mib, 3, NULL, &argmax, NULL, 0) != 0) return -1;
	if (argmax == 0) return -1;
	char *procargs = (char *)malloc(argmax);
	if (!procargs) return -1;
	if (sysctl(mib, 3, procargs, &argmax, NULL, 0) != 0) {
		free(procargs);
		return -1;
	}
	// First int is argc
	unsigned argc;
	memcpy(&argc, procargs, sizeof(argc));
	// Skip executable path
	size_t i = sizeof(argc);
	while (i < argmax && procargs[i] != 0) i++;
	// Skip null separators
	while (i < argmax && procargs[i] == 0) i++;
	// Copy args, replacing nulls with spaces
	int out = 0;
	for (unsigned arg = 0; arg < argc && i < argmax && out < bufsize - 1; arg++) {
		while (i < argmax && procargs[i] != 0 && out < bufsize - 1) {
			buf[out++] = procargs[i++];
		}
		if (out < bufsize - 1) buf[out++] = ' ';
		i++; // skip null
	}
	// Trim trailing space
	if (out > 0 && buf[out-1] == ' ') out--;
	buf[out] = '\0';
	free(procargs);
	return out > 0 ? 0 : -1;
}

// ---- per-process GPU via AGXDeviceUserClient AppUsage ----
// Activity Monitor's "GPU" column reads the same user-level data.

#define MAX_GPU_CLIENTS 512

typedef struct {
	int pid;
	char name[128];
	unsigned long long gpuTimeNs;
} agpu_client;

// Enumerates AGXDeviceUserClient children of the IOGPU service; each carries
// "IOUserClientCreator" ("pid N, name") and "AppUsage" (array of dicts with
// accumulatedGPUTime in ns). Returns the sum of accumulated GPU time per PID.
static int agpu_clients(agpu_client *out, int max) {
	int n = 0;
	io_iterator_t iter;
	if (IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("IOGPU"), &iter) != KERN_SUCCESS)
		return 0;
	io_service_t accel;
	while ((accel = IOIteratorNext(iter)) != IO_OBJECT_NULL) {
		io_iterator_t childIter = 0;
		IORegistryEntryCreateIterator(accel, kIOServicePlane, kIORegistryIterateRecursively, &childIter);
		io_service_t child;
		while (childIter && (child = IOIteratorNext(childIter)) != IO_OBJECT_NULL) {
			io_name_t cls;
			if (IOObjectGetClass(child, cls) != KERN_SUCCESS || !strstr(cls, "DeviceUserClient")) {
				IOObjectRelease(child);
				continue;
			}
			CFTypeRef creator = IORegistryEntryCreateCFProperty(child, CFSTR("IOUserClientCreator"), kCFAllocatorDefault, 0);
			if (!creator || CFGetTypeID(creator) != CFStringGetTypeID()) {
				if (creator) CFRelease(creator);
				IOObjectRelease(child);
				continue;
			}
			char buf[256] = {0};
			CFStringGetCString((CFStringRef)creator, buf, sizeof(buf), kCFStringEncodingUTF8);
			int pid = 0;
			char name[128] = {0};
			if (sscanf(buf, "pid %d, %127s", &pid, name) != 2) {
				CFRelease(creator);
				IOObjectRelease(child);
				continue;
			}
			CFTypeRef app = IORegistryEntryCreateCFProperty(child, CFSTR("AppUsage"), kCFAllocatorDefault, 0);
			unsigned long long tot = 0;
			if (app && CFGetTypeID(app) == CFArrayGetTypeID()) {
				CFIndex cnt = CFArrayGetCount((CFArrayRef)app);
				for (CFIndex i = 0; i < cnt; i++) {
					CFDictionaryRef d = CFArrayGetValueAtIndex((CFArrayRef)app, i);
					if (!d || CFGetTypeID(d) != CFDictionaryGetTypeID()) continue;
					CFTypeRef v = CFDictionaryGetValue(d, CFSTR("accumulatedGPUTime"));
					long long t = 0;
					if (v && CFGetTypeID(v) == CFNumberGetTypeID())
						CFNumberGetValue((CFNumberRef)v, kCFNumberSInt64Type, &t);
					tot += (unsigned long long)t;
				}
			}
			if (app) CFRelease(app);
			CFRelease(creator);

			// Merge into existing PID row (multiple user clients per process).
			int found = 0;
			for (int i = 0; i < n; i++) {
				if (out[i].pid == pid) { out[i].gpuTimeNs += tot; found = 1; break; }
			}
			if (!found && n < max && tot > 0) {
				out[n].pid = pid;
				strncpy(out[n].name, name, sizeof(out[n].name)-1);
				out[n].gpuTimeNs = tot;
				n++;
			}
			IOObjectRelease(child);
			if (n >= max) break;
		}
		if (childIter) IOObjectRelease(childIter);
		IOObjectRelease(accel);
		if (n >= max) break;
	}
	IOObjectRelease(iter);
	return n;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type appleGPUStat struct {
	Name        string
	Model       string
	CoreCount   int
	DevUtil     float64
	RenUtil     float64
	TilUtil     float64
	AllocSysMem uint64
	InUseSysMem uint64
	HasStats    bool
}

type appleEnergySub struct {
	h C.agreport_sub
}

func openAppleEnergy(group string) *appleEnergySub {
	cg := C.CString(group)
	s := &appleEnergySub{h: C.agreport_open_group(cg)}
	C.free(unsafe.Pointer(cg))
	if s.h.valid == 0 {
		return nil
	}
	return s
}

func (s *appleEnergySub) Close() {
	if s == nil {
		return
	}
	C.agreport_close(&s.h)
}

// Joules reads the cumulative energy consumed by the named channel since boot,
// normalized to joules. ok=false when the channel is not exposed.
func (s *appleEnergySub) Joules(channel string) (j float64, ok bool) {
	if s == nil || s.h.valid == 0 {
		return 0, false
	}
	var e C.double
	cw := C.CString(channel)
	rc := C.agreport_energy(&s.h, cw, &e)
	C.free(unsafe.Pointer(cw))
	if rc != 0 {
		return 0, false
	}
	return float64(e), true
}

type appleANEInfo struct {
	Name  string
	Arch  string
	Ver   string
	Cores int
	Found bool
}

func appleANEs() appleANEInfo {
	var c C.ane_stat
	if int(C.ane_read(&c)) != 0 {
		return appleANEInfo{}
	}
	return appleANEInfo{
		Name:  C.GoString(&c.name[0]),
		Arch:  C.GoString(&c.arch[0]),
		Ver:   C.GoString(&c.ver[0]),
		Cores: int(c.cores),
		Found: c.found != 0,
	}
}

func appleGPUs() []appleGPUStat {
	n := int(C.agpu_count())
	out := make([]appleGPUStat, 0, n)
	for i := 0; i < n; i++ {
		var c C.agpu_stat
		if int(C.agpu_read(C.int(i), &c)) != 0 {
			continue
		}
		model := C.GoString(&c.model[0])
		if model == "" {
			model = C.GoString(&c.name[0])
		}
		out = append(out, appleGPUStat{
			Name:        C.GoString(&c.name[0]),
			Model:       model,
			CoreCount:   int(c.coreCount),
			DevUtil:     float64(c.devUtil),
			RenUtil:     float64(c.renUtil),
			TilUtil:     float64(c.tilUtil),
			AllocSysMem: uint64(c.allocSysMem),
			InUseSysMem: uint64(c.inUseSysMem),
			HasStats:    c.hasPerfStats != 0,
		})
	}
	return out
}

func friendlyAppleName(native string) string {
	const prefix = "AGXAccelerator"
	if len(native) > len(prefix) && native[:len(prefix)] == prefix {
		return fmt.Sprintf("Apple Accelerator %s", native[len(prefix):])
	}
	return native
}

// appleGPUProcess tracks a process's cumulative GPU time (ns).
type appleGPUProcess struct {
	PID       int
	Name      string
	GPUTimeNS uint64
}

// appleGPUProcesses returns cumulative GPU time (since boot) per PID from the
// AGXDeviceUserClient AppUsage counters. Same source Activity Monitor's GPU
// column uses. Empty when unavailable.
func appleGPUProcesses() []appleGPUProcess {
	buf := make([]C.agpu_client, C.MAX_GPU_CLIENTS)
	n := int(C.agpu_clients((*C.agpu_client)(unsafe.Pointer(&buf[0])), C.MAX_GPU_CLIENTS))
	if n == 0 {
		return nil
	}
	out := make([]appleGPUProcess, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, appleGPUProcess{
			PID:       int(buf[i].pid),
			Name:      C.GoString(&buf[i].name[0]),
			GPUTimeNS: uint64(buf[i].gpuTimeNs),
		})
	}
	return out
}

// appleSystemMemory returns total unified memory in bytes via host_info(HOST_BASIC_INFO).
// More accurate than gopsutil for Apple Silicon GPU unified memory.
func appleSystemMemory() uint64 {
	mem := C.apple_system_memory()
	if mem.ok == 0 {
		return 0
	}
	return uint64(mem.totalBytes)
}

// appleProcessInfo returns per-process CPU time and memory via proc_pidinfo.
func appleProcessInfo(pid int) (userTime float64, kernelTime float64, virtMem uint64, residentMem uint64, ok bool) {
	info := C.apple_process_info(C.int(pid))
	if info.ok == 0 {
		return 0, 0, 0, 0, false
	}
	return float64(info.totalUserTime), float64(info.totalKernelTime),
		uint64(info.virtualMemory), uint64(info.residentMemory), true
}

// appleProcessUsername returns the username for a given PID.
func appleProcessUsername(pid int) string {
	var buf [256]C.char
	if C.apple_process_username(C.int(pid), &buf[0], C.int(len(buf))) != 0 {
		return ""
	}
	return C.GoString(&buf[0])
}

// appleProcessCommand returns the full command line for a given PID.
func appleProcessCommand(pid int) string {
	var buf [1024]C.char
	if C.apple_process_command(C.int(pid), &buf[0], C.int(len(buf))) != 0 {
		return ""
	}
	return C.GoString(&buf[0])
}
