// BSE Go HFT Benchmark Tool
// Measures: Packet rates, CPU/Memory, Latency Percentiles (P50/P75/P90/P95/P99), Packet Loss
// For HFT system evaluation
//
// ================================================================================
// USAGE GUIDE
// ================================================================================
//
// BUILD:
//   cd d:\bse\bse-go-hft
//   go build -o benchmark.exe ./cmd/benchmark/
//
// RUN EXAMPLES:
//
// 1. Benchmark EQ (Equity Cash) feed - Port 26001:
//    .\benchmark.exe -port 26001 -duration 30s
//    .\benchmark.exe -port 26001 -duration 1m
//    .\benchmark.exe -port 26001                    # Run until Ctrl+C
//
// 2. Benchmark FO (F&O Derivatives) feed - Port 26002:
//    .\benchmark.exe -port 26002 -duration 30s
//    .\benchmark.exe -port 26002 -duration 2m
//    .\benchmark.exe -port 26002                    # Run until Ctrl+C
//
// 3. Custom multicast IP (if different from default):
//    .\benchmark.exe -ip 239.1.2.5 -port 26002 -duration 1m
//
// PARAMETERS:
//   -ip string       Multicast IP address (default "239.1.2.5")
//   -port int        UDP port: 26001=EQ, 26002=FO (default 26001)
//   -duration time   Benchmark duration: 30s, 1m, 5m, etc. (default: until Ctrl+C)
//
// BSE FEED PORTS:
//   Port 26001 = EQ (Equity/Cash Market)
//   Port 26002 = FO (F&O Derivatives)
//
// OUTPUT METRICS:
//   • Throughput: Packets/sec, Records/sec, MB/s
//   • Latency: Min, Avg, Max, P50, P75, P90, P95, P99, P99.9 (microseconds)
//   • Memory: Peak MB, Average MB, GC cycles
//   • Packet Loss: Sequence gaps, missed packets, loss rate %
//
// ================================================================================

package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/ipv4"
)

// ================================================================================
// CONFIGURATION
// ================================================================================

const (
	// Default F&O Multicast Feed
	DefaultMulticastIP = "239.1.2.5"
	DefaultPort        = 26001
	BufferSize         = 2048 // Packet buffer size

	// Socket receive buffer - CRITICAL for reducing packet loss
	SocketRcvBufSize = 32 * 1024 * 1024 // 32MB socket buffer

	// Ring buffer for zero-loss processing
	RingBufferSize = 16384 // 16K slots

	// Packet structure
	HeaderSize = 36
	RecordSize = 264 // BSE uses 264-byte records

	// Latency sample storage
	MaxLatencySamples = 100000
)

// ================================================================================
// LATENCY PERCENTILES
// ================================================================================

type LatencyPercentiles struct {
	P50  float64
	P75  float64
	P90  float64
	P95  float64
	P99  float64
	P999 float64
}

// ================================================================================
// STATISTICS STRUCTURES
// ================================================================================

type PacketStats struct {
	TotalPackets   atomic.Uint64
	TotalBytes     atomic.Uint64
	TotalRecords   atomic.Uint64
	ValidPackets   atomic.Uint64
	InvalidPackets atomic.Uint64
}

type LatencyStats struct {
	// Running stats
	DecodeSum   atomic.Int64
	DecodeCount atomic.Int64
	DecodeMin   atomic.Int64
	DecodeMax   atomic.Int64

	ProcessSum   atomic.Int64
	ProcessCount atomic.Int64
	ProcessMin   atomic.Int64
	ProcessMax   atomic.Int64

	// Sample storage for percentiles
	decodeSamples  []int64
	processSamples []int64
	sampleCount    int
	maxSamples     int
	mu             sync.Mutex
}

type SequenceTracker struct {
	mu            sync.RWMutex
	tokenSequence map[uint32]*TokenSeq
	totalGaps     atomic.Int64
	totalMissed   atomic.Int64
}

type TokenSeq struct {
	LastSeq    uint32
	GapCount   int64
	MissedPkts int64
}

