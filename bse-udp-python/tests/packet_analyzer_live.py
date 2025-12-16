#!/usr/bin/env python3
"""
BSE UDP Packet Analyzer - Live Message Type Statistics
========================================================

Connects to BOTH BSE UDP feeds and analyzes:
- Message types received (2001, 2002, 2020, 2021, 2011, 2012, etc.)
- Packet counts per message type
- Data rates and bandwidth usage
- Beautiful live terminal output

Based on BSE_DIRECT_NFCAST_Manual.txt documentation

USAGE:
    python packet_analyzer_live.py --duration 60

    Options:
        --duration SECONDS    How long to capture (default: 60)
        --fo-ip IP           F&O multicast IP (default: 239.1.2.5)
        --fo-port PORT       F&O UDP port (default: 26002)
        --index-ip IP        Index multicast IP (default: 239.1.2.5)
        --index-port PORT    Index UDP port (default: 26001)
"""

import socket
import struct
import time
import sys
import argparse
from datetime import datetime, timedelta
from collections import defaultdict
from threading import Thread, Lock
import os

# ================================================================================
# MESSAGE TYPE MAPPINGS (from BSE NFCAST Manual)
# ================================================================================

MESSAGE_TYPES = {
    # Service Messages
    2001: "Time Broadcast",
    2030: "Auction Keep Alive",
    
    # Market Data Messages
    2002: "Product State Change",
    2003: "Shortage Auction Session Change",
    2004: "News Headline",
    2020: "Market Picture (Normal)",
    2021: "Market Picture (Complex)",
    2017: "Auction Market Picture",
    2027: "Odd-lot Market Picture",
    2033: "Debt Market Picture",
    2011: "Index Change (Normal)",
    2012: "Index Change (Extended)",
    2034: "Limit Price Protection Range",
    2035: "Call Auction Cancelled Quantity",
    2014: "Close Price",
    2015: "Open Interest",
    2016: "VaR Percentage",
    2022: "RBI Reference Rate",
    2028: "Implied Volatility",
}

# ================================================================================
# PACKET STATISTICS
# ================================================================================

class PacketStats:
    def __init__(self):
        self.lock = Lock()
        self.message_counts = defaultdict(int)
        self.total_packets = 0
        self.total_bytes = 0
        self.start_time = time.time()
        self.feed_name = ""
        
    def add_packet(self, message_type, packet_size):
        with self.lock:
            self.message_counts[message_type] += 1
            self.total_packets += 1
            self.total_bytes += packet_size
    
    def get_summary(self):
        with self.lock:
            elapsed = time.time() - self.start_time
            return {
                'total_packets': self.total_packets,
                'total_bytes': self.total_bytes,
                'elapsed': elapsed,
                'message_counts': dict(self.message_counts),
                'packets_per_sec': self.total_packets / elapsed if elapsed > 0 else 0,
                'bytes_per_sec': self.total_bytes / elapsed if elapsed > 0 else 0,
            }

# ================================================================================
# UDP MULTICAST LISTENER
# ================================================================================

class UDPListener(Thread):
    def __init__(self, ip, port, feed_name, stats):
        super().__init__(daemon=True)
        self.ip = ip
        self.port = port
        self.feed_name = feed_name
        self.stats = stats
        self.stats.feed_name = feed_name
        self.running = False
        self.sock = None
        
    def setup_socket(self):
        """Setup UDP multicast socket (same as BSE Python reader)"""
        try:
            # Create UDP socket
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
            sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            
            # Bind to the port
            sock.bind(('', self.port))
            
            # Join multicast group
            mreq = struct.pack("4sl", socket.inet_aton(self.ip), socket.INADDR_ANY)
            sock.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP, mreq)
            
            # Set socket timeout for graceful shutdown
            sock.settimeout(1.0)
            
            self.sock = sock
            return True
            
        except Exception as e:
            print(f"❌ {self.feed_name}: Socket setup failed: {e}")
            return False
    
    def run(self):
        """Main packet capture loop"""
        if not self.setup_socket():
            return
        
        print(f"✅ {self.feed_name}: Connected to {self.ip}:{self.port}")
        self.running = True
        
        while self.running:
            try:
                # Receive packet (max 2000 bytes as per BSE recommendation)
                packet, addr = self.sock.recvfrom(2000)
                packet_size = len(packet)
                
                if packet_size < 10:
                    continue  # Too small to have message type
                
                # Message Type at offset 8:10 (2 bytes, Little-Endian)
                # Based on BSE HFT decoder: offset 8-10, Little-Endian
                message_type = struct.unpack('<H', packet[8:10])[0]
                
                # Record packet
                self.stats.add_packet(message_type, packet_size)
                
            except socket.timeout:
                continue  # Normal timeout, check if still running
            except Exception as e:
                if self.running:  # Only log if not shutting down
                    print(f"⚠️ {self.feed_name}: Error: {e}")
        
        # Cleanup
        if self.sock:
            self.sock.close()
        print(f"🛑 {self.feed_name}: Stopped")
    
    def stop(self):
        """Stop the listener"""
        self.running = False

