/**
 * @file stats.hpp
 * @brief Comprehensive statistics tracker for BSE HFT
 */

#pragma once

// Prevent Windows min/max macro conflicts
#ifdef _WIN32
#ifndef NOMINMAX
#define NOMINMAX
#endif
#endif

#include "metrics/latency.hpp"
#include "utils/time.hpp"
#include <atomic>
#include <chrono>
#include <map>
#include <mutex>
#include <string>
#include <vector>
#include <algorithm>
#include <iostream>
#include <iomanip>
#include <sstream>

#ifdef _WIN32
#include <windows.h>
#include <psapi.h>
#endif

namespace bse {
namespace stats {

/**
 * @brief Missed token tracking
 */
struct MissedTokenPair {
    uint32_t token;
    int64_t count;
};

/**
 * @brief Comprehensive statistics tracker
 */
class Tracker {
public:
    Tracker() : start_time_(std::chrono::steady_clock::now()) {}
    
    // ========================================================================
    // Packet Statistics
    // ========================================================================
    
    void record_packet(const std::string& segment, int bytes, int records) {
        if (segment == "EQ") {
            eq_packets_.fetch_add(1, std::memory_order_relaxed);
            eq_bytes_.fetch_add(bytes, std::memory_order_relaxed);
            eq_records_.fetch_add(records, std::memory_order_relaxed);
        } else {
            fo_packets_.fetch_add(1, std::memory_order_relaxed);
            fo_bytes_.fetch_add(bytes, std::memory_order_relaxed);
            fo_records_.fetch_add(records, std::memory_order_relaxed);
        }
    }
    
    void record_quote(const std::string& segment) {
        if (segment == "EQ") {
            eq_quotes_.fetch_add(1, std::memory_order_relaxed);
        } else {
            fo_quotes_.fetch_add(1, std::memory_order_relaxed);
        }
    }
    
    void record_ring_drops(const std::string& segment, uint64_t drops) {
        if (segment == "EQ") {
            eq_ring_drops_.store(drops, std::memory_order_relaxed);
        } else {
            fo_ring_drops_.store(drops, std::memory_order_relaxed);
        }
    }
    
    void record_csv_writes(const std::string& segment, uint64_t writes) {
        if (segment == "EQ") {
            eq_csv_writes_.store(writes, std::memory_order_relaxed);
        } else {
            fo_csv_writes_.store(writes, std::memory_order_relaxed);
        }
    }
    
    void record_csv_drops(const std::string& segment, uint64_t drops) {
        if (segment == "EQ") {
            eq_csv_drops_.store(drops, std::memory_order_relaxed);
        } else {
            fo_csv_drops_.store(drops, std::memory_order_relaxed);
        }
    }
    
    // ========================================================================
    // Latency Recording
    // ========================================================================
    
    void record_decode_latency(int64_t ns) {
        decode_latency_.record(ns);
    }
    
    void record_save_latency(int64_t ns) {
        save_latency_.record(ns);
    }
    
    void record_process_latency(int64_t ns) {
        process_latency_.record(ns);
    }
    
    // ========================================================================
    // Token Tracking
    // ========================================================================
    
    void track_missed_token(uint32_t token) {
        std::lock_guard<std::mutex> lock(missed_mutex_);
        missed_tokens_[token]++;
        missed_count_.fetch_add(1, std::memory_order_relaxed);
    }
    
    std::vector<MissedTokenPair> get_top_missed_tokens(int n) const {
        std::lock_guard<std::mutex> lock(missed_mutex_);
        
        std::vector<MissedTokenPair> pairs;
        for (const auto& [token, count] : missed_tokens_) {
            pairs.push_back({token, count});
        }
        
        std::sort(pairs.begin(), pairs.end(), 
            [](const MissedTokenPair& a, const MissedTokenPair& b) {
                return a.count > b.count;
            });
        
        if (static_cast<int>(pairs.size()) > n) {
            pairs.resize(n);
        }
        
        return pairs;
    }
    
    // ========================================================================
    // File Info
    // ========================================================================
    
    void set_output_file(const std::string& segment, const std::string& path) {
        if (segment == "EQ") {
            eq_file_path_ = path;
        } else {
            fo_file_path_ = path;
        }
    }
    
