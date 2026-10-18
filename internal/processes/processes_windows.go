package processes

import (
	"context"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procMemoryInfo       = kernel32.NewProc("K32GetProcessMemoryInfo")
	procGlobalMemoryStat = kernel32.NewProc("GlobalMemoryStatusEx")
)

type memoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// Sample returns the working set and the CPU use (average over the
// process's lifetime) of the given PIDs; others are ignored. CPUPercent
// measures the current use over an interval instead.
func Sample(_ context.Context, pids []int) map[int]Stat {
	out := map[int]Stat{}
	now := time.Now()
	for _, pid := range pids {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
		if err != nil {
			continue
		}
		var mc memoryCounters
		mc.CB = uint32(unsafe.Sizeof(mc))
		if r, _, _ := procMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&mc)), uintptr(mc.CB)); r != 0 {
			s := Stat{PID: pid, RSSKB: int64(mc.WorkingSetSize / 1024)}
			var created, exited, kernel, user windows.Filetime
			if windows.GetProcessTimes(h, &created, &exited, &kernel, &user) == nil {
				s.CPU = LifetimeCPU(filetimeSeconds(kernel)+filetimeSeconds(user), now.Sub(time.Unix(0, created.Nanoseconds())))
			}
			out[pid] = s
		}
		_ = windows.CloseHandle(h)
	}
	return out
}

// filetimeSeconds converts a FILETIME duration (100 ns units) to seconds.
func filetimeSeconds(f windows.Filetime) float64 {
	return float64(uint64(f.HighDateTime)<<32|uint64(f.LowDateTime)) / 1e7
}

// CountClaude returns the number of running claude.exe processes.
func CountClaude(context.Context) int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return -1
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	n := 0
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), "claude.exe") {
			n++
		}
	}
	return n
}

// SystemSummary reads physical memory; Windows has no load average.
func SystemSummary(context.Context) System {
	var s System
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGlobalMemoryStat.Call(uintptr(unsafe.Pointer(&m))); r != 0 {
		s.MemTotal, s.MemFree, s.OK = m.TotalPhys, m.AvailPhys, m.TotalPhys > 0
	}
	return s
}

func cpuTimes(_ context.Context, pids []int) map[int]float64 {
	out := map[int]float64{}
	for _, pid := range pids {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
		if err != nil {
			continue
		}
		var created, exited, kernel, user windows.Filetime
		if windows.GetProcessTimes(h, &created, &exited, &kernel, &user) == nil {
			out[pid] = filetimeSeconds(kernel) + filetimeSeconds(user)
		}
		_ = windows.CloseHandle(h)
	}
	return out
}
