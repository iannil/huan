package main

// profile.go: opt-in CPU profiling for build performance work.
// Enabled only when HUAN_CPUPROFILE names an output file, so normal builds
// pay nothing (a nil check in defer).

import (
	"fmt"
	"os"
	"runtime/pprof"
)

// startCPUProfile starts a CPU profile when HUAN_CPUPROFILE is set.
// The returned stop function is always safe to call.
func startCPUProfile() func() {
	path := os.Getenv("HUAN_CPUPROFILE")
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cpuprofile: %v\n", err)
		return func() {}
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		fmt.Fprintf(os.Stderr, "cpuprofile: %v\n", err)
		_ = f.Close()
		return func() {}
	}
	return func() {
		pprof.StopCPUProfile()
		_ = f.Close()
	}
}
