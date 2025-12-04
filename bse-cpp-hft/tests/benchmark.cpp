/**
 * BSE C++ HFT Benchmark Tool
 * ==========================
 *
 * Measures: Packet rates, Latency Percentiles (P50/P90/P99/P99.9), Packet Loss
 * For HFT system evaluation
 *
 * ================================================================================
 * USAGE GUIDE
 * ================================================================================
 *
 * BUILD (from bse-cpp-hft directory):
 *   cl.exe /EHsc /std:c++17 /O2 /I include tests\benchmark.cpp /Fe:bin\benchmark.exe ws2_32.lib
 *
 * RUN EXAMPLES:
 *
 * 1. Benchmark EQ (Equity Cash) feed - Port 26001:
 *    .\bin\benchmark.exe -port 26001 -duration 30
 *
 * 2. Benchmark FO (F&O Derivatives) feed - Port 26002:
 *    .\bin\benchmark.exe -port 26002 -duration 60
 *
 * 3. Custom multicast IP:
 *    .\bin\benchmark.exe -ip 239.1.2.5 -port 26002 -duration 120
 *
 * PARAMETERS:
 *   -ip <ip>            Multicast IP address (default "239.1.2.5")
 *   -port <port>        UDP port: 26001=EQ, 26002=FO (default 26001)
 *   -duration <seconds> Benchmark duration (default: 30)
 *
 * OUTPUT METRICS:
 *   - Throughput: Packets/sec, Records/sec, MB/s
 *   - Latency: Min, Mean, Max, P50, P90, P99, P99.9 (microseconds)
 *   - Packet Loss: Drops, loss rate %
 *
 * ================================================================================
 */

#include <iostream>
#include <iomanip>
#include <string>
#include <sstream>
#include <vector>
#include <atomic>
#include <thread>
#include <chrono>
#include <cstring>
#include <algorithm>
#include <random>
#include <mutex>
#include <cmath>

#ifdef _WIN32
    #include <winsock2.h>
    #include <ws2tcpip.h>
    #pragma comment(lib, "ws2_32.lib")
#else
    #include <sys/socket.h>
    #include <netinet/in.h>
    #include <arpa/inet.h>
    #include <unistd.h>
#endif

using namespace std::chrono;

// ================================================================================
// CONFIGURATION
// ================================================================================

constexpr const char* DEFAULT_MULTICAST_IP = "239.1.2.5";
constexpr int DEFAULT_PORT = 26001;
constexpr int HEADER_SIZE = 36;
constexpr int RECORD_SIZE = 264;
constexpr int BUFFER_SIZE = 2048;
constexpr size_t RING_BUFFER_SIZE = 16384;
constexpr size_t MAX_LATENCY_SAMPLES = 100000;

// ================================================================================
// LATENCY TRACKER
// ================================================================================

class LatencyTracker {
public:
    void record(int64_t latency_ns) {
        std::lock_guard<std::mutex> lock(mutex_);
        count_++;
        sum_ += latency_ns;
        
        if (latency_ns < min_) min_ = latency_ns;
        if (latency_ns > max_) max_ = latency_ns;
        
        // Reservoir sampling
        if (samples_.size() < MAX_LATENCY_SAMPLES) {
            samples_.push_back(latency_ns);
        } else {
            std::uniform_int_distribution<size_t> dist(0, count_ - 1);
            size_t j = dist(rng_);
            if (j < MAX_LATENCY_SAMPLES) {
                samples_[j] = latency_ns;
            }
        }
    }
    
    struct Stats {
        uint64_t count = 0;
        double mean_us = 0;
        double min_us = 0;
        double max_us = 0;
        double p50_us = 0;
        double p90_us = 0;
        double p99_us = 0;
        double p999_us = 0;
    };
    
    Stats get_stats() {
        std::lock_guard<std::mutex> lock(mutex_);
        Stats s;
        s.count = count_;
        
        if (count_ == 0) return s;
        
        s.mean_us = (static_cast<double>(sum_) / count_) / 1000.0;
        s.min_us = min_ / 1000.0;
        s.max_us = max_ / 1000.0;
        
        if (!samples_.empty()) {
            std::vector<int64_t> sorted = samples_;
            std::sort(sorted.begin(), sorted.end());
            
            auto percentile = [&](double p) -> double {
                size_t idx = static_cast<size_t>(p * (sorted.size() - 1));
                return sorted[idx] / 1000.0;
            };
            
            s.p50_us = percentile(0.50);
            s.p90_us = percentile(0.90);
            s.p99_us = percentile(0.99);
            s.p999_us = percentile(0.999);
        }
        
        return s;
    }
    
private:
    std::mutex mutex_;
    std::vector<int64_t> samples_;
    uint64_t count_ = 0;
    int64_t sum_ = 0;
    int64_t min_ = INT64_MAX;
    int64_t max_ = 0;
    std::mt19937_64 rng_{std::random_device{}()};
};

