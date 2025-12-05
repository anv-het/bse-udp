 /**
 * BSE HFT Server - Ultra-Low Latency Multicast Receiver
 * C++17 High-Frequency Trading
 */

// Prevent Windows min/max macro conflicts
#ifdef _WIN32
#ifndef NOMINMAX
#define NOMINMAX
#endif
#endif

#include <iostream>
#include <iomanip>
#include <string>
#include <sstream>
#include <thread>
#include <atomic>
#include <chrono>
#include <csignal>
#include <ctime>
#include <vector>
#include <map>
#include <mutex>
#include <algorithm>
#include <random>
#include <cmath>
#include <memory>

#include "config/config.hpp"
#include "receiver/multicast.hpp"
#include "decoder/decoder.hpp"
#include "tokens/token_manager.hpp"
#include "buffer/ring_buffer.hpp"
#include "saver/csv_writer.hpp"
#include "domain/quote.hpp"
#include "domain/contract.hpp"
#include "stats/stats.hpp"
#include "utils/time.hpp"

using namespace std::chrono;
using namespace bse;

std::atomic<bool> g_running{true};

#ifdef _WIN32
BOOL WINAPI console_ctrl_handler(DWORD ctrl_type) {
    if (ctrl_type == CTRL_C_EVENT || ctrl_type == CTRL_BREAK_EVENT) {
        g_running.store(false);
        return TRUE;
    }
    return FALSE;
}
#endif

struct CommandLineArgs {
    std::string config_path = "config.json";
    int duration_seconds = 0;
    bool eq_only = false;
    bool fo_only = false;
    bool help = false;
    
    static int parse_duration(const std::string& s) {
        if (s.empty()) return 0;
        size_t pos = 0;
        int value = std::stoi(s, &pos);
        if (pos < s.length()) {
            char unit = s[pos];
            switch (unit) {
                case 's': return value;
                case 'm': return value * 60;
                case 'h': return value * 3600;
                default: return value;
            }
        }
        return value;
    }
    
    static CommandLineArgs parse(int argc, char* argv[]) {
        CommandLineArgs args;
        for (int i = 1; i < argc; i++) {
            std::string arg = argv[i];
            if (arg == "-help" || arg == "--help" || arg == "-h") args.help = true;
            else if (arg == "-config" && i + 1 < argc) args.config_path = argv[++i];
            else if (arg == "-duration" && i + 1 < argc) args.duration_seconds = parse_duration(argv[++i]);
            else if (arg == "-eq-only") args.eq_only = true;
            else if (arg == "-fo-only") args.fo_only = true;
        }
        return args;
    }
    
    static void print_usage(const char* prog) {
        std::cout << "Usage: " << prog << " [options]\n"
                  << "  -config <path>   Config file (default: config.json)\n"
                  << "  -duration <time> Run duration (e.g., 30s, 5m, 1h)\n"
                  << "  -eq-only         Equity feed only\n"
                  << "  -fo-only         F&O feed only\n"
                  << "  -help            Show this help\n";
    }
};

// Use the optimized LatencyTracker from metrics namespace
using LatencyTracker = metrics::LatencyTracker;

// Global comprehensive stats tracker
stats::Tracker g_stats;

struct Tracker {
    std::string feed_name;
    std::atomic<uint64_t> packets{0}, records{0}, quotes{0};
    std::atomic<uint64_t> drops{0}, bytes{0}, missed_tokens{0}, invalid_packets{0};
    LatencyTracker latency;  // Now uses bse::metrics::LatencyTracker
    system_clock::time_point start_time;
    std::mutex missed_mutex;
    std::map<uint32_t, uint64_t> missed_token_counts;
    
    Tracker(const std::string& name = "") : feed_name(name) { start_time = system_clock::now(); }
    void record_missed_token(uint32_t token) {
        missed_tokens++;
        std::lock_guard<std::mutex> lock(missed_mutex);
        missed_token_counts[token]++;
    }
    double elapsed_seconds() const {
        return duration_cast<duration<double>>(system_clock::now() - start_time).count();
    }
};

