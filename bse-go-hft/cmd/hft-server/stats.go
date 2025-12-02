// Package main - BSE HFT Server Statistics
// This file contains performance statistics tracking
package main

import (
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Stats tracks performance statistics
type Stats struct {
	TotalPackets atomic.Int64
	TotalRecords atomic.Int64
	TotalBytes   atomic.Int64
	QuotesSaved  atomic.Int64
	DecodeCount  atomic.Int64
	DecodeSum    atomic.Int64 // Sum of latencies in nanoseconds

	// Latency histogram (approximate percentiles using pre-defined buckets)
	// Buckets: <1µs, <5µs, <10µs, <50µs, <100µs, <500µs, <1ms, >=1ms
	LatencyBuckets [8]atomic.Int64

	// Memory tracking
	PeakMemory   atomic.Uint64
	MemorySample []uint64
	memMu        sync.Mutex

	// Missed tokens tracking
	MissedTokenCount atomic.Int64
	MissedTokens     map[uint32]int64
	MissedTokensMu   sync.Mutex

	startTime time.Time
}

// NewStats creates a new Stats instance
func NewStats() *Stats {
	return &Stats{
		startTime:    time.Now(),
		MissedTokens: make(map[uint32]int64),
	}
}

// AddMissedToken records a missed token
func (s *Stats) AddMissedToken(token uint32) {
	s.MissedTokenCount.Add(1)
	s.MissedTokensMu.Lock()
	s.MissedTokens[token]++
	s.MissedTokensMu.Unlock()
}

// GetMissedTokensSummary returns top N missed tokens sorted by count
func (s *Stats) GetMissedTokensSummary(topN int) []MissedTokenInfo {
	s.MissedTokensMu.Lock()
	defer s.MissedTokensMu.Unlock()

	var result []MissedTokenInfo
	for token, count := range s.MissedTokens {
		result = append(result, MissedTokenInfo{Token: token, Count: count})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	if len(result) > topN {
		return result[:topN]
	}
	return result
}

// RecordLatency records a latency sample
func (s *Stats) RecordLatency(nanos int64) {
	s.DecodeCount.Add(1)
	s.DecodeSum.Add(nanos)

	// Bucket assignment
	micros := nanos / 1000
	var idx int
	switch {
	case micros < 1:
		idx = 0
	case micros < 5:
		idx = 1
	case micros < 10:
		idx = 2
	case micros < 50:
		idx = 3
	case micros < 100:
		idx = 4
	case micros < 500:
		idx = 5
	case micros < 1000:
		idx = 6
	default:
		idx = 7
	}
	s.LatencyBuckets[idx].Add(1)
}

// SampleMemory samples current memory usage
func (s *Stats) SampleMemory() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Update peak
	for {
		peak := s.PeakMemory.Load()
		if m.Alloc <= peak {
			break
		}
		if s.PeakMemory.CompareAndSwap(peak, m.Alloc) {
			break
		}
	}

	s.memMu.Lock()
	s.MemorySample = append(s.MemorySample, m.Alloc)
	s.memMu.Unlock()
}

// GetPercentiles estimates latency percentiles from histogram
func (s *Stats) GetPercentiles() LatencyPercentiles {
	// Bucket midpoints in microseconds
	midpoints := []float64{0.5, 3, 7.5, 30, 75, 300, 750, 2000}

	total := s.DecodeCount.Load()
	if total == 0 {
		return LatencyPercentiles{}
	}

	var counts [8]int64
	for i := range counts {
		counts[i] = s.LatencyBuckets[i].Load()
	}

	getPercentile := func(p float64) float64 {
		target := int64(p * float64(total))
		var cumulative int64
		for i, c := range counts {
			cumulative += c
			if cumulative >= target {
				return midpoints[i]
			}
		}
		return midpoints[7]
	}

	return LatencyPercentiles{
		P50:  getPercentile(0.50),
		P90:  getPercentile(0.90),
		P99:  getPercentile(0.99),
		P999: getPercentile(0.999),
	}
}
