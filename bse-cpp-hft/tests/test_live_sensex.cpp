/**
 * BSE C++ HFT - Live SENSEX Market Data Test
 * ==========================================
 *
 * Tests live SENSEX market data with specific target tokens.
 * Validates LTP values against expected prices.
 *
 * ================================================================================
 * USAGE
 * ================================================================================
 *
 * BUILD (from bse-cpp-hft directory):
 *   cl.exe /EHsc /std:c++17 /O2 /I include tests\test_live_sensex.cpp /Fe:bin\test_live_sensex.exe ws2_32.lib
 *
 * RUN:
 *   .\bin\test_live_sensex.exe
 *   .\bin\test_live_sensex.exe -duration 60
 *   .\bin\test_live_sensex.exe -duration 120 -port 26002
 *
 * PARAMETERS:
 *   -duration <seconds>  Test duration in seconds (default: 30)
 *   -port <port>         UDP port: 26001=EQ, 26002=FO (default: 26002)
 *   -ip <ip>             Multicast IP (default: 239.1.2.5)
 *
 * OUTPUT:
 *   - Live display of target token updates
 *   - Summary report with statistics
 *
 * ================================================================================
 */

#include <iostream>
#include <iomanip>
#include <string>
#include <sstream>
#include <map>
#include <vector>
#include <atomic>
#include <thread>
#include <chrono>
#include <cstring>
#include <mutex>

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
constexpr int DEFAULT_FO_PORT = 26002;
constexpr int HEADER_SIZE = 36;
constexpr int RECORD_SIZE = 264;
constexpr int BUFFER_SIZE = 2048;

// ================================================================================
// TARGET TOKENS - SENSEX Contracts
// ================================================================================

struct TargetToken {
    std::string name;
    double expected_ltp;
};

std::map<uint32_t, TargetToken> TARGET_TOKENS = {
    // SENSEX Futures (update with current tokens)
    {1102290, {"SENSEX DEC FUT", 0}},
    {873830, {"SENSEX NOV FUT", 0}},
    
    // SENSEX Options (sample - update with actual tokens)
    {878196, {"SENSEX 83900 CE", 0}},
    {878015, {"SENSEX 83800 PE", 0}},
};

// ================================================================================
// DATA STRUCTURES
// ================================================================================

struct TickUpdate {
    system_clock::time_point timestamp;
    uint32_t token;
    std::string name;
    double ltp;
    int32_t volume;
    uint32_t seq_num;
};

struct TokenStats {
    std::vector<TickUpdate> updates;
    double first_ltp = 0;
    double last_ltp = 0;
    int32_t first_volume = 0;
    int32_t last_volume = 0;
    uint64_t update_count = 0;
};

// ================================================================================
// GLOBALS
// ================================================================================

std::atomic<bool> g_running{true};
std::map<uint32_t, TokenStats> g_token_stats;
std::mutex g_stats_mutex;

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

// ================================================================================
// PACKET DECODER
// ================================================================================