std::string format_number(uint64_t n) {
    std::string s = std::to_string(n);
    int pos = (int)s.length() - 3;
    while (pos > 0) { s.insert(pos, ","); pos -= 3; }
    return s;
}

std::string format_bytes(uint64_t bytes) {
    const char* units[] = {"B", "KB", "MB", "GB", "TB"};
    int unit = 0;
    double size = (double)bytes;
    while (size >= 1024 && unit < 4) { size /= 1024; unit++; }
    std::ostringstream oss;
    oss << std::fixed << std::setprecision(2) << size << " " << units[unit];
    return oss.str();
}

std::string format_rate(double rate) {
    std::ostringstream oss;
    if (rate >= 1000000) oss << std::fixed << std::setprecision(2) << (rate / 1000000) << "M";
    else if (rate >= 1000) oss << std::fixed << std::setprecision(2) << (rate / 1000) << "K";
    else oss << std::fixed << std::setprecision(0) << rate;
    return oss.str();
}

std::string format_duration(double seconds) {
    int h = (int)seconds / 3600;
    int m = ((int)seconds % 3600) / 60;
    int s = (int)seconds % 60;
    std::ostringstream oss;
    if (h > 0) oss << h << "h " << m << "m " << s << "s";
    else if (m > 0) oss << m << "m " << s << "s";
    else oss << std::fixed << std::setprecision(1) << seconds << "s";
    return oss.str();
}

void print_banner() {
    std::cout << "\n+==============================================================================+\n"
              << "|    BSE HFT C++ - Ultra-Low Latency Multicast Receiver                        |\n"
              << "+==============================================================================+\n\n";
}

void print_feed_stats(const std::string& name, Tracker& tracker) {
    double elapsed = tracker.elapsed_seconds();
    auto lat_stats = tracker.latency.get_stats();
    auto lat_pct = tracker.latency.get_percentiles();
    double pkt_rate = elapsed > 0 ? tracker.packets / elapsed : 0;
    double rec_rate = elapsed > 0 ? tracker.records / elapsed : 0;
    double byte_rate = elapsed > 0 ? tracker.bytes / elapsed : 0;
    
    std::cout << "|  [" << name << " Feed Statistics]" << std::string(60 - name.length(), ' ') << "|\n"
              << "|  Packets: " << std::setw(12) << format_number(tracker.packets)
              << "  Records: " << std::setw(12) << format_number(tracker.records)
              << "  Quotes: " << std::setw(10) << format_number(tracker.quotes) << " |\n"
              << "|  Pkt/s: " << std::setw(10) << format_rate(pkt_rate)
              << "  Rec/s: " << std::setw(12) << format_rate(rec_rate)
              << "  Throughput: " << std::setw(12) << format_bytes((uint64_t)byte_rate) << "/s |\n"
              << "|  Drops: " << std::setw(10) << format_number(tracker.drops)
              << "  Missed: " << std::setw(10) << format_number(tracker.missed_tokens)
              << "  Invalid: " << std::setw(10) << format_number(tracker.invalid_packets) << " |\n";
    
    if (lat_stats.count > 0) {
        std::cout << "|  Latency: Min=" << std::fixed << std::setprecision(1) << lat_stats.min
                  << "us  Mean=" << lat_stats.avg << "us  Max=" << lat_stats.max << "us" << std::string(25, ' ') << "|\n"
                  << "|  Percentiles: P50=" << lat_pct.p50 << "us  P90=" << lat_pct.p90
                  << "us  P99=" << lat_pct.p99 << "us  P99.9=" << lat_pct.p999 << "us |\n";
    }
}

