/**
 * BSE C++ HFT - Live Token Monitor
 * =================================
 *
 * Monitors specific tokens with LIVE tick-by-tick updates.
 * Shows real-time price changes and order book updates - matches Go version output.
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
 *   - Live tick display with price changes and order book
 *   - CSV file: data/processed_csv/YYYYMMDD_TOKEN_SYMBOL_ticks.csv
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
#include <filesystem>
#include <map>

#ifdef _WIN32
    #define NOMINMAX
    #include <winsock2.h>
    #include <ws2tcpip.h>
    #include <windows.h>
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

struct OrderBookLevel {
    double price = 0;
    int32_t qty = 0;
    int32_t orders = 0;
};

struct OrderBook {
    OrderBookLevel bid[5];
    OrderBookLevel ask[5];
    int bid_levels = 0;
    int ask_levels = 0;
};

struct TickData {
    system_clock::time_point timestamp;
    uint32_t token;
    std::string symbol;
    double ltp;
    double open;
    double high;
    double low;
    double prev_close;
    double atp;
    int32_t volume;
    uint32_t turnover_lakhs;
    uint32_t lot_size;
    uint32_t seq_num;
    OrderBook order_book;
};

struct TokenInfo {
    std::string symbol;
    std::string contract;
    std::string expiry;
    std::string option_type;
    double strike = 0;
};

// ================================================================================
// GLOBALS
// ================================================================================

std::atomic<bool> g_running{true};
std::vector<TickData> g_ticks;
uint32_t g_target_token = 0;
int g_max_ticks = 100;
double g_last_ltp = 0;
std::map<uint32_t, TokenInfo> g_token_map;
int g_packets_scanned = 0;

// ================================================================================
// TOKEN MAP LOADING
// ================================================================================

int load_contract_master() {
    std::string pattern = "data/tokens/BSE_EQD_CONTRACT_";
    std::string latest_file;
    
    try {
        for (const auto& entry : std::filesystem::directory_iterator("data/tokens")) {
            std::string filename = entry.path().filename().string();
            if (filename.find("BSE_EQD_CONTRACT_") == 0 && filename.find(".csv") != std::string::npos) {
                if (filename > latest_file) latest_file = entry.path().string();
            }
        }
    } catch (...) {}
    
    if (latest_file.empty()) return 0;
    
    std::ifstream file(latest_file);
    if (!file.is_open()) return 0;
    
    std::string line;
    int count = 0;
    bool first_line = true;
    
    while (std::getline(file, line)) {
        if (first_line) { first_line = false; continue; }
        
        std::vector<std::string> fields;
        std::stringstream ss(line);
        std::string field;
        while (std::getline(ss, field, ',')) {
            fields.push_back(field);
        }
        
        if (fields.size() < 20) continue;
        
        try {
            uint32_t token = std::stoul(fields[0]);
            if (token == 0) continue;
            
            TokenInfo info;
            info.symbol = fields[3];       // TckrSymb
            info.contract = fields.size() > 18 ? fields[18] : fields[3];  // Contract or symbol
            info.expiry = fields[4];       // Expiry
            info.option_type = fields[6];  // Option type
            
            if (fields.size() > 5) {
                try { info.strike = std::stod(fields[5]) / 100.0; } catch (...) {}
            }
            
            g_token_map[token] = info;
            count++;
        } catch (...) {}
    }
    
    return count;
}

int load_bhavcopy() {
    std::string latest_file;
    
    try {
        for (const auto& entry : std::filesystem::directory_iterator("data/tokens")) {
            std::string filename = entry.path().filename().string();
            if (filename.find("BhavCopy_BSE_CM_") == 0 && filename.find(".csv") != std::string::npos) {
                if (filename > latest_file) latest_file = entry.path().string();
            }
        }
    } catch (...) {}
    
    if (latest_file.empty()) return 0;
    
    std::ifstream file(latest_file);
    if (!file.is_open()) return 0;
    
    std::string line;
    int count = 0;
    bool first_line = true;
    
    while (std::getline(file, line)) {
        if (first_line) { first_line = false; continue; }
        
        std::vector<std::string> fields;
        std::stringstream ss(line);
        std::string field;
        while (std::getline(ss, field, ',')) {
            fields.push_back(field);
        }
        
        if (fields.size() < 8) continue;
        
        try {
            uint32_t token = std::stoul(fields[5]);  // FinInstrmId
            if (token == 0) continue;
            
            TokenInfo info;
            info.symbol = fields[7];   // TckrSymb
            info.contract = fields[7];
            
            g_token_map[token] = info;
            count++;
        } catch (...) {}
    }
    
    return count;
}

std::string get_symbol(uint32_t token) {
    auto it = g_token_map.find(token);
    if (it != g_token_map.end()) {
        if (!it->second.contract.empty()) return it->second.contract;
        return it->second.symbol;
    }
    return "TOKEN_" + std::to_string(token);
}

TokenInfo* get_token_info(uint32_t token) {
    auto it = g_token_map.find(token);
    if (it != g_token_map.end()) return &it->second;
    return nullptr;
}

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

std::string format_date(system_clock::time_point tp) {
    auto time_t_tp = system_clock::to_time_t(tp);
    std::tm tm;
#ifdef _WIN32
    localtime_s(&tm, &time_t_tp);
#else
    localtime_r(&time_t_tp, &tm);
#endif
    char buf[32];
    std::strftime(buf, sizeof(buf), "%Y-%m-%d %H:%M:%S", &tm);
    return buf;
}

std::string format_change(double current, double previous) {
    if (previous == 0) return "  NEW";
    
    double change = current - previous;
    double pct = (change / previous) * 100;
    
    std::ostringstream oss;
    oss << std::fixed << std::setprecision(2);
    
    if (change > 0.001) {  // Use small epsilon for float comparison
        oss << " \xe2\x96\xb2 +" << change << " (+" << pct << "%)";
    } else if (change < -0.001) {
        oss << " \xe2\x96\xbc " << change << " (" << pct << "%)";
    } else {
        oss << "   \xe2\x94\x80 0.00 (0.00%)";
    }
    
    return oss.str();
}

// Format change from previous close (day's change)
std::string format_day_change(double ltp, double prev_close) {
    if (prev_close == 0) return "N/A";
    
    double change = ltp - prev_close;
    double pct = (change / prev_close) * 100;
    
    std::ostringstream oss;
    oss << std::fixed << std::setprecision(2);
    
    if (change > 0) {
        oss << "+" << change << " (+" << pct << "%)";
    } else if (change < 0) {
        oss << change << " (" << pct << "%)";
    } else {
        oss << "0.00 (0.00%)";
    }
    
    return oss.str();
}

// ================================================================================
// CSV WRITER
// ================================================================================

class CSVWriter {
public:
    std::string filename;
    
    bool open(uint32_t token, const std::string& symbol) {
        auto now = system_clock::now();
        auto time_t_now = system_clock::to_time_t(now);
        std::tm tm;
#ifdef _WIN32
        localtime_s(&tm, &time_t_now);
#else
        localtime_r(&time_t_now, &tm);
#endif
        char date_buf[16];
        std::strftime(date_buf, sizeof(date_buf), "%Y%m%d", &tm);
        
        // Clean symbol for filename
        std::string safe_symbol = symbol;
        for (char& c : safe_symbol) {
            if (c == ' ' || c == '/') c = '_';
        }
        if (safe_symbol.length() > 20) safe_symbol = safe_symbol.substr(0, 20);
        
        std::filesystem::create_directories("data/processed_csv");
        
        std::ostringstream oss;
        oss << "data/processed_csv/" << date_buf << "_" << token << "_" << safe_symbol << "_ticks.csv";
        filename = oss.str();
        
        file_.open(filename);
        if (!file_.is_open()) return false;
        
        // Write header
        file_ << "timestamp,token,symbol,ltp,open,high,low,prev_close,atp,"
              << "volume,turnover_lakhs,lot_size,seq,"
              << "bid_prices,bid_qtys,bid_orders,ask_prices,ask_qtys,ask_orders\n";
        
        return true;
    }
    
    void write(const TickData& tick) {
        if (!file_.is_open()) return;
        
        // Format bid/ask as pipe-separated
        std::ostringstream bid_prices, bid_qtys, bid_orders;
        std::ostringstream ask_prices, ask_qtys, ask_orders;
        
        for (int i = 0; i < tick.order_book.bid_levels; i++) {
            if (i > 0) { bid_prices << "|"; bid_qtys << "|"; bid_orders << "|"; }
            bid_prices << std::fixed << std::setprecision(2) << tick.order_book.bid[i].price;
            bid_qtys << tick.order_book.bid[i].qty;
            bid_orders << tick.order_book.bid[i].orders;
        }
        
        for (int i = 0; i < tick.order_book.ask_levels; i++) {
            if (i > 0) { ask_prices << "|"; ask_qtys << "|"; ask_orders << "|"; }
            ask_prices << std::fixed << std::setprecision(2) << tick.order_book.ask[i].price;
            ask_qtys << tick.order_book.ask[i].qty;
            ask_orders << tick.order_book.ask[i].orders;
        }
        
        auto time_t_ts = system_clock::to_time_t(tick.timestamp);
        auto ms = duration_cast<milliseconds>(tick.timestamp.time_since_epoch()) % 1000;
        std::tm tm;
#ifdef _WIN32
        localtime_s(&tm, &time_t_ts);
#else
        localtime_r(&time_t_ts, &tm);
#endif
        char ts_buf[32];
        std::strftime(ts_buf, sizeof(ts_buf), "%Y-%m-%d %H:%M:%S", &tm);
        
        file_ << ts_buf << "." << std::setfill('0') << std::setw(3) << ms.count() << ","
              << tick.token << "," << tick.symbol << ","
              << std::fixed << std::setprecision(2)
              << tick.ltp << "," << tick.open << "," << tick.high << "," 
              << tick.low << "," << tick.prev_close << "," << tick.atp << ","
              << tick.volume << "," << tick.turnover_lakhs << "," 
              << tick.lot_size << "," << tick.seq_num << ","
              << bid_prices.str() << "," << bid_qtys.str() << "," << bid_orders.str() << ","
              << ask_prices.str() << "," << ask_qtys.str() << "," << ask_orders.str() << "\n";
        
        file_.flush();
    }
    
    void close() {
        if (file_.is_open()) file_.close();
    }
    
private:
    std::ofstream file_;
};

CSVWriter g_csv_writer;

// ================================================================================
// DISPLAY FUNCTIONS (Match Go version)
// ================================================================================

void display_tick(const TickData& tick, int tick_num, double prev_ltp) {
    std::string time_str = format_time(tick.timestamp);
    std::string tick_change_str = format_change(tick.ltp, prev_ltp);
    std::string day_change_str = format_day_change(tick.ltp, tick.prev_close);
    
    // Box drawing header (UTF-8)
    std::cout << "\n";
    for (int i = 0; i < 80; i++) std::cout << "\xe2\x95\x90";
    std::cout << "\n";
    std::cout << "  TICK #" << std::left << std::setw(6) << tick_num 
              << "  \xe2\x94\x82  " << time_str 
              << "  \xe2\x94\x82  Token: " << tick.token 
              << "  \xe2\x94\x82  " << tick.symbol << "\n";
    for (int i = 0; i < 80; i++) std::cout << "\xe2\x95\x90";
    std::cout << "\n";
    
    // LTP with tick change and day change
    std::cout << "\n  \xf0\x9f\x92\xb0 LTP: \xe2\x82\xb9" << std::fixed << std::setprecision(2) << tick.ltp << tick_change_str << "\n";
    std::cout << "  \xf0\x9f\x93\x88 Day Change: " << day_change_str << " (from Prev Close: \xe2\x82\xb9" << tick.prev_close << ")\n";
    std::cout << "  ";
    for (int i = 0; i < 72; i++) std::cout << "\xe2\x94\x80";
    std::cout << "\n";
    
    // OHLC
    std::cout << "  Open: \xe2\x82\xb9" << tick.open 
              << "  \xe2\x94\x82  High: \xe2\x82\xb9" << tick.high 
              << "  \xe2\x94\x82  Low: \xe2\x82\xb9" << tick.low 
              << "  \xe2\x94\x82  Prev: \xe2\x82\xb9" << tick.prev_close << "\n";
    std::cout << "  ATP:  \xe2\x82\xb9" << tick.atp 
              << "  \xe2\x94\x82  Volume: " << tick.volume 
              << "  \xe2\x94\x82  Turnover: \xe2\x82\xb9" << tick.turnover_lakhs << "L"
              << "  \xe2\x94\x82  Seq: " << tick.seq_num << "\n";
    
    // Order Book
    std::cout << "\n  \xf0\x9f\x93\x9a ORDER BOOK\n";
    std::cout << "  ";
    for (int i = 0; i < 72; i++) std::cout << "\xe2\x94\x80";
    std::cout << "\n";
    std::cout << "  " << std::left << std::setw(34) << "BID" << " \xe2\x94\x82 " << "ASK\n";
    std::cout << "  " << std::setw(12) << "Price" << std::setw(8) << "  Qty" << std::setw(5) << "  Ord"
              << " \xe2\x94\x82 " << std::setw(12) << "Price" << std::setw(8) << "  Qty" << std::setw(5) << "  Ord" << "\n";
    std::cout << "  ";
    for (int i = 0; i < 72; i++) std::cout << "\xe2\x94\x80";
    std::cout << "\n";
    
    int max_levels = std::max(tick.order_book.bid_levels, tick.order_book.ask_levels);
    if (max_levels > 5) max_levels = 5;
    
    for (int i = 0; i < max_levels; i++) {
        // BID side
        if (i < tick.order_book.bid_levels && tick.order_book.bid[i].qty > 0) {
            std::cout << "  \xe2\x82\xb9" << std::setw(10) << std::fixed << std::setprecision(2) << tick.order_book.bid[i].price
                      << std::setw(8) << tick.order_book.bid[i].qty
                      << std::setw(5) << tick.order_book.bid[i].orders;
        } else {
            std::cout << "  " << std::setw(12) << "--" << std::setw(8) << "--" << std::setw(5) << "--";
        }
        
        std::cout << " \xe2\x94\x82 ";
        
        // ASK side
        if (i < tick.order_book.ask_levels && tick.order_book.ask[i].qty > 0) {
            std::cout << "\xe2\x82\xb9" << std::setw(10) << std::fixed << std::setprecision(2) << tick.order_book.ask[i].price
                      << std::setw(8) << tick.order_book.ask[i].qty
                      << std::setw(5) << tick.order_book.ask[i].orders;
        } else {
            std::cout << std::setw(12) << "--" << std::setw(8) << "--" << std::setw(5) << "--";
        }
        
        std::cout << "\n";
    }
    
    std::cout << "  ";
    for (int i = 0; i < 72; i++) std::cout << "\xe2\x94\x80";
    std::cout << "\n";
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
    
    g_packets_scanned++;
    
    // Show search progress every 100 packets if no ticks yet
    if (g_ticks.empty() && g_packets_scanned % 100 == 0) {
        std::cout << "   \xe2\x8f\xb3 Searching... " << g_packets_scanned 
                  << " packets scanned, waiting for token " << g_target_token << "...\n";
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
        tick.symbol = get_symbol(token);
        tick.open = read_i32_le(record + 4) / 100.0;
        tick.prev_close = read_i32_le(record + 8) / 100.0;
        tick.high = read_i32_le(record + 12) / 100.0;
        tick.low = read_i32_le(record + 16) / 100.0;
        tick.volume = read_i32_le(record + 24);
        tick.turnover_lakhs = read_u32_le(record + 28);
        tick.lot_size = read_u32_le(record + 32);
        tick.ltp = read_i32_le(record + 36) / 100.0;
        tick.seq_num = read_u32_le(record + 44);
        tick.atp = read_i32_le(record + 84) / 100.0;
        
        // Decode order book (5 levels, each level is 32 bytes: 16 bid + 16 ask)
        const uint8_t* ob_ptr = record + 104;
        tick.order_book.bid_levels = 0;
        tick.order_book.ask_levels = 0;
        
        for (int j = 0; j < 5; j++) {
            const uint8_t* level = ob_ptr + (j * 32);
            
            // BID
            int32_t bid_price = read_i32_le(level + 0);
            int32_t bid_qty = read_i32_le(level + 4);
            int32_t bid_orders = read_i32_le(level + 8);
            
            if (bid_qty > 0) {
                tick.order_book.bid[tick.order_book.bid_levels].price = bid_price / 100.0;
                tick.order_book.bid[tick.order_book.bid_levels].qty = bid_qty;
                tick.order_book.bid[tick.order_book.bid_levels].orders = bid_orders;
                tick.order_book.bid_levels++;
            }
            
            // ASK
            int32_t ask_price = read_i32_le(level + 16);
            int32_t ask_qty = read_i32_le(level + 20);
            int32_t ask_orders = read_i32_le(level + 24);
            
            if (ask_qty > 0) {
                tick.order_book.ask[tick.order_book.ask_levels].price = ask_price / 100.0;
                tick.order_book.ask[tick.order_book.ask_levels].qty = ask_qty;
                tick.order_book.ask[tick.order_book.ask_levels].orders = ask_orders;
                tick.order_book.ask_levels++;
            }
        }
        
        // Display tick (like Go version)
        display_tick(tick, static_cast<int>(g_ticks.size()) + 1, g_last_ltp);
        
        // Save to CSV
        g_csv_writer.write(tick);
        
        g_ticks.push_back(tick);
        g_last_ltp = tick.ltp;
        
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
    
    std::cout << "   \xe2\x9c\x85 Connected!\n\n";
    std::cout << "\xf0\x9f\x9a\x80 STARTING LIVE MONITOR (Press Ctrl+C to stop)\n";
    std::cout << "   Watching for token " << g_target_token << " (" << get_symbol(g_target_token) << ")\n";
    std::cout << "   Max ticks: " << g_max_ticks << "\n";
    for (int i = 0; i < 80; i++) std::cout << "=";
    std::cout << "\n";
    
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
    g_csv_writer.close();
    
    std::cout << "\n\n";
    for (int i = 0; i < 80; i++) std::cout << "=";
    std::cout << "\n";
    std::cout << "                         \xf0\x9f\x93\x8a MONITOR SUMMARY\n";
    for (int i = 0; i < 80; i++) std::cout << "=";
    std::cout << "\n";
    
    std::cout << "  Token:          " << g_target_token << "\n";
    std::cout << "  Symbol:         " << get_symbol(g_target_token) << "\n";
    std::cout << "  Ticks Captured: " << g_ticks.size() << "\n";
    std::cout << "  CSV File:       " << g_csv_writer.filename << "\n";
    
    if (g_ticks.empty()) {
        std::cout << "\n  \xe2\x9a\xa0\xef\xb8\x8f  No ticks captured for this token\n";
        for (int i = 0; i < 80; i++) std::cout << "=";
        std::cout << "\n";
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
    
    for (int i = 0; i < 80; i++) std::cout << "-";
    std::cout << "\n";
    std::cout << "  PRICE STATISTICS\n";
    for (int i = 0; i < 80; i++) std::cout << "-";
    std::cout << "\n";
    std::cout << std::fixed << std::setprecision(2);
    std::cout << "  First LTP:      \xe2\x82\xb9" << std::setw(12) << g_ticks.front().ltp
              << "    Last LTP:       \xe2\x82\xb9" << std::setw(12) << g_ticks.back().ltp << "\n";
    std::cout << "  Min LTP:        \xe2\x82\xb9" << std::setw(12) << min_ltp
              << "    Max LTP:        \xe2\x82\xb9" << std::setw(12) << max_ltp << "\n";
    std::cout << "  Avg LTP:        \xe2\x82\xb9" << std::setw(12) << avg_ltp
              << "    Range:          \xe2\x82\xb9" << std::setw(12) << range << "\n";
    std::cout << "  Total Change:   " << std::showpos << std::setw(12) << total_change 
              << " (" << std::setw(6) << total_change_pct << "%)\n";
    std::cout << std::noshowpos;
    
    for (int i = 0; i < 80; i++) std::cout << "-";
    std::cout << "\n";
    std::cout << "  VOLUME STATISTICS\n";
    for (int i = 0; i < 80; i++) std::cout << "-";
    std::cout << "\n";
    std::cout << "  First Vol:      " << std::setw(12) << g_ticks.front().volume
              << "    Last Vol:       " << std::setw(12) << g_ticks.back().volume << "\n";
    std::cout << "  Max Vol:        " << std::setw(12) << max_volume << "\n";
    
    for (int i = 0; i < 80; i++) std::cout << "=";
    std::cout << "\n";
}

// ================================================================================
// MAIN
// ================================================================================

int main(int argc, char* argv[]) {
#ifdef _WIN32
    // Enable UTF-8 console output
    SetConsoleOutputCP(65001);
#endif
    
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
    
    // Load token maps
    std::cout << "\n\xf0\x9f\x93\x82 Loading token maps...\n";
    int fo_count = load_contract_master();
    int eq_count = load_bhavcopy();
    std::cout << "   \xe2\x9c\x85 Loaded " << fo_count << " F&O contracts + " << eq_count 
              << " EQ scripts = " << g_token_map.size() << " total\n";
    
    // Print banner (like Go version)
    for (int i = 0; i < 80; i++) std::cout << "=";
    std::cout << "\n";
    std::cout << "BSE C++ HFT - LIVE TOKEN MONITOR\n";
    for (int i = 0; i < 80; i++) std::cout << "=";
    std::cout << "\n";
    std::cout << "Token: " << g_target_token << "\n";
    
    std::string port_name = (port == 26001) ? "CM - Equity" : "F&O - Derivatives";
    std::cout << "Port:  " << port << " (" << port_name << ")\n";
    std::cout << "Time:  " << format_date(system_clock::now()) << "\n";
    
    // Show token info
    std::string symbol = get_symbol(g_target_token);
    std::cout << "\n\xf0\x9f\x93\x8a Token " << g_target_token << " \xe2\x86\x92 " << symbol << "\n";
    
    TokenInfo* info = get_token_info(g_target_token);
    if (info) {
        std::cout << "   Symbol:     " << info->symbol << "\n";
        std::cout << "   Contract:   " << info->contract << "\n";
        if (!info->expiry.empty()) {
            std::cout << "   Expiry:     " << info->expiry << "\n";
        }
        if (!info->option_type.empty() && info->option_type != "XX") {
            std::cout << "   Option:     " << info->option_type << "\n";
            if (info->strike > 0) {
                std::cout << "   Strike:     \xe2\x82\xb9" << std::fixed << std::setprecision(2) << info->strike << "\n";
            }
        }
    }
    
    // Open CSV file
    if (!g_csv_writer.open(g_target_token, symbol)) {
        std::cerr << "\xe2\x9d\x8c Failed to create CSV file\n";
        return 1;
    }
    std::cout << "\n\xf0\x9f\x92\xbe CSV File: " << g_csv_writer.filename << "\n";
    
    // Connect
    std::cout << "\n\xf0\x9f\x93\xa1 Connecting to " << ip << ":" << port << "...\n";
    
    // Start receiver
    receive_loop(ip, port);
    
    print_summary();
    
    return 0;
}
