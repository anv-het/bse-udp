// BSE Equity (CM) Feed Statistics & Benchmark Tool
// Measures: Packet rates, CPU/Memory usage, Processing latency
// For HFT system evaluation
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
	// BSE Equity (CM) Multicast Feed
	DefaultMulticastIP = "239.1.2.5"
	DefaultPort        = 26001
	BufferSize         = 65536

	// Statistics intervals
	StatsIntervalSec = 1 // Per-second stats
	ReportInterval5m = 5 // 5-minute summary
)

// ================================================================================
// STATISTICS STRUCTURES
// ================================================================================

// PacketStats holds packet-level statistics
type PacketStats struct {
	TotalPackets   uint64
	TotalBytes     uint64
	ValidPackets   uint64
	InvalidPackets uint64
	DroppedPackets uint64
	PacketSizes    map[int]uint64 // Distribution of packet sizes
	mu             sync.RWMutex
}

// LatencyStats holds timing statistics (in nanoseconds)
type LatencyStats struct {
	DecodeTotal  int64
	DecodeCount  int64
	DecodeMin    int64
	DecodeMax    int64
	ProcessTotal int64
	ProcessCount int64
	ProcessMin   int64
	ProcessMax   int64

	// Sample storage for percentile calculation
	DecodeSamples  []int64
	ProcessSamples []int64
	maxSamples     int
	mu             sync.Mutex
}

// LatencyPercentiles holds calculated percentile values
type LatencyPercentiles struct {
	P50 float64 // Median
	P75 float64
	P90 float64
	P95 float64
	P99 float64
}

// SystemStats holds system resource usage
type SystemStats struct {
	StartTime       time.Time
	LastSampleTime  time.Time
	MemAlloc        uint64
	MemSys          uint64
	NumGoroutines   int
	NumGC           uint32
	GCPauseTotal    uint64
	CPUUsagePercent float64
}

// IntervalStats holds per-interval statistics
type IntervalStats struct {
	Timestamp         time.Time
	PacketsPerSec     uint64
	BytesPerSec       uint64
	RecordsPerSec     uint64
	AvgDecodeLatency  float64 // microseconds
	AvgProcessLatency float64 // microseconds
	MemAllocMB        float64
	NumGoroutines     int
}

// BenchmarkResult holds final benchmark results
type BenchmarkResult struct {
	Duration            time.Duration
	TotalPackets        uint64
	TotalBytes          uint64
	TotalRecords        uint64
	AvgPacketsPerSec    float64
	AvgBytesPerSec      float64
	AvgRecordsPerSec    float64
	MaxPacketsPerSec    uint64
	MinPacketsPerSec    uint64
	AvgDecodeLatencyUs  float64
	MaxDecodeLatencyUs  float64
	MinDecodeLatencyUs  float64
	DecodePercentiles   LatencyPercentiles
	AvgProcessLatencyUs float64
	MaxProcessLatencyUs float64
	MinProcessLatencyUs float64
	ProcessPercentiles  LatencyPercentiles
	PeakMemoryMB        float64
	AvgMemoryMB         float64
	TotalGCPauses       uint64
}

// ================================================================================
// STATISTICS COLLECTOR
// ================================================================================

type StatsCollector struct {
	packetStats  *PacketStats
	latencyStats *LatencyStats
	systemStats  *SystemStats
	intervalHist []IntervalStats

	// Atomic counters for lock-free updates
	packetsThisSec uint64
	bytesThisSec   uint64
	recordsThisSec uint64
	totalRecords   uint64

	// Peak tracking
	maxPacketsPerSec   uint64
	minPacketsPerSec   uint64
	peakMemoryMB       float64
	totalMemorySamples float64
	memorySampleCount  int

	mu sync.Mutex
}

