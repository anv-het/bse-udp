/**
 * @file token_manager.hpp
 * @brief Token File Management - BhavCopy and Contract Master with API Download
 * 
 * Full port from Go version with:
 * - API download from BSE extranet
 * - Retry logic with delays
 * - Holiday calendar for trading days
 * - Fallback to older cached files
 * - File cleanup (keep N most recent)
 * - Flexible column detection
 */

#pragma once

#include "domain/contract.hpp"
#include "utils/http_client.hpp"
#include "utils/base64.hpp"
#include <string>
#include <vector>
#include <map>
#include <set>
#include <filesystem>
#include <fstream>
#include <sstream>
#include <iostream>
#include <algorithm>
#include <chrono>
#include <thread>
#include <iomanip>
#include <ctime>

namespace bse {
namespace tokens {

/**
 * @brief Token file manager with API download capability
 * 
 * Handles:
 * - Downloading BhavCopy and Contract Master from BSE API
 * - Caching files locally
 * - Loading tokens from CSV files
 * - File cleanup (keep N most recent)
 * - Holiday-aware date calculation
 */
class TokenManager {
public:
    TokenManager(const std::string& tokens_dir, const std::string& api_base_url)
        : tokens_dir_(tokens_dir)
        , api_base_url_(api_base_url)
        , max_retries_(3)
        , keep_files_(2) {
        
        // Incremental retry delays: 5s, 10s, 15s
        retry_delays_ = {5, 10, 15};
        
        // Create tokens directory if needed
        std::filesystem::create_directories(tokens_dir);
        
        // Initialize BSE holidays for 2025
        init_holidays();
    }
    
    /**
     * @brief Load BhavCopy (Equity tokens) - downloads if needed
     */
    bool load_bhavcopy(domain::TokenMap& token_map) {
        auto target_date = get_target_date();
        std::cout << "\n   📊 Loading BhavCopy (target date: " << format_date(target_date, "%d-%m-%Y") << ")\n";
        
        // Try to get file (download if needed)
        std::string csv_path = get_latest_file("EQ", target_date);
        
        if (csv_path.empty()) {
            std::cout << "   ❌ Failed to get BhavCopy file\n";
            return false;
        }
        
        std::cout << "   📂 Loading: " << std::filesystem::path(csv_path).filename().string() << "\n";
        return parse_bhavcopy(csv_path, token_map);
    }
    
    /**
     * @brief Load Contract Master (F&O tokens) - downloads if needed
     */
    bool load_contract_master(domain::TokenMap& token_map) {
        auto target_date = get_target_date();
        std::cout << "\n   📊 Loading Contract Master (target date: " << format_date(target_date, "%d-%m-%Y") << ")\n";
        
        // Try to get file (download if needed)
        std::string csv_path = get_latest_file("FO", target_date);
        
        if (csv_path.empty()) {
            std::cout << "   ❌ Failed to get Contract Master file\n";
            return false;
        }
        
        std::cout << "   📂 Loading: " << std::filesystem::path(csv_path).filename().string() << "\n";
        return parse_contract_master(csv_path, token_map);
    }

private:
    std::string tokens_dir_;
    std::string api_base_url_;
    int max_retries_;
    std::vector<int> retry_delays_;  // Incremental delays: 5s, 10s, 15s
    int keep_files_;
    std::set<std::string> holidays_;  // Format: YYYY-MM-DD
    
    // ========================================================================
    // Holiday Calendar
    // ========================================================================
    
    void init_holidays() {
        // BSE holidays for 2025
        holidays_ = {
            "2025-01-26",  // Republic Day
            "2025-03-14",  // Holi
            "2025-03-31",  // Id-Ul-Fitr
            "2025-04-10",  // Shri Mahavir Jayanti
            "2025-04-14",  // Dr. Ambedkar Jayanti
            "2025-04-18",  // Good Friday
            "2025-05-01",  // Maharashtra Day
            "2025-08-15",  // Independence Day
            "2025-08-27",  // Janmashtami
            "2025-10-02",  // Gandhi Jayanti
            "2025-10-21",  // Diwali Laxmi Pujan
            "2025-10-22",  // Diwali Balipratipada
            "2025-11-05",  // Prakash Gurpurab
            "2025-11-26",  // Guru Nanak Jayanti
            "2025-12-25",  // Christmas
            
            // BSE holidays for 2026 (tentative - verify with BSE)
            "2026-01-26",  // Republic Day
            "2026-03-03",  // Holi
            "2026-03-20",  // Id-Ul-Fitr (Eid)
            "2026-03-30",  // Shri Mahavir Jayanti
            "2026-04-03",  // Good Friday
            "2026-04-14",  // Dr. Ambedkar Jayanti
            "2026-05-01",  // Maharashtra Day
            "2026-05-27",  // Id-Ul-Adha (Bakri Id)
            "2026-08-15",  // Independence Day / Janmashtami
            "2026-10-02",  // Gandhi Jayanti
            "2026-10-09",  // Dussehra
            "2026-11-09",  // Diwali Laxmi Pujan
            "2026-11-10",  // Diwali Balipratipada  
            "2026-11-16",  // Guru Nanak Jayanti
            "2026-12-25",  // Christmas
        };
    }
    
