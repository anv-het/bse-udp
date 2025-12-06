"""
BSE Market Data Reader - Python Performance Benchmark
=====================================================

Benchmark tool to measure Python implementation performance
for comparison with Go implementation.

Usage:
    python src/benchmark.py --duration 30 --workers 1

Output:
    benchmark_python.json - Results in JSON format
"""

import argparse
import json
import multiprocessing
import struct
import time
from dataclasses import dataclass, asdict
from typing import List, Dict
import gc
import os
import psutil


@dataclass
class BenchmarkResults:
    """Stores benchmark results"""
    language: str
    duration_ns: int
    packets_processed: int
    bytes_processed: int
    packets_per_sec: float
    mb_per_sec: float
    latency_p50_us: float
    latency_p90_us: float
    latency_p99_us: float
    latency_max_us: float
    latency_mean_us: float
    memory_heap_mb: float
    memory_rss_mb: float
    gc_count: int
    gc_pause_total_ms: float


class LatencyTracker:
    """Tracks processing latencies"""
    
    def __init__(self):
        self.samples = []
    
    def record(self, duration_ns: int):
        """Record a latency sample in nanoseconds"""
        self.samples.append(duration_ns)
    
    def get_stats(self):
        """Calculate latency statistics"""
        if not self.samples:
            return 0, 0, 0, 0, 0
        
        sorted_samples = sorted(self.samples)
        n = len(sorted_samples)
        
        # Calculate percentiles (convert ns to μs)
        p50 = sorted_samples[int(n * 0.50)] / 1000.0
        p90 = sorted_samples[int(n * 0.90)] / 1000.0
        p99 = sorted_samples[int(n * 0.99)] / 1000.0
        max_val = sorted_samples[-1] / 1000.0
        mean = sum(sorted_samples) / n / 1000.0
        
        return p50, p90, p99, max_val, mean


def simulate_packet_processing(packet: bytes) -> Dict:
    """
    Simulate BSE packet processing (decode/normalize/serialize)
    Matches Python implementation patterns
    """
    # Simulate binary decoding (struct.unpack)
    token = struct.unpack('<I', packet[0:4])[0]
    open_price = struct.unpack('<i', packet[4:8])[0]
    prev_close = struct.unpack('<i', packet[8:12])[0]
    high_price = struct.unpack('<i', packet[12:16])[0]
    low_price = struct.unpack('<i', packet[16:20])[0]
    volume = struct.unpack('<i', packet[24:28])[0]
    ltp = struct.unpack('<i', packet[36:40])[0]
    
    # Simulate normalization (dict creation)
    result = {
        'token': token,
        'ltp': ltp / 100.0,  # paise to rupees
        'volume': volume,
        'open': open_price / 100.0,
        'high': high_price / 100.0,
        'low': low_price / 100.0,
        'prev_close': prev_close / 100.0,
    }
    
    # Simulate JSON serialization
    json_str = json.dumps(result)
    
    return result


def worker_process(duration: float, result_queue):
    """Worker process for multiprocessing benchmark"""
    latency = LatencyTracker()
    packets_processed = 0
    bytes_processed = 0
    
    packet = bytearray(828)  # Typical BSE packet size
    start_time = time.time()
    
    while time.time() - start_time < duration:
        pkt_start = time.perf_counter_ns()
        
        # Process packet
        simulate_packet_processing(bytes(packet))
        
        pkt_duration = time.perf_counter_ns() - pkt_start
        latency.record(pkt_duration)
        
        packets_processed += 1
        bytes_processed += 828
        
        # Small delay to prevent CPU saturation
        time.sleep(0.000001)  # 1 microsecond
    
    # Send results back
    p50, p90, p99, max_val, mean = latency.get_stats()
    result_queue.put({
        'packets_processed': packets_processed,
        'bytes_processed': bytes_processed,
        'latency_p50': p50,
        'latency_p90': p90,
        'latency_p99': p99,
        'latency_max': max_val,
        'latency_mean': mean
    })