func NewStatsCollector() *StatsCollector {
	maxSamples := 100000 // Keep up to 100k samples for percentile calculation
	return &StatsCollector{
		packetStats: &PacketStats{
			PacketSizes: make(map[int]uint64),
		},
		latencyStats: &LatencyStats{
			DecodeMin:      int64(^uint64(0) >> 1), // Max int64
			ProcessMin:     int64(^uint64(0) >> 1),
			DecodeSamples:  make([]int64, 0, maxSamples),
			ProcessSamples: make([]int64, 0, maxSamples),
			maxSamples:     maxSamples,
		},
		systemStats: &SystemStats{
			StartTime: time.Now(),
		},
		intervalHist:     make([]IntervalStats, 0, 3600), // 1 hour of per-second stats
		minPacketsPerSec: ^uint64(0),                     // Max uint64
	}
}

// RecordPacket records a received packet
func (s *StatsCollector) RecordPacket(size int, valid bool) {
	atomic.AddUint64(&s.packetStats.TotalPackets, 1)
	atomic.AddUint64(&s.packetStats.TotalBytes, uint64(size))
	atomic.AddUint64(&s.packetsThisSec, 1)
	atomic.AddUint64(&s.bytesThisSec, uint64(size))

	if valid {
		atomic.AddUint64(&s.packetStats.ValidPackets, 1)
	} else {
		atomic.AddUint64(&s.packetStats.InvalidPackets, 1)
	}

	// Record packet size distribution (with lock)
	s.packetStats.mu.Lock()
	s.packetStats.PacketSizes[size]++
	s.packetStats.mu.Unlock()
}

// RecordRecords records number of records decoded from a packet
func (s *StatsCollector) RecordRecords(count int) {
	atomic.AddUint64(&s.recordsThisSec, uint64(count))
	atomic.AddUint64(&s.totalRecords, uint64(count))
}

// RecordDecodeLatency records decode operation timing
func (s *StatsCollector) RecordDecodeLatency(nanos int64) {
	s.latencyStats.mu.Lock()
	defer s.latencyStats.mu.Unlock()

	s.latencyStats.DecodeTotal += nanos
	s.latencyStats.DecodeCount++
	if nanos < s.latencyStats.DecodeMin {
		s.latencyStats.DecodeMin = nanos
	}
	if nanos > s.latencyStats.DecodeMax {
		s.latencyStats.DecodeMax = nanos
	}

	// Store sample for percentile calculation
	if len(s.latencyStats.DecodeSamples) < s.latencyStats.maxSamples {
		s.latencyStats.DecodeSamples = append(s.latencyStats.DecodeSamples, nanos)
	}
}

// RecordProcessLatency records full processing pipeline timing
func (s *StatsCollector) RecordProcessLatency(nanos int64) {
	s.latencyStats.mu.Lock()
	defer s.latencyStats.mu.Unlock()

	s.latencyStats.ProcessTotal += nanos
	s.latencyStats.ProcessCount++
	if nanos < s.latencyStats.ProcessMin {
		s.latencyStats.ProcessMin = nanos
	}
	if nanos > s.latencyStats.ProcessMax {
		s.latencyStats.ProcessMax = nanos
	}

	// Store sample for percentile calculation
	if len(s.latencyStats.ProcessSamples) < s.latencyStats.maxSamples {
		s.latencyStats.ProcessSamples = append(s.latencyStats.ProcessSamples, nanos)
	}
}

// calculatePercentile calculates the percentile value from sorted samples
func calculatePercentile(sortedSamples []int64, percentile float64) float64 {
	if len(sortedSamples) == 0 {
		return 0
	}
	index := int(float64(len(sortedSamples)-1) * percentile / 100.0)
	return float64(sortedSamples[index]) / 1000.0 // Convert ns to µs
}