    /**
     * @brief Check if a date is a trading day
     */
    bool is_trading_day(std::tm date) const {
        // Check weekend (0=Sunday, 6=Saturday)
        if (date.tm_wday == 0 || date.tm_wday == 6) {
            return false;
        }
        
        // Check holiday
        std::string date_str = format_date_tm(date, "%Y-%m-%d");
        return holidays_.find(date_str) == holidays_.end();
    }
    
    /**
     * @brief Get target date (previous trading day)
     */
    std::tm get_target_date() const {
        auto now = std::chrono::system_clock::now();
        auto time_t_now = std::chrono::system_clock::to_time_t(now);
        std::tm target;
        
#ifdef _WIN32
        localtime_s(&target, &time_t_now);
#else
        localtime_r(&time_t_now, &target);
#endif
        
        // Go back one day
        target.tm_mday -= 1;
        std::mktime(&target);  // Normalize
        
        // Keep going back until we find a trading day
        while (!is_trading_day(target)) {
            target.tm_mday -= 1;
            std::mktime(&target);  // Normalize
        }
        
        return target;
    }
    
    /**
     * @brief Get last N trading dates for fallback
     */
    std::vector<std::tm> get_trading_dates(int count) const {
        std::vector<std::tm> dates;
        std::tm current = get_target_date();
        
        while (static_cast<int>(dates.size()) < count) {
            if (is_trading_day(current)) {
                dates.push_back(current);
            }
            current.tm_mday -= 1;
            std::mktime(&current);  // Normalize
        }
        
        return dates;
    }
    
    // ========================================================================
    // Date Formatting
    // ========================================================================
    
    std::string format_date(std::tm date, const std::string& fmt) const {
        return format_date_tm(date, fmt);
    }
    
    static std::string format_date_tm(std::tm date, const std::string& fmt) {
        char buffer[64];
        std::strftime(buffer, sizeof(buffer), fmt.c_str(), &date);
        return std::string(buffer);
    }
    
    std::string get_month_name(int month) const {
        static const char* months[] = {
            "JAN", "FEB", "MAR", "APR", "MAY", "JUN",
            "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"
        };
        return months[month];
    }
    
    std::string get_weekday_name(int wday) const {
        static const char* days[] = {
            "Sunday", "Monday", "Tuesday", "Wednesday", 
            "Thursday", "Friday", "Saturday"
        };
        return days[wday];
    }
    
    // ========================================================================
    // File Management
    // ========================================================================
    
    /**
     * @brief Get expected filename for a date
     */
    std::string get_expected_filename(std::tm date, const std::string& feed_type) const {
        std::string date_ddmmyyyy = format_date(date, "%d%m%Y");
        
        if (feed_type == "EQ") {
            return "BhavCopy_BSE_CM_" + date_ddmmyyyy + ".csv";
        }
        return "BSE_EQD_CONTRACT_" + date_ddmmyyyy + ".csv";
    }
    