def run_benchmark(duration: float, workers: int) -> BenchmarkResults:
    """Run Python benchmark"""
    print("╔══════════════════════════════════════════════════════════════════════════════╗")
    print("║                    PYTHON IMPLEMENTATION BENCHMARK                           ║")
    print("╚══════════════════════════════════════════════════════════════════════════════╝")
    print(f"  Duration: {duration}s | Workers: {workers} | CPU Cores: {multiprocessing.cpu_count()}\n")
    
    # Force garbage collection
    gc.collect()
    
    # Get initial memory
    process = psutil.Process()
    initial_mem = process.memory_info().rss
    gc_stats_before = gc.get_stats()
    
    # Start time
    start_time = time.time()
    
    if workers == 1:
        # Single-threaded (Python default due to GIL)
        latency = LatencyTracker()
        packets_processed = 0
        bytes_processed = 0
        
        packet = bytearray(828)
        
        while time.time() - start_time < duration:
            pkt_start = time.perf_counter_ns()
            
            simulate_packet_processing(bytes(packet))
            
            pkt_duration = time.perf_counter_ns() - pkt_start
            latency.record(pkt_duration)
            
            packets_processed += 1
            bytes_processed += 828
            
            time.sleep(0.000001)
        
        p50, p90, p99, max_val, mean = latency.get_stats()
    
    else:
        # Multiprocessing (to bypass GIL)
        result_queue = multiprocessing.Queue()
        processes = []
        
        for _ in range(workers):
            p = multiprocessing.Process(target=worker_process, args=(duration, result_queue))
            p.start()
            processes.append(p)
        
        # Wait for completion
        for p in processes:
            p.join()
        
        # Aggregate results
        packets_processed = 0
        bytes_processed = 0
        all_latencies = []
        
        while not result_queue.empty():
            result = result_queue.get()
            packets_processed += result['packets_processed']
            bytes_processed += result['bytes_processed']
            all_latencies.append({
                'p50': result['latency_p50'],
                'p90': result['latency_p90'],
                'p99': result['latency_p99'],
                'max': result['latency_max'],
                'mean': result['latency_mean']
            })
        
        # Average latencies across workers
        p50 = sum(l['p50'] for l in all_latencies) / len(all_latencies)
        p90 = sum(l['p90'] for l in all_latencies) / len(all_latencies)
        p99 = sum(l['p99'] for l in all_latencies) / len(all_latencies)
        max_val = max(l['max'] for l in all_latencies)
        mean = sum(l['mean'] for l in all_latencies) / len(all_latencies)
    
    elapsed_ns = int((time.time() - start_time) * 1_000_000_000)
    
    # Final memory
    final_mem = process.memory_info().rss
    gc_stats_after = gc.get_stats()
    
    # GC statistics
    gc_count = gc.get_count()[0]
    gc_pause_total_ms = 0  # Python doesn't expose GC pause times directly
    
    # Calculate results
    results = BenchmarkResults(
        language="Python",
        duration_ns=elapsed_ns,
        packets_processed=packets_processed,
        bytes_processed=bytes_processed,
        packets_per_sec=packets_processed / (elapsed_ns / 1_000_000_000),
        mb_per_sec=bytes_processed / 1024 / 1024 / (elapsed_ns / 1_000_000_000),
        latency_p50_us=p50,
        latency_p90_us=p90,
        latency_p99_us=p99,
        latency_max_us=max_val,
        latency_mean_us=mean,
        memory_heap_mb=(final_mem - initial_mem) / 1024 / 1024,
        memory_rss_mb=final_mem / 1024 / 1024,
        gc_count=gc_count,
        gc_pause_total_ms=gc_pause_total_ms
    )
    
    return results


def print_results(r: BenchmarkResults):
    """Print benchmark results"""
    print(f"\n✅ {r.language} Benchmark Results:")
    print("━" * 80)
    print(f"  ⏱️  Duration:          {r.duration_ns / 1_000_000_000:.1f}s")
    print(f"  📦 Packets Processed:  {r.packets_processed:,}")
    print(f"  📊 Throughput:         {r.packets_per_sec:.0f} pkt/s ({r.mb_per_sec:.2f} MB/s)")
    print("  ─────────────────────────────────────────────────────────────")
    print(f"  🎯 Latency P50:        {r.latency_p50_us:.1f} μs")
    print(f"  🎯 Latency P90:        {r.latency_p90_us:.1f} μs")
    print(f"  🎯 Latency P99:        {r.latency_p99_us:.1f} μs")
    print(f"  🎯 Latency Max:        {r.latency_max_us:.1f} μs")
    print(f"  🎯 Latency Mean:       {r.latency_mean_us:.1f} μs")
    print("  ─────────────────────────────────────────────────────────────")
    print(f"  💾 Memory Heap:        {r.memory_heap_mb:.2f} MB")
    print(f"  💾 Memory RSS:         {r.memory_rss_mb:.2f} MB")
    print(f"  🗑️  GC Count:           {r.gc_count}")
    print("━" * 80)


def main():
    parser = argparse.ArgumentParser(description='BSE Python Performance Benchmark')
    parser.add_argument('--duration', type=float, default=30.0, help='Benchmark duration in seconds')
    parser.add_argument('--workers', type=int, default=1, help='Number of worker processes (1=single-threaded due to GIL)')
    parser.add_argument('--output', type=str, default='benchmark_python.json', help='Output JSON file')
    args = parser.parse_args()
    
    print("╔══════════════════════════════════════════════════════════════════════════════╗")
    print("║       BSE MARKET DATA READER - PYTHON PERFORMANCE BENCHMARK                 ║")
    print("╚══════════════════════════════════════════════════════════════════════════════╝\n")
    
    # Run benchmark
    results = run_benchmark(args.duration, args.workers)
    
    # Print results
    print_results(results)
    
    # Save to JSON
    with open(args.output, 'w') as f:
        json.dump(asdict(results), f, indent=2)
    
    print(f"\n💾 Results saved to: {args.output}")
    print("\n💡 Tip: Run Go benchmark and use --compare flag to see comparison")
    print(f"   go run cmd/benchmark/main.go --compare {args.output}")


if __name__ == '__main__':
    main()
