package processes

import (
	"strconv"
	"strings"
	"time"
)

// ParseLoadAvg reads the three load averages of /proc/loadavg.
func ParseLoadAvg(doc string) (l1, l5, l15 float64, ok bool) {
	f := strings.Fields(doc)
	if len(f) < 3 {
		return 0, 0, 0, false
	}
	var v [3]float64
	for i := range v {
		x, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			return 0, 0, 0, false
		}
		v[i] = x
	}
	return v[0], v[1], v[2], true
}

// ParseMemInfo reads MemTotal and MemAvailable (bytes) of /proc/meminfo;
// kernels before 3.14 lack MemAvailable, so MemFree stands in.
func ParseMemInfo(doc string) (total, avail uint64) {
	vals := map[string]uint64{}
	for _, l := range strings.Split(doc, "\n") {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		if len(f) == 0 {
			continue
		}
		n, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			continue
		}
		vals[k] = n * 1024 // the values are in kB
	}
	avail, ok := vals["MemAvailable"]
	if !ok {
		avail = vals["MemFree"]
	}
	return vals["MemTotal"], avail
}

// LifetimeCPU is the average CPU use in percent (100 = one core) of a
// process that used cpuSeconds over its age.
func LifetimeCPU(cpuSeconds float64, age time.Duration) float64 {
	if age <= 0 {
		return 0
	}
	return cpuSeconds / age.Seconds() * 100
}