void print_final_report(Tracker& eq_tracker, Tracker& fo_tracker, bool eq_enabled, bool fo_enabled) {
    auto now = system_clock::now();
    auto time_t_now = system_clock::to_time_t(now);
    std::tm tm_now;
#ifdef _WIN32
    localtime_s(&tm_now, &time_t_now);
#else
    localtime_r(&time_t_now, &tm_now);
#endif
    char time_buf[32];
    std::strftime(time_buf, sizeof(time_buf), "%Y-%m-%d %H:%M:%S", &tm_now);
    
    uint64_t total_packets = 0, total_records = 0, total_quotes = 0;
    uint64_t total_drops = 0, total_bytes = 0, total_missed = 0;
    double max_elapsed = 0;
    
    if (eq_enabled) {
        total_packets += eq_tracker.packets;
        total_records += eq_tracker.records;
        total_quotes += eq_tracker.quotes;
        total_drops += eq_tracker.drops;
        total_bytes += eq_tracker.bytes;
        total_missed += eq_tracker.missed_tokens;
        max_elapsed = std::max(max_elapsed, eq_tracker.elapsed_seconds());
    }
    if (fo_enabled) {
        total_packets += fo_tracker.packets;
        total_records += fo_tracker.records;
        total_quotes += fo_tracker.quotes;
        total_drops += fo_tracker.drops;
        total_bytes += fo_tracker.bytes;
        total_missed += fo_tracker.missed_tokens;
        max_elapsed = std::max(max_elapsed, fo_tracker.elapsed_seconds());
    }
    
    std::cout << "\n+==============================================================================+\n"
              << "|                     BSE HFT BENCHMARK REPORT                                 |\n"
              << "+==============================================================================+\n"
              << "|  Timestamp: " << time_buf << std::string(48, ' ') << "|\n"
              << "|  Duration: " << std::setw(10) << format_duration(max_elapsed)
              << "    Data: " << std::setw(15) << format_bytes(total_bytes) << std::string(28, ' ') << "|\n"
              << "+------------------------------------------------------------------------------+\n";
    
    if (eq_enabled) print_feed_stats("Equity", eq_tracker);
    if (fo_enabled) print_feed_stats("F&O", fo_tracker);
    
    std::cout << "+------------------------------------------------------------------------------+\n"
              << "|  SUMMARY                                                                     |\n"
              << "|  Total Packets: " << std::setw(12) << format_number(total_packets)
              << "  Total Records: " << std::setw(12) << format_number(total_records) << std::string(18, ' ') << "|\n"
              << "|  Total Quotes: " << std::setw(13) << format_number(total_quotes)
              << "  Total Drops: " << std::setw(13) << format_number(total_drops) << std::string(18, ' ') << "|\n";
    
    double drop_rate = total_packets > 0 ? (100.0 * total_drops / total_packets) : 0;
    std::string rating;
    if (drop_rate == 0) rating = "PERFECT - Zero packet drops!";
    else if (drop_rate < 0.001) rating = "EXCELLENT - Sub-millisecond grade";
    else if (drop_rate < 0.01) rating = "GOOD - Production ready";
    else rating = "NEEDS IMPROVEMENT";
    
    std::cout << "|  Drop Rate: " << std::fixed << std::setprecision(4) << drop_rate << "%" << std::string(52, ' ') << "|\n"
              << "|  Rating: " << rating << std::string(68 - rating.length(), ' ') << "|\n"
              << "+==============================================================================+\n\n";
}

void print_live_stats(Tracker& eq_tracker, Tracker& fo_tracker, bool eq_enabled, bool fo_enabled) {
    std::cout << "\r[";
    if (eq_enabled) {
        double elapsed = eq_tracker.elapsed_seconds();
        double rate = elapsed > 0 ? eq_tracker.packets / elapsed : 0;
        std::cout << "EQ: " << format_number(eq_tracker.packets) << " pkts, " << format_rate(rate) << "/s";
        if (eq_tracker.drops > 0) std::cout << ", " << eq_tracker.drops << " drops";
    }
    if (eq_enabled && fo_enabled) std::cout << " | ";
    if (fo_enabled) {
        double elapsed = fo_tracker.elapsed_seconds();
        double rate = elapsed > 0 ? fo_tracker.packets / elapsed : 0;
        std::cout << "FO: " << format_number(fo_tracker.packets) << " pkts, " << format_rate(rate) << "/s";
        if (fo_tracker.drops > 0) std::cout << ", " << fo_tracker.drops << " drops";
    }
    std::cout << "]        " << std::flush;
}

