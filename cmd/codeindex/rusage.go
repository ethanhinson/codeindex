package main

import (
	"runtime"
	"syscall"
)

// peakRSSMB reports this process's peak resident set size in MB.
// ru_maxrss is bytes on darwin and kilobytes on linux (verified empirically
// against /usr/bin/time -l on darwin).
func peakRSSMB() float64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return float64(ru.Maxrss) / 1e6
	}
	return float64(ru.Maxrss) / 1e3
}
