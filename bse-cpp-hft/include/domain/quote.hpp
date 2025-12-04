/**
 * @file quote.hpp
 * @brief Quote structure for processed market data
 */

#pragma once

#include <string>
#include <cstdint>
#include <sstream>
#include <iomanip>
#include <chrono>

namespace bse {
namespace domain {

/**
 * @brief Complete quote with token mapping information
 */
struct Quote {
    // Timestamp
    int64_t timestamp_ns;
    
    // Token info
    uint32_t token;
    std::string symbol;
    std::string symbol_name;
    std::string expiry;
    std::string option_type;  // CE, PE, FUT, or empty for EQ
    double strike_price;
    std::string segment;      // EQ or FO
    
    // Market data
    double ltp;
    double open;
    double high;
    double low;
    double prev_close;
    double atp;
    int64_t volume;
    double turnover;          // In lakhs
    int lot_size;
    uint32_t sequence_num;
    
    // Order book (5 levels)
    double bid_prices[5];
    int64_t bid_qtys[5];
    double ask_prices[5];
    int64_t ask_qtys[5];
    
    // Format timestamp for CSV
    std::string timestamp_string() const {
        auto ns = timestamp_ns;
        auto seconds = ns / 1'000'000'000LL;
        auto millis = (ns % 1'000'000'000LL) / 1'000'000LL;
        
        auto time_point = std::chrono::system_clock::time_point(
            std::chrono::seconds(seconds)
        );
        auto time_t_val = std::chrono::system_clock::to_time_t(time_point);
        
        std::tm tm_val;
#ifdef _WIN32
        localtime_s(&tm_val, &time_t_val);
#else
        localtime_r(&time_t_val, &tm_val);
#endif
        
        std::ostringstream oss;
        oss << std::put_time(&tm_val, "%Y-%m-%d %H:%M:%S")
            << "." << std::setfill('0') << std::setw(3) << millis;
        return oss.str();
    }
    
    // Format bid prices for CSV
    std::string bid_prices_string() const {
        std::ostringstream oss;
        for (int i = 0; i < 5; ++i) {
            if (i > 0) oss << ",";
            oss << std::fixed << std::setprecision(2) << bid_prices[i];
        }
        return oss.str();
    }
    
    // Format bid quantities for CSV
    std::string bid_qtys_string() const {
        std::ostringstream oss;
        for (int i = 0; i < 5; ++i) {
            if (i > 0) oss << ",";
            oss << bid_qtys[i];
        }
        return oss.str();
    }
    
    // Format ask prices for CSV
    std::string ask_prices_string() const {
        std::ostringstream oss;
        for (int i = 0; i < 5; ++i) {
            if (i > 0) oss << ",";
            oss << std::fixed << std::setprecision(2) << ask_prices[i];
        }
        return oss.str();
    }
    
    // Format ask quantities for CSV
    std::string ask_qtys_string() const {
        std::ostringstream oss;
        for (int i = 0; i < 5; ++i) {
            if (i > 0) oss << ",";
            oss << ask_qtys[i];
        }
        return oss.str();
    }
};

} // namespace domain
} // namespace bse