type SystemStats struct {
	StartTime    time.Time
	PeakMemory   atomic.Uint64
	MemorySum    atomic.Uint64
	SampleCount  atomic.Uint64
	TotalGCPause atomic.Uint64
}

// ================================================================================
// STATS COLLECTOR
// ================================================================================

type StatsCollector struct {
	packets   PacketStats
	latency   LatencyStats
	sequence  SequenceTracker
	system    SystemStats
	tokens    sync.Map
	startTime time.Time
}

func NewStatsCollector() *StatsCollector {
	sc := &StatsCollector{
		startTime: time.Now(),
	}

	// Initialize latency tracking
	sc.latency.DecodeMin.Store(math.MaxInt64)
	sc.latency.ProcessMin.Store(math.MaxInt64)
	sc.latency.decodeSamples = make([]int64, 0, MaxLatencySamples)
	sc.latency.processSamples = make([]int64, 0, MaxLatencySamples)
	sc.latency.maxSamples = MaxLatencySamples

	// Initialize sequence tracking
	sc.sequence.tokenSequence = make(map[uint32]*TokenSeq)

	// Initialize system stats
	sc.system.StartTime = time.Now()

	return sc
}

func (sc *StatsCollector) RecordPacket(size int, valid bool) {
	sc.packets.TotalPackets.Add(1)
	sc.packets.TotalBytes.Add(uint64(size))
	if valid {
		sc.packets.ValidPackets.Add(1)
	} else {
		sc.packets.InvalidPackets.Add(1)
	}
}

func (sc *StatsCollector) RecordRecords(count int) {
	sc.packets.TotalRecords.Add(uint64(count))
}

func (sc *StatsCollector) RecordDecodeLatency(ns int64) {
	sc.latency.DecodeSum.Add(ns)
	sc.latency.DecodeCount.Add(1)

	// Update min
	for {
		old := sc.latency.DecodeMin.Load()
		if ns >= old || sc.latency.DecodeMin.CompareAndSwap(old, ns) {
			break
		}
	}
	// Update max
	for {
		old := sc.latency.DecodeMax.Load()
		if ns <= old || sc.latency.DecodeMax.CompareAndSwap(old, ns) {
			break
		}
	}

	// Store sample
	sc.latency.mu.Lock()
	if sc.latency.sampleCount < sc.latency.maxSamples {
		sc.latency.decodeSamples = append(sc.latency.decodeSamples, ns)
		sc.latency.sampleCount++
	}
	sc.latency.mu.Unlock()
}

func (sc *StatsCollector) RecordProcessLatency(ns int64) {
	sc.latency.ProcessSum.Add(ns)
	sc.latency.ProcessCount.Add(1)

	// Update min
	for {
		old := sc.latency.ProcessMin.Load()
		if ns >= old || sc.latency.ProcessMin.CompareAndSwap(old, ns) {
			break
		}
	}
	// Update max
	for {
		old := sc.latency.ProcessMax.Load()
		if ns <= old || sc.latency.ProcessMax.CompareAndSwap(old, ns) {
			break
		}
	}

	// Store sample
	sc.latency.mu.Lock()
	if len(sc.latency.processSamples) < sc.latency.maxSamples {
		sc.latency.processSamples = append(sc.latency.processSamples, ns)
	}
	sc.latency.mu.Unlock()
}

func (sc *StatsCollector) TrackToken(token uint32) {
	sc.tokens.Store(token, struct{}{})
}

func (sc *StatsCollector) TrackSequence(token, sequence uint32) {
	sc.sequence.mu.Lock()
	defer sc.sequence.mu.Unlock()

	ts, exists := sc.sequence.tokenSequence[token]
	if !exists {
		sc.sequence.tokenSequence[token] = &TokenSeq{LastSeq: sequence}
		return
	}

	if sequence > ts.LastSeq+1 {
		gap := int64(sequence - ts.LastSeq - 1)
		ts.GapCount++
		ts.MissedPkts += gap
		sc.sequence.totalGaps.Add(1)
		sc.sequence.totalMissed.Add(gap)
	}

	ts.LastSeq = sequence
}

