/**
 * @file token_manager.hpp
 * @brief Token File Management - BhavCopy and Contract Master loading
 */

#pragma once

#include "domain/contract.hpp"
#include <string>
#include <vector>
#include <filesystem>
#include <fstream>
#include <sstream>
#include <iostream>
#include <algorithm>
#include <regex>
#include <chrono>
#include <iomanip>

namespace bse {
namespace tokens {

/**
 * @brief Token file manager
 * 
 * Handles:
 * - Loading BhavCopy CSV (EQ tokens)
 * - Loading Contract Master CSV (FO tokens)
 * - File cleanup (keep N most recent)
 */
class TokenManager {
public:
    TokenManager(const std::string& tokens_dir, const std::string& api_base_url)
        : tokens_dir_(tokens_dir)
        , api_base_url_(api_base_url) {
        
        // Create tokens directory if needed
        std::filesystem::create_directories(tokens_dir);
    }
    
    /**
     * @brief Load BhavCopy (Equity tokens)
     * 
     * BhavCopy CSV format:
     * TradDt,BizDt,Sgmt,Src,FinInstrmTp,FinInstrmId,ISIN,TckrSymb,SctyNm,...
     * 
     * Key columns:
     * - FinInstrmId (column 5): Token ID
     * - TckrSymb (column 7): Symbol
     * - SctyNm (column 8): Security Name
     */
    bool load_bhavcopy(domain::TokenMap& token_map) {
        std::cout << "\n   📂 Looking for BhavCopy files..." << std::endl;
        
        // Find latest BhavCopy file
        std::string pattern = "BhavCopy_BSE_CM_";
        std::string latest_file = find_latest_file(pattern);
        
        if (latest_file.empty()) {
            std::cout << "   ❌ No BhavCopy file found in " << tokens_dir_ << std::endl;
            return false;
        }
        
        std::cout << "   📄 Loading: " << std::filesystem::path(latest_file).filename().string() << std::endl;
        
        std::ifstream file(latest_file);
        if (!file.is_open()) {
            std::cout << "   ❌ Failed to open file" << std::endl;
            return false;
        }
        
        std::string line;
        int line_num = 0;
        int loaded = 0;
        
        while (std::getline(file, line)) {
            line_num++;
            
            // Skip header
            if (line_num == 1) continue;
            
            // Parse CSV line
            auto fields = parse_csv_line(line);
            if (fields.size() < 10) continue;
            
            try {
                domain::Contract contract;
                
                // FinInstrmId (Token) - column index 5
                contract.token = static_cast<uint32_t>(std::stoul(fields[5]));
                
                // TckrSymb (Symbol) - column index 7
                contract.symbol = fields[7];
                
                // SctyNm (Security Name) - column index 8 (if exists)
                if (fields.size() > 8) {
                    contract.symbol_name = fields[8];
                }
                
                contract.segment = "EQ";
                contract.source = "BhavCopy";
                contract.instrument_type = "EQ";
                
                token_map.set(contract.token, contract);
                loaded++;
                
            } catch (const std::exception& e) {
                // Skip invalid lines
                continue;
            }
        }
        
        std::cout << "   ✅ Loaded " << loaded << " EQ tokens" << std::endl;
        return loaded > 0;
    }
    
