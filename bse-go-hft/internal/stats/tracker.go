// Package stats provides unified statistics tracking for BSE HFT system
package stats

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"bse-go-hft/internal/metrics"
)

// Tracker provides comprehensive statistics for HFT pipeline
type Tracker struct {
	startTime time.Time

	// Packet stats (per segment)
	eqPackets atomic.Uint64
	foPackets atomic.Uint64
	eqBytes   atomic.Uint64
	foBytes   atomic.Uint64
	eqRecords atomic.Uint64
	foRecords atomic.Uint64
	eqQuotes  atomic.Uint64
	foQuotes  atomic.Uint64

	// Ring buffer drops (ACTUAL packet loss)
	eqRingDrops atomic.Uint64
	foRingDrops atomic.Uint64

	// Latency tracking
	decodeLatency  *metrics.LatencyTracker
	processLatency *metrics.LatencyTracker
	saveLatency    *metrics.LatencyTracker

	// System tracking
	systemCollector *metrics.SystemCollector

	// Sequence tracking
	sequenceTracker *metrics.SequenceTracker

	// Missed tokens
	missedTokens     map[uint32]int64
	missedTokensMu   sync.Mutex
	missedTokenCount atomic.Uint64

	// Output file info
	eqFilePath string
	foFilePath string
}

// NewTracker creates a new statistics tracker
func NewTracker() *Tracker {
	return &Tracker{
		startTime:       time.Now(),
		decodeLatency:   metrics.NewLatencyTracker(),
		processLatency:  metrics.NewLatencyTracker(),
		saveLatency:     metrics.NewLatencyTracker(),
		systemCollector: metrics.NewSystemCollector(),
		sequenceTracker: metrics.NewSequenceTracker(),
		missedTokens:    make(map[uint32]int64),
	}
}

// RecordPacket records a packet reception
func (t *Tracker) RecordPacket(segment string, bytes int, records int) {
	if segment == "EQ" {
		t.eqPackets.Add(1)
		t.eqBytes.Add(uint64(bytes))
		t.eqRecords.Add(uint64(records))
	} else {
		t.foPackets.Add(1)
		t.foBytes.Add(uint64(bytes))
		t.foRecords.Add(uint64(records))
	}
}

// RecordQuote records a quote save
func (t *Tracker) RecordQuote(segment string) {
	if segment == "EQ" {
		t.eqQuotes.Add(1)
	} else {
		t.foQuotes.Add(1)
	}
}

// RecordRingDrops records ring buffer overflow drops (ACTUAL packet loss)
func (t *Tracker) RecordRingDrops(segment string, drops uint64) {
	if segment == "EQ" {
		t.eqRingDrops.Store(drops)
	} else {
		t.foRingDrops.Store(drops)
	}
}

// GetRingDrops returns total ring buffer drops
func (t *Tracker) GetRingDrops() uint64 {
	return t.eqRingDrops.Load() + t.foRingDrops.Load()
}

// RecordDecodeLatency records decode latency in nanoseconds
func (t *Tracker) RecordDecodeLatency(ns int64) {
	t.decodeLatency.Record(ns)
}

// RecordProcessLatency records end-to-end process latency in nanoseconds
func (t *Tracker) RecordProcessLatency(ns int64) {
	t.processLatency.Record(ns)
}

// RecordSaveLatency records save latency in nanoseconds
func (t *Tracker) RecordSaveLatency(ns int64) {
	t.saveLatency.Record(ns)
}

// TrackSequence tracks a sequence number for a token
func (t *Tracker) TrackSequence(token, sequence uint32) {
	t.sequenceTracker.Track(token, sequence)
}

// TrackMissedToken records a token not found in token map
func (t *Tracker) TrackMissedToken(token uint32) {
	t.missedTokensMu.Lock()
	t.missedTokens[token]++
	t.missedTokensMu.Unlock()
	t.missedTokenCount.Add(1)
}

// SetOutputFile sets the output file path for a segment
func (t *Tracker) SetOutputFile(segment, filePath string) {
	if segment == "EQ" {
		t.eqFilePath = filePath
	} else {
		t.foFilePath = filePath
	}
}

// MissedTokenPair represents a token and its miss count
type MissedTokenPair struct {
	Token uint32
	Count int64
}