    /**
     * @brief Get the latest file, downloading if needed
     */
    std::string get_latest_file(const std::string& feed_type, std::tm target_date) {
        // Check if file exists
        std::string filename = get_expected_filename(target_date, feed_type);
        std::string file_path = (std::filesystem::path(tokens_dir_) / filename).string();
        
        if (std::filesystem::exists(file_path)) {
            std::cout << "   ✅ Using cached file: " << filename << "\n";
            return file_path;
        }
        
        // Download with retries
        std::cout << "   📥 Downloading from API (target: " << format_date(target_date, "%d-%m-%Y") << ")...\n";
        
        bool connection_failed = false;
        for (int retry = 1; retry <= max_retries_ && !connection_failed; ++retry) {
            std::cout << "   🔄 Attempt " << retry << "/" << max_retries_ 
                      << " for " << format_date(target_date, "%d-%m-%Y") 
                      << " (" << get_weekday_name(target_date.tm_wday) << ")...\n";
            
            auto [csv_bytes, is_connection_error] = fetch_from_api(target_date, feed_type);
            
            if (!csv_bytes.empty()) {
                // Save file
                std::ofstream out(file_path, std::ios::binary);
                if (out.is_open()) {
                    out.write(reinterpret_cast<const char*>(csv_bytes.data()), csv_bytes.size());
                    out.close();
                    std::cout << "   ✅ Downloaded and saved: " << filename << "\n";
                    
                    // Cleanup old files
                    cleanup_old_files(feed_type);
                    
                    return file_path;
                }
            }
            
            // Skip retries on connection errors (not on BSE network)
            if (is_connection_error) {
                std::cout << "   ⚠️  Connection failed - not on BSE network, using cached files\n";
                connection_failed = true;
                break;
            }
            
            if (retry < max_retries_) {
                int delay = retry_delays_[static_cast<size_t>(retry - 1)];
                std::cout << "   ⚠️  Retry " << retry << " failed, waiting " 
                          << delay << "s before next attempt...\n";
                std::this_thread::sleep_for(std::chrono::seconds(delay));
            }
        }
        
        // Fallback to older cached files
        std::cout << "   🔍 Looking for older cached files...\n";
        auto fallback_dates = get_trading_dates(7);
        
        for (const auto& date : fallback_dates) {
            std::string fallback_filename = get_expected_filename(date, feed_type);
            std::string fallback_path = (std::filesystem::path(tokens_dir_) / fallback_filename).string();
            
            if (std::filesystem::exists(fallback_path)) {
                std::cout << "   📁 Using fallback: " << fallback_filename << "\n";
                return fallback_path;
            }
        }
        
        // Last resort: find any matching file
        std::string pattern = (feed_type == "EQ") ? "BhavCopy_BSE_CM_" : "BSE_EQD_CONTRACT_";
        std::string any_file = find_any_file(pattern);
        
        if (!any_file.empty()) {
            std::cout << "   📁 Using existing file: " << std::filesystem::path(any_file).filename().string() << "\n";
            return any_file;
        }
        
        return "";
    }
    
    /**
     * @brief Find any file matching pattern
     */
    std::string find_any_file(const std::string& pattern) {
        std::vector<std::filesystem::path> matches;
        
        try {
            for (const auto& entry : std::filesystem::directory_iterator(tokens_dir_)) {
                if (entry.is_regular_file()) {
                    std::string filename = entry.path().filename().string();
                    if (filename.find(pattern) != std::string::npos &&
                        filename.find(".csv") != std::string::npos) {
                        matches.push_back(entry.path());
                    }
                }
            }
        } catch (...) {
            return "";
        }
        
        if (matches.empty()) return "";
        
        // Sort and return latest
        std::sort(matches.begin(), matches.end());
        return matches.back().string();
    }
    
    /**
     * @brief Cleanup old files (keep N most recent)
     */
    void cleanup_old_files(const std::string& feed_type) {
        std::string pattern = (feed_type == "EQ") ? "BhavCopy_BSE_CM_" : "BSE_EQD_CONTRACT_";
        
        std::vector<std::filesystem::path> files;
        
        try {
            for (const auto& entry : std::filesystem::directory_iterator(tokens_dir_)) {
                if (entry.is_regular_file()) {
                    std::string filename = entry.path().filename().string();
                    if (filename.find(pattern) != std::string::npos &&
                        filename.find(".csv") != std::string::npos) {
                        files.push_back(entry.path());
                    }
                }
            }
        } catch (...) {
            return;
        }
        
        if (files.size() <= static_cast<size_t>(keep_files_)) {
            return;
        }
        
        // Sort by filename (contains date)
        std::sort(files.begin(), files.end());
        
        // Remove oldest files
        int to_remove = static_cast<int>(files.size()) - keep_files_;
        for (int i = 0; i < to_remove; ++i) {
            try {
                std::filesystem::remove(files[i]);
                std::cout << "   🗑️  Deleted old file: " << files[i].filename().string() << "\n";
            } catch (...) {
                // Ignore errors
            }
        }
        
        if (to_remove > 0) {
            std::cout << "   🧹 Cleanup: Deleted " << to_remove << " old files, kept " << keep_files_ << "\n";
        }
    }
    