class FeedProcessor {
public:
    FeedProcessor(const config::MulticastConfig& mc_config, domain::TokenMap& token_map,
                  Tracker& tracker, const std::string& segment, const std::string& output_dir)
        : mc_config_(mc_config), token_map_(token_map), tracker_(tracker),
          segment_(segment), running_(true), ring_buffer_(16384), decoder_(token_map),
          csv_writer_(output_dir, segment) {}
    
    void start() {
        receiver_thread_ = std::thread(&FeedProcessor::receive_loop, this);
        processor_thread_ = std::thread(&FeedProcessor::process_loop, this);
    }
    
    void stop() {
        running_.store(false);
        // Signal receiver to stop its internal loop
        if (receiver_) {
            receiver_->stop();
        }
        if (receiver_thread_.joinable()) receiver_thread_.join();
        if (processor_thread_.joinable()) processor_thread_.join();
    }
    
    size_t csv_count() const { return csv_writer_.count(); }
    const std::string& csv_file() const { return csv_writer_.file_path(); }
    
private:
    void receive_loop() {
        receiver::ReceiverConfig recv_config;
        recv_config.multicast_ip = mc_config_.ip;
        recv_config.port = mc_config_.port;
        recv_config.buffer_size = mc_config_.buffer_size;
        recv_config.socket_rcv_buf = mc_config_.socket_rcv_buf;
        recv_config.read_timeout_ms = 50;  // Short timeout for responsive shutdown
        
        // Create receiver with callback
        receiver_ = std::make_unique<receiver::MulticastReceiver>(recv_config, 
            [this](const uint8_t* data, uint32_t length) {
                if (!ring_buffer_.try_push(data, length)) tracker_.drops++;
                else tracker_.bytes += length;
            });
        
        if (!receiver_->connect()) {
            std::cerr << "[" << segment_ << "] Failed to connect\n";
            return;
        }
        std::cout << "[" << segment_ << "] Connected to " << mc_config_.ip << ":" << mc_config_.port << "\n";
        
        // Run receive loop with external stop check for responsive Ctrl+C shutdown
        // The stop check is evaluated on every timeout (50ms), ensuring quick exit
        receiver_->receive_loop([this]() { 
            return !running_.load() || !g_running.load(); 
        });
        
        receiver_->stop();
        receiver_->close();
    }
    
    void process_loop() {
        std::vector<domain::Quote> quotes;
        quotes.reserve(100);
        uint8_t buffer[2048];
        uint32_t length;
        
        while (running_ && g_running) {
            if (!ring_buffer_.try_pop(buffer, &length)) {
                std::this_thread::sleep_for(microseconds(10));
                continue;
            }
            tracker_.packets++;
            int64_t now = utils::now_ns();
            quotes.clear();
            int decoded = decoder_.decode_packet(buffer, length, quotes, segment_);
            
            // Update global stats tracker
            g_stats.record_packet(segment_, length, decoded);
            
            if (decoded > 0) {
                tracker_.records += decoded;
                for (const auto& quote : quotes) {
                    if (token_map_.contains(quote.token)) {
                        tracker_.quotes++;
                        int64_t latency_ns = utils::now_ns() - now;
                        tracker_.latency.record(latency_ns);
                        
                        // Record in global stats
                        g_stats.record_quote(segment_);
                        g_stats.record_decode_latency(latency_ns);
                        g_stats.record_process_latency(latency_ns);
                        
                        // Save quote to CSV
                        csv_writer_.save(quote);
                    } else {
                        tracker_.record_missed_token(quote.token);
                        g_stats.track_missed_token(quote.token);
                    }
                }
            } else {
                tracker_.invalid_packets++;
            }
        }
        
        // Update ring drops and CSV stats in global tracker
        g_stats.record_ring_drops(segment_, tracker_.drops.load());
        g_stats.record_csv_writes(segment_, csv_writer_.count());
    }
    
    const config::MulticastConfig& mc_config_;
    domain::TokenMap& token_map_;
    Tracker& tracker_;
    std::string segment_;
    std::atomic<bool> running_;
    buffer::RingBuffer ring_buffer_;
    decoder::Decoder decoder_;
    saver::CSVWriter csv_writer_;
    std::unique_ptr<receiver::MulticastReceiver> receiver_;
    std::thread receiver_thread_;
    std::thread processor_thread_;
};

