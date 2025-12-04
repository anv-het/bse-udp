/**
 * BSE C++ HFT - Live Token Monitor
 * =================================
 *
 * Monitors specific tokens with LIVE tick-by-tick updates.
 * Shows real-time price changes and order book updates.
 *
 * ================================================================================
 * USAGE
 * ================================================================================
 *
 * BUILD (from bse-cpp-hft directory):
 *   cl.exe /EHsc /std:c++17 /O2 /I include tests\test_live_token.cpp /Fe:bin\test_live_token.exe ws2_32.lib
 *
 * RUN EXAMPLES:
 *
 * 1. Monitor Reliance (CM - Equity Cash):
 *    .\bin\test_live_token.exe -token 500325 -port 26001 -ticks 50
 *
 * 2. Monitor SENSEX Future (F&O):
 *    .\bin\test_live_token.exe -token 1102290 -port 26002 -ticks 50
 *
 * 3. Monitor BANKEX Option (F&O):
 *    .\bin\test_live_token.exe -token 880000 -port 26002 -ticks 100
 *
 * PARAMETERS:
 *   -token <id>     Token ID to monitor (required)
 *   -port <port>    UDP port: 26001=EQ, 26002=FO (default: 26002)
 *   -ip <ip>        Multicast IP (default: 239.1.2.5)
 *   -ticks <count>  Max ticks to capture (default: 100)
 *
 * OUTPUT:
 *   - Live tick display with price changes
 *   - Summary statistics at the end
 *
 * ================================================================================
 */

#include <iostream>
#include <iomanip>
#include <string>
#include <vector>
#include <atomic>
#include <thread>
#include <chrono>
#include <cstring>
#include <fstream>
#include <sstream>
#include <algorithm>
#include <cmath>

#ifdef _WIN32
    #define NOMINMAX
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
constexpr int DEFAULT_PORT = 26002;
constexpr int HEADER_SIZE = 36;
constexpr int RECORD_SIZE = 264;
constexpr int BUFFER_SIZE = 2048;

// ================================================================================
// DATA STRUCTURES
// ================================================================================

struct OrderBook {
    double bid_prices[5];
    int32_t bid_qtys[5];
    double ask_prices[5];
    int32_t ask_qtys[5];
};

struct TickData {
    system_clock::time_point timestamp;
    uint32_t token;
    double ltp;
    double open;
    double high;
    double low;
    double prev_close;
    double atp;
    int32_t volume;
    uint32_t turnover;
    uint32_t lot_size;
    uint32_t seq_num;
    OrderBook order_book;
    double ltp_change;
    double ltp_change_pct;
};

// ================================================================================
// GLOBALS
// ================================================================================

std::atomic<bool> g_running{true};
std::vector<TickData> g_ticks;
uint32_t g_target_token = 0;
int g_max_ticks = 100;
double g_last_ltp = 0;

// ================================================================================
// HELPER FUNCTIONS
// ================================================================================

inline uint32_t read_u32_le(const uint8_t* ptr) {
    return static_cast<uint32_t>(ptr[0]) |
           (static_cast<uint32_t>(ptr[1]) << 8) |
           (static_cast<uint32_t>(ptr[2]) << 16) |
           (static_cast<uint32_t>(ptr[3]) << 24);
}

inline int32_t read_i32_le(const uint8_t* ptr) {
    return static_cast<int32_t>(read_u32_le(ptr));
}

std::string format_time(system_clock::time_point tp) {
    auto time_t_tp = system_clock::to_time_t(tp);
    auto ms = duration_cast<milliseconds>(tp.time_since_epoch()) % 1000;
    std::tm tm;
#ifdef _WIN32
    localtime_s(&tm, &time_t_tp);
#else
    localtime_r(&time_t_tp, &tm);
#endif
    char buf[32];
    std::strftime(buf, sizeof(buf), "%H:%M:%S", &tm);
    std::ostringstream oss;
    oss << buf << "." << std::setfill('0') << std::setw(3) << ms.count();
    return oss.str();
}

std::string format_change(double change) {
    std::ostringstream oss;
    if (change > 0) oss << "\033[32m+" << std::fixed << std::setprecision(2) << change << "\033[0m";
    else if (change < 0) oss << "\033[31m" << std::fixed << std::setprecision(2) << change << "\033[0m";
    else oss << "  0.00";
    return oss.str();
}

// ================================================================================
// PACKET DECODER
// ================================================================================

