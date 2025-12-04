/**
 * @file contract.hpp
 * @brief Contract and TokenMap for token → symbol mapping
 */

#pragma once

#include <string>
#include <cstdint>
#include <unordered_map>
#include <shared_mutex>
#include <optional>

namespace bse {
namespace domain {

/**
 * @brief Contract information from BhavCopy or Contract Master
 */
struct Contract {
    uint32_t token;
    std::string symbol;
    std::string symbol_name;
    std::string instrument_type;  // EQ, SO, IO, SF, IF
    std::string expiry;
    std::string option_type;      // CE, PE, FUT, or empty for EQ
    double strike_price;
    int lot_size;
    std::string segment;          // EQ or FO
    double prev_close;
    std::string source;           // BhavCopy or ContractMaster
};

/**
 * @brief Thread-safe token to contract map
 * 
 * Uses shared_mutex for read-heavy workloads:
 * - Multiple readers (decode threads) can access simultaneously
 * - Single writer (token loader) has exclusive access
 */
class TokenMap {
public:
    TokenMap() = default;
    
    // Add or update a contract (thread-safe)
    void set(uint32_t token, const Contract& contract) {
        std::unique_lock lock(mutex_);
        contracts_[token] = contract;
    }
    
    // Get a contract by token (thread-safe, lock-free read path)
    std::optional<Contract> get(uint32_t token) const {
        std::shared_lock lock(mutex_);
        auto it = contracts_.find(token);
        if (it != contracts_.end()) {
            return it->second;
        }
        return std::nullopt;
    }
    
    // Check if token exists (fast path)
    bool contains(uint32_t token) const {
        std::shared_lock lock(mutex_);
        return contracts_.find(token) != contracts_.end();
    }
    
    // Get symbol for token (convenience method)
    std::string get_symbol(uint32_t token) const {
        std::shared_lock lock(mutex_);
        auto it = contracts_.find(token);
        if (it != contracts_.end()) {
            return it->second.symbol;
        }
        return "";
    }
    
    // Get number of tokens
    size_t size() const {
        std::shared_lock lock(mutex_);
        return contracts_.size();
    }
    
    // Clear all tokens
    void clear() {
        std::unique_lock lock(mutex_);
        contracts_.clear();
    }
    
    // Iterate over all contracts (with lock held)
    template<typename Func>
    void for_each(Func&& func) const {
        std::shared_lock lock(mutex_);
        for (const auto& [token, contract] : contracts_) {
            func(token, contract);
        }
    }

private:
    mutable std::shared_mutex mutex_;
    std::unordered_map<uint32_t, Contract> contracts_;
};

} // namespace domain
} // namespace bse
