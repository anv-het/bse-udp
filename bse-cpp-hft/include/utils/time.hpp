/**
 * @file time.hpp
 * @brief High-precision timing utilities for HFT
 */

#pragma once

#include <chrono>
#include <cstdint>

namespace bse {
namespace utils {

/**
 * @brief Get current time in nanoseconds since epoch
 * 
 * NOTE: Uses system_clock for correct Unix epoch time (not high_resolution_clock
 * which may use arbitrary epoch on Windows)
 */
inline int64_t now_ns() {
    auto now = std::chrono::system_clock::now();
    return std::chrono::duration_cast<std::chrono::nanoseconds>(
        now.time_since_epoch()
    ).count();
}

/**
 * @brief Get current time in microseconds since epoch
 */
inline int64_t now_us() {
    auto now = std::chrono::system_clock::now();
    return std::chrono::duration_cast<std::chrono::microseconds>(
        now.time_since_epoch()
    ).count();
}

/**
 * @brief Get current time in milliseconds since epoch
 */
inline int64_t now_ms() {
    auto now = std::chrono::system_clock::now();
    return std::chrono::duration_cast<std::chrono::milliseconds>(
        now.time_since_epoch()
    ).count();
}

/**
 * @brief High-resolution timer for latency measurement
 */
class Timer {
public:
    Timer() : start_(std::chrono::high_resolution_clock::now()) {}
    
    void reset() {
        start_ = std::chrono::high_resolution_clock::now();
    }
    
    int64_t elapsed_ns() const {
        auto now = std::chrono::high_resolution_clock::now();
        return std::chrono::duration_cast<std::chrono::nanoseconds>(now - start_).count();
    }
    
    double elapsed_us() const {
        return elapsed_ns() / 1000.0;
    }
    
    double elapsed_ms() const {
        return elapsed_ns() / 1000000.0;
    }
    
    double elapsed_sec() const {
        return elapsed_ns() / 1000000000.0;
    }

private:
    std::chrono::high_resolution_clock::time_point start_;
};

/**
 * @brief Scoped timer that records elapsed time on destruction
 */
template<typename Callback>
class ScopedTimer {
public:
    ScopedTimer(Callback cb) : callback_(std::move(cb)), start_(std::chrono::high_resolution_clock::now()) {}
    
    ~ScopedTimer() {
        auto end = std::chrono::high_resolution_clock::now();
        int64_t ns = std::chrono::duration_cast<std::chrono::nanoseconds>(end - start_).count();
        callback_(ns);
    }

private:
    Callback callback_;
    std::chrono::high_resolution_clock::time_point start_;
};

} // namespace utils
} // namespace bse
