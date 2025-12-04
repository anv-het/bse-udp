/**
 * @file csv_writer.hpp
 * @brief High-Performance Async CSV Writer
 * 
 * Features:
 * - Async batched writes (non-blocking save)
 * - Large buffer for efficient I/O
 * - Lock-free queue for inter-thread communication
 */

#pragma once

#include "domain/quote.hpp"
#include <string>
#include <fstream>
#include <sstream>
#include <iomanip>
#include <queue>
#include <mutex>
#include <condition_variable>
#include <thread>
#include <atomic>
#include <memory>
#include <filesystem>
#include <chrono>

namespace bse {
namespace saver {

/**
 * @brief CSV Writer configuration
 */
struct CSVWriterConfig {
    size_t batch_size = 100;
    size_t buffer_size = 128 * 1024;  // 128KB
    size_t queue_size = 10000;
    size_t flush_interval_ms = 100;
};

/**
 * @brief Async CSV Writer for quotes
 */
class CSVWriter {
public:
    CSVWriter(const std::string& output_dir, const std::string& feed_type,
              const CSVWriterConfig& config = CSVWriterConfig())
        : config_(config)
        , feed_type_(feed_type)
        , count_(0)
        , running_(false)
        , queue_dropped_(0) {
        
        // Create output directory
        std::filesystem::create_directories(output_dir);
        
        // Generate filename with date
        auto now = std::chrono::system_clock::now();
        auto time_t_now = std::chrono::system_clock::to_time_t(now);
        std::tm tm_now;
#ifdef _WIN32
        localtime_s(&tm_now, &time_t_now);
#else
        localtime_r(&time_t_now, &tm_now);
#endif
        
        std::ostringstream filename;
        filename << output_dir << "/"
                 << std::put_time(&tm_now, "%Y%m%d")
                 << "_" << feed_type << "_quotes.csv";
        file_path_ = filename.str();
        
        // Check if file exists
        bool file_exists = std::filesystem::exists(file_path_);
        
        // Open file in append mode
        file_.open(file_path_, std::ios::app | std::ios::out);
        if (!file_.is_open()) {
            throw std::runtime_error("Failed to open CSV file: " + file_path_);
        }
        
        // Set buffer size
        buffer_.resize(config_.buffer_size);
        file_.rdbuf()->pubsetbuf(buffer_.data(), buffer_.size());
        
        // Write header if new file
        if (!file_exists) {
            write_header();
        }
        
        // Start async writer thread
        running_ = true;
        writer_thread_ = std::thread(&CSVWriter::writer_loop, this);
    }
    
    ~CSVWriter() {
        close();
    }
    
    /**
     * @brief Queue quote for async writing (non-blocking)
     * @return true if queued, false if queue full (dropped)
     */
    bool save(const domain::Quote& quote) {
        std::unique_lock<std::mutex> lock(queue_mutex_, std::try_to_lock);
        if (!lock.owns_lock()) {
            // Queue is busy, try without blocking
            return try_queue(quote);
        }
        
        if (queue_.size() >= config_.queue_size) {
            queue_dropped_++;
            return false;
        }
        
        queue_.push(quote);
        queue_cv_.notify_one();
        return true;
    }
    
    /**
     * @brief Close writer gracefully
     */
    void close() {
        if (running_) {
            running_ = false;
            queue_cv_.notify_all();
            
            if (writer_thread_.joinable()) {
                writer_thread_.join();
            }
            
            // Flush remaining
            flush_remaining();
            file_.close();
        }
    }
    
    // Accessors
    const std::string& file_path() const { return file_path_; }
    size_t count() const { return count_; }
    size_t queue_dropped() const { return queue_dropped_; }

private:
    bool try_queue(const domain::Quote& quote) {
        std::lock_guard<std::mutex> lock(queue_mutex_);
        if (queue_.size() >= config_.queue_size) {
            queue_dropped_++;
            return false;
        }
        queue_.push(quote);
        queue_cv_.notify_one();
        return true;
    }
    
    void write_header() {
        file_ << "timestamp,token,symbol,symbol_name,expiry,option_type,strike_price,"
              << "ltp,open,high,low,prev_close,atp,volume,turnover_lakhs,lot_size,sequence_num,"
              << "bid_prices,bid_qtys,ask_prices,ask_qtys,segment\n";
        file_.flush();
    }
    
    void writer_loop() {
        std::vector<domain::Quote> batch;
        batch.reserve(config_.batch_size);
        
        while (running_ || !queue_.empty()) {
            // Wait for data or timeout
            {
                std::unique_lock<std::mutex> lock(queue_mutex_);
                queue_cv_.wait_for(lock, std::chrono::milliseconds(config_.flush_interval_ms),
                    [this] { return !queue_.empty() || !running_; });
                
                // Drain queue into batch
                while (!queue_.empty() && batch.size() < config_.batch_size) {
                    batch.push_back(std::move(queue_.front()));
                    queue_.pop();
                }
            }
            
            // Write batch
            if (!batch.empty()) {
                write_batch(batch);
                batch.clear();
            }
        }
    }
    
    void write_batch(const std::vector<domain::Quote>& batch) {
        for (const auto& quote : batch) {
            write_quote(quote);
        }
        
        // Periodic flush
        if (count_ % 1000 == 0) {
            file_.flush();
        }
    }
    
    void write_quote(const domain::Quote& quote) {
        file_ << quote.timestamp_string() << ","
              << quote.token << ","
              << quote.symbol << ","
              << quote.symbol_name << ","
              << quote.expiry << ","
              << quote.option_type << ","
              << std::fixed << std::setprecision(2) << quote.strike_price << ","
              << quote.ltp << ","
              << quote.open << ","
              << quote.high << ","
              << quote.low << ","
              << quote.prev_close << ","
              << quote.atp << ","
              << quote.volume << ","
              << quote.turnover << ","
              << quote.lot_size << ","
              << quote.sequence_num << ","
              << "\"" << quote.bid_prices_string() << "\","
              << "\"" << quote.bid_qtys_string() << "\","
              << "\"" << quote.ask_prices_string() << "\","
              << "\"" << quote.ask_qtys_string() << "\","
              << quote.segment << "\n";
        
        count_++;
    }
    
    void flush_remaining() {
        std::lock_guard<std::mutex> lock(queue_mutex_);
        while (!queue_.empty()) {
            write_quote(queue_.front());
            queue_.pop();
        }
        file_.flush();
    }
    
    CSVWriterConfig config_;
    std::string feed_type_;
    std::string file_path_;
    std::ofstream file_;
    std::vector<char> buffer_;
    size_t count_;
    
    // Async queue
    std::queue<domain::Quote> queue_;
    std::mutex queue_mutex_;
    std::condition_variable queue_cv_;
    std::thread writer_thread_;
    std::atomic<bool> running_;
    size_t queue_dropped_;
};

} // namespace saver
} // namespace bse
