/**
 * @file packet.hpp
 * @brief BSE NFCAST Protocol Packet Structures (Zero-Copy)
 * 
 * ALL FIELDS ARE LITTLE-ENDIAN (validated empirically)
 * 
 * Packet Format:
 * - Header: 36 bytes
 * - Records: N × 264 bytes each
 * - Max packet size: 36 + 6×264 = 1620 bytes
 */

#pragma once

#include <cstdint>
#include <cstring>

namespace bse {
namespace domain {

// ============================================================================
// BSE Protocol Constants
// ============================================================================

constexpr uint32_t HEADER_SIZE = 36;
constexpr uint32_t RECORD_SIZE = 264;
constexpr uint32_t MAX_RECORDS = 6;
constexpr uint32_t MAX_PACKET_SIZE = HEADER_SIZE + (MAX_RECORDS * RECORD_SIZE);  // 1620

constexpr uint16_t MSG_TYPE_EQUITY = 2020;
constexpr uint16_t MSG_TYPE_DERIVATIVE = 2021;

// ============================================================================
// Record Field Offsets (ALL LITTLE-ENDIAN)
// ============================================================================

constexpr uint32_t OFF_TOKEN       = 0;    // uint32_t - Token ID
constexpr uint32_t OFF_OPEN        = 4;    // int32_t  - Open (paise)
constexpr uint32_t OFF_PREV_CLOSE  = 8;    // int32_t  - Previous Close (paise)
constexpr uint32_t OFF_HIGH        = 12;   // int32_t  - High (paise)
constexpr uint32_t OFF_LOW         = 16;   // int32_t  - Low (paise)
constexpr uint32_t OFF_VOLUME      = 24;   // int32_t  - Volume
constexpr uint32_t OFF_TURNOVER    = 28;   // uint32_t - Turnover (lakhs)
constexpr uint32_t OFF_LOT_SIZE    = 32;   // uint32_t - Lot Size
constexpr uint32_t OFF_LTP         = 36;   // int32_t  - LTP (paise)
constexpr uint32_t OFF_LTQ         = 40;   // uint32_t - Last Traded Qty
constexpr uint32_t OFF_SEQUENCE    = 44;   // uint32_t - Sequence Number
constexpr uint32_t OFF_ATP         = 84;   // int32_t  - Avg Traded Price (paise)
constexpr uint32_t OFF_ORDER_BOOK  = 104;  // 160 bytes - 5 levels × 32 bytes

// ============================================================================
// Packed Structures for Zero-Copy Parsing
// ============================================================================

#pragma pack(push, 1)

/**
 * @brief BSE Packet Header (36 bytes)
 * 
 * Layout:
 * - Bytes 0-3:   Leading zeros (0x00000000)
 * - Bytes 4-5:   Format ID (Big-Endian: 0x0234)
 * - Bytes 6-7:   Reserved
 * - Bytes 8-9:   Message Type (Little-Endian: 2020/2021)
 * - Bytes 10-19: Reserved
 * - Bytes 20-21: Hour (Little-Endian)
 * - Bytes 22-23: Minute (Little-Endian)
 * - Bytes 24-25: Second (Little-Endian)
 * - Bytes 26-27: Millisecond (Little-Endian)
 * - Bytes 28-35: Reserved
 */
struct PacketHeader {
    uint32_t leading_zeros;    // 0-3
    uint16_t format_id;        // 4-5 (Big-Endian)
    uint16_t reserved1;        // 6-7
    uint16_t message_type;     // 8-9 (Little-Endian)
    uint8_t  reserved2[10];    // 10-19
    uint16_t hour;             // 20-21 (Little-Endian)
    uint16_t minute;           // 22-23 (Little-Endian)
    uint16_t second;           // 24-25 (Little-Endian)
    uint16_t millisecond;      // 26-27 (Little-Endian)
    uint8_t  reserved3[8];     // 28-35
    
    // Check if valid BSE packet
    inline bool is_valid() const {
        return message_type == MSG_TYPE_EQUITY || message_type == MSG_TYPE_DERIVATIVE;
    }
    
    inline bool is_equity() const {
        return message_type == MSG_TYPE_EQUITY;
    }
    
