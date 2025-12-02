// Package metrics provides system statistics collection
package metrics

import (
	"runtime"
	"sync/atomic"
	"time"
)

// SystemStats holds system resource statistics
type SystemStats struct {
	// Memory stats
	HeapAlloc  uint64 // Current heap allocation in bytes
	HeapSys    uint64 // Total heap memory from OS
	HeapInuse  uint64 // Heap memory in use
	StackInuse uint64 // Stack memory in use
	TotalAlloc uint64 // Total allocations (cumulative)
	PeakMemory uint64 // Peak memory usage

	// GC stats
	NumGC        uint32 // Number of GC cycles
	GCPauseTotal uint64 // Total GC pause time in nanoseconds
	LastGCPause  uint64 // Last GC pause time

	// Runtime stats
	NumGoroutine int // Number of goroutines
	NumCPU       int // Number of CPUs
	GOMAXPROCS   int // GOMAXPROCS setting
}

// SystemCollector collects system-level statistics
type SystemCollector struct {
	startTime  time.Time
	lastSample time.Time

	// Peak tracking
	peakMemory atomic.Uint64

	// Cumulative stats
	totalSamples atomic.Uint64
	memorySum    atomic.Uint64
}

// NewSystemCollector creates a new system stats collector
func NewSystemCollector() *SystemCollector {
	return &SystemCollector{
		startTime:  time.Now(),
		lastSample: time.Now(),
	}
}

// Collect samples current system statistics
func (sc *SystemCollector) Collect() SystemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Update peak memory
	for {
		peak := sc.peakMemory.Load()
		if m.Alloc <= peak || sc.peakMemory.CompareAndSwap(peak, m.Alloc) {
			break
		}
	}

	// Update cumulative stats
	sc.totalSamples.Add(1)
	sc.memorySum.Add(m.Alloc)
	sc.lastSample = time.Now()

	return SystemStats{
		HeapAlloc:    m.Alloc,
		HeapSys:      m.Sys,
		HeapInuse:    m.HeapInuse,
		StackInuse:   m.StackInuse,
		TotalAlloc:   m.TotalAlloc,
		PeakMemory:   sc.peakMemory.Load(),
		NumGC:        m.NumGC,
		GCPauseTotal: m.PauseTotalNs,
		LastGCPause:  m.PauseNs[(m.NumGC+255)%256],
		NumGoroutine: runtime.NumGoroutine(),
		NumCPU:       runtime.NumCPU(),
		GOMAXPROCS:   runtime.GOMAXPROCS(0),
	}
}

// GetAverageMemory returns the average memory usage in bytes
func (sc *SystemCollector) GetAverageMemory() float64 {
	samples := sc.totalSamples.Load()
	if samples == 0 {
		return 0
	}
	return float64(sc.memorySum.Load()) / float64(samples)
}

// GetPeakMemory returns the peak memory usage in bytes
func (sc *SystemCollector) GetPeakMemory() uint64 {
	return sc.peakMemory.Load()
}

// GetUptime returns the time since collector started
func (sc *SystemCollector) GetUptime() time.Duration {
	return time.Since(sc.startTime)
}

// Reset clears all collected statistics
func (sc *SystemCollector) Reset() {
	sc.startTime = time.Now()
	sc.lastSample = time.Now()
	sc.peakMemory.Store(0)
	sc.totalSamples.Store(0)
	sc.memorySum.Store(0)
}

// FormatBytes formats bytes to human-readable string
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return formatUint(b) + " B"
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return formatFloat(float64(b)/float64(div)) + " " + string("KMGTPE"[exp]) + "B"
}

func formatUint(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func formatFloat(f float64) string {
	// Simple formatting without fmt package for performance
	if f < 10 {
		return formatFloatPrecision(f, 2)
	} else if f < 100 {
		return formatFloatPrecision(f, 1)
	}
	return formatFloatPrecision(f, 0)
}

func formatFloatPrecision(f float64, precision int) string {
	// Multiply to get precision, then format
	multiplier := 1.0
	for i := 0; i < precision; i++ {
		multiplier *= 10
	}

	val := int64(f*multiplier + 0.5)

	if precision == 0 {
		return formatInt64(val)
	}

	intPart := val / int64(multiplier)
	fracPart := val % int64(multiplier)

	result := formatInt64(intPart) + "."

	// Pad fractional part with leading zeros
	for i := precision - 1; i >= 0; i-- {
		div := int64(1)
		for j := 0; j < i; j++ {
			div *= 10
		}
		if fracPart < div {
			result += "0"
		}
	}
	if fracPart > 0 {
		result += formatInt64(fracPart)
	}

	return result
}

func formatInt64(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}

	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