// GetLatencyPercentiles calculates and returns latency percentiles
func (s *StatsCollector) GetLatencyPercentiles() (decode, process LatencyPercentiles) {
	s.latencyStats.mu.Lock()
	defer s.latencyStats.mu.Unlock()

	// Sort decode samples
	if len(s.latencyStats.DecodeSamples) > 0 {
		sortedDecode := make([]int64, len(s.latencyStats.DecodeSamples))
		copy(sortedDecode, s.latencyStats.DecodeSamples)
		sort.Slice(sortedDecode, func(i, j int) bool { return sortedDecode[i] < sortedDecode[j] })

		decode = LatencyPercentiles{
			P50: calculatePercentile(sortedDecode, 50),
			P75: calculatePercentile(sortedDecode, 75),
			P90: calculatePercentile(sortedDecode, 90),
			P95: calculatePercentile(sortedDecode, 95),
			P99: calculatePercentile(sortedDecode, 99),
		}
	}

	// Sort process samples
	if len(s.latencyStats.ProcessSamples) > 0 {
		sortedProcess := make([]int64, len(s.latencyStats.ProcessSamples))
		copy(sortedProcess, s.latencyStats.ProcessSamples)
		sort.Slice(sortedProcess, func(i, j int) bool { return sortedProcess[i] < sortedProcess[j] })

		process = LatencyPercentiles{
			P50: calculatePercentile(sortedProcess, 50),
			P75: calculatePercentile(sortedProcess, 75),
			P90: calculatePercentile(sortedProcess, 90),
			P95: calculatePercentile(sortedProcess, 95),
			P99: calculatePercentile(sortedProcess, 99),
		}
	}

	return decode, process
}

// CaptureIntervalStats captures per-second statistics
func (s *StatsCollector) CaptureIntervalStats() IntervalStats {
	// Atomically swap counters
	packets := atomic.SwapUint64(&s.packetsThisSec, 0)
	bytes := atomic.SwapUint64(&s.bytesThisSec, 0)
	records := atomic.SwapUint64(&s.recordsThisSec, 0)

	// Track peaks
	if packets > s.maxPacketsPerSec {
		s.maxPacketsPerSec = packets
	}
	if packets < s.minPacketsPerSec && packets > 0 {
		s.minPacketsPerSec = packets
	}

	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	memMB := float64(memStats.Alloc) / 1024 / 1024
	if memMB > s.peakMemoryMB {
		s.peakMemoryMB = memMB
	}
	s.totalMemorySamples += memMB
	s.memorySampleCount++

	// Calculate average latencies
	s.latencyStats.mu.Lock()
	avgDecodeUs := float64(0)
	if s.latencyStats.DecodeCount > 0 {
		avgDecodeUs = float64(s.latencyStats.DecodeTotal) / float64(s.latencyStats.DecodeCount) / 1000.0
	}
	avgProcessUs := float64(0)
	if s.latencyStats.ProcessCount > 0 {
		avgProcessUs = float64(s.latencyStats.ProcessTotal) / float64(s.latencyStats.ProcessCount) / 1000.0
	}
	s.latencyStats.mu.Unlock()

	stats := IntervalStats{
		Timestamp:         time.Now(),
		PacketsPerSec:     packets,
		BytesPerSec:       bytes,
		RecordsPerSec:     records,
		AvgDecodeLatency:  avgDecodeUs,
		AvgProcessLatency: avgProcessUs,
		MemAllocMB:        memMB,
		NumGoroutines:     runtime.NumGoroutine(),
	}

	s.mu.Lock()
	s.intervalHist = append(s.intervalHist, stats)
	s.mu.Unlock()

	return stats
}