// GetTopMissedTokens returns the top N missed tokens sorted by count
func (t *Tracker) GetTopMissedTokens(n int) []MissedTokenPair {
	t.missedTokensMu.Lock()
	defer t.missedTokensMu.Unlock()

	pairs := make([]MissedTokenPair, 0, len(t.missedTokens))
	for token, count := range t.missedTokens {
		pairs = append(pairs, MissedTokenPair{Token: token, Count: count})
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Count > pairs[j].Count
	})

	if n > len(pairs) {
		n = len(pairs)
	}
	return pairs[:n]
}

// GetUniqueMissedTokenCount returns count of unique missed tokens
func (t *Tracker) GetUniqueMissedTokenCount() int {
	t.missedTokensMu.Lock()
	defer t.missedTokensMu.Unlock()
	return len(t.missedTokens)
}

// SampleSystem samples current system stats
func (t *Tracker) SampleSystem() {
	t.systemCollector.Collect()
}

// Elapsed returns time since start
func (t *Tracker) Elapsed() time.Duration {
	return time.Since(t.startTime)
}

// TotalPackets returns total packets received
func (t *Tracker) TotalPackets() uint64 {
	return t.eqPackets.Load() + t.foPackets.Load()
}

// TotalBytes returns total bytes received
func (t *Tracker) TotalBytes() uint64 {
	return t.eqBytes.Load() + t.foBytes.Load()
}

// TotalRecords returns total records decoded
func (t *Tracker) TotalRecords() uint64 {
	return t.eqRecords.Load() + t.foRecords.Load()
}

// TotalQuotes returns total quotes saved
func (t *Tracker) TotalQuotes() uint64 {
	return t.eqQuotes.Load() + t.foQuotes.Load()
}

// MissedTokenCount returns count of missed token lookups
func (t *Tracker) MissedTokenCount() uint64 {
	return t.missedTokenCount.Load()
}

// PacketsPerSecond returns current packets/sec
func (t *Tracker) PacketsPerSecond() float64 {
	elapsed := t.Elapsed().Seconds()
	if elapsed == 0 {
		return 0
	}
	return float64(t.TotalPackets()) / elapsed
}

// RecordsPerSecond returns current records/sec
func (t *Tracker) RecordsPerSecond() float64 {
	elapsed := t.Elapsed().Seconds()
	if elapsed == 0 {
		return 0
	}
	return float64(t.TotalRecords()) / elapsed
}

// BytesPerSecond returns current bytes/sec
func (t *Tracker) BytesPerSecond() float64 {
	elapsed := t.Elapsed().Seconds()
	if elapsed == 0 {
		return 0
	}
	return float64(t.TotalBytes()) / elapsed
}

// GetDecodePercentiles returns decode latency percentiles
func (t *Tracker) GetDecodePercentiles() metrics.LatencyPercentiles {
	return t.decodeLatency.GetPercentiles()
}

// GetProcessPercentiles returns process latency percentiles
func (t *Tracker) GetProcessPercentiles() metrics.LatencyPercentiles {
	return t.processLatency.GetPercentiles()
}

// GetSavePercentiles returns save latency percentiles
func (t *Tracker) GetSavePercentiles() metrics.LatencyPercentiles {
	return t.saveLatency.GetPercentiles()
}

// GetDecodeStats returns decode latency statistics (min/avg/max)
func (t *Tracker) GetDecodeStats() metrics.LatencyStats {
	return t.decodeLatency.GetStats()
}

// GetProcessStats returns process latency statistics (min/avg/max)
func (t *Tracker) GetProcessStats() metrics.LatencyStats {
	return t.processLatency.GetStats()
}

// GetSaveStats returns save latency statistics (min/avg/max)
func (t *Tracker) GetSaveStats() metrics.LatencyStats {
	return t.saveLatency.GetStats()
}

// GetSequenceStats returns sequence tracking statistics
func (t *Tracker) GetSequenceStats() metrics.SequenceStats {
	return t.sequenceTracker.GetStats()
}

// GetSystemStats returns current system statistics
func (t *Tracker) GetSystemStats() metrics.SystemStats {
	return t.systemCollector.Collect()
}