func (sc *StatsCollector) SampleSystem() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Update peak memory
	for {
		peak := sc.system.PeakMemory.Load()
		if m.Alloc <= peak || sc.system.PeakMemory.CompareAndSwap(peak, m.Alloc) {
			break
		}
	}

	sc.system.MemorySum.Add(m.Alloc)
	sc.system.SampleCount.Add(1)
	sc.system.TotalGCPause.Store(m.PauseTotalNs)
}

func (sc *StatsCollector) GetUniqueTokenCount() int {
	count := 0
	sc.tokens.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

func (sc *StatsCollector) GetSequenceStats() (gaps, missed, tracked int64) {
	sc.sequence.mu.RLock()
	tracked = int64(len(sc.sequence.tokenSequence))
	sc.sequence.mu.RUnlock()
	return sc.sequence.totalGaps.Load(), sc.sequence.totalMissed.Load(), tracked
}

func (sc *StatsCollector) GetDecodePercentiles() LatencyPercentiles {
	sc.latency.mu.Lock()
	defer sc.latency.mu.Unlock()

	if len(sc.latency.decodeSamples) == 0 {
		return LatencyPercentiles{}
	}

	sorted := make([]int64, len(sc.latency.decodeSamples))
	copy(sorted, sc.latency.decodeSamples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	n := len(sorted)
	getP := func(p float64) float64 {
		idx := int(float64(n-1) * p)
		return float64(sorted[idx]) / 1000.0
	}

	return LatencyPercentiles{
		P50:  getP(0.50),
		P75:  getP(0.75),
		P90:  getP(0.90),
		P95:  getP(0.95),
		P99:  getP(0.99),
		P999: getP(0.999),
	}
}

func (sc *StatsCollector) GetProcessPercentiles() LatencyPercentiles {
	sc.latency.mu.Lock()
	defer sc.latency.mu.Unlock()

	if len(sc.latency.processSamples) == 0 {
		return LatencyPercentiles{}
	}

	sorted := make([]int64, len(sc.latency.processSamples))
	copy(sorted, sc.latency.processSamples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	n := len(sorted)
	getP := func(p float64) float64 {
		idx := int(float64(n-1) * p)
		return float64(sorted[idx]) / 1000.0
	}

	return LatencyPercentiles{
		P50:  getP(0.50),
		P75:  getP(0.75),
		P90:  getP(0.90),
		P95:  getP(0.95),
		P99:  getP(0.99),
		P999: getP(0.999),
	}
}

// ================================================================================
// BENCHMARK RESULT
// ================================================================================

type BenchmarkResult struct {
	Duration time.Duration

	// Throughput
	TotalPackets     uint64
	TotalBytes       uint64
	TotalRecords     uint64
	AvgPacketsPerSec float64
	AvgBytesPerSec   float64
	AvgRecordsPerSec float64

	// Decode Latency (µs)
	MinDecodeLatencyUs float64
	AvgDecodeLatencyUs float64
	MaxDecodeLatencyUs float64
	DecodePercentiles  LatencyPercentiles

	// Process Latency (µs)
	MinProcessLatencyUs float64
	AvgProcessLatencyUs float64
	MaxProcessLatencyUs float64
	ProcessPercentiles  LatencyPercentiles

	// Memory
	PeakMemoryMB  float64
	AvgMemoryMB   float64
	TotalGCPauses uint32

	// Packet Loss - Actual vs Sequence
	RingOverflow   uint64 // Actual drops due to ring buffer overflow
	SequenceGaps   int64  // Gaps in sequence numbers (normal for sparse updates)
	MissedPackets  int64  // Estimated missed based on sequence gaps
	TrackedTokens  int64
	PacketLossRate float64 // Based on ring overflow, not sequence gaps

	// Tokens
	UniqueTokens int
}

func (sc *StatsCollector) GetResult() BenchmarkResult {
	duration := time.Since(sc.startTime)
	secs := duration.Seconds()

	result := BenchmarkResult{
		Duration:     duration,
		TotalPackets: sc.packets.TotalPackets.Load(),
		TotalBytes:   sc.packets.TotalBytes.Load(),
		TotalRecords: sc.packets.TotalRecords.Load(),
		UniqueTokens: sc.GetUniqueTokenCount(),
	}

	// Throughput
	if secs > 0 {
		result.AvgPacketsPerSec = float64(result.TotalPackets) / secs
		result.AvgBytesPerSec = float64(result.TotalBytes) / secs
		result.AvgRecordsPerSec = float64(result.TotalRecords) / secs
	}

	// Decode latency
	decodeCount := sc.latency.DecodeCount.Load()
	if decodeCount > 0 {
		minDecode := sc.latency.DecodeMin.Load()
		if minDecode == math.MaxInt64 {
			minDecode = 0
		}
		result.MinDecodeLatencyUs = float64(minDecode) / 1000.0
		result.AvgDecodeLatencyUs = float64(sc.latency.DecodeSum.Load()) / float64(decodeCount) / 1000.0
		result.MaxDecodeLatencyUs = float64(sc.latency.DecodeMax.Load()) / 1000.0
		result.DecodePercentiles = sc.GetDecodePercentiles()
	}

	// Process latency
	processCount := sc.latency.ProcessCount.Load()
	if processCount > 0 {
		minProcess := sc.latency.ProcessMin.Load()
		if minProcess == math.MaxInt64 {
			minProcess = 0
		}
		result.MinProcessLatencyUs = float64(minProcess) / 1000.0
		result.AvgProcessLatencyUs = float64(sc.latency.ProcessSum.Load()) / float64(processCount) / 1000.0
		result.MaxProcessLatencyUs = float64(sc.latency.ProcessMax.Load()) / 1000.0
		result.ProcessPercentiles = sc.GetProcessPercentiles()
	}

	// Memory
	result.PeakMemoryMB = float64(sc.system.PeakMemory.Load()) / (1024 * 1024)
	sampleCount := sc.system.SampleCount.Load()
	if sampleCount > 0 {
		result.AvgMemoryMB = float64(sc.system.MemorySum.Load()) / float64(sampleCount) / (1024 * 1024)
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	result.TotalGCPauses = m.NumGC

	// Packet loss
	gaps, missed, tracked := sc.GetSequenceStats()
	result.SequenceGaps = gaps
	result.MissedPackets = missed
	result.TrackedTokens = tracked
	if result.TotalRecords > 0 {
		result.PacketLossRate = float64(missed) / float64(result.TotalRecords) * 100.0
	}

	return result
}

// ================================================================================
// RING BUFFER FOR ZERO-LOSS PACKET CAPTURE
// ================================================================================

type RingSlot struct {
	Data   [2048]byte
	Length int
}

type RingBuffer struct {
	slots    []RingSlot
	head     atomic.Uint64
	tail     atomic.Uint64
	mask     uint64
	overflow atomic.Uint64
}

func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		slots: make([]RingSlot, size),
		mask:  uint64(size - 1),
	}
}

func (rb *RingBuffer) Push(data []byte, length int) bool {
	head := rb.head.Load()
	tail := rb.tail.Load()

	// Check if full
	if head-tail >= uint64(len(rb.slots)) {
		rb.overflow.Add(1)
		return false
	}

	slot := &rb.slots[head&rb.mask]
	copy(slot.Data[:], data[:length])
	slot.Length = length

	rb.head.Add(1)
	return true
}

func (rb *RingBuffer) Pop() ([]byte, int, bool) {
	tail := rb.tail.Load()
	head := rb.head.Load()

	if tail >= head {
		return nil, 0, false
	}

	slot := &rb.slots[tail&rb.mask]
	rb.tail.Add(1)

	return slot.Data[:slot.Length], slot.Length, true
}

func (rb *RingBuffer) Overflow() uint64 {
	return rb.overflow.Load()
}

// ================================================================================
// MULTICAST RECEIVER
// ================================================================================

type MulticastReceiver struct {
	ip        string
	port      int
	conn      *net.UDPConn
	pconn     *ipv4.PacketConn
	collector *StatsCollector
	ringBuf   *RingBuffer
}

func NewMulticastReceiver(ip string, port int, collector *StatsCollector) *MulticastReceiver {
	return &MulticastReceiver{
		ip:        ip,
		port:      port,
		collector: collector,
		ringBuf:   NewRingBuffer(RingBufferSize),
	}
}

func (r *MulticastReceiver) Connect() error {
	addr := fmt.Sprintf("%s:%d", r.ip, r.port)
	gaddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, gaddr)
	if err != nil {
		return err
	}
	r.conn = conn

	// Set LARGE receive buffer - critical for reducing packet loss
	if err := conn.SetReadBuffer(SocketRcvBufSize); err != nil {
		log.Printf("Warning: Could not set socket buffer to %dMB: %v", SocketRcvBufSize/(1024*1024), err)
	}

	r.pconn = ipv4.NewPacketConn(conn)

	return nil
}

// ReceiveLoop - fast packet capture to ring buffer (separate goroutine)
func (r *MulticastReceiver) ReceiveLoop(ctx context.Context) {
	buffer := make([]byte, BufferSize)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Use short deadline to allow context cancellation
		r.conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))

		n, _, err := r.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		// Push to ring buffer immediately (minimal processing)
		r.ringBuf.Push(buffer, n)
	}
}