// GetFinalResult computes final benchmark results
func (s *StatsCollector) GetFinalResult() BenchmarkResult {
	duration := time.Since(s.systemStats.StartTime)
	totalPackets := atomic.LoadUint64(&s.packetStats.TotalPackets)
	totalBytes := atomic.LoadUint64(&s.packetStats.TotalBytes)
	totalRecords := atomic.LoadUint64(&s.totalRecords)

	durationSec := duration.Seconds()
	if durationSec == 0 {
		durationSec = 1
	}

	s.latencyStats.mu.Lock()
	avgDecodeUs := float64(0)
	maxDecodeUs := float64(s.latencyStats.DecodeMax) / 1000.0
	minDecodeUs := float64(0)
	if s.latencyStats.DecodeMin < int64(^uint64(0)>>1) {
		minDecodeUs = float64(s.latencyStats.DecodeMin) / 1000.0
	}
	if s.latencyStats.DecodeCount > 0 {
		avgDecodeUs = float64(s.latencyStats.DecodeTotal) / float64(s.latencyStats.DecodeCount) / 1000.0
	}
	avgProcessUs := float64(0)
	maxProcessUs := float64(s.latencyStats.ProcessMax) / 1000.0
	minProcessUs := float64(0)
	if s.latencyStats.ProcessMin < int64(^uint64(0)>>1) {
		minProcessUs = float64(s.latencyStats.ProcessMin) / 1000.0
	}
	if s.latencyStats.ProcessCount > 0 {
		avgProcessUs = float64(s.latencyStats.ProcessTotal) / float64(s.latencyStats.ProcessCount) / 1000.0
	}
	s.latencyStats.mu.Unlock()

	// Get percentiles
	decodePercentiles, processPercentiles := s.GetLatencyPercentiles()

	avgMemMB := float64(0)
	if s.memorySampleCount > 0 {
		avgMemMB = s.totalMemorySamples / float64(s.memorySampleCount)
	}

	// Get GC stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return BenchmarkResult{
		Duration:            duration,
		TotalPackets:        totalPackets,
		TotalBytes:          totalBytes,
		TotalRecords:        totalRecords,
		AvgPacketsPerSec:    float64(totalPackets) / durationSec,
		AvgBytesPerSec:      float64(totalBytes) / durationSec,
		AvgRecordsPerSec:    float64(totalRecords) / durationSec,
		MaxPacketsPerSec:    s.maxPacketsPerSec,
		MinPacketsPerSec:    s.minPacketsPerSec,
		AvgDecodeLatencyUs:  avgDecodeUs,
		MaxDecodeLatencyUs:  maxDecodeUs,
		MinDecodeLatencyUs:  minDecodeUs,
		DecodePercentiles:   decodePercentiles,
		AvgProcessLatencyUs: avgProcessUs,
		MaxProcessLatencyUs: maxProcessUs,
		MinProcessLatencyUs: minProcessUs,
		ProcessPercentiles:  processPercentiles,
		PeakMemoryMB:        s.peakMemoryMB,
		AvgMemoryMB:         avgMemMB,
		TotalGCPauses:       uint64(memStats.NumGC),
	}
}

// ================================================================================
// SIMPLE DECODER (Minimal overhead for benchmarking)
// ================================================================================

// DecodePacketMinimal decodes packet with minimal overhead, returns record count
func DecodePacketMinimal(packet []byte) (recordCount int, valid bool) {
	if len(packet) < 36 {
		return 0, false
	}

	// Validate leading zeros
	if packet[0] != 0 || packet[1] != 0 || packet[2] != 0 || packet[3] != 0 {
		return 0, false
	}

	// Check message type (offset 8-9)
	msgType := uint16(packet[8]) | uint16(packet[9])<<8
	if msgType != 2020 && msgType != 2021 {
		return 0, false
	}

	// Calculate number of records (264 bytes each)
	recordSize := 264
	headerSize := 36
	dataSize := len(packet) - headerSize
	recordCount = dataSize / recordSize

	return recordCount, true
}

// ================================================================================
// MULTICAST RECEIVER
// ================================================================================

type MulticastReceiver struct {
	multicastIP string
	port        int
	conn        *net.UDPConn
	stats       *StatsCollector
}

func NewMulticastReceiver(ip string, port int, stats *StatsCollector) *MulticastReceiver {
	return &MulticastReceiver{
		multicastIP: ip,
		port:        port,
		stats:       stats,
	}
}