    inline bool is_derivative() const {
        return message_type == MSG_TYPE_DERIVATIVE;
    }
};

static_assert(sizeof(PacketHeader) == HEADER_SIZE, "PacketHeader must be 36 bytes");

/**
 * @brief Order Book Level (16 bytes per side)
 * 
 * Layout:
 * - Bytes 0-3:  Price (int32_t, paise)
 * - Bytes 4-7:  Quantity (int32_t)
 * - Bytes 8-11: Number of Orders (int32_t)
 * - Bytes 12-15: Reserved
 */
struct OrderBookLevel {
    int32_t  price;       // Paise
    int32_t  quantity;
    int32_t  num_orders;
    uint32_t reserved;
};

static_assert(sizeof(OrderBookLevel) == 16, "OrderBookLevel must be 16 bytes");

/**
 * @brief Order Book Entry (32 bytes = Bid + Ask for one level)
 */
struct OrderBookEntry {
    OrderBookLevel bid;
    OrderBookLevel ask;
};

static_assert(sizeof(OrderBookEntry) == 32, "OrderBookEntry must be 32 bytes");

/**
 * @brief BSE Market Data Record (264 bytes)
 * 
 * This is the raw wire format - DO NOT modify field order!
 * All prices are in paise (divide by 100 for rupees).
 */
struct alignas(8) RawRecord {
    uint32_t token;            // 0-3:    Token ID
    int32_t  open;             // 4-7:    Open (paise)
    int32_t  prev_close;       // 8-11:   Previous Close (paise)
    int32_t  high;             // 12-15:  High (paise)
    int32_t  low;              // 16-19:  Low (paise)
    uint32_t reserved1;        // 20-23:  Reserved
    int32_t  volume;           // 24-27:  Volume
    uint32_t turnover;         // 28-31:  Turnover (lakhs)
    uint32_t lot_size;         // 32-35:  Lot Size
    int32_t  ltp;              // 36-39:  Last Traded Price (paise)
    uint32_t ltq;              // 40-43:  Last Traded Quantity
    uint32_t sequence;         // 44-47:  Sequence Number
    uint8_t  reserved2[36];    // 48-83:  Reserved
    int32_t  atp;              // 84-87:  Average Traded Price (paise)
    uint8_t  reserved3[16];    // 88-103: Reserved
    OrderBookEntry order_book[5]; // 104-263: 5 levels × 32 bytes
    
    // Check if record is valid (non-empty)
    inline bool is_valid() const {
        return token > 1;  // Token 0 or 1 = empty slot
    }
    
    // Get LTP in rupees
    inline double ltp_rupees() const {
        return static_cast<double>(ltp) / 100.0;
    }
    
    // Get open in rupees
    inline double open_rupees() const {
        return static_cast<double>(open) / 100.0;
    }
    
    // Get high in rupees
    inline double high_rupees() const {
        return static_cast<double>(high) / 100.0;
    }
    
    // Get low in rupees
    inline double low_rupees() const {
        return static_cast<double>(low) / 100.0;
    }
    
    // Get prev_close in rupees
    inline double prev_close_rupees() const {
        return static_cast<double>(prev_close) / 100.0;
    }
    
    // Get ATP in rupees
    inline double atp_rupees() const {
        return static_cast<double>(atp) / 100.0;
    }
};

static_assert(sizeof(RawRecord) == RECORD_SIZE, "RawRecord must be 264 bytes");

#pragma pack(pop)

// ============================================================================
// Decoded Record (with floating-point prices for application use)
// ============================================================================

/**
 * @brief Decoded market data record with float prices
 * 
 * This is used after decoding for application logic.
 * Prices are in rupees (not paise).
 */
struct DecodedRecord {
    uint32_t token;
    uint32_t sequence;
    uint32_t lot_size;
    uint32_t ltq;
    int64_t  volume;
    double   turnover;     // In lakhs
    
    // Prices in rupees
    double   ltp;
    double   open;
    double   high;
    double   low;
    double   prev_close;
    double   atp;
    
    // Order book (5 levels)
    double   bid_prices[5];
    int64_t  bid_qtys[5];
    double   ask_prices[5];
    int64_t  ask_qtys[5];
    
    // Timestamp (system time at decode)
    int64_t  timestamp_ns;
    
    // Initialize from raw record (inline for speed)
    inline void from_raw(const RawRecord* raw, int64_t ts_ns) {
        token = raw->token;
        sequence = raw->sequence;
        lot_size = raw->lot_size;
        ltq = raw->ltq;
        volume = raw->volume;
        turnover = static_cast<double>(raw->turnover);
        timestamp_ns = ts_ns;
        
        // Convert paise to rupees
        constexpr double PAISE_TO_RUPEES = 0.01;
        ltp = raw->ltp * PAISE_TO_RUPEES;
        open = raw->open * PAISE_TO_RUPEES;
        high = raw->high * PAISE_TO_RUPEES;
        low = raw->low * PAISE_TO_RUPEES;
        prev_close = raw->prev_close * PAISE_TO_RUPEES;
        atp = raw->atp * PAISE_TO_RUPEES;
        
        // Order book
        for (int i = 0; i < 5; ++i) {
            bid_prices[i] = raw->order_book[i].bid.price * PAISE_TO_RUPEES;
            bid_qtys[i] = raw->order_book[i].bid.quantity;
            ask_prices[i] = raw->order_book[i].ask.price * PAISE_TO_RUPEES;
            ask_qtys[i] = raw->order_book[i].ask.quantity;
        }
    }
};

} // namespace domain
} // namespace bse
