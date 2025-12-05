/**
 * @file multicast.hpp
 * @brief UDP Multicast Receiver for BSE Feeds
 */

#pragma once

#include <cstdint>
#include <string>
#include <functional>
#include <atomic>
#include <thread>
#include <chrono>

#ifdef _WIN32
    #include <winsock2.h>
    #include <ws2tcpip.h>
    #pragma comment(lib, "ws2_32.lib")
    using socket_t = SOCKET;
    #define INVALID_SOCK INVALID_SOCKET
    #define SOCKET_ERROR_VAL SOCKET_ERROR
    #define CLOSE_SOCKET closesocket
#else
    #include <sys/socket.h>
    #include <netinet/in.h>
    #include <arpa/inet.h>
    #include <unistd.h>
    #include <fcntl.h>
    #include <errno.h>
    using socket_t = int;
    #define INVALID_SOCK (-1)
    #define SOCKET_ERROR_VAL (-1)
    #define CLOSE_SOCKET close
#endif

namespace bse {
namespace receiver {

/**
 * @brief Receiver configuration
 */
struct ReceiverConfig {
    std::string multicast_ip = "239.1.2.5";
    int port = 26001;
    int buffer_size = 2048;
    int socket_rcv_buf = 32 * 1024 * 1024;  // 32MB
    int read_timeout_ms = 50;                // Timeout for responsive shutdown
};

using StopCheck = std::function<bool()>;

/**
 * @brief Packet handler callback
 */
using PacketHandler = std::function<void(const uint8_t* data, uint32_t length)>;

/**
 * @brief UDP Multicast Receiver
 */
class MulticastReceiver {
public:
    MulticastReceiver(const ReceiverConfig& config, PacketHandler handler)
        : config_(config)
        , handler_(std::move(handler))
        , socket_(INVALID_SOCK)
        , running_(false)
        , packets_received_(0)
        , bytes_received_(0)
        , errors_(0) {
    }
    
    ~MulticastReceiver() {
        close();
    }
    
    /**
     * @brief Connect and join multicast group
     */
    bool connect() {
#ifdef _WIN32
        // Initialize Winsock
        WSADATA wsa_data;
        if (WSAStartup(MAKEWORD(2, 2), &wsa_data) != 0) {
            return false;
        }
#endif
        
        // Create UDP socket
        socket_ = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
        if (socket_ == INVALID_SOCK) {
            return false;
        }
        
        // Allow address reuse
        int reuse = 1;
        setsockopt(socket_, SOL_SOCKET, SO_REUSEADDR, 
                   reinterpret_cast<const char*>(&reuse), sizeof(reuse));
        
        // Set receive buffer size
        int rcv_buf = config_.socket_rcv_buf;
        setsockopt(socket_, SOL_SOCKET, SO_RCVBUF,
                   reinterpret_cast<const char*>(&rcv_buf), sizeof(rcv_buf));
        
        // Set receive timeout
#ifdef _WIN32
        DWORD timeout = config_.read_timeout_ms;
        setsockopt(socket_, SOL_SOCKET, SO_RCVTIMEO,
                   reinterpret_cast<const char*>(&timeout), sizeof(timeout));
#else
        struct timeval tv;
        tv.tv_sec = config_.read_timeout_ms / 1000;
        tv.tv_usec = (config_.read_timeout_ms % 1000) * 1000;
        setsockopt(socket_, SOL_SOCKET, SO_RCVTIMEO,
                   reinterpret_cast<const char*>(&tv), sizeof(tv));
#endif
        
        // Bind to port
        struct sockaddr_in local_addr{};
        local_addr.sin_family = AF_INET;
        local_addr.sin_port = htons(static_cast<uint16_t>(config_.port));
        local_addr.sin_addr.s_addr = htonl(INADDR_ANY);
        
        if (bind(socket_, reinterpret_cast<struct sockaddr*>(&local_addr), 
                 sizeof(local_addr)) == SOCKET_ERROR_VAL) {
            close();
            return false;
        }
        
        // Join multicast group
        struct ip_mreq mreq{};
        mreq.imr_multiaddr.s_addr = inet_addr(config_.multicast_ip.c_str());
        mreq.imr_interface.s_addr = htonl(INADDR_ANY);
        
        if (setsockopt(socket_, IPPROTO_IP, IP_ADD_MEMBERSHIP,
                       reinterpret_cast<const char*>(&mreq), sizeof(mreq)) == SOCKET_ERROR_VAL) {
            close();
            return false;
        }
        
        return true;
    }
    
    /**
     * @brief Start receive loop (blocking)
     * @param external_stop Optional external stop check function for responsive shutdown
     */
    void receive_loop(StopCheck external_stop = nullptr) {
        if (socket_ == INVALID_SOCK) return;
        
        running_ = true;
        uint8_t buffer[2048];
        
        while (running_) {
            // Check external stop condition for responsive shutdown
            if (external_stop && external_stop()) {
                running_ = false;
                break;
            }
            
            int n = recv(socket_, reinterpret_cast<char*>(buffer), sizeof(buffer), 0);
            
            if (n > 0) {
                packets_received_++;
                bytes_received_ += n;
                
                if (handler_) {
                    handler_(buffer, static_cast<uint32_t>(n));
                }
            } else if (n == 0) {
                // Connection closed
                break;
            } else {
                // Error or timeout - check stop condition on timeout
#ifdef _WIN32
                int err = WSAGetLastError();
                if (err == WSAETIMEDOUT) {
                    // Timeout - check for shutdown
                    if (external_stop && external_stop()) {
                        running_ = false;
                        break;
                    }
                } else {
                    errors_++;
                }
#else
                if (errno == EAGAIN || errno == EWOULDBLOCK) {
                    // Timeout - check for shutdown
                    if (external_stop && external_stop()) {
                        running_ = false;
                        break;
                    }
                } else {
                    errors_++;
                }
#endif
            }
        }
    }
    
    /**
     * @brief Stop receive loop
     */
    void stop() {
        running_ = false;
    }
    
    /**
     * @brief Close socket
     */
    void close() {
        running_ = false;
        if (socket_ != INVALID_SOCK) {
            CLOSE_SOCKET(socket_);
            socket_ = INVALID_SOCK;
        }
#ifdef _WIN32
        WSACleanup();
#endif
    }
    
    // Statistics
    uint64_t packets_received() const { return packets_received_; }
    uint64_t bytes_received() const { return bytes_received_; }
    uint64_t errors() const { return errors_; }
    bool is_running() const { return running_; }

private:
    ReceiverConfig config_;
    PacketHandler handler_;
    socket_t socket_;
    std::atomic<bool> running_;
    
    // Statistics
    uint64_t packets_received_;
    uint64_t bytes_received_;
    uint64_t errors_;
};

} // namespace receiver
} // namespace bse