func (r *MulticastReceiver) Connect() error {
	addr := net.UDPAddr{
		IP:   net.IPv4zero,
		Port: r.port,
	}

	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		return fmt.Errorf("failed to create UDP socket: %w", err)
	}

	// Join multicast group
	group := net.ParseIP(r.multicastIP)
	p := ipv4.NewPacketConn(conn)
	if err := p.JoinGroup(nil, &net.UDPAddr{IP: group, Port: r.port}); err != nil {
		log.Printf("Warning: failed to join multicast group: %v", err)
	}

	// Set large buffer for HFT
	if err := conn.SetReadBuffer(BufferSize); err != nil {
		log.Printf("Warning: failed to set read buffer: %v", err)
	}

	r.conn = conn
	return nil
}

func (r *MulticastReceiver) ReceiveLoop(ctx context.Context) {
	buffer := make([]byte, BufferSize)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set short deadline to check context
		r.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, _, err := r.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("Read error: %v", err)
				continue
			}
		}

		// Start timing
		processStart := time.Now()

		// Decode packet
		decodeStart := time.Now()
		recordCount, valid := DecodePacketMinimal(buffer[:n])
		decodeEnd := time.Now()

		// Record statistics
		r.stats.RecordPacket(n, valid)
		if valid {
			r.stats.RecordRecords(recordCount)
		}
		r.stats.RecordDecodeLatency(decodeEnd.Sub(decodeStart).Nanoseconds())
		r.stats.RecordProcessLatency(time.Since(processStart).Nanoseconds())
	}
}

func (r *MulticastReceiver) Close() {
	if r.conn != nil {
		r.conn.Close()
	}
}

// ================================================================================
// STATISTICS REPORTER
// ================================================================================

func PrintLiveStats(stats IntervalStats, elapsed time.Duration) {
	fmt.Printf("\r[%s] Pkts/s: %6d | Bytes/s: %8d | Records/s: %6d | Decode: %6.1fµs | Mem: %6.1fMB | Goroutines: %3d",
		elapsed.Truncate(time.Second),
		stats.PacketsPerSec,
		stats.BytesPerSec,
		stats.RecordsPerSec,
		stats.AvgDecodeLatency,
		stats.MemAllocMB,
		stats.NumGoroutines,
	)
}

func Print5MinSummary(collector *StatsCollector, elapsed time.Duration) {
	fmt.Println("\n")
	fmt.Println("================================================================================")
	fmt.Printf("  5-MINUTE SUMMARY @ %s (Elapsed: %s)\n", time.Now().Format("15:04:05"), elapsed.Truncate(time.Second))
	fmt.Println("================================================================================")

	totalPackets := atomic.LoadUint64(&collector.packetStats.TotalPackets)
	totalBytes := atomic.LoadUint64(&collector.packetStats.TotalBytes)
	totalRecords := atomic.LoadUint64(&collector.totalRecords)
	validPackets := atomic.LoadUint64(&collector.packetStats.ValidPackets)

	fmt.Printf("  Total Packets:      %d\n", totalPackets)
	fmt.Printf("  Total Bytes:        %d (%.2f MB)\n", totalBytes, float64(totalBytes)/1024/1024)
	fmt.Printf("  Total Records:      %d\n", totalRecords)
	fmt.Printf("  Valid Packets:      %d (%.1f%%)\n", validPackets, float64(validPackets)/float64(totalPackets)*100)
	fmt.Printf("  Max Packets/s:      %d\n", collector.maxPacketsPerSec)
	fmt.Printf("  Min Packets/s:      %d\n", collector.minPacketsPerSec)
	fmt.Printf("  Peak Memory:        %.2f MB\n", collector.peakMemoryMB)

	// Packet size distribution
	fmt.Println("\n  Packet Size Distribution:")
	collector.packetStats.mu.RLock()
	for size, count := range collector.packetStats.PacketSizes {
		fmt.Printf("    %d bytes: %d packets\n", size, count)
	}
	collector.packetStats.mu.RUnlock()

	fmt.Println("================================================================================\n")
}