    // ========================================================================
    // Getters
    // ========================================================================
    
    double elapsed_seconds() const {
        auto now = std::chrono::steady_clock::now();
        return std::chrono::duration<double>(now - start_time_).count();
    }
    
    uint64_t total_packets() const { 
        return eq_packets_.load() + fo_packets_.load(); 
    }
    
    uint64_t total_records() const { 
        return eq_records_.load() + fo_records_.load(); 
    }
    
    uint64_t total_quotes() const { 
        return eq_quotes_.load() + fo_quotes_.load(); 
    }
    
    uint64_t total_bytes() const { 
        return eq_bytes_.load() + fo_bytes_.load(); 
    }
    
    uint64_t total_ring_drops() const { 
        return eq_ring_drops_.load() + fo_ring_drops_.load(); 
    }
    
    uint64_t total_csv_drops() const {
        return eq_csv_drops_.load() + fo_csv_drops_.load();
    }
    
    uint64_t missed_count() const {
        return missed_count_.load();
    }
    
    size_t unique_missed_count() const {
        std::lock_guard<std::mutex> lock(missed_mutex_);
        return missed_tokens_.size();
    }
    
    // ========================================================================
    // Live Stats Output
    // ========================================================================
    
    void print_live_stats(int token_count) const {
        double elapsed = elapsed_seconds();
        uint64_t packets = total_packets();
        uint64_t records = total_records();
        uint64_t drops = total_ring_drops();
        
        double pps = elapsed > 0 ? packets / elapsed : 0;
        double rps = elapsed > 0 ? records / elapsed : 0;
        
        std::cout << "\r[" << static_cast<int>(elapsed) << "s] "
                  << "EQ: " << eq_packets_.load() << " pkts, " 
                  << eq_records_.load() << " recs (" << std::fixed << std::setprecision(1) << rps/2 << "/s) | "
                  << "FO: " << fo_packets_.load() << " pkts, "
                  << fo_records_.load() << " recs (" << std::fixed << std::setprecision(1) << rps/2 << "/s) | "
                  << "Drops: EQ=" << eq_ring_drops_.load() << " FO=" << fo_ring_drops_.load()
                  << std::flush;
    }
    
    // ========================================================================
    // Final Report (Beautiful Formatting like Go version)
    // ========================================================================
    
