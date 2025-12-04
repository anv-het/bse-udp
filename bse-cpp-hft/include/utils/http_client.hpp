/**
 * @file http_client.hpp
 * @brief HTTP client using WinHTTP for Windows
 */

#pragma once

#ifdef _WIN32
#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <windows.h>
#include <winhttp.h>
#pragma comment(lib, "winhttp.lib")
#endif

#include <string>
#include <vector>
#include <stdexcept>
#include <sstream>
#include <iostream>

namespace bse {
namespace utils {

/**
 * @brief HTTP Response structure
 */
struct HttpResponse {
    int status_code = 0;
    std::string body;
    bool success = false;
    std::string error;
};

/**
 * @brief Simple HTTP client using WinHTTP
 */
class HttpClient {
public:
    HttpClient() : timeout_ms_(30000) {}
    
    void set_timeout(int timeout_ms) {
        timeout_ms_ = timeout_ms;
    }
    
    /**
     * @brief POST JSON request
     */
    HttpResponse post_json(const std::string& url, const std::string& json_body) {
        HttpResponse response;
        
#ifdef _WIN32
        // Parse URL
        std::string host, path;
        int port = 443;
        bool use_ssl = true;
        
        if (!parse_url(url, host, path, port, use_ssl)) {
            response.error = "Invalid URL: " + url;
            return response;
        }
        
        // Initialize WinHTTP
        HINTERNET hSession = WinHttpOpen(
            L"BSE-HFT-CPP/1.0",
            WINHTTP_ACCESS_TYPE_DEFAULT_PROXY,
            WINHTTP_NO_PROXY_NAME,
            WINHTTP_NO_PROXY_BYPASS,
            0
        );
        
        if (!hSession) {
            response.error = "WinHttpOpen failed: " + std::to_string(GetLastError());
            return response;
        }
        
        // Set timeouts
        WinHttpSetTimeouts(hSession, timeout_ms_, timeout_ms_, timeout_ms_, timeout_ms_);
        
        // Connect to host
        std::wstring whost = to_wide(host);
        HINTERNET hConnect = WinHttpConnect(hSession, whost.c_str(), port, 0);
        
        if (!hConnect) {
            response.error = "WinHttpConnect failed: " + std::to_string(GetLastError());
            WinHttpCloseHandle(hSession);
            return response;
        }
        
        // Create request
        std::wstring wpath = to_wide(path);
        DWORD flags = use_ssl ? WINHTTP_FLAG_SECURE : 0;
        
        HINTERNET hRequest = WinHttpOpenRequest(
            hConnect,
            L"POST",
            wpath.c_str(),
            NULL,
            WINHTTP_NO_REFERER,
            WINHTTP_DEFAULT_ACCEPT_TYPES,
            flags
        );
        
        if (!hRequest) {
            response.error = "WinHttpOpenRequest failed: " + std::to_string(GetLastError());
            WinHttpCloseHandle(hConnect);
            WinHttpCloseHandle(hSession);
            return response;
        }
        
        // Set headers
        const wchar_t* headers = L"Content-Type: application/json\r\n";
        WinHttpAddRequestHeaders(hRequest, headers, -1, WINHTTP_ADDREQ_FLAG_ADD);
        
        // Send request
        BOOL result = WinHttpSendRequest(
            hRequest,
            WINHTTP_NO_ADDITIONAL_HEADERS,
            0,
            (LPVOID)json_body.c_str(),
            static_cast<DWORD>(json_body.length()),
            static_cast<DWORD>(json_body.length()),
            0
        );
        
        if (!result) {
            response.error = "WinHttpSendRequest failed: " + std::to_string(GetLastError());
            WinHttpCloseHandle(hRequest);
            WinHttpCloseHandle(hConnect);
            WinHttpCloseHandle(hSession);
            return response;
        }
        
        // Receive response
        result = WinHttpReceiveResponse(hRequest, NULL);
        
        if (!result) {
            response.error = "WinHttpReceiveResponse failed: " + std::to_string(GetLastError());
            WinHttpCloseHandle(hRequest);
            WinHttpCloseHandle(hConnect);
            WinHttpCloseHandle(hSession);
            return response;
        }
        
        // Get status code
        DWORD status_code = 0;
        DWORD size = sizeof(status_code);
        WinHttpQueryHeaders(
            hRequest,
            WINHTTP_QUERY_STATUS_CODE | WINHTTP_QUERY_FLAG_NUMBER,
            WINHTTP_HEADER_NAME_BY_INDEX,
            &status_code,
            &size,
            WINHTTP_NO_HEADER_INDEX
        );
        response.status_code = static_cast<int>(status_code);
        
        // Read body
        std::string body;
        DWORD bytes_available = 0;
        
        do {
            bytes_available = 0;
            if (!WinHttpQueryDataAvailable(hRequest, &bytes_available)) break;
            if (bytes_available == 0) break;
            
            std::vector<char> buffer(bytes_available + 1);
            DWORD bytes_read = 0;
            
            if (WinHttpReadData(hRequest, buffer.data(), bytes_available, &bytes_read)) {
                body.append(buffer.data(), bytes_read);
            }
        } while (bytes_available > 0);
        
        response.body = body;
        response.success = (status_code == 200);
        
        // Cleanup
        WinHttpCloseHandle(hRequest);
        WinHttpCloseHandle(hConnect);
        WinHttpCloseHandle(hSession);
#else
        response.error = "HTTP client not implemented for this platform";
#endif
        
        return response;
    }

private:
    int timeout_ms_;
    
    /**
     * @brief Parse URL into components
     */
    bool parse_url(const std::string& url, std::string& host, std::string& path, 
                   int& port, bool& use_ssl) {
        std::string remaining = url;
        
        // Check protocol
        if (remaining.find("https://") == 0) {
            use_ssl = true;
            port = 443;
            remaining = remaining.substr(8);
        } else if (remaining.find("http://") == 0) {
            use_ssl = false;
            port = 80;
            remaining = remaining.substr(7);
        } else {
            return false;
        }
        
        // Find path
        size_t path_pos = remaining.find('/');
        if (path_pos != std::string::npos) {
            host = remaining.substr(0, path_pos);
            path = remaining.substr(path_pos);
        } else {
            host = remaining;
            path = "/";
        }
        
        // Check for port in host
        size_t port_pos = host.find(':');
        if (port_pos != std::string::npos) {
            port = std::stoi(host.substr(port_pos + 1));
            host = host.substr(0, port_pos);
        }
        
        return !host.empty();
    }
    
#ifdef _WIN32
    /**
     * @brief Convert string to wide string
     */
    std::wstring to_wide(const std::string& str) {
        if (str.empty()) return L"";
        int size = MultiByteToWideChar(CP_UTF8, 0, str.c_str(), -1, NULL, 0);
        std::wstring wstr(size, 0);
        MultiByteToWideChar(CP_UTF8, 0, str.c_str(), -1, &wstr[0], size);
        return wstr;
    }
#endif
};

} // namespace utils
} // namespace bse