// ================================================================================
// RING BUFFER
// ================================================================================

struct PacketSlot {
    uint8_t data[BUFFER_SIZE];
    int length = 0;
    int64_t recv_time_ns = 0;
};

class RingBuffer {
public:
    RingBuffer() : slots_(RING_BUFFER_SIZE) {}
    
    bool try_push(const uint8_t* data, int length, int64_t recv_time_ns) {
        size_t next = (write_pos_ + 1) % RING_BUFFER_SIZE;
        if (next == read_pos_.load(std::memory_order_acquire)) {
            return false;  // Full
        }
        std::memcpy(slots_[write_pos_].data, data, length);
        slots_[write_pos_].length = length;
        slots_[write_pos_].recv_time_ns = recv_time_ns;
        write_pos_ = next;
        return true;
    }
    
    PacketSlot* try_pop() {
        if (read_pos_.load(std::memory_order_relaxed) == write_pos_) {
            return nullptr;  // Empty
        }
        return &slots_[read_pos_];
    }
    
    void commit_pop() {
        read_pos_.store((read_pos_ + 1) % RING_BUFFER_SIZE, std::memory_order_release);
    }
    
private:
    std::vector<PacketSlot> slots_;
    size_t write_pos_ = 0;
    std::atomic<size_t> read_pos_{0};
};

// ================================================================================
// GLOBALS
// ================================================================================

std::atomic<bool> g_running{true};
std::atomic<uint64_t> g_packets{0};
std::atomic<uint64_t> g_records{0};
std::atomic<uint64_t> g_bytes{0};
std::atomic<uint64_t> g_drops{0};
LatencyTracker g_latency;
RingBuffer g_ring_buffer;

// ================================================================================
// TIME FUNCTIONS
// ================================================================================

int64_t now_ns() {
    return duration_cast<nanoseconds>(system_clock::now().time_since_epoch()).count();
}

// ================================================================================
// HELPER FUNCTIONS
// ================================================================================

inline uint16_t read_u16_le(const uint8_t* ptr) {
    return static_cast<uint16_t>(ptr[0]) | (static_cast<uint16_t>(ptr[1]) << 8);
}

std::string format_number(uint64_t n) {
    std::string s = std::to_string(n);
    int pos = static_cast<int>(s.length()) - 3;
    while (pos > 0) {
        s.insert(pos, ",");
        pos -= 3;
    }
    return s;
}

std::string format_bytes(uint64_t bytes) {
    const char* units[] = {"B", "KB", "MB", "GB"};
    int unit = 0;
    double size = static_cast<double>(bytes);
    while (size >= 1024 && unit < 3) {
        size /= 1024;
        unit++;
    }
    std::ostringstream oss;
    oss << std::fixed << std::setprecision(2) << size << " " << units[unit];
    return oss.str();
}

std::string format_rate(double rate) {
    std::ostringstream oss;
    if (rate >= 1000000) oss << std::fixed << std::setprecision(2) << (rate / 1000000) << "M";
    else if (rate >= 1000) oss << std::fixed << std::setprecision(2) << (rate / 1000) << "K";
    else oss << std::fixed << std::setprecision(0) << rate;
    return oss.str();
}

// ================================================================================
// RECEIVER THREAD
// ================================================================================

void receiver_thread(const std::string& ip, int port) {
#ifdef _WIN32
    WSADATA wsa_data;
    WSAStartup(MAKEWORD(2, 2), &wsa_data);
#endif
    
    int sock = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
    if (sock < 0) {
        std::cerr << "Failed to create socket\n";
        return;
    }
    
    int reuse = 1;
    setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, (const char*)&reuse, sizeof(reuse));
    
    int rcv_buf = 32 * 1024 * 1024;
    setsockopt(sock, SOL_SOCKET, SO_RCVBUF, (const char*)&rcv_buf, sizeof(rcv_buf));
    