    void print_final_report(int token_count) const {
        double elapsed = elapsed_seconds();
        
        std::cout << "\n\n";
        std::cout << "╔════════════════════════════════════════════════════════════════════════════════════╗\n";
        std::cout << "║                         📊 BSE HFT BENCHMARK REPORT                                ║\n";
        std::cout << "╚════════════════════════════════════════════════════════════════════════════════════╝\n";
        
        // Duration
        std::cout << "\n┌──────────────────────────────────────────────────────────────────────────────────┐\n";
        std::cout << "│  ⏱️  DURATION                                                                     │\n";
        std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
        std::cout << "│    Total Runtime:        " << std::setw(52) << std::fixed << std::setprecision(2) << elapsed << " seconds│\n";
        
        // Get current time
        auto now = std::chrono::system_clock::now();
        auto time_t_now = std::chrono::system_clock::to_time_t(now);
        auto start_time_t = std::chrono::system_clock::to_time_t(
            std::chrono::system_clock::now() - std::chrono::duration_cast<std::chrono::seconds>(
                std::chrono::steady_clock::now() - start_time_));
        
        std::tm tm_start, tm_end;
#ifdef _WIN32
        localtime_s(&tm_start, &start_time_t);
        localtime_s(&tm_end, &time_t_now);
#else
        localtime_r(&start_time_t, &tm_start);
        localtime_r(&time_t_now, &tm_end);
#endif
        
        std::ostringstream start_ss, end_ss;
        start_ss << std::put_time(&tm_start, "%H:%M:%S");
        end_ss << std::put_time(&tm_end, "%H:%M:%S");
        
        std::cout << "│    Start Time:           " << std::setw(56) << std::left << start_ss.str() << "│\n";
        std::cout << "│    End Time:             " << std::setw(56) << std::left << end_ss.str() << "│\n";
        std::cout << "└──────────────────────────────────────────────────────────────────────────────────┘\n";
        
        // Feed Breakdown
        std::cout << "\n┌──────────────────────────────────────────────────────────────────────────────────┐\n";
        std::cout << "│  📊 FEED BREAKDOWN                                                               │\n";
        std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
        std::cout << "│    EQ (Equity Cash):     " << std::setw(8) << eq_packets_.load() << " pkts    "
                  << std::setw(8) << eq_records_.load() << " recs    "
                  << std::setw(8) << eq_quotes_.load() << " quotes          │\n";
        std::cout << "│    FO (F&O Derivatives): " << std::setw(8) << fo_packets_.load() << " pkts    "
                  << std::setw(8) << fo_records_.load() << " recs    "
                  << std::setw(8) << fo_quotes_.load() << " quotes       │\n";
        std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
        std::cout << "│    TOTAL:                " << std::setw(8) << total_packets() << " pkts    "
                  << std::setw(8) << total_records() << " recs    "
                  << std::setw(8) << total_quotes() << " quotes   │\n";
        std::cout << "└──────────────────────────────────────────────────────────────────────────────────┘\n";
        
        // Throughput
        std::cout << "\n┌──────────────────────────────────────────────────────────────────────────────────┐\n";
        std::cout << "│  🚀 THROUGHPUT                                                                   │\n";
        std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
        
        if (elapsed > 0) {
            double pps = total_packets() / elapsed;
            double rps = total_records() / elapsed;
            double mbps = total_bytes() / elapsed / 1024.0 / 1024.0;
            
            std::cout << "│    Packets/sec:          " << std::setw(56) << std::fixed << std::setprecision(2) << pps << "│\n";
            std::cout << "│    Records/sec:          " << std::setw(56) << std::fixed << std::setprecision(2) << rps << "│\n";
            std::cout << "│    Data Rate:            " << std::setw(52) << std::fixed << std::setprecision(3) << mbps << " MB/s│\n";
            std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
            
            double ms_per_pkt = elapsed * 1000.0 / total_packets();
            double us_per_rec = elapsed * 1000000.0 / total_records();
            double ns_per_rec = elapsed * 1000000000.0 / total_records();
            
            std::cout << "│    Avg Time/Packet:      " << std::setw(52) << std::fixed << std::setprecision(4) << ms_per_pkt << " ms│\n";
            std::cout << "│    Avg Time/Record:      " << std::setw(52) << std::fixed << std::setprecision(2) << us_per_rec << " µs│\n";
            std::cout << "│    Avg Time/Record:      " << std::setw(52) << std::fixed << std::setprecision(0) << ns_per_rec << " ns│\n";
        }
        std::cout << "└──────────────────────────────────────────────────────────────────────────────────┘\n";
        
        // Decode Rate Projections
        std::cout << "\n┌──────────────────────────────────────────────────────────────────────────────────┐\n";
        std::cout << "│  📈 DECODE RATE PROJECTIONS                                                      │\n";
        std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
        
        if (elapsed > 0) {
            double rps = total_records() / elapsed;
            std::cout << "│    Per Second:           " << std::setw(44) << std::fixed << std::setprecision(0) << rps << " records/sec│\n";
            std::cout << "│    Per Minute:           " << std::setw(44) << std::fixed << std::setprecision(0) << rps * 60 << " records/min│\n";
            std::cout << "│    Per 30 Minutes:       " << std::setw(42) << std::fixed << std::setprecision(0) << rps * 1800 << " records/30min│\n";
            std::cout << "│    Per Hour:             " << std::setw(45) << std::fixed << std::setprecision(0) << rps * 3600 << " records/hr│\n";
            std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
            
            double qps = total_quotes() / elapsed;
            std::cout << "│    Quotes/sec:           " << std::setw(48) << std::fixed << std::setprecision(0) << qps << " quotes/sec│\n";
            std::cout << "│    Quotes/min:           " << std::setw(48) << std::fixed << std::setprecision(0) << qps * 60 << " quotes/min│\n";
            std::cout << "│    Quotes/hour:          " << std::setw(49) << std::fixed << std::setprecision(0) << qps * 3600 << " quotes/hr│\n";
        }
        std::cout << "└──────────────────────────────────────────────────────────────────────────────────┘\n";
        
        // Output Files
        std::cout << "\n┌──────────────────────────────────────────────────────────────────────────────────┐\n";
        std::cout << "│  📄 OUTPUT FILES                                                                 │\n";
        std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
        if (!eq_file_path_.empty()) {
            std::cout << "│    EQ CSV:              " << std::setw(56) << std::left << get_filename(eq_file_path_) << " │\n";
            std::cout << "│    EQ Rows:             " << std::setw(56) << std::left << eq_quotes_.load() << " │\n";
        }
        if (!fo_file_path_.empty()) {
            std::cout << "│    FO CSV:              " << std::setw(56) << std::left << get_filename(fo_file_path_) << " │\n";
            std::cout << "│    FO Rows:             " << std::setw(56) << std::left << fo_quotes_.load() << " │\n";
        }
        std::cout << "└──────────────────────────────────────────────────────────────────────────────────┘\n";
        
        // Latency Analysis
        auto decode_p = decode_latency_.get_percentiles();
        auto save_p = save_latency_.get_percentiles();
        auto process_p = process_latency_.get_percentiles();
        
        auto decode_s = decode_latency_.get_stats();
        auto save_s = save_latency_.get_stats();
        auto process_s = process_latency_.get_stats();
        
        std::cout << "\n┌──────────────────────────────────────────────────────────────────────────────────┐\n";
        std::cout << "│  ⚡ LATENCY ANALYSIS (microseconds)                                              │\n";
        std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
        std::cout << "│    Metric            Decode          Save            Total                       │\n";
        std::cout << "│    ─────────────────────────────────────────────────────────                     │\n";
        
        std::cout << std::fixed << std::setprecision(2);
        std::cout << "│    Min            " << std::setw(8) << decode_s.min << " µs    " 
                  << std::setw(8) << save_s.min << " µs    " 
                  << std::setw(8) << process_s.min << " µs                       │\n";
        std::cout << "│    Avg            " << std::setw(8) << decode_s.avg << " µs    " 
                  << std::setw(8) << save_s.avg << " µs    " 
                  << std::setw(8) << process_s.avg << " µs                      │\n";
        std::cout << "│    Max            " << std::setw(8) << decode_s.max << " µs    " 
                  << std::setw(8) << save_s.max << " µs    " 
                  << std::setw(8) << process_s.max << " µs                      │\n";
        std::cout << "│    ─────────────────────────────────────────────────────────                     │\n";
        std::cout << "│    P50            " << std::setw(8) << decode_p.p50 << " µs    " 
                  << std::setw(8) << save_p.p50 << " µs    " 
                  << std::setw(8) << process_p.p50 << " µs                      │\n";
        std::cout << "│    P90            " << std::setw(8) << decode_p.p90 << " µs    " 
                  << std::setw(8) << save_p.p90 << " µs    " 
                  << std::setw(8) << process_p.p90 << " µs                      │\n";
        std::cout << "│    P99            " << std::setw(8) << decode_p.p99 << " µs    " 
                  << std::setw(8) << save_p.p99 << " µs    " 
                  << std::setw(8) << process_p.p99 << " µs                      │\n";
        std::cout << "│    P99.9          " << std::setw(8) << decode_p.p999 << " µs    " 
                  << std::setw(8) << save_p.p999 << " µs    " 
                  << std::setw(8) << process_p.p999 << " µs                     │\n";
        std::cout << "└──────────────────────────────────────────────────────────────────────────────────┘\n";
        
        // Ring Buffer Drops
        uint64_t eq_drops = eq_ring_drops_.load();
        uint64_t fo_drops = fo_ring_drops_.load();
        uint64_t total_drops = eq_drops + fo_drops;
        double drop_rate = (total_packets() + total_drops) > 0 
            ? (static_cast<double>(total_drops) / (total_packets() + total_drops) * 100.0)
            : 0.0;
        
        std::cout << "\n┌──────────────────────────────────────────────────────────────────────────────────┐\n";
        std::cout << "│  📦 RING BUFFER DROPS (Actual Packet Loss)                                       │\n";
        std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
        std::cout << "│    EQ Ring Drops:        " << std::setw(56) << std::left << eq_drops << "│\n";
        std::cout << "│    FO Ring Drops:        " << std::setw(56) << std::left << fo_drops << "│\n";
        std::cout << "│    Total Drops:          " << std::setw(56) << std::left << total_drops << "│\n";
        
        std::ostringstream drop_rate_ss;
        drop_rate_ss << std::fixed << std::setprecision(4) << drop_rate << "%";
        std::cout << "│    Drop Rate:            " << std::setw(56) << std::left << drop_rate_ss.str() << "│\n";
        std::cout << "└──────────────────────────────────────────────────────────────────────────────────┘\n";
        
        // CSV Writer Stats
        std::cout << "\n┌──────────────────────────────────────────────────────────────────────────────────┐\n";
        std::cout << "│  💾 CSV WRITER STATS                                                             │\n";
        std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
        std::cout << "│    EQ CSV Records:       " << std::setw(56) << std::left << eq_csv_writes_.load() << "│\n";
        std::cout << "│    EQ CSV Dropped:       " << std::setw(56) << std::left << eq_csv_drops_.load() << "│\n";
        std::cout << "│    FO CSV Records:       " << std::setw(56) << std::left << fo_csv_writes_.load() << "│\n";
        std::cout << "│    FO CSV Dropped:       " << std::setw(56) << std::left << fo_csv_drops_.load() << "│\n";
        std::cout << "└──────────────────────────────────────────────────────────────────────────────────┘\n";
        
        // Missed Tokens
        std::cout << "\n┌──────────────────────────────────────────────────────────────────────────────────┐\n";
        std::cout << "│  ⚠️  MISSED TOKENS (Not in Token Master)                                          │\n";
        std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
        std::cout << "│    Total Missed:         " << std::setw(56) << std::left << missed_count_.load() << "│\n";
        std::cout << "│    Unique Tokens:        " << std::setw(56) << std::left << unique_missed_count() << "│\n";
        
        auto top_missed = get_top_missed_tokens(10);
        if (!top_missed.empty()) {
            std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
            std::cout << "│    Top Missed Tokens (Token → Count):                                            │\n";
            int i = 1;
            for (const auto& pair : top_missed) {
                std::cout << "│      " << std::setw(2) << i++ << ". Token " 
                          << std::setw(12) << std::left << pair.token << " → " 
                          << std::setw(3) << pair.count << " occurrences                                   │\n";
            }
        }
        std::cout << "└──────────────────────────────────────────────────────────────────────────────────┘\n";
        
        // System Resources
        std::cout << "\n┌──────────────────────────────────────────────────────────────────────────────────┐\n";
        std::cout << "│  💾 SYSTEM RESOURCES                                                             │\n";
        std::cout << "├──────────────────────────────────────────────────────────────────────────────────┤\n";
        
#ifdef _WIN32
        PROCESS_MEMORY_COUNTERS pmc;
        if (GetProcessMemoryInfo(GetCurrentProcess(), &pmc, sizeof(pmc))) {
            std::cout << "│    Peak Memory:          " << std::setw(52) << std::left 
                      << metrics::format_bytes(pmc.PeakWorkingSetSize) << "    │\n";
            std::cout << "│    Current Memory:       " << std::setw(52) << std::left 
                      << metrics::format_bytes(pmc.WorkingSetSize) << "    │\n";
        }
        
        SYSTEM_INFO sysinfo;
        GetSystemInfo(&sysinfo);
        std::cout << "│    CPU Cores:            " << std::setw(56) << std::left 
                  << sysinfo.dwNumberOfProcessors << " │\n";
#endif

        std::cout << "│    Threads:              " << std::setw(56) << std::left << "4 (main + 2 recv + 1 csv) │\n";
        std::cout << "└──────────────────────────────────────────────────────────────────────────────────┘\n";
        
        // HFT Assessment
        std::cout << "\n╔══════════════════════════════════════════════════════════════════════════════════╗\n";
        std::cout << "║  🏆 HFT PERFORMANCE ASSESSMENT                                                   ║\n";
        std::cout << "╠══════════════════════════════════════════════════════════════════════════════════╣\n";
        
        // Performance rating
        if (process_p.p99 < 50) {
            std::cout << "║    Status:  ✨ ULTRA HFT - P99 < 50µs (Professional Trading Grade)              ║\n";
        } else if (process_p.p99 < 100) {
            std::cout << "║    Status:  ✅ EXCELLENT - P99 < 100µs (HFT Grade)                               ║\n";
        } else if (process_p.p99 < 500) {
            std::cout << "║    Status:  ✅ GOOD - P99 < 500µs (Low Latency Trading)                          ║\n";
        } else if (process_p.p99 < 1000) {
            std::cout << "║    Status:  ⚠️  ACCEPTABLE - P99 < 1ms (Algo Trading)                            ║\n";
        } else {
            std::cout << "║    Status:  ❌ NEEDS OPTIMIZATION - P99 >= 1ms                                   ║\n";
        }
        
        // Packet Loss
        if (total_drops == 0) {
            std::cout << "║    Drops:   ✅ ZERO PACKET DROPS (Perfect capture)                               ║\n";
        } else if (drop_rate < 0.01) {
            std::cout << "║    Drops:   ⚠️  MINIMAL - " << total_drops << " drops (" 
                      << std::fixed << std::setprecision(4) << drop_rate << "%)                            ║\n";
        } else {
            std::cout << "║    Drops:   ❌ PACKET LOSS - increase ring buffer                                 ║\n";
        }
        
        // Data quality
        double missed_ratio = total_records() > 0 
            ? (static_cast<double>(missed_count_.load()) / total_records() * 100.0)
            : 0.0;
        
        if (missed_ratio < 1) {
            std::cout << "║    Data:    ✨ EXCELLENT - " << std::fixed << std::setprecision(2) 
                      << missed_ratio << "% token miss rate                                    ║\n";
        } else if (missed_ratio < 5) {
            std::cout << "║    Data:    ✅ GOOD - " << std::fixed << std::setprecision(2) 
                      << missed_ratio << "% token miss rate                                         ║\n";
        } else {
            std::cout << "║    Data:    ❌ CHECK TOKEN MAP - " << std::fixed << std::setprecision(2) 
                      << missed_ratio << "% miss rate                                 ║\n";
        }
        
        std::cout << "╚══════════════════════════════════════════════════════════════════════════════════╝\n";
        std::cout << std::endl;
    }
    
private:
    std::chrono::steady_clock::time_point start_time_;
    