# ================================================================================
# TERMINAL DISPLAY
# ================================================================================

def clear_screen():
    """Clear terminal screen"""
    os.system('cls' if os.name == 'nt' else 'clear')

def format_bytes(bytes):
    """Format bytes to human-readable"""
    for unit in ['B', 'KB', 'MB', 'GB']:
        if bytes < 1024.0:
            return f"{bytes:.2f} {unit}"
        bytes /= 1024.0
    return f"{bytes:.2f} TB"

def print_stats_table(fo_stats, index_stats, elapsed, duration, show_all=True):
    """Print beautiful statistics table with ALL message types"""
    clear_screen()
    
    # Header
    print("=" * 100)
    print("🔍 BSE UDP PACKET ANALYZER - LIVE STATISTICS (ALL MESSAGE TYPES)")
    print("=" * 100)
    print(f"⏱️  Running: {elapsed:.1f}s / {duration}s  |  Press Ctrl+C to stop\n")
    
    # F&O Feed Stats
    fo_summary = fo_stats.get_summary()
    print("📊 F&O FEED (239.1.2.5:26002)")
    print("-" * 100)
    print(f"Total Packets: {fo_summary['total_packets']:,}  |  "
          f"Total Bytes: {format_bytes(fo_summary['total_bytes'])}  |  "
          f"Rate: {fo_summary['packets_per_sec']:.1f} pkts/s  |  "
          f"Bandwidth: {format_bytes(fo_summary['bytes_per_sec'])}/s")
    print()
    
    print(f"{'Message Type':<30} {'Code':<8} {'Count':<12} {'%':<8} {'Pkts/sec':<12} {'Status':<8}")
    print("-" * 100)
    
    # Show ALL message types (even with 0 count)
    for msg_code in sorted(MESSAGE_TYPES.keys()):
        msg_name = MESSAGE_TYPES[msg_code]
        count = fo_summary['message_counts'].get(msg_code, 0)
        pct = (count / fo_summary['total_packets'] * 100) if fo_summary['total_packets'] > 0 and count > 0 else 0
        rate = count / fo_summary['elapsed'] if fo_summary['elapsed'] > 0 and count > 0 else 0
        status = "✅ RECV" if count > 0 else "⏸️ NONE"
        
        # Highlight active messages
        if count > 0:
            print(f"{msg_name:<30} {msg_code:<8} {count:<12,} {pct:<7.2f}% {rate:<12.2f} {status:<8}")
        else:
            print(f"{msg_name:<30} {msg_code:<8} {count:<12} {pct:<7.2f}% {rate:<12.2f} {status:<8}")
    
    print("\n")
    
    # Index Feed Stats
    index_summary = index_stats.get_summary()
    print("📊 INDEX FEED (239.1.2.5:26001)")
    print("-" * 100)
    print(f"Total Packets: {index_summary['total_packets']:,}  |  "
          f"Total Bytes: {format_bytes(index_summary['total_bytes'])}  |  "
          f"Rate: {index_summary['packets_per_sec']:.1f} pkts/s  |  "
          f"Bandwidth: {format_bytes(index_summary['bytes_per_sec'])}/s")
    print()
    
    print(f"{'Message Type':<30} {'Code':<8} {'Count':<12} {'%':<8} {'Pkts/sec':<12} {'Status':<8}")
    print("-" * 100)
    
    # Show ALL message types (even with 0 count)
    for msg_code in sorted(MESSAGE_TYPES.keys()):
        msg_name = MESSAGE_TYPES[msg_code]
        count = index_summary['message_counts'].get(msg_code, 0)
        pct = (count / index_summary['total_packets'] * 100) if index_summary['total_packets'] > 0 and count > 0 else 0
        rate = count / index_summary['elapsed'] if index_summary['elapsed'] > 0 and count > 0 else 0
        status = "✅ RECV" if count > 0 else "⏸️ NONE"
        
        # Highlight active messages
        if count > 0:
            print(f"{msg_name:<30} {msg_code:<8} {count:<12,} {pct:<7.2f}% {rate:<12.2f} {status:<8}")
        else:
            print(f"{msg_name:<30} {msg_code:<8} {count:<12} {pct:<7.2f}% {rate:<12.2f} {status:<8}")
    
    print("\n" + "=" * 100)