#ifdef _WIN32
    DWORD timeout = 50;
    setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, (const char*)&timeout, sizeof(timeout));
#else
    struct timeval tv;
    tv.tv_sec = 0;
    tv.tv_usec = 50000;
    setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
#endif
    
    struct sockaddr_in local_addr{};
    local_addr.sin_family = AF_INET;
    local_addr.sin_port = htons(static_cast<uint16_t>(port));
    local_addr.sin_addr.s_addr = htonl(INADDR_ANY);
    
    if (bind(sock, (struct sockaddr*)&local_addr, sizeof(local_addr)) < 0) {
        std::cerr << "Failed to bind\n";
        return;
    }
    
    struct ip_mreq mreq{};
    mreq.imr_multiaddr.s_addr = inet_addr(ip.c_str());
    mreq.imr_interface.s_addr = htonl(INADDR_ANY);
    
    if (setsockopt(sock, IPPROTO_IP, IP_ADD_MEMBERSHIP, (const char*)&mreq, sizeof(mreq)) < 0) {
        std::cerr << "Failed to join multicast\n";
        return;
    }
    
    std::cout << "Connected to " << ip << ":" << port << "\n\n";
    
    uint8_t buffer[BUFFER_SIZE];
    
    while (g_running) {
        int n = recv(sock, (char*)buffer, sizeof(buffer), 0);
        if (n > 0) {
            int64_t recv_time = now_ns();
            if (!g_ring_buffer.try_push(buffer, n, recv_time)) {
                g_drops++;
            } else {
                g_bytes += n;
            }
        }
    }
    
#ifdef _WIN32
    closesocket(sock);
    WSACleanup();
#else
    close(sock);
#endif
}

// ================================================================================
// PROCESSOR THREAD
// ================================================================================

void processor_thread() {
    while (g_running) {
        PacketSlot* slot = g_ring_buffer.try_pop();
        if (!slot) {
            std::this_thread::sleep_for(microseconds(10));
            continue;
        }
        
        g_packets++;
        
        // Decode packet
        if (slot->length >= HEADER_SIZE + RECORD_SIZE) {
            uint16_t msg_type = read_u16_le(slot->data + 8);
            if (msg_type == 2020 || msg_type == 2040) {
                int num_records = (slot->length - HEADER_SIZE) / RECORD_SIZE;
                g_records += num_records;
                
                // Record latency
                int64_t latency = now_ns() - slot->recv_time_ns;
                g_latency.record(latency);
            }
        }
        
        g_ring_buffer.commit_pop();
    }
}

// ================================================================================
// PRINT FUNCTIONS
// ================================================================================

void print_banner() {
    std::cout << "\n";
    std::cout << "+==============================================================================+\n";
    std::cout << "|                     BSE HFT C++ BENCHMARK TOOL                              |\n";
    std::cout << "+==============================================================================+\n";
}

void print_live_stats(double elapsed) {
    double pkt_rate = elapsed > 0 ? g_packets / elapsed : 0;
    double rec_rate = elapsed > 0 ? g_records / elapsed : 0;
    double byte_rate = elapsed > 0 ? g_bytes / elapsed : 0;
    
    std::cout << "\r[" << std::fixed << std::setprecision(1) << elapsed << "s] "
              << "Pkts: " << format_number(g_packets) << " (" << format_rate(pkt_rate) << "/s)  "
              << "Recs: " << format_number(g_records) << " (" << format_rate(rec_rate) << "/s)  "
              << "Rate: " << format_bytes(static_cast<uint64_t>(byte_rate)) << "/s  "
              << "Drops: " << g_drops
              << "        " << std::flush;
}