    // Packet stats
    std::atomic<uint64_t> eq_packets_{0};
    std::atomic<uint64_t> fo_packets_{0};
    std::atomic<uint64_t> eq_bytes_{0};
    std::atomic<uint64_t> fo_bytes_{0};
    std::atomic<uint64_t> eq_records_{0};
    std::atomic<uint64_t> fo_records_{0};
    std::atomic<uint64_t> eq_quotes_{0};
    std::atomic<uint64_t> fo_quotes_{0};
    
    // Ring buffer drops
    std::atomic<uint64_t> eq_ring_drops_{0};
    std::atomic<uint64_t> fo_ring_drops_{0};
    
    // CSV stats
    std::atomic<uint64_t> eq_csv_writes_{0};
    std::atomic<uint64_t> fo_csv_writes_{0};
    std::atomic<uint64_t> eq_csv_drops_{0};
    std::atomic<uint64_t> fo_csv_drops_{0};
    
    // Latency trackers
    mutable metrics::LatencyTracker decode_latency_;
    mutable metrics::LatencyTracker save_latency_;
    mutable metrics::LatencyTracker process_latency_;
    
    // Missed tokens
    mutable std::mutex missed_mutex_;
    std::map<uint32_t, int64_t> missed_tokens_;
    std::atomic<uint64_t> missed_count_{0};
    
    // File paths
    std::string eq_file_path_;
    std::string fo_file_path_;
    
    static std::string get_filename(const std::string& path) {
        size_t pos = path.find_last_of("/\\");
        return pos != std::string::npos ? path.substr(pos + 1) : path;
    }
};

} // namespace stats
} // namespace bse