// ProcessLoop - decode packets from ring buffer (separate goroutine)
func (r *MulticastReceiver) ProcessLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data, length, ok := r.ringBuf.Pop()
		if !ok {
			// Buffer empty, yield CPU
			runtime.Gosched()
			continue
		}

		if length < HeaderSize {
			r.collector.RecordPacket(length, false)
			continue
		}

		// Start timing
		processStart := time.Now()

		// Decode packet
		decodeStart := time.Now()
		valid, numRecords := r.decodePacket(data[:length])
		decodeEnd := time.Now()

		if valid {
			r.collector.RecordPacket(length, true)
			r.collector.RecordRecords(numRecords)
			r.collector.RecordDecodeLatency(decodeEnd.Sub(decodeStart).Nanoseconds())
		} else {
			r.collector.RecordPacket(length, false)
		}

		// Record full process time
		r.collector.RecordProcessLatency(time.Since(processStart).Nanoseconds())
	}
}

// GetOverflow returns ring buffer overflow count (actual packet drops)
func (r *MulticastReceiver) GetOverflow() uint64 {
	return r.ringBuf.Overflow()
}

func (r *MulticastReceiver) decodePacket(packet []byte) (bool, int) {
	// Parse header
	// formatID := binary.BigEndian.Uint16(packet[4:6])  // Don't validate - BSE uses multiple formatIDs
	msgType := binary.LittleEndian.Uint16(packet[8:10])

	// Validate message type only (2020=EQ, 2021=FO)
	if msgType != 2020 && msgType != 2021 {
		return false, 0
	}

	// Calculate records - BSE uses 264 bytes per record, not 66!
	dataLen := len(packet) - HeaderSize
	numRecords := dataLen / 264 // Fixed: use correct record size
	if numRecords > 8 {
		numRecords = 8
	}

	// Parse records
	validCount := 0
	offset := HeaderSize

	for i := 0; i < numRecords; i++ {
		if offset+264 > len(packet) {
			break
		}

		recordData := packet[offset : offset+264]
		token := binary.LittleEndian.Uint32(recordData[0:4])

		if token > 1 {
			r.collector.TrackToken(token)

			// Track sequence for packet loss detection
			if len(recordData) >= 48 {
				seqNum := binary.LittleEndian.Uint32(recordData[44:48])
				r.collector.TrackSequence(token, seqNum)
			}

			validCount++
		}

		offset += 264 // Fixed: use correct record size
	}

	return true, validCount
}

