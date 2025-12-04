/**
 * @file base64.hpp
 * @brief Base64 encoding/decoding utilities
 */

#pragma once

#include <string>
#include <vector>
#include <stdexcept>

namespace bse {
namespace utils {

/**
 * @brief Base64 decoder
 */
class Base64 {
public:
    /**
     * @brief Decode base64 string to bytes
     */
    static std::vector<uint8_t> decode(const std::string& encoded) {
        static const std::string chars = 
            "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
        
        std::vector<uint8_t> result;
        result.reserve(encoded.size() * 3 / 4);
        
        int val = 0;
        int bits = -8;
        
        for (unsigned char c : encoded) {
            if (c == '=') break;
            if (c == ' ' || c == '\n' || c == '\r' || c == '\t') continue;
            
            size_t pos = chars.find(c);
            if (pos == std::string::npos) continue;
            
            val = (val << 6) + static_cast<int>(pos);
            bits += 6;
            
            if (bits >= 0) {
                result.push_back(static_cast<uint8_t>((val >> bits) & 0xFF));
                bits -= 8;
            }
        }
        
        return result;
    }
    
    /**
     * @brief Decode base64 string to string
     */
    static std::string decode_string(const std::string& encoded) {
        auto bytes = decode(encoded);
        return std::string(bytes.begin(), bytes.end());
    }
};

} // namespace utils
} // namespace bse