    // ========================================================================
    // API Download
    // ========================================================================
    
    /**
     * @brief Fetch file from BSE API
     * @return pair<bytes, is_connection_error>
     */
    std::pair<std::vector<uint8_t>, bool> fetch_from_api(std::tm date, const std::string& feed_type) {
        // Build file path like Go version
        std::string date_dd_mm_yyyy = format_date(date, "%d-%m-%Y");
        std::string date_ddmmyyyy = format_date(date, "%d%m%Y");
        std::string date_yyyymmdd = format_date(date, "%Y%m%d");
        std::string month_year = get_month_name(date.tm_mon) + "-" + std::to_string(date.tm_year + 1900);
        
        std::string file_name, file_path;
        
        if (feed_type == "EQ") {
            file_name = "BhavCopy_BSE_CM_0_0_0_" + date_yyyymmdd + "_F_0000.csv";
            file_path = "EQ/Common/" + month_year + "/" + date_dd_mm_yyyy + "/" + file_name;
        } else {
            file_name = "BSE_EQD_CONTRACT_" + date_ddmmyyyy + ".csv";
            file_path = "FNO/Common/" + month_year + "/" + date_dd_mm_yyyy + "/" + file_name;
        }
        
        std::cout << "   📡 API Request: " << file_name << "\n";
        std::cout << "   📂 Path: " << file_path << "\n";
        
        // Build JSON request body
        std::ostringstream json_body;
        json_body << "{\"path\":\"" << file_path << "\",\"file_name\":\"" << file_name << "\"}";
        
        // Make HTTP request
        utils::HttpClient client;
        client.set_timeout(30000);  // 30 seconds
        
        std::string url = api_base_url_ + "?api_type=erp";
        auto response = client.post_json(url, json_body.str());
        
        if (!response.success) {
            std::cout << "   ❌ HTTP error: " << response.error << "\n";
            // Check if it's a connection error (WinHttpConnect failed)
            bool is_connection_error = response.error.find("WinHttpConnect") != std::string::npos ||
                                       response.error.find("WinHttpOpen") != std::string::npos;
            return {{}, is_connection_error};
        }
        
        if (response.status_code != 200) {
            std::cout << "   ❌ HTTP " << response.status_code << "\n";
            return {{}, false};
        }
        
        // Parse JSON response
        // Looking for: {"status":"success","data":{"file_content":"BASE64..."}}
        std::string status = extract_json_string(response.body, "status");
        if (status != "success") {
            std::string message = extract_json_string(response.body, "message");
            std::cout << "   ❌ API error: " << message << "\n";
            return {{}, false};
        }
        
        std::string file_content = extract_json_string(response.body, "file_content");
        if (file_content.empty()) {
            std::cout << "   ❌ Empty file content\n";
            return {{}, false};
        }
        
        // Decode base64
        auto csv_bytes = utils::Base64::decode(file_content);
        std::cout << "   ✅ Received " << csv_bytes.size() << " bytes\n";
        
        return {csv_bytes, false};
    }
    
    /**
     * @brief Extract string value from JSON (simple parser)
     */
    std::string extract_json_string(const std::string& json, const std::string& key) {
        std::string search = "\"" + key + "\":\"";
        size_t pos = json.find(search);
        if (pos == std::string::npos) return "";
        
        pos += search.length();
        size_t end = json.find("\"", pos);
        if (end == std::string::npos) return "";
        
        return json.substr(pos, end - pos);
    }
    
    // ========================================================================
    // CSV Parsing
    // ========================================================================
    