// PrintLiveStats prints a single line of live statistics
func (t *Tracker) PrintLiveStats(tokenCount int) {
	elapsed := t.Elapsed().Seconds()
	packets := t.TotalPackets()
	records := t.TotalRecords()
	missed := t.MissedTokenCount()
	drops := t.GetRingDrops()

	pps := float64(0)
	rps := float64(0)
	if elapsed > 0 {
		pps = float64(packets) / elapsed
		rps = float64(records) / elapsed
	}

	fmt.Printf("\r[%.0fs] Pkts: %d (%.0f/s) | Records: %d (%.0f/s) | EQ: %d | FO: %d | Drops: %d | Missed: %d | Tokens: %d",
		elapsed, packets, pps, records, rps,
		t.eqQuotes.Load(), t.foQuotes.Load(),
		drops, missed, tokenCount)
}

// PrintFinalReport prints comprehensive final statistics with beautiful formatting
func (t *Tracker) PrintFinalReport(tokenCount int) {
	elapsed := t.Elapsed()
	elapsedSec := elapsed.Seconds()

	fmt.Println("\n")
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                         📊 BSE HFT BENCHMARK REPORT                          ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")

	// DURATION SECTION
	fmt.Println("\n┌──────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  ⏱️  DURATION                                                                 │")
	fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│    Total Runtime:        %-52.2f seconds│\n", elapsedSec)
	fmt.Printf("│    Start Time:           %-56s│\n", t.startTime.Format("15:04:05.000"))
	fmt.Printf("│    End Time:             %-56s│\n", time.Now().Format("15:04:05.000"))
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")

	// FEED BREAKDOWN
	fmt.Println("\n┌──────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  📊 FEED BREAKDOWN                                                           │")
	fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│    EQ (Equity Cash):     %-8d pkts    %-8d recs    %-8d quotes   │\n",
		t.eqPackets.Load(), t.eqRecords.Load(), t.eqQuotes.Load())
	fmt.Printf("│    FO (F&O Derivatives): %-8d pkts    %-8d recs    %-8d quotes   │\n",
		t.foPackets.Load(), t.foRecords.Load(), t.foQuotes.Load())
	fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│    TOTAL:                %-8d pkts    %-8d recs    %-8d quotes   │\n",
		t.TotalPackets(), t.TotalRecords(), t.TotalQuotes())
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")

	// THROUGHPUT
	fmt.Println("\n┌──────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  🚀 THROUGHPUT                                                               │")
	fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
	if elapsedSec > 0 {
		pps := float64(t.TotalPackets()) / elapsedSec
		rps := float64(t.TotalRecords()) / elapsedSec
		mbps := float64(t.TotalBytes()) / elapsedSec / 1024 / 1024

		fmt.Printf("│    Packets/sec:          %-56.2f│\n", pps)
		fmt.Printf("│    Records/sec:          %-56.2f│\n", rps)
		fmt.Printf("│    Data Rate:            %-52.3f MB/s│\n", mbps)
		fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")

		// Average time per packet/record
		msPerPacket := elapsedSec * 1000.0 / float64(t.TotalPackets())
		usPerRecord := elapsedSec * 1000000.0 / float64(t.TotalRecords())
		nsPerRecord := elapsedSec * 1000000000.0 / float64(t.TotalRecords())

		fmt.Printf("│    Avg Time/Packet:      %-52.4f ms│\n", msPerPacket)
		fmt.Printf("│    Avg Time/Record:      %-52.2f µs│\n", usPerRecord)
		fmt.Printf("│    Avg Time/Record:      %-52.0f ns│\n", nsPerRecord)
	}
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")

	// DECODE RATE PROJECTIONS
	fmt.Println("\n┌──────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  📈 DECODE RATE PROJECTIONS                                                  │")
	fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
	if elapsedSec > 0 {
		rps := float64(t.TotalRecords()) / elapsedSec
		fmt.Printf("│    Per Second:           %-44.0f records/sec│\n", rps)
		fmt.Printf("│    Per Minute:           %-44.0f records/min│\n", rps*60)
		fmt.Printf("│    Per 30 Minutes:       %-42.0f records/30min│\n", rps*1800)
		fmt.Printf("│    Per Hour:             %-45.0f records/hr│\n", rps*3600)
		fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
		qps := float64(t.TotalQuotes()) / elapsedSec
		fmt.Printf("│    Quotes/sec:           %-48.0f quotes/sec│\n", qps)
		fmt.Printf("│    Quotes/min:           %-48.0f quotes/min│\n", qps*60)
		fmt.Printf("│    Quotes/hour:          %-49.0f quotes/hr│\n", qps*3600)
	}
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")

	// OUTPUT FILES
	fmt.Println("\n┌──────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  📄 OUTPUT FILES                                                             │")
	fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
	if t.eqFilePath != "" {
		fmt.Printf("│    EQ CSV:              %-56s│\n", filepath.Base(t.eqFilePath))
		fmt.Printf("│    EQ Rows:             %-56d│\n", t.eqQuotes.Load())
	}
	if t.foFilePath != "" {
		fmt.Printf("│    FO CSV:              %-56s│\n", filepath.Base(t.foFilePath))
		fmt.Printf("│    FO Rows:             %-56d│\n", t.foQuotes.Load())
	}
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")

	// LATENCY
	decodeP := t.GetDecodePercentiles()
	saveP := t.GetSavePercentiles()
	processP := t.GetProcessPercentiles()

	// Get average stats
	decodeS := t.GetDecodeStats()
	saveS := t.GetSaveStats()
	processS := t.GetProcessStats()

	fmt.Println("\n┌──────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  ⚡ LATENCY ANALYSIS (microseconds)                                          │")
	fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
	fmt.Println("│    Metric            Decode          Save            Total                   │")
	fmt.Println("│    ─────────────────────────────────────────────────────────                 │")
	fmt.Printf("│    Min            %8.2f µs    %8.2f µs    %8.2f µs                   │\n", decodeS.Min, saveS.Min, processS.Min)
	fmt.Printf("│    Avg            %8.2f µs    %8.2f µs    %8.2f µs                   │\n", decodeS.Avg, saveS.Avg, processS.Avg)
	fmt.Printf("│    Max            %8.2f µs    %8.2f µs    %8.2f µs                   │\n", decodeS.Max, saveS.Max, processS.Max)
	fmt.Println("│    ─────────────────────────────────────────────────────────                 │")
	fmt.Printf("│    P50            %8.2f µs    %8.2f µs    %8.2f µs                   │\n", decodeP.P50, saveP.P50, processP.P50)
	fmt.Printf("│    P90            %8.2f µs    %8.2f µs    %8.2f µs                   │\n", decodeP.P90, saveP.P90, processP.P90)
	fmt.Printf("│    P99            %8.2f µs    %8.2f µs    %8.2f µs                   │\n", decodeP.P99, saveP.P99, processP.P99)
	fmt.Printf("│    P99.9          %8.2f µs    %8.2f µs    %8.2f µs                   │\n", decodeP.P999, saveP.P999, processP.P999)
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")

	// RING BUFFER DROPS (ACTUAL PACKET LOSS)
	eqDrops := t.eqRingDrops.Load()
	foDrops := t.foRingDrops.Load()
	totalDrops := eqDrops + foDrops
	totalPackets := t.TotalPackets()
	dropRate := float64(0)
	if totalPackets+totalDrops > 0 {
		dropRate = float64(totalDrops) / float64(totalPackets+totalDrops) * 100.0
	}

	fmt.Println("\n┌──────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  📦 RING BUFFER DROPS (Actual Packet Loss)                                   │")
	fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│    EQ Ring Drops:        %-56d│\n", eqDrops)
	fmt.Printf("│    FO Ring Drops:        %-56d│\n", foDrops)
	fmt.Printf("│    Total Drops:          %-56d│\n", totalDrops)
	fmt.Printf("│    Drop Rate:            %-56s│\n", fmt.Sprintf("%.4f%%", dropRate))
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")

	// SEQUENCE & TOKEN TRACKING
	seqStats := t.GetSequenceStats()

	fmt.Println("\n┌──────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  📉 SEQUENCE & TOKEN TRACKING                                                │")
	fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│    Unique Tokens:        %-56d│\n", seqStats.UniqueTokens)
	fmt.Printf("│    Sequence Gaps:        %-56d│\n", seqStats.TotalGaps)
	fmt.Printf("│    Missed Packets:       %-56d│\n", seqStats.TotalMissed)
	fmt.Printf("│    Token Map Size:       %-56d│\n", tokenCount)
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")

	// MISSED TOKENS
	missedCount := t.MissedTokenCount()
	uniqueMissed := t.GetUniqueMissedTokenCount()

	fmt.Println("\n┌──────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  ⚠️  MISSED TOKENS (Not in Token Master)                                      │")
	fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf(" │    Total Missed:         %-56d│\n", missedCount)
	fmt.Printf(" │    Unique Tokens:        %-56d│\n", uniqueMissed)

	if uniqueMissed > 0 {
		fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
		fmt.Println("│    Top Missed Tokens (Token → Count):                                        │")

		topMissed := t.GetTopMissedTokens(10)
		for i, pair := range topMissed {
			fmt.Printf("│      %2d. Token %-12d → %-3d occurrences                                │\n",
				i+1, pair.Token, pair.Count)
		}
	}
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")

	// SYSTEM RESOURCES
	sysStats := t.GetSystemStats()

	fmt.Println("\n┌──────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  💾 SYSTEM RESOURCES                                                         │")
	fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│    Peak Memory:          %-56s    │\n", metrics.FormatBytes(sysStats.PeakMemory))
	fmt.Printf("│    Current Memory:       %-56s    │\n", metrics.FormatBytes(sysStats.HeapAlloc))
	fmt.Printf("│    Goroutines:           %-56d    │\n", runtime.NumGoroutine())
	fmt.Printf("│    GC Cycles:            %-56d    │\n", sysStats.NumGC)
	fmt.Printf("│    CPU Cores:            %-56d    │\n", sysStats.NumCPU)
	fmt.Printf("│    GOMAXPROCS:           %-56d    │\n", sysStats.GOMAXPROCS)
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")

	// HFT ASSESSMENT
	fmt.Println("\n╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║  🏆 HFT PERFORMANCE ASSESSMENT                                               ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════════════════════╣")

	// Performance rating based on P99 latency
	if processP.P99 < 50 {
		fmt.Println("║    Status:  ✨ ULTRA HFT - P99 < 50µs (Professional Trading Grade)           ║")
	} else if processP.P99 < 100 {
		fmt.Println("║    Status:  ✅ EXCELLENT - P99 < 100µs (HFT Grade)                            ║")
	} else if processP.P99 < 500 {
		fmt.Println("║    Status:  ✅ GOOD - P99 < 500µs (Low Latency Trading)                       ║")
	} else if processP.P99 < 1000 {
		fmt.Println("║    Status:  ⚠️  ACCEPTABLE - P99 < 1ms (Algo Trading)                         ║")
	} else {
		fmt.Println("║    Status:  ❌ NEEDS OPTIMIZATION - P99 >= 1ms                                ║")
	}

	// Packet Loss assessment (based on ACTUAL ring drops, not sequence gaps)
	if totalDrops == 0 {
		fmt.Println("║    Drops:   ✅ ZERO PACKET DROPS (Perfect capture)                           ║")
	} else if dropRate < 0.01 {
		fmt.Printf("║    Drops:   ⚠️  MINIMAL - %d drops (%.4f%%)                                    ║\n", totalDrops, dropRate)
	} else {
		fmt.Printf("║    Drops:   ❌ PACKET LOSS - %d drops (%.2f%%) - increase ring buffer          ║\n", totalDrops, dropRate)
	}

	// Data quality assessment
	missedRatio := float64(0)
	if t.TotalRecords() > 0 {
		missedRatio = float64(missedCount) / float64(t.TotalRecords()) * 100
	}
	if missedRatio < 1 {
		fmt.Printf("║    Data:    ✅ EXCELLENT - %.2f%% token miss rate                              ║\n", missedRatio)
	} else if missedRatio < 5 {
		fmt.Printf("║    Data:    ⚠️  GOOD - %.2f%% token miss rate                                  ║\n", missedRatio)
	} else {
		fmt.Printf("║    Data:    ❌ CHECK TOKEN MAP - %.2f%% miss rate                              ║\n", missedRatio)
	}

	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
}