void decode_packet(const uint8_t* data, int length) {
    if (length < HEADER_SIZE + RECORD_SIZE) return;
    if (static_cast<int>(g_ticks.size()) >= g_max_ticks) {
        g_running = false;
        return;
    }
    
    int num_records = (length - HEADER_SIZE) / RECORD_SIZE;
    auto now = system_clock::now();
    
    for (int i = 0; i < num_records; i++) {
        const uint8_t* record = data + HEADER_SIZE + (i * RECORD_SIZE);
        
        uint32_t token = read_u32_le(record);
        
        // Check if this is our target token
        if (token != g_target_token) continue;
        
        TickData tick;
        tick.timestamp = now;
        tick.token = token;
        tick.open = read_i32_le(record + 4) / 100.0;
        tick.prev_close = read_i32_le(record + 8) / 100.0;
        tick.high = read_i32_le(record + 12) / 100.0;
        tick.low = read_i32_le(record + 16) / 100.0;
        tick.volume = read_i32_le(record + 24);
        tick.turnover = read_u32_le(record + 28);
        tick.lot_size = read_u32_le(record + 32);
        tick.ltp = read_i32_le(record + 36) / 100.0;
        tick.seq_num = read_u32_le(record + 44);
        tick.atp = read_i32_le(record + 84) / 100.0;
        
        // Decode order book (5 levels, each level is 32 bytes: 16 bid + 16 ask)
        const uint8_t* ob_ptr = record + 104;
        for (int j = 0; j < 5; j++) {
            const uint8_t* level = ob_ptr + (j * 32);
            tick.order_book.bid_qtys[j] = read_i32_le(level + 0);
            tick.order_book.bid_prices[j] = read_i32_le(level + 8) / 100.0;
            tick.order_book.ask_qtys[j] = read_i32_le(level + 16);
            tick.order_book.ask_prices[j] = read_i32_le(level + 24) / 100.0;
        }
        
        // Calculate change
        tick.ltp_change = tick.ltp - g_last_ltp;
        tick.ltp_change_pct = g_last_ltp > 0 ? (tick.ltp_change / g_last_ltp * 100) : 0;
        
        g_ticks.push_back(tick);
        g_last_ltp = tick.ltp;
        
        // Print live update
        std::cout << "[" << format_time(now) << "] "
                  << "LTP: " << std::fixed << std::setprecision(2) << std::setw(10) << tick.ltp
                  << " " << format_change(tick.ltp_change)
                  << " Vol: " << std::setw(10) << tick.volume
                  << " Bid: " << std::setw(8) << tick.order_book.bid_prices[0]
                  << " Ask: " << std::setw(8) << tick.order_book.ask_prices[0]
                  << " [" << g_ticks.size() << "/" << g_max_ticks << "]"
                  << "\n";
        
        if (static_cast<int>(g_ticks.size()) >= g_max_ticks) {
            g_running = false;
            return;
        }
    }
}

// ================================================================================
// MULTICAST RECEIVER
// ================================================================================

void receive_loop(const std::string& ip, int port) {
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
    DWORD timeout = 100;
    setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, (const char*)&timeout, sizeof(timeout));
#else
    struct timeval tv;
    tv.tv_sec = 0;
    tv.tv_usec = 100000;
    setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