func PrintFinalReport(result BenchmarkResult) {
	fmt.Println("\n")
	fmt.Println("████████████████████████████████████████████████████████████████████████████████")
	fmt.Println("                      BSE EQUITY (CM) FEED BENCHMARK RESULTS                    ")
	fmt.Println("████████████████████████████████████████████████████████████████████████████████")
	fmt.Println()

	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ PACKET STATISTICS                                                           │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Duration:              %-54s│\n", result.Duration.Truncate(time.Second))
	fmt.Printf("│ Total Packets:         %-54d│\n", result.TotalPackets)
	fmt.Printf("│ Total Bytes:           %-54s│\n", fmt.Sprintf("%d (%.2f MB)", result.TotalBytes, float64(result.TotalBytes)/1024/1024))
	fmt.Printf("│ Total Records:         %-54d│\n", result.TotalRecords)
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ THROUGHPUT                                                                  │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Avg Packets/sec:       %-54.2f│\n", result.AvgPacketsPerSec)
	fmt.Printf("│ Max Packets/sec:       %-54d│\n", result.MaxPacketsPerSec)
	minPkts := result.MinPacketsPerSec
	if minPkts == ^uint64(0) {
		minPkts = 0
	}
	fmt.Printf("│ Min Packets/sec:       %-54d│\n", minPkts)
	fmt.Printf("│ Avg Bytes/sec:         %-54s│\n", fmt.Sprintf("%.2f (%.2f KB/s)", result.AvgBytesPerSec, result.AvgBytesPerSec/1024))
	fmt.Printf("│ Avg Records/sec:       %-54.2f│\n", result.AvgRecordsPerSec)
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ LATENCY - DECODE (Packet Parsing Time)                                      │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Min:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.MinDecodeLatencyUs))
	fmt.Printf("│ Avg:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.AvgDecodeLatencyUs))
	fmt.Printf("│ Max:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.MaxDecodeLatencyUs))
	fmt.Printf("│ P50 (Median):          %-54s│\n", fmt.Sprintf("%.2f µs", result.DecodePercentiles.P50))
	fmt.Printf("│ P75:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.DecodePercentiles.P75))
	fmt.Printf("│ P90:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.DecodePercentiles.P90))
	fmt.Printf("│ P95:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.DecodePercentiles.P95))
	fmt.Printf("│ P99:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.DecodePercentiles.P99))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ LATENCY - PROCESS (Full Pipeline Time)                                      │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Min:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.MinProcessLatencyUs))
	fmt.Printf("│ Avg:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.AvgProcessLatencyUs))
	fmt.Printf("│ Max:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.MaxProcessLatencyUs))
	fmt.Printf("│ P50 (Median):          %-54s│\n", fmt.Sprintf("%.2f µs", result.ProcessPercentiles.P50))
	fmt.Printf("│ P75:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.ProcessPercentiles.P75))
	fmt.Printf("│ P90:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.ProcessPercentiles.P90))
	fmt.Printf("│ P95:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.ProcessPercentiles.P95))
	fmt.Printf("│ P99:                   %-54s│\n", fmt.Sprintf("%.2f µs", result.ProcessPercentiles.P99))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ MEMORY & SYSTEM                                                             │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Peak Memory:           %-54s│\n", fmt.Sprintf("%.2f MB", result.PeakMemoryMB))
	fmt.Printf("│ Avg Memory:            %-54s│\n", fmt.Sprintf("%.2f MB", result.AvgMemoryMB))
	fmt.Printf("│ Total GC Pauses:       %-54d│\n", result.TotalGCPauses)
	fmt.Printf("│ GOMAXPROCS:            %-54d│\n", runtime.GOMAXPROCS(0))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ HFT READINESS ASSESSMENT                                                    │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")

	// Assess HFT readiness using P99 latency
	p99LatencyOK := result.ProcessPercentiles.P99 < 100
	latencyOK := result.AvgProcessLatencyUs < 100 // <100µs is good for HFT
	throughputOK := result.AvgPacketsPerSec > 100 // Can handle >100 packets/sec
	memoryOK := result.PeakMemoryMB < 500         // <500MB is reasonable

	if p99LatencyOK && latencyOK {
		fmt.Printf("│ ✅ Latency:            %-54s│\n", fmt.Sprintf("EXCELLENT (P99: %.1fµs, Avg: %.1fµs)", result.ProcessPercentiles.P99, result.AvgProcessLatencyUs))
	} else if result.ProcessPercentiles.P99 < 1000 {
		fmt.Printf("│ ⚠️  Latency:            %-54s│\n", fmt.Sprintf("ACCEPTABLE (P99: %.1fµs < 1ms)", result.ProcessPercentiles.P99))
	} else {
		fmt.Printf("│ ❌ Latency:            %-54s│\n", fmt.Sprintf("NEEDS WORK (P99: %.1fµs > 1ms)", result.ProcessPercentiles.P99))
	}

	if throughputOK {
		fmt.Printf("│ ✅ Throughput:         %-54s│\n", "GOOD (Can handle feed rate)")
	} else {
		fmt.Printf("│ ⚠️  Throughput:         %-54s│\n", "LOW (Check network)")
	}

	if memoryOK {
		fmt.Printf("│ ✅ Memory:             %-54s│\n", "EFFICIENT (<500MB peak)")
	} else {
		fmt.Printf("│ ⚠️  Memory:             %-54s│\n", "HIGH (Consider optimization)")
	}

	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("████████████████████████████████████████████████████████████████████████████████")
}

// ================================================================================
// MAIN
// ================================================================================

func main() {
	// Command line flags
	multicastIP := flag.String("ip", DefaultMulticastIP, "Multicast IP address")
	port := flag.Int("port", DefaultPort, "UDP port")
	duration := flag.Duration("duration", 0, "Benchmark duration (0 = run until Ctrl+C)")
	flag.Parse()

	fmt.Println("================================================================================")
	fmt.Println("        BSE EQUITY (CM) FEED STATISTICS & BENCHMARK TOOL                       ")
	fmt.Println("================================================================================")
	fmt.Printf("Multicast IP:    %s\n", *multicastIP)
	fmt.Printf("Port:            %d\n", *port)
	fmt.Printf("Buffer Size:     %d bytes\n", BufferSize)
	fmt.Printf("GOMAXPROCS:      %d\n", runtime.GOMAXPROCS(0))
	fmt.Printf("Start Time:      %s\n", time.Now().Format("2006-01-02 15:04:05"))
	if *duration > 0 {
		fmt.Printf("Duration:        %s\n", *duration)
	} else {
		fmt.Printf("Duration:        Until Ctrl+C\n")
	}
	fmt.Println("================================================================================")
	fmt.Println()

	// Initialize
	stats := NewStatsCollector()
	receiver := NewMulticastReceiver(*multicastIP, *port, stats)

	// Connect
	fmt.Println("Connecting to multicast feed...")
	if err := receiver.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer receiver.Close()
	fmt.Println("✅ Connected! Receiving packets...")
	fmt.Println()

	// Context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	if *duration > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), *duration)
	}
	defer cancel()

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\n⚠️  Received interrupt signal, stopping...")
		cancel()
	}()

	// Start receiver
	go receiver.ReceiveLoop(ctx)

	// Statistics ticker
	ticker := time.NewTicker(time.Second)
	ticker5m := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	defer ticker5m.Stop()

	startTime := time.Now()

	// Main loop
	for {
		select {
		case <-ctx.Done():
			// Final report
			result := stats.GetFinalResult()
			PrintFinalReport(result)
			return

		case <-ticker.C:
			intervalStats := stats.CaptureIntervalStats()
			PrintLiveStats(intervalStats, time.Since(startTime))

		case <-ticker5m.C:
			Print5MinSummary(stats, time.Since(startTime))
		}
	}
}