# ================================================================================
# MAIN
# ================================================================================

def main():
    parser = argparse.ArgumentParser(description='BSE UDP Packet Analyzer')
    parser.add_argument('--duration', type=int, default=60, help='Capture duration in seconds')
    parser.add_argument('--fo-ip', default='239.1.2.5', help='F&O multicast IP')
    parser.add_argument('--fo-port', type=int, default=26002, help='F&O UDP port')
    parser.add_argument('--index-ip', default='239.1.2.5', help='Index multicast IP')
    parser.add_argument('--index-port', type=int, default=26001, help='Index UDP port')
    parser.add_argument('--save', action='store_true', help='Save results to file')
    args = parser.parse_args()
    
    print("\n🚀 BSE UDP Packet Analyzer Starting...")
    print(f"📡 F&O Feed: {args.fo_ip}:{args.fo_port}")
    print(f"📡 Index Feed: {args.index_ip}:{args.index_port}")
    print(f"⏱️  Duration: {args.duration} seconds\n")
    
    # Create stats trackers
    fo_stats = PacketStats()
    index_stats = PacketStats()
    
    # Start listeners
    fo_listener = UDPListener(args.fo_ip, args.fo_port, "F&O Feed", fo_stats)
    index_listener = UDPListener(args.index_ip, args.index_port, "Index Feed", index_stats)
    
    fo_listener.start()
    index_listener.start()
    
    time.sleep(2)  # Wait for connections
    
    # Main display loop
    start_time = time.time()
    end_time = start_time + args.duration
    
    try:
        while time.time() < end_time:
            elapsed = time.time() - start_time
            print_stats_table(fo_stats, index_stats, elapsed, args.duration)
            time.sleep(1)  # Update every second
        
        # Final statistics
        print("\n\n✅ CAPTURE COMPLETE!")
        print_stats_table(fo_stats, index_stats, args.duration, args.duration)
        
    except KeyboardInterrupt:
        print("\n\n⏹️  Stopped by user")
        elapsed = time.time() - start_time
        print_stats_table(fo_stats, index_stats, elapsed, args.duration)
    
    finally:
        # Stop listeners
        fo_listener.stop()
        index_listener.stop()
        
        # Wait for threads to finish
        fo_listener.join(timeout=2)
        index_listener.join(timeout=2)
        
        print("\n📊 Session Summary:")
        print("-" * 100)
        fo_summary = fo_stats.get_summary()
        index_summary = index_stats.get_summary()
        
        print(f"\n🔹 F&O Feed:")
        print(f"   Total Packets: {fo_summary['total_packets']:,}")
        print(f"   Total Data: {format_bytes(fo_summary['total_bytes'])}")
        print(f"   Message Types: {len(fo_summary['message_counts'])}")
        print(f"   Average Rate: {fo_summary['packets_per_sec']:.1f} pkts/s")
        
        print(f"\n🔹 Index Feed:")
        print(f"   Total Packets: {index_summary['total_packets']:,}")
        print(f"   Total Data: {format_bytes(index_summary['total_bytes'])}")
        print(f"   Message Types: {len(index_summary['message_counts'])}")
        print(f"   Average Rate: {index_summary['packets_per_sec']:.1f} pkts/s")
        
        # Save to file if requested
        if args.save:
            timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
            filename = f"packet_analysis_{timestamp}.txt"
            
            # Ensure target directory exists: <repo>/tests/massage_analysis_result/
            base_dir = os.path.dirname(__file__)
            result_dir = os.path.join(base_dir, 'tests', 'massage_analysis_result')
            os.makedirs(result_dir, exist_ok=True)
            fullpath = os.path.join(result_dir, filename)
            
            try:
                with open(fullpath, 'w', encoding='utf-8') as f:
                    f.write("BSE UDP PACKET ANALYSIS REPORT (ALL MESSAGE TYPES)\n")
                    f.write("=" * 100 + "\n")
                    f.write(f"Timestamp: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")
                    f.write(f"Duration: {fo_summary['elapsed']:.1f} seconds\n\n")
                    
                    f.write("F&O FEED (239.1.2.5:26002)\n")
                    f.write("-" * 100 + "\n")
                    f.write(f"Total Packets: {fo_summary['total_packets']:,}\n")
                    f.write(f"Total Bytes: {format_bytes(fo_summary['total_bytes'])}\n")
                    f.write(f"Average Rate: {fo_summary['packets_per_sec']:.1f} pkts/s\n")
                    f.write(f"Message Types Received: {len(fo_summary['message_counts'])}\n\n")
                    
                    f.write(f"{'Message Type':<30} {'Code':<10} {'Count':<15} {'Percentage':<15} {'Status':<10}\n")
                    f.write("-" * 100 + "\n")
                    
                    # Show ALL message types (even with 0 count)
                    for msg_code in sorted(MESSAGE_TYPES.keys()):
                        msg_name = MESSAGE_TYPES[msg_code]
                        count = fo_summary['message_counts'].get(msg_code, 0)
                        pct = (count / fo_summary['total_packets'] * 100) if fo_summary['total_packets'] > 0 and count > 0 else 0
                        status = "RECEIVED" if count > 0 else "NOT RECV"
                        f.write(f"{msg_name:<30} {msg_code:<10} {count:<15,} {pct:<14.2f}% {status:<10}\n")
                    
                    f.write("\n\nINDEX FEED (239.1.2.5:26001)\n")
                    f.write("-" * 100 + "\n")
                    f.write(f"Total Packets: {index_summary['total_packets']:,}\n")
                    f.write(f"Total Bytes: {format_bytes(index_summary['total_bytes'])}\n")
                    f.write(f"Average Rate: {index_summary['packets_per_sec']:.1f} pkts/s\n")
                    f.write(f"Message Types Received: {len(index_summary['message_counts'])}\n\n")
                    
                    f.write(f"{'Message Type':<30} {'Code':<10} {'Count':<15} {'Percentage':<15} {'Status':<10}\n")
                    f.write("-" * 100 + "\n")
                    
                    # Show ALL message types (even with 0 count)
                    for msg_code in sorted(MESSAGE_TYPES.keys()):
                        msg_name = MESSAGE_TYPES[msg_code]
                        count = index_summary['message_counts'].get(msg_code, 0)
                        pct = (count / index_summary['total_packets'] * 100) if index_summary['total_packets'] > 0 and count > 0 else 0
                        status = "RECEIVED" if count > 0 else "NOT RECV"
                        f.write(f"{msg_name:<30} {msg_code:<10} {count:<15,} {pct:<14.2f}% {status:<10}\n")
                
                print(f"\n💾 Results saved to: {fullpath}")
            except Exception as e:
                print(f"❌ Failed to save results to {fullpath}: {e}")
        
        print("\n" + "=" * 100)
        print("✅ Analysis complete!")

if __name__ == "__main__":
    main()
