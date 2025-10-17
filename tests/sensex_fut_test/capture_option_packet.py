"""
Script to capture and analyze a raw packet for SENSEX option token 878196
Expected LTP: ₹486 (48600 paise)
This will help us find the correct byte offsets
"""

import socket
import struct
import time
from datetime import datetime

# BSE multicast details
MULTICAST_IP = "239.1.2.5"
PORT = 26002
TARGET_TOKEN = 878196  # SENSEX 83900 CE

print("="*100)
print(f"CAPTURING RAW PACKET FOR TOKEN {TARGET_TOKEN}")
print("="*100)
print(f"Expected LTP: ₹486.00 (48600 paise or 4860000 centipaise)")
print(f"Looking in multicast: {MULTICAST_IP}:{PORT}")
print()

# Create socket
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
sock.bind(('', PORT))

# Join multicast group
mreq = struct.pack("4sl", socket.inet_aton(MULTICAST_IP), socket.INADDR_ANY)
sock.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP, mreq)
sock.settimeout(5)

print("Listening for packets containing token 878196...")
print()

packets_checked = 0
start_time = time.time()

try:
    while time.time() - start_time < 60:  # Max 60 seconds
        try:
            data, addr = sock.recvfrom(2048)
            packets_checked += 1
            
            if packets_checked % 10 == 0:
                print(f"  ... checked {packets_checked} packets ...")
            
            # Check if this packet contains our token (search for token bytes)
            token_bytes = struct.pack('<I', TARGET_TOKEN)  # Little-Endian
            
            if token_bytes in data:
                print(f"\n✅ FOUND TOKEN {TARGET_TOKEN} in packet!")
                print(f"   Packet size: {len(data)} bytes")
                print(f"   Time: {datetime.now().strftime('%H:%M:%S')}")
                print()
                
                # Find offset of token
                token_offset = data.index(token_bytes)
                print(f"Token found at offset: {token_offset}")
                print()
                
                # Show hex dump around token
                start = max(0, token_offset)
                end = min(len(data), token_offset + 150)
                
                print("HEX DUMP (Token onwards):")
                print("-"*100)
                chunk = data[start:end]
                for i in range(0, len(chunk), 16):
                    offset_label = f"{start+i:03d}"
                    hex_bytes = ' '.join(f"{b:02x}" for b in chunk[i:i+16])
                    ascii_str = ''.join(chr(b) if 32 <= b < 127 else '.' for b in chunk[i:i+16])
                    print(f"{offset_label}: {hex_bytes:<48} {ascii_str}")
                
                print()
                print("="*100)
                print("SEARCHING FOR LTP VALUE (₹486 = 48600 paise)")
                print("="*100)
                print()
                
                # Search for 48600 in different formats
                # As int32 Little-Endian
                ltp_le = struct.pack('<i', 48600)
                if ltp_le in data:
                    offset = data.index(ltp_le)
                    print(f"✅ Found 48600 (Little-Endian int32) at offset {offset}")
                    print(f"   Offset from token: +{offset - token_offset}")
                
                # As int32 Big-Endian
                ltp_be = struct.pack('>i', 48600)
                if ltp_be in data:
                    offset = data.index(ltp_be)
                    print(f"✅ Found 48600 (Big-Endian int32) at offset {offset}")
                    print(f"   Offset from token: +{offset - token_offset}")
                
                # Try in centipaise (4860000)
                ltp_cp_le = struct.pack('<i', 4860000)
                if ltp_cp_le in data:
                    offset = data.index(ltp_cp_le)
                    print(f"✅ Found 4860000 centipaise (Little-Endian int32) at offset {offset}")
                    print(f"   Offset from token: +{offset - token_offset}")
                
                ltp_cp_be = struct.pack('>i', 4860000)
                if ltp_cp_be in data:
                    offset = data.index(ltp_cp_be)
                    print(f"✅ Found 4860000 centipaise (Big-Endian int32) at offset {offset}")
                    print(f"   Offset from token: +{offset - token_offset}")
                
                # Save packet to file
                timestamp_str = datetime.now().strftime('%Y%m%d_%H%M%S')
                filename = f"data/raw_packets/token_{TARGET_TOKEN}_{timestamp_str}.bin"
                with open(filename, 'wb') as f:
                    f.write(data)
                print()
                print(f"✅ Packet saved to: {filename}")
                print()
                
                # Parse nearby values
                print("VALUES AROUND TOKEN LOCATION:")
                print("-"*100)
                record_start = token_offset
                
                # Try parsing at various offsets
                for offset in range(0, 80, 4):
                    if record_start + offset + 4 <= len(data):
                        val_le = struct.unpack('<i', data[record_start+offset:record_start+offset+4])[0]
                        val_be = struct.unpack('>i', data[record_start+offset:record_start+offset+4])[0]
                        
                        # Highlight if close to expected values
                        marker = ""
                        if 40000 < val_le < 60000:
                            marker = " ← Possible LTP (LE)!"
                        elif 40000 < val_be < 60000:
                            marker = " ← Possible LTP (BE)!"
                        
                        if marker or offset < 24:  # Show first 24 bytes always
                            print(f"  Offset +{offset:2d}: LE={val_le:>12,} | BE={val_be:>12,}{marker}")
                
                print()
                print("="*100)
                print("ANALYSIS COMPLETE - Use above data to update decoder.py offsets")
                print("="*100)
                break
                
        except socket.timeout:
            continue

except KeyboardInterrupt:
    print("\n⚠️  Interrupted by user")

finally:
    sock.close()

if packets_checked > 0:
    print(f"\nChecked {packets_checked} packets in {time.time() - start_time:.1f} seconds")
else:
    print("\n❌ No packets received - market might be closed")