void print_final_report(double elapsed) {
    auto lat = g_latency.get_stats();
    
    double pkt_rate = elapsed > 0 ? g_packets / elapsed : 0;
    double rec_rate = elapsed > 0 ? g_records / elapsed : 0;
    double byte_rate = elapsed > 0 ? g_bytes / elapsed : 0;
    double drop_rate = g_packets > 0 ? (100.0 * g_drops / (g_packets + g_drops)) : 0;
    
    std::cout << "\n\n";
    std::cout << "+==============================================================================+\n";
    std::cout << "|                         BENCHMARK RESULTS                                   |\n";
    std::cout << "+==============================================================================+\n";
    std::cout << "| Duration: " << std::fixed << std::setprecision(1) << elapsed << "s"
              << "  Data: " << format_bytes(g_bytes) << "\n";
    std::cout << "+------------------------------------------------------------------------------+\n";
    std::cout << "| THROUGHPUT                                                                   |\n";
    std::cout << "|   Packets:  " << std::setw(15) << format_number(g_packets) 
              << "  Rate: " << std::setw(10) << format_rate(pkt_rate) << "/s                |\n";
    std::cout << "|   Records:  " << std::setw(15) << format_number(g_records)
              << "  Rate: " << std::setw(10) << format_rate(rec_rate) << "/s                |\n";
    std::cout << "|   Data:     " << std::setw(15) << format_bytes(g_bytes)
              << "  Rate: " << std::setw(10) << format_bytes(static_cast<uint64_t>(byte_rate)) << "/s           |\n";
    std::cout << "+------------------------------------------------------------------------------+\n";
    std::cout << "| LATENCY (microseconds)                                                       |\n";
    std::cout << "|   Min: " << std::fixed << std::setprecision(1) << std::setw(8) << lat.min_us
              << "  Mean: " << std::setw(8) << lat.mean_us
              << "  Max: " << std::setw(8) << lat.max_us << "                    |\n";
    std::cout << "|   P50: " << std::setw(8) << lat.p50_us
              << "  P90:  " << std::setw(8) << lat.p90_us
              << "  P99: " << std::setw(8) << lat.p99_us
              << "  P99.9: " << std::setw(6) << lat.p999_us << "  |\n";
    std::cout << "+------------------------------------------------------------------------------+\n";
    std::cout << "| RELIABILITY                                                                  |\n";
    std::cout << "|   Drops: " << std::setw(10) << format_number(g_drops)
              << "  Drop Rate: " << std::fixed << std::setprecision(4) << drop_rate << "%"
              << "                               |\n";
    
    std::string rating;
    if (drop_rate == 0) rating = "PERFECT - Zero packet drops!";
    else if (drop_rate < 0.001) rating = "EXCELLENT - Sub-millisecond grade";
    else if (drop_rate < 0.01) rating = "GOOD - Production ready";
    else rating = "NEEDS IMPROVEMENT";
    
    std::cout << "|   Rating: " << rating << std::string(66 - rating.length(), ' ') << "|\n";
    std::cout << "+==============================================================================+\n\n";
}

// ================================================================================
// MAIN
// ================================================================================

int main(int argc, char* argv[]) {
    std::string ip = DEFAULT_MULTICAST_IP;
    int port = DEFAULT_PORT;
    int duration = 30;
    
    for (int i = 1; i < argc; i++) {
        std::string arg = argv[i];
        if (arg == "-ip" && i + 1 < argc) ip = argv[++i];
        else if (arg == "-port" && i + 1 < argc) port = std::stoi(argv[++i]);
        else if (arg == "-duration" && i + 1 < argc) duration = std::stoi(argv[++i]);
        else if (arg == "-help" || arg == "--help") {
            std::cout << "Usage: " << argv[0] << " [options]\n"
                      << "  -ip <ip>              Multicast IP (default: 239.1.2.5)\n"
                      << "  -port <port>          UDP port: 26001=EQ, 26002=FO (default: 26001)\n"
                      << "  -duration <seconds>   Benchmark duration (default: 30)\n";
            return 0;
        }
    }
    
    print_banner();
    std::cout << "| IP: " << ip << "  Port: " << port << "  Duration: " << duration << "s\n";
    std::cout << "+------------------------------------------------------------------------------+\n";
    
    // Start threads
    std::thread recv_thread(receiver_thread, ip, port);
    std::thread proc_thread(processor_thread);
    
    // Main loop
    auto start = steady_clock::now();
    auto last_print = start;
    
    while (g_running) {
        std::this_thread::sleep_for(milliseconds(100));
        
        auto now = steady_clock::now();
        auto elapsed = duration_cast<seconds>(now - start).count();
        
        if (elapsed >= duration) {
            g_running = false;
            break;
        }
        
        if (duration_cast<milliseconds>(now - last_print).count() >= 500) {
            print_live_stats(duration_cast<std::chrono::duration<double>>(now - start).count());
            last_print = now;
        }
    }
    
    recv_thread.join();
    proc_thread.join();
    
    double total_elapsed = duration_cast<std::chrono::duration<double>>(steady_clock::now() - start).count();
    print_final_report(total_elapsed);
    
    return 0;
}
