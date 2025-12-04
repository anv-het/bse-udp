/**
 * @file latency.hpp
 * @brief HFT-grade latency tracking with percentiles
 */

#pragma once

#include <atomic>
#include <vector>
#include <algorithm>
#include <cmath>
#include <mutex>

namespace bse {
namespace metrics {

/**
 * @brief Latency percentiles structure
 */
struct LatencyPercentiles {
    double p50 = 0.0;
    double p90 = 0.0;
    double p95 = 0.0;
    double p99 = 0.0;
    double p999 = 0.0;
};

/**
 * @brief Latency statistics (min/avg/max)
 */
struct LatencyStats {
    double min = 0.0;
    double avg = 0.0;
    double max = 0.0;
    uint64_t count = 0;
};

/**
 * @brief High-performance latency tracker with reservoir sampling
 */
class LatencyTracker {
public:
    static constexpr size_t RESERVOIR_SIZE = 10000;
    
    LatencyTracker() : samples_(RESERVOIR_SIZE, 0), sample_count_(0) {}
    
    /**
     * @brief Record a latency sample in nanoseconds
     */
    void record(int64_t ns) {
        double us = static_cast<double>(ns) / 1000.0;  // Convert to microseconds
        
        // Update min/max/sum atomically
        double current_min = min_.load(std::memory_order_relaxed);
        while (us < current_min && !min_.compare_exchange_weak(current_min, us)) {}
        
        double current_max = max_.load(std::memory_order_relaxed);
        while (us > current_max && !max_.compare_exchange_weak(current_max, us)) {}
        
        // Reservoir sampling for percentiles
        uint64_t count = count_.fetch_add(1, std::memory_order_relaxed);
        
        if (count < RESERVOIR_SIZE) {
            samples_[count] = us;
            sample_count_.store(count + 1, std::memory_order_relaxed);
        } else {
            // Random replacement (simplified for HFT)
            size_t idx = count % RESERVOIR_SIZE;
            samples_[idx] = us;
        }
        
        // Update sum for average
        sum_.fetch_add(static_cast<int64_t>(us * 1000), std::memory_order_relaxed);
    }
    
    /**
     * @brief Get latency percentiles
     */
    LatencyPercentiles get_percentiles() const {
        LatencyPercentiles p;
        
        size_t n = std::min(sample_count_.load(), RESERVOIR_SIZE);
        if (n == 0) return p;
        
        // Copy and sort samples
        std::vector<double> sorted(samples_.begin(), samples_.begin() + n);
        std::sort(sorted.begin(), sorted.end());
        
        p.p50 = sorted[static_cast<size_t>(n * 0.50)];
        p.p90 = sorted[static_cast<size_t>(n * 0.90)];
        p.p95 = sorted[static_cast<size_t>(n * 0.95)];
        p.p99 = sorted[std::min(static_cast<size_t>(n * 0.99), n - 1)];
        p.p999 = sorted[std::min(static_cast<size_t>(n * 0.999), n - 1)];
        
        return p;
    }
    
    /**
     * @brief Get latency statistics
     */
    LatencyStats get_stats() const {
        LatencyStats s;
        s.count = count_.load();
        s.min = min_.load();
        s.max = max_.load();
        
        if (s.count > 0) {
            s.avg = static_cast<double>(sum_.load()) / 1000.0 / s.count;
        }
        
        return s;
    }
    
    void reset() {
        count_.store(0);
        sample_count_.store(0);
        min_.store(1e9);
        max_.store(0);
        sum_.store(0);
    }
    
private:
    std::vector<double> samples_;
    std::atomic<size_t> sample_count_;
    std::atomic<uint64_t> count_{0};
    std::atomic<double> min_{1e9};
    std::atomic<double> max_{0};
    std::atomic<int64_t> sum_{0};
};

/**
 * @brief System resource tracking
 */
struct SystemStats {
    size_t peak_memory = 0;
    size_t current_memory = 0;
    int num_threads = 0;
    int num_cpu = 0;
};

/**
 * @brief Format bytes to human-readable string
 */
inline std::string format_bytes(size_t bytes) {
    const char* units[] = {"B", "KB", "MB", "GB"};
    int unit = 0;
    double size = static_cast<double>(bytes);
    
    while (size >= 1024 && unit < 3) {
        size /= 1024;
        unit++;
    }
    
    char buf[32];
    snprintf(buf, sizeof(buf), "%.1f %s", size, units[unit]);
    return std::string(buf);
}

} // namespace metrics
} // namespace bse