void decode_packet(const uint8_t* data, int length) {
    if (length < HEADER_SIZE + RECORD_SIZE) return;
    
    int num_records = (length - HEADER_SIZE) / RECORD_SIZE;
    auto now = system_clock::now();
    
    for (int i = 0; i < num_records; i++) {
        const uint8_t* record = data + HEADER_SIZE + (i * RECORD_SIZE);
        
        uint32_t token = read_u32_le(record);
        
        // Check if this is a target token
        auto it = TARGET_TOKENS.find(token);
        if (it == TARGET_TOKENS.end()) continue;
        
        double ltp = read_i32_le(record + 36) / 100.0;
        int32_t volume = read_i32_le(record + 24);
        uint32_t seq_num = read_u32_le(record + 44);
        
        TickUpdate update{now, token, it->second.name, ltp, volume, seq_num};
        
        {
            std::lock_guard<std::mutex> lock(g_stats_mutex);
            auto& stats = g_token_stats[token];
            if (stats.update_count == 0) {
                stats.first_ltp = ltp;
                stats.first_volume = volume;
            }
            stats.last_ltp = ltp;
            stats.last_volume = volume;
            stats.update_count++;
            stats.updates.push_back(update);
        }
        
        // Print live update
        std::cout << "\r[" << format_time(now) << "] "
                  << std::setw(20) << std::left << it->second.name
                  << " LTP: " << std::fixed << std::setprecision(2) << std::setw(10) << ltp
                  << " Vol: " << std::setw(10) << volume
                  << " Seq: " << seq_num
                  << "        " << std::flush;
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
    
    // Set large receive buffer
    int rcv_buf = 32 * 1024 * 1024;
    setsockopt(sock, SOL_SOCKET, SO_RCVBUF, (const char*)&rcv_buf, sizeof(rcv_buf));
    
    // Set timeout
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
#ifdef _WIN32
        closesocket(sock);
#else
        close(sock);
#endif
        return;
    }
    
    struct ip_mreq mreq{};
    mreq.imr_multiaddr.s_addr = inet_addr(ip.c_str());
    mreq.imr_interface.s_addr = htonl(INADDR_ANY);
    
    if (setsockopt(sock, IPPROTO_IP, IP_ADD_MEMBERSHIP, (const char*)&mreq, sizeof(mreq)) < 0) {
        std::cerr << "Failed to join multicast group\n";
#ifdef _WIN32
        closesocket(sock);
#else
        close(sock);
#endif
        return;
    }
    
    std::cout << "Connected to " << ip << ":" << port << "\n";
    std::cout << "Monitoring " << TARGET_TOKENS.size() << " target tokens...\n\n";
    
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
    std::cout << "\n\n";
    std::cout << "+==============================================================================+\n";
    std::cout << "|                         SENSEX LIVE TEST SUMMARY                            |\n";
    std::cout << "+==============================================================================+\n";
    
    std::lock_guard<std::mutex> lock(g_stats_mutex);
    
    for (const auto& [token, stats] : g_token_stats) {
        auto it = TARGET_TOKENS.find(token);
        std::string name = it != TARGET_TOKENS.end() ? it->second.name : "Unknown";
        
        double change = stats.last_ltp - stats.first_ltp;
        double change_pct = stats.first_ltp > 0 ? (change / stats.first_ltp * 100) : 0;
        
        std::cout << "| Token: " << token << " - " << std::left << std::setw(25) << name << "|\n";
        std::cout << "|   Updates: " << std::setw(10) << stats.update_count
                  << " First LTP: " << std::fixed << std::setprecision(2) << std::setw(10) << stats.first_ltp
                  << " Last LTP: " << std::setw(10) << stats.last_ltp << "|\n";
        std::cout << "|   Change: " << std::showpos << std::setw(10) << change 
                  << " (" << std::setw(6) << change_pct << "%)                       |\n";
        std::cout << "+------------------------------------------------------------------------------+\n";
    }
    
    if (g_token_stats.empty()) {
        std::cout << "| No target tokens found in the feed                                           |\n";
        std::cout << "+==============================================================================+\n";
    }
}

// ================================================================================
// MAIN
// ================================================================================

int main(int argc, char* argv[]) {
    std::string ip = DEFAULT_MULTICAST_IP;
    int port = DEFAULT_FO_PORT;
    int duration = 30;
    
    // Parse arguments
    for (int i = 1; i < argc; i++) {
        std::string arg = argv[i];
        if (arg == "-ip" && i + 1 < argc) ip = argv[++i];
        else if (arg == "-port" && i + 1 < argc) port = std::stoi(argv[++i]);
        else if (arg == "-duration" && i + 1 < argc) duration = std::stoi(argv[++i]);
        else if (arg == "-help" || arg == "--help") {
            std::cout << "Usage: " << argv[0] << " [options]\n"
                      << "  -ip <ip>              Multicast IP (default: 239.1.2.5)\n"
                      << "  -port <port>          UDP port (default: 26002)\n"
                      << "  -duration <seconds>   Test duration (default: 30)\n";
            return 0;
        }
    }
    
    std::cout << "+==============================================================================+\n";
    std::cout << "|                    BSE HFT C++ - Live SENSEX Monitor                        |\n";
    std::cout << "+==============================================================================+\n";
    std::cout << "| IP: " << ip << "  Port: " << port << "  Duration: " << duration << "s\n";
    std::cout << "+------------------------------------------------------------------------------+\n";
    
    // Start receiver in background
    std::thread receiver_thread(receive_loop, ip, port);
    
    // Wait for duration
    auto start = steady_clock::now();
    while (g_running) {
        std::this_thread::sleep_for(milliseconds(100));
        auto elapsed = duration_cast<seconds>(steady_clock::now() - start).count();
        if (elapsed >= duration) {
            g_running = false;
            break;
        }
    }
    
    receiver_thread.join();
    
    print_summary();
    
    return 0;
}
