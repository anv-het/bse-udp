/**
 * @file config.hpp
 * @brief Configuration loader from JSON
 */

#pragma once

#include <string>
#include <fstream>
#include <sstream>
#include <stdexcept>
#include <regex>

namespace bse {
namespace config {

/**
 * @brief Multicast configuration
 */
struct MulticastConfig {
    std::string ip = "239.1.2.5";
    int port = 26001;
    int buffer_size = 2048;
    int socket_rcv_buf = 32 * 1024 * 1024;
    int read_timeout_ms = 50;
};

/**
 * @brief Segment enable flags
 */
struct SegmentsConfig {
    bool cm_enabled = true;
    bool fo_enabled = true;
};

/**
 * @brief API configuration
 */
struct APIConfig {
    std::string base_url = "https://bfrapi.bfrconnect.com";
};

/**
 * @brief Data management configuration
 */
struct DataManagementConfig {
    std::string output_dir = "data/processed_csv";
    std::string tokens_dir = "data/tokens";
    int keep_files = 2;
};

/**
 * @brief Performance tuning configuration
 */
struct PerformanceConfig {
    int ring_buffer_size = 16384;
    int csv_batch_size = 100;
    int csv_buffer_size = 131072;
    int csv_channel_size = 10000;
};

/**
 * @brief Complete application configuration
 */
struct Config {
    MulticastConfig multicast_cm;   // Default port 26001
    MulticastConfig multicast_fo;   // Will be set to 26002 in constructor
    SegmentsConfig segments;
    APIConfig api;
    DataManagementConfig data_management;
    PerformanceConfig performance;
    
    Config() {
        // Ensure F&O uses correct default port
        multicast_fo.port = 26002;
    }
    
    /**
     * @brief Load configuration from JSON file
     */
    static Config load(const std::string& path) {
        std::ifstream file(path);
        if (!file.is_open()) {
            throw std::runtime_error("Cannot open config file: " + path);
        }
        
        std::stringstream buffer;
        buffer << file.rdbuf();
        std::string json = buffer.str();
        
        Config cfg;
        
        // Parse multicast_cm
        cfg.multicast_cm.ip = extract_string(json, "multicast_cm", "ip", "239.1.2.5");
        cfg.multicast_cm.port = extract_int(json, "multicast_cm", "port", 26001);
        cfg.multicast_cm.buffer_size = extract_int(json, "multicast_cm", "buffer_size", 2048);
        cfg.multicast_cm.socket_rcv_buf = extract_int(json, "multicast_cm", "socket_rcv_buf", 33554432);
        cfg.multicast_cm.read_timeout_ms = extract_int(json, "multicast_cm", "read_timeout_ms", 50);
        
        // Parse multicast_fo
        cfg.multicast_fo.ip = extract_string(json, "multicast_fo", "ip", "239.1.2.5");
        cfg.multicast_fo.port = extract_int(json, "multicast_fo", "port", 26002);
        cfg.multicast_fo.buffer_size = extract_int(json, "multicast_fo", "buffer_size", 2048);
        cfg.multicast_fo.socket_rcv_buf = extract_int(json, "multicast_fo", "socket_rcv_buf", 33554432);
        cfg.multicast_fo.read_timeout_ms = extract_int(json, "multicast_fo", "read_timeout_ms", 50);
        
        // Parse segments
        cfg.segments.cm_enabled = extract_bool(json, "segments", "cm_enabled", true);
        cfg.segments.fo_enabled = extract_bool(json, "segments", "fo_enabled", true);
        
        // Parse api
        cfg.api.base_url = extract_string(json, "api", "base_url", "https://bfrapi.bfrconnect.com");
        
        // Parse data_management
        cfg.data_management.output_dir = extract_string(json, "data_management", "output_dir", "data/processed_csv");
        cfg.data_management.tokens_dir = extract_string(json, "data_management", "tokens_dir", "data/tokens");
        cfg.data_management.keep_files = extract_int(json, "data_management", "keep_files", 2);
        
        // Parse performance
        cfg.performance.ring_buffer_size = extract_int(json, "performance", "ring_buffer_size", 16384);
        cfg.performance.csv_batch_size = extract_int(json, "performance", "csv_batch_size", 100);
        cfg.performance.csv_buffer_size = extract_int(json, "performance", "csv_buffer_size", 131072);
        cfg.performance.csv_channel_size = extract_int(json, "performance", "csv_channel_size", 10000);
        
        return cfg;
    }
    
    /**
     * @brief Load configuration or return defaults
     */
    static Config load_or_default(const std::string& path) {
        try {
            return load(path);
        } catch (...) {
            return Config{};  // Return defaults
        }
    }

private:
    // Simple JSON value extraction (no external library dependency)
    static std::string extract_string(const std::string& json, const std::string& section, 
                                      const std::string& key, const std::string& default_val) {
        // Find section
        size_t section_pos = json.find("\"" + section + "\"");
        if (section_pos == std::string::npos) return default_val;
        
        // Find key within section
        size_t key_pos = json.find("\"" + key + "\"", section_pos);
        if (key_pos == std::string::npos) return default_val;
        
        // Find value
        size_t colon_pos = json.find(":", key_pos);
        if (colon_pos == std::string::npos) return default_val;
        
        size_t quote_start = json.find("\"", colon_pos);
        if (quote_start == std::string::npos) return default_val;
        
        size_t quote_end = json.find("\"", quote_start + 1);
        if (quote_end == std::string::npos) return default_val;
        
        return json.substr(quote_start + 1, quote_end - quote_start - 1);
    }
    
    static int extract_int(const std::string& json, const std::string& section,
                           const std::string& key, int default_val) {
        size_t section_pos = json.find("\"" + section + "\"");
        if (section_pos == std::string::npos) return default_val;
        
        size_t key_pos = json.find("\"" + key + "\"", section_pos);
        if (key_pos == std::string::npos) return default_val;
        
        size_t colon_pos = json.find(":", key_pos);
        if (colon_pos == std::string::npos) return default_val;
        
        // Skip whitespace
        size_t val_start = colon_pos + 1;
        while (val_start < json.size() && (json[val_start] == ' ' || json[val_start] == '\t')) {
            val_start++;
        }
        
        // Extract number
        size_t val_end = val_start;
        while (val_end < json.size() && (std::isdigit(json[val_end]) || json[val_end] == '-')) {
            val_end++;
        }
        
        try {
            return std::stoi(json.substr(val_start, val_end - val_start));
        } catch (...) {
            return default_val;
        }
    }
    
    static bool extract_bool(const std::string& json, const std::string& section,
                             const std::string& key, bool default_val) {
        size_t section_pos = json.find("\"" + section + "\"");
        if (section_pos == std::string::npos) return default_val;
        
        size_t key_pos = json.find("\"" + key + "\"", section_pos);
        if (key_pos == std::string::npos) return default_val;
        
        size_t colon_pos = json.find(":", key_pos);
        if (colon_pos == std::string::npos) return default_val;
        
        // Look for true/false
        if (json.find("true", colon_pos) < json.find(",", colon_pos) &&
            json.find("true", colon_pos) < json.find("}", colon_pos)) {
            return true;
        }
        if (json.find("false", colon_pos) < json.find(",", colon_pos) &&
            json.find("false", colon_pos) < json.find("}", colon_pos)) {
            return false;
        }
        
        return default_val;
    }
};

} // namespace config
} // namespace bse
