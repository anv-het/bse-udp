"""
BSE Market Data Live Statistics & Benchmark Module
===================================================

Provides real-time statistics display and final benchmark report.
Similar output format to the Go implementation.

Features:
- Packet/byte throughput tracking
- Decode/process latency percentiles (P50, P75, P90, P95, P99)
- Memory usage monitoring
- Live stats display every 5 seconds
- Final benchmark report on shutdown

Author: BSE Integration Team
Date: December 2025
"""

import os
import time
import threading
import statistics
from dataclasses import dataclass, field
from typing import List, Dict, Optional
from datetime import datetime

try:
    import psutil
    HAS_PSUTIL = True
except ImportError:
    HAS_PSUTIL = False


@dataclass
class LatencyStats:
    """Stores latency measurements for percentile calculation."""
    samples: List[float] = field(default_factory=list)
    max_samples: int = 100000  # Keep last 100k samples
    
    def add(self, latency_us: float):
        """Add a latency sample in microseconds."""
        self.samples.append(latency_us)
        if len(self.samples) > self.max_samples:
            self.samples = self.samples[-self.max_samples:]
    
    def get_percentile(self, p: float) -> float:
        """Get percentile value (p is 0-100)."""
        if not self.samples:
            return 0.0
        sorted_samples = sorted(self.samples)
        idx = int(len(sorted_samples) * p / 100)
        idx = min(idx, len(sorted_samples) - 1)
        return sorted_samples[idx]
    
    def get_stats(self) -> Dict[str, float]:
        """Get all latency statistics."""
        if not self.samples:
            return {
                'min': 0.0, 'avg': 0.0, 'max': 0.0,
                'p50': 0.0, 'p75': 0.0, 'p90': 0.0, 'p95': 0.0, 'p99': 0.0
            }
        
        return {
            'min': min(self.samples),
            'avg': statistics.mean(self.samples),
            'max': max(self.samples),
            'p50': self.get_percentile(50),
            'p75': self.get_percentile(75),
            'p90': self.get_percentile(90),
            'p95': self.get_percentile(95),
            'p99': self.get_percentile(99)
        }


@dataclass 
class SegmentStats:
    """Statistics for a single segment (CM or FO)."""
    segment: str
    start_time: float = field(default_factory=time.time)
    
    # Packet stats
    total_packets: int = 0
    total_bytes: int = 0
    total_records: int = 0
    total_quotes: int = 0
    
    # Per-second tracking for throughput
    packets_per_second: List[int] = field(default_factory=list)
    bytes_per_second: List[int] = field(default_factory=list)
    records_per_second: List[int] = field(default_factory=list)
    
    # Current interval counters
    interval_packets: int = 0
    interval_bytes: int = 0
    interval_records: int = 0
    interval_quotes: int = 0
    last_interval_time: float = field(default_factory=time.time)
    
    # Latency tracking
    decode_latency: LatencyStats = field(default_factory=LatencyStats)
    process_latency: LatencyStats = field(default_factory=LatencyStats)
    
    # Memory tracking (MB)
    memory_samples: List[float] = field(default_factory=list)
    
    # Lock for thread safety
    _lock: threading.Lock = field(default_factory=threading.Lock)
    
    def add_packet(self, packet_size: int, records: int = 0, quotes: int = 0,
                   decode_time_us: float = 0, process_time_us: float = 0):
        """Record a processed packet."""
        with self._lock:
            self.total_packets += 1
            self.total_bytes += packet_size
            self.total_records += records
            self.total_quotes += quotes
            
            self.interval_packets += 1
            self.interval_bytes += packet_size
            self.interval_records += records
            self.interval_quotes += quotes
            
            if decode_time_us > 0:
                self.decode_latency.add(decode_time_us)
            if process_time_us > 0:
                self.process_latency.add(process_time_us)
    
    def record_interval(self) -> Dict:
        """Record current interval stats and reset counters. Returns interval data."""
        with self._lock:
            interval_data = {
                'packets': self.interval_packets,
                'bytes': self.interval_bytes,
                'records': self.interval_records,
                'quotes': self.interval_quotes
            }
            
            self.packets_per_second.append(self.interval_packets)
            self.bytes_per_second.append(self.interval_bytes)
            self.records_per_second.append(self.interval_records)
            
            # Record memory
            if HAS_PSUTIL:
                try:
                    process = psutil.Process(os.getpid())
                    mem_mb = process.memory_info().rss / (1024 * 1024)
                    self.memory_samples.append(mem_mb)
                    interval_data['memory_mb'] = mem_mb
                except:
                    pass
            
            # Reset interval counters
            self.interval_packets = 0
            self.interval_bytes = 0
            self.interval_records = 0
            self.interval_quotes = 0
            self.last_interval_time = time.time()
            
            return interval_data
    
    def get_throughput_stats(self) -> Dict:
        """Get throughput statistics."""
        with self._lock:
            if not self.packets_per_second:
                return {'avg_pps': 0, 'max_pps': 0, 'min_pps': 0,
                        'avg_bps': 0, 'avg_rps': 0}
            
            return {
                'avg_pps': statistics.mean(self.packets_per_second) if self.packets_per_second else 0,
                'max_pps': max(self.packets_per_second) if self.packets_per_second else 0,
                'min_pps': min(self.packets_per_second) if self.packets_per_second else 0,
                'avg_bps': statistics.mean(self.bytes_per_second) if self.bytes_per_second else 0,
                'avg_rps': statistics.mean(self.records_per_second) if self.records_per_second else 0
            }
    
    def get_memory_stats(self) -> Dict:
        """Get memory statistics."""
        with self._lock:
            if not self.memory_samples:
                if HAS_PSUTIL:
                    try:
                        process = psutil.Process(os.getpid())
                        current_mb = process.memory_info().rss / (1024 * 1024)
                        return {'peak': current_mb, 'avg': current_mb, 'current': current_mb}
                    except:
                        pass
                return {'peak': 0, 'avg': 0, 'current': 0}
            
            return {
                'peak': max(self.memory_samples),
                'avg': statistics.mean(self.memory_samples),
                'current': self.memory_samples[-1] if self.memory_samples else 0
            }