    /**
     * @brief Parse BhavCopy CSV file
     */
    bool parse_bhavcopy(const std::string& csv_path, domain::TokenMap& token_map) {
        std::ifstream file(csv_path);
        if (!file.is_open()) {
            std::cout << "   ❌ Failed to open file\n";
            return false;
        }
        
        std::string line;
        
        // Read header
        if (!std::getline(file, line)) {
            std::cout << "   ❌ Empty file\n";
            return false;
        }
        
        // Find column indices
        auto header = parse_csv_line(line);
        auto col_idx = build_column_index(header);
        
        // Find columns (multiple possible names like Go version)
        int token_col = find_column(col_idx, {"fininstrmid", "sc_code", "scripcode"});
        int symbol_col = find_column(col_idx, {"tckrsymb", "sc_name", "scripname"});
        int name_col = find_column(col_idx, {"fininstrmfulnm", "scty_nm", "securityname", "sctynm"});
        
        if (token_col == -1 || symbol_col == -1) {
            std::cout << "   ❌ Required columns not found in BhavCopy\n";
            std::cout << "   Available columns: ";
            for (const auto& [name, idx] : col_idx) {
                std::cout << name << "(" << idx << ") ";
            }
            std::cout << "\n";
            return false;
        }
        
        std::cout << "   Token column: " << token_col << " (" << header[token_col] << ")\n";
        std::cout << "   Symbol column: " << symbol_col << " (" << header[symbol_col] << ")\n";
        if (name_col != -1) {
            std::cout << "   Name column: " << name_col << " (" << header[name_col] << ")\n";
        }
        
        int count = 0;
        int sample_count = 0;
        
        while (std::getline(file, line)) {
            auto fields = parse_csv_line(line);
            
            if (static_cast<int>(fields.size()) <= token_col || 
                static_cast<int>(fields.size()) <= symbol_col) {
                continue;
            }
            
            try {
                uint32_t token = static_cast<uint32_t>(std::stoul(fields[token_col]));
                if (token == 0) continue;
                
                std::string symbol = fields[symbol_col];
                if (symbol.empty()) continue;
                
                domain::Contract contract;
                contract.token = token;
                contract.symbol = symbol;
                
                if (name_col != -1 && static_cast<int>(fields.size()) > name_col) {
                    contract.symbol_name = fields[name_col];
                } else {
                    contract.symbol_name = symbol;
                }
                
                contract.segment = "EQ";
                contract.source = "BhavCopy";
                contract.instrument_type = "EQ";
                
                token_map.set(token, contract);
                count++;
                
                // Show sample
                if (sample_count < 5) {
                    std::cout << "      " << token << " → " << symbol 
                              << " (" << contract.symbol_name << ")\n";
                    sample_count++;
                }
                
            } catch (...) {
                continue;
            }
        }
        
        std::cout << "   ✅ Loaded " << count << " equity scripts from BhavCopy\n";
        return count > 0;
    }
    