#endif
    
    struct sockaddr_in local_addr{};
    local_addr.sin_family = AF_INET;
    local_addr.sin_port = htons(static_cast<uint16_t>(port));
    local_addr.sin_addr.s_addr = htonl(INADDR_ANY);
    
    if (bind(sock, (struct sockaddr*)&local_addr, sizeof(local_addr)) < 0) {
        std::cerr << "Failed to bind socket\n";
        return;
    }
    
    struct ip_mreq mreq{};
    mreq.imr_multiaddr.s_addr = inet_addr(ip.c_str());
    mreq.imr_interface.s_addr = htonl(INADDR_ANY);
    
    if (setsockopt(sock, IPPROTO_IP, IP_ADD_MEMBERSHIP, (const char*)&mreq, sizeof(mreq)) < 0) {
        std::cerr << "Failed to join multicast group\n";
        return;
    }
    
    std::cout << "Connected to " << ip << ":" << port << "\n";
    std::cout << "Monitoring token " << g_target_token << " for " << g_max_ticks << " ticks...\n\n";
    
    uint8_t buffer[BUFFER_SIZE];
    
    while (g_running) {
        int n = recv(sock, (char*)buffer, sizeof(buffer), 0);
        if (n > 0) {
            decode_packet(buffer, n);
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
// PRINT SUMMARY
// ================================================================================

void print_summary() {
    if (g_ticks.empty()) {
        std::cout << "\nNo ticks captured for token " << g_target_token << "\n";
        return;
    }
    
    // Calculate statistics
    double min_ltp = g_ticks[0].ltp, max_ltp = g_ticks[0].ltp;
    double sum_ltp = 0;
    int32_t max_volume = 0;
    
    for (const auto& tick : g_ticks) {
        min_ltp = std::min(min_ltp, tick.ltp);
        max_ltp = std::max(max_ltp, tick.ltp);
        sum_ltp += tick.ltp;
        max_volume = std::max(max_volume, tick.volume);
    }
    
    double avg_ltp = sum_ltp / g_ticks.size();
    double range = max_ltp - min_ltp;
    double total_change = g_ticks.back().ltp - g_ticks.front().ltp;
    double total_change_pct = g_ticks.front().ltp > 0 ? (total_change / g_ticks.front().ltp * 100) : 0;
    
    std::cout << "\n";
    std::cout << "+==============================================================================+\n";
    std::cout << "|                         TOKEN MONITOR SUMMARY                               |\n";
    std::cout << "+==============================================================================+\n";
    std::cout << "| Token: " << std::setw(15) << g_target_token << "  Ticks: " << std::setw(10) << g_ticks.size() << "                    |\n";
    std::cout << "+------------------------------------------------------------------------------+\n";
    std::cout << "| PRICE STATISTICS                                                             |\n";
    std::cout << "|   First LTP:  " << std::fixed << std::setprecision(2) << std::setw(12) << g_ticks.front().ltp
              << "    Last LTP:   " << std::setw(12) << g_ticks.back().ltp << "            |\n";
    std::cout << "|   Min LTP:    " << std::setw(12) << min_ltp
              << "    Max LTP:    " << std::setw(12) << max_ltp << "            |\n";
    std::cout << "|   Avg LTP:    " << std::setw(12) << avg_ltp
              << "    Range:      " << std::setw(12) << range << "            |\n";
    std::cout << "|   Total Change: " << std::showpos << std::setw(10) << total_change 
              << " (" << std::setw(6) << total_change_pct << "%)                          |\n";
    std::cout << std::noshowpos;
    std::cout << "+------------------------------------------------------------------------------+\n";
    std::cout << "| VOLUME STATISTICS                                                            |\n";
    std::cout << "|   First Vol:  " << std::setw(12) << g_ticks.front().volume
              << "    Last Vol:   " << std::setw(12) << g_ticks.back().volume << "            |\n";
    std::cout << "|   Max Vol:    " << std::setw(12) << max_volume << "                                       |\n";
    std::cout << "+==============================================================================+\n";
}

// ================================================================================
// MAIN
// ================================================================================

int main(int argc, char* argv[]) {
    std::string ip = DEFAULT_MULTICAST_IP;
    int port = DEFAULT_PORT;
    
    // Parse arguments
    for (int i = 1; i < argc; i++) {
        std::string arg = argv[i];
        if (arg == "-ip" && i + 1 < argc) ip = argv[++i];
        else if (arg == "-port" && i + 1 < argc) port = std::stoi(argv[++i]);
        else if (arg == "-token" && i + 1 < argc) g_target_token = std::stoul(argv[++i]);
        else if (arg == "-ticks" && i + 1 < argc) g_max_ticks = std::stoi(argv[++i]);
        else if (arg == "-help" || arg == "--help") {
            std::cout << "Usage: " << argv[0] << " [options]\n"
                      << "  -token <id>       Token ID to monitor (required)\n"
                      << "  -ip <ip>          Multicast IP (default: 239.1.2.5)\n"
                      << "  -port <port>      UDP port: 26001=EQ, 26002=FO (default: 26002)\n"
                      << "  -ticks <count>    Max ticks to capture (default: 100)\n";
            return 0;
        }
    }
    
    if (g_target_token == 0) {
        std::cerr << "Error: -token is required\n";
        std::cerr << "Usage: " << argv[0] << " -token <id> [-port <port>] [-ticks <count>]\n";
        return 1;
    }
    
    std::cout << "+==============================================================================+\n";
    std::cout << "|                      BSE HFT C++ - Live Token Monitor                       |\n";
    std::cout << "+==============================================================================+\n";
    std::cout << "| Token: " << g_target_token << "  Port: " << port << "  Max Ticks: " << g_max_ticks << "\n";
    std::cout << "+------------------------------------------------------------------------------+\n";
    
    // Start receiver
    receive_loop(ip, port);
    
    print_summary();
    
    return 0;
}
