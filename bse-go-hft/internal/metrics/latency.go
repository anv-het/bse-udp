// Package metrics provides HFT-grade statistics collection
package metrics

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// MaxLatencySamples is the maximum number of samples to store for percentiles
	MaxLatencySamples = 100000
)

// LatencyTracker tracks latencies with percentile calculation
// Thread-safe for concurrent recording
type LatencyTracker struct {
	// Running statistics (lock-free)
	sum   atomic.Int64
	count atomic.Int64
	min   atomic.Int64
	max   atomic.Int64

	// Sample storage for percentiles
	mu          sync.Mutex
	samples     []int64
	sampleCount int
	maxSamples  int
}

// NewLatencyTracker creates a new tracker with pre-allocated storage
func NewLatencyTracker() *LatencyTracker {
	lt := &LatencyTracker{
		samples:    make([]int64, 0, MaxLatencySamples),
		maxSamples: MaxLatencySamples,
	}
	lt.min.Store(math.MaxInt64)
	lt.max.Store(0)
	return lt
}

// Record records a latency sample in nanoseconds
func (lt *LatencyTracker) Record(latencyNs int64) {
	// Update running stats atomically
	lt.sum.Add(latencyNs)
	lt.count.Add(1)

	// Update min with CAS loop
	for {
		old := lt.min.Load()
		if latencyNs >= old || lt.min.CompareAndSwap(old, latencyNs) {
			break
		}
	}

	// Update max with CAS loop
	for {
		old := lt.max.Load()
		if latencyNs <= old || lt.max.CompareAndSwap(old, latencyNs) {
			break
		}
	}

	// Store sample for percentiles (with lock, but fast path)
	lt.mu.Lock()
	if lt.sampleCount < lt.maxSamples {
		lt.samples = append(lt.samples, latencyNs)
		lt.sampleCount++
	}
	lt.mu.Unlock()
}

// LatencyPercentiles holds calculated percentile values in microseconds
type LatencyPercentiles struct {
	P50  float64 // Median
	P75  float64
	P90  float64
	P95  float64
	P99  float64
	P999 float64 // For HFT - 99.9th percentile
}

// GetPercentiles calculates percentiles from stored samples
// Returns values in microseconds (µs)
func (lt *LatencyTracker) GetPercentiles() LatencyPercentiles {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if lt.sampleCount == 0 {
		return LatencyPercentiles{}
	}

	// Copy samples for sorting
	sorted := make([]int64, lt.sampleCount)
	copy(sorted, lt.samples[:lt.sampleCount])
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	n := len(sorted)
	getPercentile := func(p float64) float64 {
		idx := int(float64(n-1) * p)
		if idx >= n {
			idx = n - 1
		}
		return float64(sorted[idx]) / 1000.0 // ns to µs
	}

	return LatencyPercentiles{
		P50:  getPercentile(0.50),
		P75:  getPercentile(0.75),
		P90:  getPercentile(0.90),
		P95:  getPercentile(0.95),
		P99:  getPercentile(0.99),
		P999: getPercentile(0.999),
	}
}

// LatencyStats returns basic latency statistics
type LatencyStats struct {
	Min   float64 // µs
	Avg   float64 // µs
	Max   float64 // µs
	Count int64
}

// GetStats returns basic latency statistics in microseconds
func (lt *LatencyTracker) GetStats() LatencyStats {
	count := lt.count.Load()
	if count == 0 {
		return LatencyStats{}
	}

	minVal := lt.min.Load()
	if minVal == math.MaxInt64 {
		minVal = 0
	}

	return LatencyStats{
		Min:   float64(minVal) / 1000.0,
		Avg:   float64(lt.sum.Load()) / float64(count) / 1000.0,
		Max:   float64(lt.max.Load()) / 1000.0,
		Count: count,
	}
}

// Reset clears all latency data
func (lt *LatencyTracker) Reset() {
	lt.sum.Store(0)
	lt.count.Store(0)
	lt.min.Store(math.MaxInt64)
	lt.max.Store(0)

	lt.mu.Lock()
	lt.samples = lt.samples[:0]
	lt.sampleCount = 0
	lt.mu.Unlock()
}

// SequenceTracker tracks packet sequences per token for loss detection
type SequenceTracker struct {
	mu            sync.RWMutex
	tokenSequence map[uint32]*TokenSequence

	// Aggregate stats (lock-free)
	totalGaps    atomic.Int64
	totalMissed  atomic.Int64
	totalTracked atomic.Int64
}

// TokenSequence tracks sequence for a single token
type TokenSequence struct {
	LastSeq    uint32
	GapCount   int64
	MissedPkts int64
	FirstSeen  time.Time
	LastSeen   time.Time
}

// NewSequenceTracker creates a new sequence tracker
func NewSequenceTracker() *SequenceTracker {
	return &SequenceTracker{
		tokenSequence: make(map[uint32]*TokenSequence),
	}
}

// Track records a sequence number for a token
// Returns true if this is a new token, false if existing
func (st *SequenceTracker) Track(token, sequence uint32) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	ts, exists := st.tokenSequence[token]
	if !exists {
		st.tokenSequence[token] = &TokenSequence{
			LastSeq:   sequence,
			FirstSeen: time.Now(),
			LastSeen:  time.Now(),
		}
		st.totalTracked.Add(1)
		return true
	}

	// Check for gap (sequence should increment)
	if sequence > ts.LastSeq+1 {
		gap := int64(sequence - ts.LastSeq - 1)
		ts.GapCount++
		ts.MissedPkts += gap
		st.totalGaps.Add(1)
		st.totalMissed.Add(gap)
	}

	ts.LastSeq = sequence
	ts.LastSeen = time.Now()
	return false
}

// SequenceStats holds sequence tracking statistics
type SequenceStats struct {
	TotalGaps    int64
	TotalMissed  int64
	TotalTracked int64
	UniqueTokens int
}

// GetStats returns sequence tracking statistics
func (st *SequenceTracker) GetStats() SequenceStats {
	st.mu.RLock()
	uniqueTokens := len(st.tokenSequence)
	st.mu.RUnlock()

	return SequenceStats{
		TotalGaps:    st.totalGaps.Load(),
		TotalMissed:  st.totalMissed.Load(),
		TotalTracked: st.totalTracked.Load(),
		UniqueTokens: uniqueTokens,
	}
}

// Reset clears all sequence data
func (st *SequenceTracker) Reset() {
	st.mu.Lock()
	st.tokenSequence = make(map[uint32]*TokenSequence)
	st.mu.Unlock()

	st.totalGaps.Store(0)
	st.totalMissed.Store(0)
	st.totalTracked.Store(0)
}