func (r *MulticastReceiver) Close() error {
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

// ================================================================================
// REPORTING
// ================================================================================

func PrintLiveStats(collector *StatsCollector, elapsed time.Duration, overflow uint64) {
	result := collector.GetResult()
	gaps, missed, _ := collector.GetSequenceStats()

	fmt.Printf("\r[%s] Pkts: %d (%.0f/s) | Records: %d | Tokens: %d | Drops: %d | SeqGaps: %d | Mem: %.1fMB",
		elapsed.Round(time.Second),
		result.TotalPackets,
		result.AvgPacketsPerSec,
		result.TotalRecords,
		result.UniqueTokens,
		overflow,
		gaps,
		result.PeakMemoryMB,
	)
	_ = missed // Not shown in live stats
}

func PrintFinalReport(result BenchmarkResult) {
	fmt.Println("\n")
	fmt.Println("████████████████████████████████████████████████████████████████████████████████")
	fmt.Println("█                      BSE HFT BENCHMARK - FINAL REPORT                       █")
	fmt.Println("████████████████████████████████████████████████████████████████████████████████")
	fmt.Println()

	// Duration
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ BENCHMARK SUMMARY                                                           │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Duration:              %-54s│\n", result.Duration.Round(time.Millisecond))
	fmt.Printf("│ Total Packets:         %-54d│\n", result.TotalPackets)
	fmt.Printf("│ Total Records:         %-54d│\n", result.TotalRecords)
	fmt.Printf("│ Unique Tokens:         %-54d│\n", result.UniqueTokens)
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	// Throughput
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ THROUGHPUT                                                                  │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Packets/sec:           %-54.2f│\n", result.AvgPacketsPerSec)
	fmt.Printf("│ Records/sec:           %-54.2f│\n", result.AvgRecordsPerSec)
	fmt.Printf("│ Throughput:            %-54s│\n", fmt.Sprintf("%.2f MB/s", result.AvgBytesPerSec/(1024*1024)))
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	// Average time per packet/record
	avgTimePerPacketMs := 0.0
	avgTimePerRecordUs := 0.0
	avgTimePerRecordNs := 0.0
	if result.AvgPacketsPerSec > 0 {
		avgTimePerPacketMs = 1000.0 / result.AvgPacketsPerSec
	}
	if result.AvgRecordsPerSec > 0 {
		avgTimePerRecordUs = 1000000.0 / result.AvgRecordsPerSec
		avgTimePerRecordNs = 1000000000.0 / result.AvgRecordsPerSec
	}
	fmt.Printf("│ Avg Time/Packet:       %-54s│\n", fmt.Sprintf("%.4f ms", avgTimePerPacketMs))
	fmt.Printf("│ Avg Time/Record:       %-54s│\n", fmt.Sprintf("%.2f µs", avgTimePerRecordUs))
	fmt.Printf("│ Avg Time/Record:       %-54s│\n", fmt.Sprintf("%.0f ns", avgTimePerRecordNs))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	// Decode Latency
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ LATENCY - DECODE (Packet Parsing)                                           │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Min:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.MinDecodeLatencyUs))
	fmt.Printf("│ Avg:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.AvgDecodeLatencyUs))
	fmt.Printf("│ Max:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.MaxDecodeLatencyUs))
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ P50 (Median):          %-54s│\n", fmt.Sprintf("%.2f µs", result.DecodePercentiles.P50))
	fmt.Printf("│ P75:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.DecodePercentiles.P75))
	fmt.Printf("│ P90:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.DecodePercentiles.P90))
	fmt.Printf("│ P95:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.DecodePercentiles.P95))
	fmt.Printf("│ P99:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.DecodePercentiles.P99))
	fmt.Printf("│ P99.9:                 %-54s│\n", fmt.Sprintf("%.2f µs", result.DecodePercentiles.P999))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	// Process Latency
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ LATENCY - PROCESS (Full Pipeline)                                           │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Min:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.MinProcessLatencyUs))
	fmt.Printf("│ Avg:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.AvgProcessLatencyUs))
	fmt.Printf("│ Max:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.MaxProcessLatencyUs))
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ P50 (Median):          %-54s│\n", fmt.Sprintf("%.2f µs", result.ProcessPercentiles.P50))
	fmt.Printf("│ P75:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.ProcessPercentiles.P75))
	fmt.Printf("│ P90:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.ProcessPercentiles.P90))
	fmt.Printf("│ P95:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.ProcessPercentiles.P95))
	fmt.Printf("│ P99:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.ProcessPercentiles.P99))
	fmt.Printf("│ P99.9:                 %-54s│\n", fmt.Sprintf("%.2f µs", result.ProcessPercentiles.P999))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	// Packet Loss - UPDATED to show both metrics
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ PACKET LOSS ANALYSIS                                                        │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Println("│ 📦 ACTUAL DROPS (Ring Buffer Overflow):                                     │")
	fmt.Printf("│    Ring Overflow:      %-54d│\n", result.RingOverflow)
	fmt.Printf("│    Drop Rate:          %-54s│\n", fmt.Sprintf("%.4f%%", result.PacketLossRate))
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Println("│ 📊 SEQUENCE GAPS (Normal for sparse token updates):                         │")
	fmt.Printf("│    Sequence Gaps:      %-54d│\n", result.SequenceGaps)
	fmt.Printf("│    Gap Packets:        %-54d│\n", result.MissedPackets)
	fmt.Printf("│    Tracked Tokens:     %-54d│\n", result.TrackedTokens)
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	// Memory
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ MEMORY & SYSTEM                                                             │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Peak Memory:           %-54s│\n", fmt.Sprintf("%.2f MB", result.PeakMemoryMB))
	fmt.Printf("│ Avg Memory:            %-54s│\n", fmt.Sprintf("%.2f MB", result.AvgMemoryMB))
	fmt.Printf("│ GC Cycles:             %-54d│\n", result.TotalGCPauses)
	fmt.Printf("│ GOMAXPROCS:            %-54d│\n", runtime.GOMAXPROCS(0))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	// HFT Assessment
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ HFT READINESS ASSESSMENT                                                    │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")

	// Latency assessment using P99
	p99OK := result.ProcessPercentiles.P99 < 100
	avgOK := result.AvgProcessLatencyUs < 50
	if p99OK && avgOK {
		fmt.Printf("│ ✅ Latency:            %-54s│\n",
			fmt.Sprintf("EXCELLENT (P99: %.1fµs, Avg: %.1fµs)", result.ProcessPercentiles.P99, result.AvgProcessLatencyUs))
	} else if result.ProcessPercentiles.P99 < 1000 {
		fmt.Printf("│ ⚠️  Latency:            %-54s│\n",
			fmt.Sprintf("ACCEPTABLE (P99: %.1fµs < 1ms)", result.ProcessPercentiles.P99))
	} else {
		fmt.Printf("│ ❌ Latency:            %-54s│\n",
			fmt.Sprintf("NEEDS WORK (P99: %.1fµs)", result.ProcessPercentiles.P99))
	}

	// Throughput assessment
	if result.AvgPacketsPerSec > 100 {
		fmt.Printf("│ ✅ Throughput:         %-54s│\n", "GOOD (Can handle feed rate)")
	} else {
		fmt.Printf("│ ⚠️  Throughput:         %-54s│\n", "LOW (Check network)")
	}

	// Memory assessment
	if result.PeakMemoryMB < 100 {
		fmt.Printf("│ ✅ Memory:             %-54s│\n", "EXCELLENT (<100MB)")
	} else if result.PeakMemoryMB < 500 {
		fmt.Printf("│ ✅ Memory:             %-54s│\n", "GOOD (<500MB)")
	} else {
		fmt.Printf("│ ⚠️  Memory:             %-54s│\n", "HIGH (Consider optimization)")
	}

	// Packet loss assessment - based on ACTUAL drops, not sequence gaps
	if result.RingOverflow == 0 {
		fmt.Printf("│ ✅ Packet Loss:        %-54s│\n", "ZERO DROPS (Ring buffer handled all)")
	} else if result.PacketLossRate < 0.1 {
		fmt.Printf("│ ⚠️  Packet Loss:        %-54s│\n",
			fmt.Sprintf("MINIMAL (%d drops, %.4f%%)", result.RingOverflow, result.PacketLossRate))
	} else {
		fmt.Printf("│ ❌ Packet Loss:        %-54s│\n",
			fmt.Sprintf("DROPS DETECTED (%d, %.2f%%)", result.RingOverflow, result.PacketLossRate))
	}

	// Sequence gaps info (not a problem)
	if result.SequenceGaps > 0 {
		fmt.Printf("│ ℹ️  Sequence Gaps:      %-54s│\n",
			fmt.Sprintf("%d gaps (normal for multi-token feeds)", result.SequenceGaps))
	}

	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("████████████████████████████████████████████████████████████████████████████████")
}

// ================================================================================
// MAIN
// ================================================================================

func main() {
	multicastIP := flag.String("ip", DefaultMulticastIP, "Multicast IP address")
	port := flag.Int("port", DefaultPort, "UDP port")
	duration := flag.Duration("duration", 0, "Benchmark duration (0 = run until Ctrl+C)")
	flag.Parse()

	fmt.Println("================================================================================")
	fmt.Println("         BSE GO HFT BENCHMARK - COMPREHENSIVE STATISTICS                       ")
	fmt.Println("================================================================================")
	fmt.Printf("Multicast IP:    %s\n", *multicastIP)
	fmt.Printf("Port:            %d\n", *port)
	fmt.Printf("Socket Buffer:   %d MB\n", SocketRcvBufSize/(1024*1024))
	fmt.Printf("Ring Buffer:     %d slots\n", RingBufferSize)
	fmt.Printf("GOMAXPROCS:      %d\n", runtime.GOMAXPROCS(0))
	fmt.Printf("Start Time:      %s\n", time.Now().Format("2006-01-02 15:04:05"))
	if *duration > 0 {
		fmt.Printf("Duration:        %s\n", *duration)
	} else {
		fmt.Printf("Duration:        Until Ctrl+C\n")
	}
	fmt.Println("================================================================================")
	fmt.Println()
	fmt.Println("Metrics tracked:")
	fmt.Println("  • Latency: P50, P75, P90, P95, P99, P99.9 (µs)")
	fmt.Println("  • Ring Buffer Overflow: Actual packet drops")
	fmt.Println("  • Sequence Gaps: Per-token sequence tracking")
	fmt.Println("  • Memory: Peak, Average, GC cycles")
	fmt.Println("  • Throughput: Packets/sec, Records/sec, MB/s")
	fmt.Println()

	collector := NewStatsCollector()
	receiver := NewMulticastReceiver(*multicastIP, *port, collector)

	fmt.Println("Connecting to multicast feed...")
	if err := receiver.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer receiver.Close()
	fmt.Println("✅ Connected! Receiving packets...")
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	if *duration > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), *duration)
	}
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\n⚠️  Received interrupt signal, generating report...")
		cancel()
	}()

	// Start receiver goroutine (fast packet capture)
	go receiver.ReceiveLoop(ctx)

	// Start processor goroutine (decode packets)
	go receiver.ProcessLoop(ctx)

	// Stats ticker
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			result := collector.GetResult()
			result.RingOverflow = receiver.GetOverflow()
			// Calculate actual loss rate from ring overflow
			if result.TotalPackets > 0 {
				result.PacketLossRate = float64(result.RingOverflow) / float64(result.TotalPackets+result.RingOverflow) * 100.0
			}
			PrintFinalReport(result)
			return

		case <-ticker.C:
			collector.SampleSystem()
			PrintLiveStats(collector, time.Since(startTime), receiver.GetOverflow())
		}
	}
}