int main(int argc, char* argv[]) {
    auto args = CommandLineArgs::parse(argc, argv);
    if (args.help) { CommandLineArgs::print_usage(argv[0]); return 0; }
    
#ifdef _WIN32
    SetConsoleCtrlHandler(console_ctrl_handler, TRUE);
    // Enable UTF-8 console output for Unicode box-drawing characters
    SetConsoleOutputCP(65001);  // UTF-8 code page
#else
    std::signal(SIGINT, [](int) { g_running.store(false); });
#endif
    
    print_banner();
    std::cout << "Loading config from: " << args.config_path << "\n";
    
    config::Config cfg;
    try { cfg = config::Config::load(args.config_path); }
    catch (const std::exception& e) {
        std::cerr << "Failed to load config: " << e.what() << "\nUsing defaults\n";
    }
    
    bool eq_enabled = !args.fo_only && cfg.segments.cm_enabled;
    bool fo_enabled = !args.eq_only && cfg.segments.fo_enabled;
    
    std::cout << "Feed Mode: ";
    if (eq_enabled && fo_enabled) std::cout << "Dual (Equity + F&O)\n";
    else if (eq_enabled) std::cout << "Equity Only\n";
    else std::cout << "F&O Only\n";
    
    if (args.duration_seconds > 0) std::cout << "Duration: " << format_duration(args.duration_seconds) << "\n";
    else std::cout << "Duration: Unlimited (Ctrl+C to stop)\n";
    
    std::cout << "\nLoading token files...\n";
    domain::TokenMap token_map;
    tokens::TokenManager token_mgr(cfg.data_management.tokens_dir, cfg.api.base_url);
    token_mgr.load_bhavcopy(token_map);
    token_mgr.load_contract_master(token_map);
    std::cout << "Total tokens loaded: " << token_map.size() << "\n";
    
    Tracker eq_tracker("Equity");
    Tracker fo_tracker("F&O");
    
    std::cout << "\nStarting feeds...\n\n";
    std::unique_ptr<FeedProcessor> eq_feed, fo_feed;
    std::string output_dir = cfg.data_management.output_dir;
    
    if (eq_enabled) {
        eq_feed = std::make_unique<FeedProcessor>(cfg.multicast_cm, token_map, eq_tracker, "EQ", output_dir);
        eq_feed->start();
    }
    if (fo_enabled) {
        fo_feed = std::make_unique<FeedProcessor>(cfg.multicast_fo, token_map, fo_tracker, "FO", output_dir);
        fo_feed->start();
    }
    
    auto start = steady_clock::now();
    auto last_print = start;
    
    while (g_running) {
        std::this_thread::sleep_for(milliseconds(100));
        if (args.duration_seconds > 0) {
            auto elapsed = duration_cast<seconds>(steady_clock::now() - start).count();
            if (elapsed >= args.duration_seconds) {
                std::cout << "\n\nDuration reached, shutting down...\n";
                break;
            }
        }
        auto now = steady_clock::now();
        if (duration_cast<seconds>(now - last_print).count() >= 1) {
            print_live_stats(eq_tracker, fo_tracker, eq_enabled, fo_enabled);
            last_print = now;
        }
    }
    
    std::cout << "\n\nStopping feeds...\n";
    
    // Get CSV file info before stopping
    std::string eq_csv_file, fo_csv_file;
    size_t eq_csv_count = 0, fo_csv_count = 0;
    if (eq_feed) {
        eq_csv_file = eq_feed->csv_file();
        eq_csv_count = eq_feed->csv_count();
        g_stats.set_output_file("EQ", eq_csv_file);
    }
    if (fo_feed) {
        fo_csv_file = fo_feed->csv_file();
        fo_csv_count = fo_feed->csv_count();
        g_stats.set_output_file("FO", fo_csv_file);
    }
    
    if (eq_feed) eq_feed->stop();
    if (fo_feed) fo_feed->stop();
    
    // Print quick summary first
    print_final_report(eq_tracker, fo_tracker, eq_enabled, fo_enabled);
    
    // Now print the comprehensive beautiful report
    g_stats.print_final_report(token_map.size());
    
    return 0;
}