    /**
     * @brief Load Contract Master (F&O tokens)
     * 
     * Contract Master CSV format:
     * Token,InstrumentType,InstId,Symbol,Expiry,StrikePrice,OptionType,CALevel,...
     * 
     * Key columns:
     * - Token (column 0): Token ID
     * - Symbol (column 3): Underlying symbol
     * - Expiry (column 4): Expiry date
     * - StrikePrice (column 5): Strike price (paise)
     * - OptionType (column 6): CE/PE
     */
    bool load_contract_master(domain::TokenMap& token_map) {
        std::cout << "\n   📂 Looking for Contract Master files..." << std::endl;
        
        // Find latest Contract file
        std::string pattern = "BSE_EQD_CONTRACT_";
        std::string latest_file = find_latest_file(pattern);
        
        if (latest_file.empty()) {
            std::cout << "   ❌ No Contract Master file found in " << tokens_dir_ << std::endl;
            return false;
        }
        
        std::cout << "   📄 Loading: " << std::filesystem::path(latest_file).filename().string() << std::endl;
        
        std::ifstream file(latest_file);
        if (!file.is_open()) {
            std::cout << "   ❌ Failed to open file" << std::endl;
            return false;
        }
        
        std::string line;
        int line_num = 0;
        int loaded = 0;
        
        while (std::getline(file, line)) {
            line_num++;
            
            // Skip header (first line)
            if (line_num == 1 && line.find("Token") != std::string::npos) continue;
            
            // Parse CSV line
            auto fields = parse_csv_line(line);
            if (fields.size() < 7) continue;
            
            try {
                domain::Contract contract;
                
                // Token - column 0
                contract.token = static_cast<uint32_t>(std::stoul(fields[0]));
                if (contract.token == 0) continue;
                
                // InstrumentType - column 1
                if (fields.size() > 1) {
                    contract.instrument_type = fields[1];
                }
                
                // Symbol - column 3
                contract.symbol = fields[3];
                
                // Expiry - column 4
                if (fields.size() > 4) {
                    contract.expiry = fields[4];
                }
                
                // StrikePrice - column 5 (in paise, convert to rupees)
                if (fields.size() > 5 && !fields[5].empty()) {
                    try {
                        contract.strike_price = std::stod(fields[5]) / 100.0;
                    } catch (...) {
                        contract.strike_price = 0;
                    }
                }
                
                // OptionType - column 6 (CE/PE)
                if (fields.size() > 6) {
                    contract.option_type = fields[6];
                    // If no option type, it's a future
                    if (contract.option_type.empty() && !contract.expiry.empty()) {
                        contract.option_type = "FUT";
                    }
                }
                
                // Build symbol name
                if (!contract.expiry.empty()) {
                    std::ostringstream oss;
                    oss << contract.symbol;
                    if (contract.strike_price > 0) {
                        oss << " " << static_cast<int>(contract.strike_price);
                    }
                    if (!contract.option_type.empty()) {
                        oss << " " << contract.option_type;
                    }
                    // Shorten expiry (24-DEC-2025 → 24-DEC)
                    if (contract.expiry.size() > 6) {
                        oss << " " << contract.expiry.substr(0, 6);
                    }
                    contract.symbol_name = oss.str();
                } else {
                    contract.symbol_name = contract.symbol;
                }
                
                contract.segment = "FO";
                contract.source = "ContractMaster";
                
                token_map.set(contract.token, contract);
                loaded++;
                
            } catch (const std::exception& e) {
                // Skip invalid lines
                continue;
            }
        }
        
        std::cout << "   ✅ Loaded " << loaded << " F&O tokens" << std::endl;
        return loaded > 0;
    }
    
    /**
     * @brief Cleanup old token files (keep N most recent)
     */
    void cleanup_old_files(const std::string& pattern, int keep_count) {
        std::vector<std::filesystem::path> files;
        
        for (const auto& entry : std::filesystem::directory_iterator(tokens_dir_)) {
            if (entry.is_regular_file()) {
                std::string filename = entry.path().filename().string();
                if (filename.find(pattern) != std::string::npos) {
                    files.push_back(entry.path());
                }
            }
        }
        
        if (files.size() <= static_cast<size_t>(keep_count)) {
            return;
        }
        
        // Sort by filename (contains date)
        std::sort(files.begin(), files.end());
        
        // Remove oldest files
        int to_remove = files.size() - keep_count;
        for (int i = 0; i < to_remove; ++i) {
            try {
                std::filesystem::remove(files[i]);
                std::cout << "   🗑️  Removed old file: " << files[i].filename().string() << std::endl;
            } catch (...) {
                // Ignore errors
            }
        }
    }

private:
    /**
     * @brief Find the latest file matching pattern
     */
    std::string find_latest_file(const std::string& pattern) {
        std::vector<std::filesystem::path> matches;
        
        try {
            for (const auto& entry : std::filesystem::directory_iterator(tokens_dir_)) {
                if (entry.is_regular_file()) {
                    std::string filename = entry.path().filename().string();
                    if (filename.find(pattern) != std::string::npos &&
                        filename.find(".csv") != std::string::npos &&
                        filename.find("_fetched") == std::string::npos) {
                        matches.push_back(entry.path());
                    }
                }
            }
        } catch (...) {
            return "";
        }
        
        if (matches.empty()) {
            return "";
        }
        
        // Sort and return latest (by filename which contains date)
        std::sort(matches.begin(), matches.end());
        return matches.back().string();
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
    
    std::string tokens_dir_;
    std::string api_base_url_;
};

} // namespace tokens
} // namespace bse
