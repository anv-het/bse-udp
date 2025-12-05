# BSE C++ HFT - Improvement Plan

**Created:** December 5, 2025  
**Status:** Active Development

---

## 🔴 Priority 1: Critical (Before Production)

### 1.1 Remove Duplicate LatencyTracker
- [ ] **File:** `src/main.cpp` lines 95-130
- [ ] Remove local `LatencyTracker` class
- [ ] Use `bse::metrics::LatencyTracker` from `include/metrics/latency.hpp`
- [ ] Update all references in `Tracker` struct

### 1.2 Remove Duplicate Tracker Struct  
- [ ] **File:** `src/main.cpp` lines 145-165
- [ ] Remove local `Tracker` struct
- [ ] Use `bse::stats::Tracker` from `include/stats/stats.hpp`
- [ ] Consolidate all stats into single global tracker

### 1.3 Fix Graceful Shutdown
- [ ] **File:** `include/receiver/multicast.hpp`
- [ ] Add proper stop flag checking in receive loop
- [ ] Implement non-blocking receive with select() or poll()
- [ ] Ensure Ctrl+C responds within 100ms

### 1.4 Add WinHTTP to Build
- [ ] **File:** `build_manual.bat`
- [ ] Add `/link winhttp.lib ws2_32.lib` to compile command
- [ ] Test API download functionality

---

## 🟡 Priority 2: High (Should Fix)

### 2.1 Network Reconnection
- [ ] **File:** `include/receiver/multicast.hpp`
- [ ] Add `reconnect()` method
- [ ] Implement exponential backoff (1s, 2s, 4s, 8s, max 30s)
- [ ] Log reconnection attempts

### 2.2 Dynamic Holiday Calendar
- [ ] **File:** `include/tokens/token_manager.hpp`
- [ ] Load holidays from `config.json`
- [ ] Add 2026 holidays
- [ ] Consider BSE API for holiday list

### 2.3 Proper Logging
- [ ] Create `include/utils/logger.hpp`
- [ ] Replace `std::cout` with structured logging
- [ ] Add log levels (DEBUG, INFO, WARN, ERROR)
- [ ] Add file output option

### 2.4 Health Check Endpoint
- [ ] Add simple HTTP server for health checks
- [ ] Return JSON with current stats
- [ ] Enable external monitoring

---

## 🟢 Priority 3: Performance (Optional)

### 3.1 Lock-Free CSV Queue
- [ ] **File:** `include/saver/csv_writer.hpp`
- [ ] Replace `std::queue + mutex` with SPSC queue
- [ ] Remove all locks from save() path
- [ ] Benchmark improvement

### 3.2 Memory Pool for Quotes
- [ ] Create `include/memory/object_pool.hpp`
- [ ] Pre-allocate 10,000 Quote objects
- [ ] Eliminate runtime allocations in hot path

### 3.3 Record Prefetching
- [ ] **File:** `include/decoder/decoder.hpp`
- [ ] Add `_mm_prefetch` for next record
- [ ] Measure latency improvement

### 3.4 Configurable Ring Buffer
- [ ] Make ring buffer size configurable
- [ ] Add auto-scaling based on drop rate
- [ ] Default to 32K for high volatility

---

## 🔵 Priority 4: Nice to Have

### 4.1 Prometheus Metrics
- [ ] Add `/metrics` HTTP endpoint
- [ ] Export latency histograms
- [ ] Export packet/record counters
- [ ] Create Grafana dashboard

### 4.2 SIMD Decoding
- [ ] Implement AVX2 batch decode
- [ ] Process 4 records at once
- [ ] Only if throughput becomes issue

### 4.3 Binary Output Format
- [ ] Add FlatBuffers schema
- [ ] Parallel binary output option
- [ ] For downstream C++ consumers

### 4.4 Unit Tests
- [ ] Create `tests/unit/` directory
- [ ] Test ring buffer
- [ ] Test decoder
- [ ] Test token manager
- [ ] Add CI/CD integration

---

## 📊 Progress Tracking

| Task | Status | Assignee | Due Date |
|------|--------|----------|----------|
| Remove duplicate LatencyTracker | ⬜ Not Started | - | - |
| Remove duplicate Tracker | ⬜ Not Started | - | - |
| Fix graceful shutdown | ⬜ Not Started | - | - |
| Add WinHTTP to build | ⬜ Not Started | - | - |
| Network reconnection | ⬜ Not Started | - | - |
| Dynamic holidays | ⬜ Not Started | - | - |

---

## ✅ Completed

| Task | Date | Notes |
|------|------|-------|
| Add P60/P65 percentiles | 2025-12-05 | Added to latency.hpp and stats.hpp |
| Incremental retry delays | 2025-12-05 | 5s, 10s, 15s instead of fixed 10s |
| Update test_live_token output | 2025-12-05 | Matches Go version format |
| Create optimization report | 2025-12-05 | docs/OPTIMIZATION_REPORT.md |

---

*Last Updated: December 5, 2025*