    /**
     * @brief Parse Contract Master CSV file
     */
    bool parse_contract_master(const std::string& csv_path, domain::TokenMap& token_map) {
        std::ifstream file(csv_path);
        if (!file.is_open()) {
            std::cout << "   ❌ Failed to open file\n";
            return false;
        }
        
        std::string line;
        
        // Read header
        if (!std::getline(file, line)) {
            std::cout << "   ❌ Empty file\n";
            return false;
        }
        
        // Find column indices
        auto header = parse_csv_line(line);
        auto col_idx = build_column_index(header);
        
        // Find columns (multiple possible names like Go version)
        int token_col = find_column(col_idx, {"fininstrmid", "sctyid", "token"});
        int symbol_col = find_column(col_idx, {"tckrsymb", "undrlyng", "symbol"});
        int name_col = find_column(col_idx, {"sctylngnm", "sctyname", "symbolname"});
        int expiry_col = find_column(col_idx, {"xprydt", "exprdt", "expiry"});
        int strike_col = find_column(col_idx, {"strkpric", "strkrt", "strikeprice"});
        int option_col = find_column(col_idx, {"optntp", "opttype", "optiontype"});
        int instr_col = find_column(col_idx, {"fininstrmnm", "instrtyp", "instrumenttype"});
        int lot_col = find_column(col_idx, {"minlot", "lotsize"});
        
        if (token_col == -1 || symbol_col == -1) {
            std::cout << "   ❌ Required columns not found in Contract Master\n";
            std::cout << "   Available columns: ";
            for (const auto& [name, idx] : col_idx) {
                std::cout << name << "(" << idx << ") ";
            }
            std::cout << "\n";
            return false;
        }
        
        std::cout << "   Token column: " << token_col << " (" << header[token_col] << ")\n";
        std::cout << "   Symbol column: " << symbol_col << " (" << header[symbol_col] << ")\n";
        if (name_col != -1) {
            std::cout << "   Name column: " << name_col << " (" << header[name_col] << ")\n";
        }
        
        int count = 0;
        
        while (std::getline(file, line)) {
            auto fields = parse_csv_line(line);
            
            if (static_cast<int>(fields.size()) <= token_col || 
                static_cast<int>(fields.size()) <= symbol_col) {
                continue;
            }
            
            try {
                uint32_t token = static_cast<uint32_t>(std::stoul(fields[token_col]));
                if (token == 0) continue;
                
                std::string symbol = fields[symbol_col];
                if (symbol.empty()) continue;
                
                domain::Contract contract;
                contract.token = token;
                contract.symbol = symbol;
                contract.segment = "FO";
                contract.source = "ContractMaster";
                
                if (name_col != -1 && static_cast<int>(fields.size()) > name_col) {
                    contract.symbol_name = fields[name_col];
                }
                
                if (expiry_col != -1 && static_cast<int>(fields.size()) > expiry_col) {
                    contract.expiry = fields[expiry_col];
                }
                
                if (strike_col != -1 && static_cast<int>(fields.size()) > strike_col) {
                    try {
                        contract.strike_price = std::stod(fields[strike_col]) / 100.0;  // paise to rupees
                    } catch (...) {
                        contract.strike_price = 0;
                    }
                }
                
                if (option_col != -1 && static_cast<int>(fields.size()) > option_col) {
                    contract.option_type = fields[option_col];
                }
                
                if (instr_col != -1 && static_cast<int>(fields.size()) > instr_col) {
                    contract.instrument_type = fields[instr_col];
                }
                
                if (lot_col != -1 && static_cast<int>(fields.size()) > lot_col) {
                    try {
                        contract.lot_size = std::stoi(fields[lot_col]);
                    } catch (...) {
                        contract.lot_size = 0;
                    }
                }
                
                // Build symbol name if not set
                if (contract.symbol_name.empty()) {
                    std::ostringstream oss;
                    oss << contract.symbol;
                    if (contract.strike_price > 0) {
                        oss << " " << static_cast<int>(contract.strike_price);
                    }
                    if (!contract.option_type.empty()) {
                        oss << " " << contract.option_type;
                    }
                    if (!contract.expiry.empty() && contract.expiry.size() > 6) {
                        oss << " " << contract.expiry.substr(0, 6);
                    }
                    contract.symbol_name = oss.str();
                }
                
                token_map.set(token, contract);
                count++;
                
            } catch (...) {
                continue;
            }
        }
        
        std::cout << "   ✅ Loaded " << count << " contracts from Contract Master\n";
        return count > 0;
    }
    
    /**
     * @brief Build column name to index map (lowercase)
     */
    std::map<std::string, int> build_column_index(const std::vector<std::string>& header) {
        std::map<std::string, int> col_idx;
        for (size_t i = 0; i < header.size(); ++i) {
            std::string name = header[i];
            std::transform(name.begin(), name.end(), name.begin(), ::tolower);
            // Remove whitespace
            name.erase(std::remove_if(name.begin(), name.end(), ::isspace), name.end());
            col_idx[name] = static_cast<int>(i);
        }
        return col_idx;
    }
    
    /**
     * @brief Find column by multiple possible names
     */
    int find_column(const std::map<std::string, int>& col_idx, 
                    const std::vector<std::string>& names) {
        for (const auto& name : names) {
            auto it = col_idx.find(name);
            if (it != col_idx.end()) {
                return it->second;
            }
        }
        return -1;
    }
    
    /**
     * @brief Parse CSV line handling quoted fields
     */
    std::vector<std::string> parse_csv_line(const std::string& line) {
        std::vector<std::string> fields;
        std::string field;
        bool in_quotes = false;
        
        for (char c : line) {
            if (c == '"') {
                in_quotes = !in_quotes;
            } else if (c == ',' && !in_quotes) {
                fields.push_back(trim(field));
                field.clear();
            } else {
                field += c;
            }
        }
        fields.push_back(trim(field));
        
        return fields;
    }
    
    /**
     * @brief Trim whitespace from string
     */
    static std::string trim(const std::string& str) {
        size_t start = str.find_first_not_of(" \t\r\n");
        if (start == std::string::npos) return "";
        size_t end = str.find_last_not_of(" \t\r\n");
        return str.substr(start, end - start + 1);
    }
};

} // namespace tokens
} // namespace bse