class LiveStatsTracker:
    """
    Main statistics tracking class.
    Tracks statistics for all segments and provides reporting.
    """
    
    def __init__(self):
        self.start_time = time.time()
        self.segments: Dict[str, SegmentStats] = {}
        self._lock = threading.Lock()
        self._interval_thread: Optional[threading.Thread] = None
        self._running = False
        self._print_lock = threading.Lock()
        
    def add_segment(self, segment: str):
        """Add a segment to track."""
        with self._lock:
            if segment not in self.segments:
                self.segments[segment] = SegmentStats(segment=segment)
    
    def get_segment(self, segment: str) -> SegmentStats:
        """Get stats for a segment."""
        with self._lock:
            if segment not in self.segments:
                self.add_segment(segment)
            return self.segments[segment]
    
    def start_interval_tracking(self, interval_sec: int = 5):
        """Start background thread to record interval stats."""
        self._running = True
        self._interval_thread = threading.Thread(
            target=self._interval_loop,
            args=(interval_sec,),
            daemon=True,
            name="LiveStatsInterval"
        )
        self._interval_thread.start()
    
    def _interval_loop(self, interval_sec: int):
        """Background loop to record interval stats and print live display."""
        last_print = time.time()
        
        while self._running:
            time.sleep(1)  # Check every second
            
            elapsed = time.time() - last_print
            if elapsed >= interval_sec:
                # Record interval for all segments and get data
                interval_data = {}
                for seg, seg_stats in self.segments.items():
                    interval_data[seg] = seg_stats.record_interval()
                
                # Print live stats
                self._print_live_stats(interval_data)
                last_print = time.time()
    
    def _print_live_stats(self, interval_data: Dict):
        """Print live statistics line."""
        with self._print_lock:
            total_elapsed = int(time.time() - self.start_time)
            
            lines = []
            for segment, data in interval_data.items():
                stats = self.segments[segment]
                pps = data['packets']
                bps = data['bytes']
                rps = data['records']
                
                decode_avg = stats.decode_latency.get_stats()['avg']
                mem = data.get('memory_mb', stats.get_memory_stats()['current'])
                
                line = (
                    f"[{total_elapsed}s] [{segment}] Pkts/s: {pps:>5} | "
                    f"Bytes/s: {bps:>8} | Records/s: {rps:>5} | "
                    f"Decode: {decode_avg:>6.1f}µs | Mem: {mem:>6.1f}MB"
                )
                lines.append(line)
            
            # Print all segment lines
            print()  # New line before stats
            for line in lines:
                print(line)
    
    def stop(self):
        """Stop interval tracking."""
        self._running = False
        if self._interval_thread:
            self._interval_thread.join(timeout=2)
    
    def get_duration(self) -> int:
        """Get total duration in seconds."""
        return int(time.time() - self.start_time)
    
    def print_header(self, multicast_ip: str = "239.1.2.5", 
                     cm_port: int = 26001, fo_port: int = 26002,
                     buffer_size: int = 2048,
                     cm_enabled: bool = True, fo_enabled: bool = True):
        """Print startup header similar to Go version."""
        import platform
        import multiprocessing
        
        start_str = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        cpu_count = multiprocessing.cpu_count()
        
        print("=" * 80)
        print("        BSE PYTHON DUAL FEED STATISTICS & BENCHMARK TOOL")
        print("=" * 80)
        print(f"Multicast IP:    {multicast_ip}")
        if cm_enabled:
            print(f"CM Port:         {cm_port}")
        if fo_enabled:
            print(f"FO Port:         {fo_port}")
        print(f"Buffer Size:     {buffer_size} bytes")
        print(f"CPU Cores:       {cpu_count}")
        print(f"Python:          {platform.python_version()}")
        print(f"Start Time:      {start_str}")
        print(f"Duration:        Until Ctrl+C")
        print("=" * 80)
        print()
        print("Connecting to multicast feed...")
    
    def print_connected(self):
        """Print connection success message."""
        print("✅ Connected! Receiving packets...")
        print()
    
    def print_final_report(self):
        """Print final benchmark report similar to Go version."""
        duration = self.get_duration()
        
        print("\n")
        print("█" * 80)
        print("                      BSE PYTHON FEED BENCHMARK RESULTS")
        print("█" * 80)
        print()
        
        for segment, stats in self.segments.items():
            self._print_segment_report(segment, stats, duration)
        
        # Print combined stats if multiple segments
        if len(self.segments) > 1:
            self._print_combined_report(duration)
        
        # Print HFT readiness
        self._print_hft_assessment()
        
        print("█" * 80)
        print()
    
    def _print_segment_report(self, segment: str, stats: SegmentStats, duration: int):
        """Print report for a single segment."""
        segment_name = "EQUITY CASH (CM)" if segment == "CM" else "DERIVATIVES (F&O)"
        
        throughput = stats.get_throughput_stats()
        decode_lat = stats.decode_latency.get_stats()
        process_lat = stats.process_latency.get_stats()
        memory = stats.get_memory_stats()
        
        print(f"┌{'─' * 77}┐")
        print(f"│ {segment_name:^75} │")
        print(f"├{'─' * 77}┤")
        
        # Packet Statistics
        print(f"│ {'PACKET STATISTICS':<75} │")
        print(f"├{'─' * 77}┤")
        self._print_row("Duration:", f"{duration}s")
        self._print_row("Total Packets:", f"{stats.total_packets:,}")
        
        bytes_mb = stats.total_bytes / (1024 * 1024)
        self._print_row("Total Bytes:", f"{stats.total_bytes:,} ({bytes_mb:.2f} MB)")
        self._print_row("Total Records:", f"{stats.total_records:,}")
        self._print_row("Total Quotes Saved:", f"{stats.total_quotes:,}")
        print(f"└{'─' * 77}┘")
        print()
        
        # Throughput
        print(f"┌{'─' * 77}┐")
        print(f"│ {'THROUGHPUT':<75} │")
        print(f"├{'─' * 77}┤")
        self._print_row("Avg Packets/sec:", f"{throughput['avg_pps']:.2f}")
        self._print_row("Max Packets/sec:", str(throughput['max_pps']))
        self._print_row("Min Packets/sec:", str(throughput['min_pps']))
        
        avg_bps = throughput['avg_bps']
        avg_kbps = avg_bps / 1024
        self._print_row("Avg Bytes/sec:", f"{avg_bps:.2f} ({avg_kbps:.2f} KB/s)")
        self._print_row("Avg Records/sec:", f"{throughput['avg_rps']:.2f}")
        print(f"└{'─' * 77}┘")
        print()
        
        # Decode Latency
        print(f"┌{'─' * 77}┐")
        print(f"│ {'LATENCY - DECODE (Packet Parsing Time)':<75} │")
        print(f"├{'─' * 77}┤")
        self._print_latency_rows(decode_lat)
        print(f"└{'─' * 77}┘")
        print()
        
        # Process Latency
        print(f"┌{'─' * 77}┐")
        print(f"│ {'LATENCY - PROCESS (Full Pipeline Time)':<75} │")
        print(f"├{'─' * 77}┤")
        self._print_latency_rows(process_lat)
        print(f"└{'─' * 77}┘")
        print()
        
        # Memory
        print(f"┌{'─' * 77}┐")
        print(f"│ {'MEMORY & SYSTEM':<75} │")
        print(f"├{'─' * 77}┤")
        self._print_row("Peak Memory:", f"{memory['peak']:.2f} MB")
        self._print_row("Avg Memory:", f"{memory['avg']:.2f} MB")
        
        thread_count = threading.active_count()
        self._print_row("Active Threads:", str(thread_count))
        print(f"└{'─' * 77}┘")
        print()
    
    def _print_row(self, label: str, value: str):
        """Print a single row with proper formatting."""
        label_formatted = f"{label:<20}"
        padding = 52 - len(value)
        print(f"│ {label_formatted}   {value}{' ' * max(0, padding)} │")
    
    def _print_latency_rows(self, lat_stats: Dict):
        """Print latency statistic rows."""
        rows = [
            ('Min:', f"{lat_stats['min']:.2f} µs"),
            ('Avg:', f"{lat_stats['avg']:.2f} µs"),
            ('Max:', f"{lat_stats['max']:.2f} µs"),
            ('P50 (Median):', f"{lat_stats['p50']:.2f} µs"),
            ('P75:', f"{lat_stats['p75']:.2f} µs"),
            ('P90:', f"{lat_stats['p90']:.2f} µs"),
            ('P95:', f"{lat_stats['p95']:.2f} µs"),
            ('P99:', f"{lat_stats['p99']:.2f} µs"),
        ]
        
        for label, value in rows:
            self._print_row(label, value)
    
    def _print_combined_report(self, duration: int):
        """Print combined statistics for all segments."""
        total_packets = sum(s.total_packets for s in self.segments.values())
        total_bytes = sum(s.total_bytes for s in self.segments.values())
        total_records = sum(s.total_records for s in self.segments.values())
        total_quotes = sum(s.total_quotes for s in self.segments.values())
        
        print(f"┌{'─' * 77}┐")
        print(f"│ {'COMBINED TOTALS (ALL SEGMENTS)':^75} │")
        print(f"├{'─' * 77}┤")
        self._print_row("Total Packets:", f"{total_packets:,}")
        
        bytes_mb = total_bytes / (1024 * 1024)
        self._print_row("Total Bytes:", f"{total_bytes:,} ({bytes_mb:.2f} MB)")
        self._print_row("Total Records:", f"{total_records:,}")
        self._print_row("Total Quotes:", f"{total_quotes:,}")
        print(f"└{'─' * 77}┘")
        print()
    
    def _print_hft_assessment(self):
        """Print HFT readiness assessment."""
        print(f"┌{'─' * 77}┐")
        print(f"│ {'HFT READINESS ASSESSMENT':^75} │")
        print(f"├{'─' * 77}┤")
        
        # Aggregate latency stats
        all_p99 = []
        all_avg = []
        all_throughput = []
        all_memory = []
        
        for stats in self.segments.values():
            process_lat = stats.process_latency.get_stats()
            all_p99.append(process_lat['p99'])
            all_avg.append(process_lat['avg'])
            all_throughput.append(stats.get_throughput_stats()['avg_pps'])
            all_memory.append(stats.get_memory_stats()['peak'])
        
        max_p99 = max(all_p99) if all_p99 else 0
        avg_lat = statistics.mean(all_avg) if all_avg else 0
        avg_throughput = statistics.mean(all_throughput) if all_throughput else 0
        max_memory = max(all_memory) if all_memory else 0
        
        # Latency assessment
        if max_p99 < 100:
            lat_line = f"✅ Latency:            EXCELLENT (P99: {max_p99:.1f}µs, Avg: {avg_lat:.1f}µs)"
        elif max_p99 < 1000:
            lat_line = f"⚠️  Latency:            GOOD (P99: {max_p99:.1f}µs)"
        else:
            lat_line = f"❌ Latency:            NEEDS IMPROVEMENT (P99: {max_p99:.1f}µs)"
        print(f"│ {lat_line:<75} │")
        
        # Throughput assessment
        if avg_throughput > 100:
            tp_line = "✅ Throughput:         GOOD (Can handle feed rate)"
        else:
            tp_line = "⚠️  Throughput:         LOW (Check connection)"
        print(f"│ {tp_line:<75} │")
        
        # Memory assessment
        if max_memory < 500:
            mem_line = "✅ Memory:             EFFICIENT (<500MB peak)"
        elif max_memory < 1000:
            mem_line = "⚠️  Memory:             MODERATE (<1GB peak)"
        else:
            mem_line = "❌ Memory:             HIGH (>1GB peak)"
        print(f"│ {mem_line:<75} │")
        
        print(f"└{'─' * 77}┘")
        print()


# Global tracker instance
_tracker: Optional[LiveStatsTracker] = None


def get_tracker() -> LiveStatsTracker:
    """Get or create the global stats tracker."""
    global _tracker
    if _tracker is None:
        _tracker = LiveStatsTracker()
    return _tracker


def reset_tracker() -> LiveStatsTracker:
    """Reset the global stats tracker."""
    global _tracker
    _tracker = LiveStatsTracker()
    return _tracker
