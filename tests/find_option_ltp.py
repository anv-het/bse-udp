#!/usr/bin/env python3
"""
Find correct LTP offset for SENSEX options by analyzing raw packets.
Expected value: Token 878196 (SENSEX 83900 CE) should have LTP around ₹486.00 (48,600 paise)
Current decoder shows ₹228.15 (22,815 paise) - about 53% too low
"""

import struct
import glob
from pathlib import Path

# Target token and expected value
TARGET_TOKEN = 878196  # SENSEX 83900 CE
EXPECTED_LTP_PAISE = 48600  # ₹486.00 in paise
TOLERANCE = 0.15  # 15% tolerance for matching

def analyze_packet(packet_path: Path):
    """Analyze a single packet file for the target token"""
    with open(packet_path, 'rb') as f:
        packet = f.read()
    
    # Parse header (36 bytes)
    if len(packet) < 36:
        return None
    
    format_id = struct.unpack('>H', packet[4:6])[0]
    msg_type = struct.unpack('<H', packet[8:10])[0]
    
    # Skip non-2020 packets
    if msg_type != 2020:
        return None
    
    # Parse records (67 bytes each, starting at offset 36)
    results = []
    offset = 36
    record_num = 0
    
    while offset + 67 <= len(packet):
        record = packet[offset:offset+67]
        record_num += 1
        
        # Token is at +0 (Little-Endian)
        token = struct.unpack('<I', record[0:4])[0]
        
        if token == TARGET_TOKEN:
            print(f"\n{'='*100}")
            print(f"FOUND TOKEN {TARGET_TOKEN} in {packet_path.name} at record #{record_num}")
            print(f"{'='*100}")
            
            # Current decoder reads LTP at offset +4
            current_ltp_bytes = record[4:8]
            current_ltp = struct.unpack('<i', current_ltp_bytes)[0]
            print(f"\nCurrent decoder (offset +4 LE): {current_ltp:,} paise = ₹{current_ltp/100:,.2f}")
            print(f"Expected value: {EXPECTED_LTP_PAISE:,} paise = ₹{EXPECTED_LTP_PAISE/100:,.2f}")
            print(f"Error: {abs(current_ltp - EXPECTED_LTP_PAISE) / EXPECTED_LTP_PAISE * 100:.1f}%")
            
            # Hex dump first 67 bytes
            print(f"\n{'HEX DUMP OF RECORD':-^100}")
            for i in range(0, 67, 16):
                hex_part = ' '.join(f'{b:02x}' for b in record[i:i+16])
                print(f"Offset {i:3d}: {hex_part}")
            
            # Scan all 4-byte offsets for potential matches
            print(f"\n{'SCANNING ALL 4-BYTE VALUES IN RECORD':-^100}")
            matches = []
            
            for i in range(0, 64):  # Up to offset 63
                # Try Little-Endian signed
                val_le = struct.unpack('<i', record[i:i+4])[0]
                if val_le > 0:  # Only positive values
                    error = abs(val_le - EXPECTED_LTP_PAISE) / EXPECTED_LTP_PAISE
                    if error < TOLERANCE:
                        matches.append((i, 'LE', val_le, error))
                        print(f"✓ Offset +{i:3d}: LE signed = {val_le:11,} paise = ₹{val_le/100:9,.2f} (error: {error*100:5.1f}%)")
                
                # Try Big-Endian signed
                val_be = struct.unpack('>i', record[i:i+4])[0]
                if val_be > 0:
                    error = abs(val_be - EXPECTED_LTP_PAISE) / EXPECTED_LTP_PAISE
                    if error < TOLERANCE:
                        matches.append((i, 'BE', val_be, error))
                        print(f"✓ Offset +{i:3d}: BE signed = {val_be:11,} paise = ₹{val_be/100:9,.2f} (error: {error*100:5.1f}%)")
            
            if not matches:
                print("❌ NO MATCHES FOUND within 15% tolerance")
                print("\nClosest values (top 10):")
                all_values = []
                for i in range(0, 64):
                    val_le = struct.unpack('<i', record[i:i+4])[0]
                    if val_le > 0:
                        error = abs(val_le - EXPECTED_LTP_PAISE) / EXPECTED_LTP_PAISE
                        all_values.append((i, 'LE', val_le, error))
                    val_be = struct.unpack('>i', record[i:i+4])[0]
                    if val_be > 0:
                        error = abs(val_be - EXPECTED_LTP_PAISE) / EXPECTED_LTP_PAISE
                        all_values.append((i, 'BE', val_be, error))
                
                # Sort by error and show top 10
                all_values.sort(key=lambda x: x[3])
                for i, endian, val, error in all_values[:10]:
                    print(f"  Offset +{i:3d}: {endian} = {val:11,} paise = ₹{val/100:9,.2f} (error: {error*100:5.1f}%)")
            else:
                print(f"\nFOUND {len(matches)} potential match(es) within {TOLERANCE*100:.0f}% tolerance")
                for off, endian, val, error in matches:
                    print(f"  ✓ Offset +{off:3d}: {endian} = {val:11,} paise = ₹{val/100:9,.2f} (error: {error*100:5.1f}%)")
            
            return {
                'packet': packet_path.name,
                'record_num': record_num,
                'current_ltp': current_ltp,
                'matches': matches
            }
        
        offset += 67
    
    return None

def main():
    """Scan all raw packets for token 878196"""
    packet_dir = Path('data/raw_packets')
    
    if not packet_dir.exists():
        print(f"❌ Directory not found: {packet_dir}")
        return
    
    # Get all .bin files sorted by modification time (newest first)
    packets = sorted(packet_dir.glob('*.bin'), key=lambda p: p.stat().st_mtime, reverse=True)
    
    print(f"Searching for token {TARGET_TOKEN} (SENSEX 83900 CE) in {len(packets)} packets...")
    print(f"Expected LTP: ₹{EXPECTED_LTP_PAISE/100:,.2f} ({EXPECTED_LTP_PAISE:,} paise)")
    
    found_count = 0
    for packet_path in packets[:100]:  # Check first 100 packets
        result = analyze_packet(packet_path)
        if result:
            found_count += 1
            if found_count >= 3:  # Stop after finding 3 packets
                break
    
    if found_count == 0:
        print(f"\n❌ Token {TARGET_TOKEN} not found in first 100 packets")
    else:
        print(f"\n✅ Found token {TARGET_TOKEN} in {found_count} packet(s)")

if __name__ == '__main__':
    main()
