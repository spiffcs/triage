package cmd

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
)

// Profiler manages CPU, memory, and trace profiling.
type Profiler struct {
	cpuFile   *os.File
	traceFile *os.File

	cpuProfile string
	memProfile string
	tracePath  string
}

// NewProfiler creates a new profiler with the specified profile paths.
// Empty paths disable the corresponding profile.
func NewProfiler(cpuProfile, memProfile, tracePath string) *Profiler {
	return &Profiler{
		cpuProfile: cpuProfile,
		memProfile: memProfile,
		tracePath:  tracePath,
	}
}

// Start begins CPU profiling and execution tracing if configured.
func (p *Profiler) Start() error {
	if p.cpuProfile != "" {
		f, err := os.Create(p.cpuProfile)
		if err != nil {
			return fmt.Errorf("could not create CPU profile: %w", err)
		}
		p.cpuFile = f
		if err := pprof.StartCPUProfile(f); err != nil {
			p.cpuFile.Close()
			p.cpuFile = nil
			return fmt.Errorf("could not start CPU profile: %w", err)
		}
	}

	if p.tracePath != "" {
		f, err := os.Create(p.tracePath)
		if err != nil {
			p.stopCPU()
			return fmt.Errorf("could not create trace: %w", err)
		}
		p.traceFile = f
		if err := trace.Start(f); err != nil {
			p.traceFile.Close()
			p.traceFile = nil
			p.stopCPU()
			return fmt.Errorf("could not start trace: %w", err)
		}
	}

	return nil
}

// Stop ends all profiling and writes memory profile if configured.
func (p *Profiler) Stop() {
	// Stop trace first
	if p.traceFile != nil {
		trace.Stop()
		if err := p.traceFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "could not close trace file: %v\n", err)
		}
		p.traceFile = nil
	}

	// Stop CPU profiling
	p.stopCPU()

	// Write memory profile
	if p.memProfile != "" {
		f, err := os.Create(p.memProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not create memory profile: %v\n", err)
			return
		}
		defer func() {
			if err := f.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "could not close memory profile file: %v\n", err)
			}
		}()
		runtime.GC() // Get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "could not write memory profile: %v\n", err)
		}
	}
}

func (p *Profiler) stopCPU() {
	if p.cpuFile != nil {
		pprof.StopCPUProfile()
		if err := p.cpuFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "could not close CPU profile file: %v\n", err)
		}
		p.cpuFile = nil
	}
}
