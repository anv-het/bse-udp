"""
Find which packet in our captures contains token 861384 with LTP closest to 118003.20
Then analyze that specific packet's structure
"""

import struct
import os
import glob

def decode_packet_simple(packet):
    """Simple decode to extract token and current offset+63 value"""
    if len(packet) < 100:
        return None
    
    # Header is 36 bytes
    record_start = 36
    
    # Token at offset 0 (Little-Endian)
    token = struct.unpack('<I', packet[record_start:record_start+4])[0]
    
    if token <= 1:
        return None
    
    # Current decoder reads offset +63 (Big-Endian)
    if len(packet) >= record_start + 67:
        value_be = struct.unpack('>i', packet[record_start+63:record_start+67])[0]
        ltp_rupees = value_be / 100.0
    else:
        ltp_rupees = 0
    
    return {
        'token': token,
        'ltp': ltp_rupees,
        'value_paise': value_be if len(packet) >= record_start + 67 else 0,
        'packet': packet,
        'record_start': record_start
    }

def hex_dump_range(data, start, end):
    """Hex dump a specific range"""
    result = []
    for i in range(start, end, 16):
        chunk_end = min(i+16, end)
        hex_part = ' '.join(f'{b:02x}' for b in data[i:chunk_end])
        ascii_part = ''.join(chr(b) if 32 <= b < 127 else '.' for b in data[i:chunk_end])
        result.append(f'  {i:04x}: {hex_part:<48} {ascii_part}')
    return '\n'.join(result)

def scan_all_offsets(packet, record_start, expected_paise):
    """Scan all 4-byte values in the record and find matches"""
    matches = []
    
    # Scan up to 200 bytes from record start
    for offset in range(0, min(200, len(packet) - record_start - 4)):
        pos = record_start + offset
        
        # Try Big-Endian signed
        try:
            value_be_s = struct.unpack('>i', packet[pos:pos+4])[0]
            if abs(value_be_s - expected_paise) / max(expected_paise, 1) < 0.05:  # Within 5%
                matches.append({
                    'offset': offset,
                    'encoding': 'big-endian signed',
                    'value': value_be_s,
                    'rupees': value_be_s / 100.0
                })
        except:
            pass
        
        # Try Little-Endian signed
        try:
            value_le_s = struct.unpack('<i', packet[pos:pos+4])[0]
            if abs(value_le_s - expected_paise) / max(expected_paise, 1) < 0.05:
                matches.append({
                    'offset': offset,
                    'encoding': 'little-endian signed',
                    'value': value_le_s,
                    'rupees': value_le_s / 100.0
                })
        except:
            pass
    
    return matches

def main():
    print("="*100)
    print("FINDING PACKET WITH LTP = 118,003.20")
    print("="*100)
    print()
    
    target_ltp = 118003.20
    target_paise = int(target_ltp * 100)  # 11800320
    
    print(f"Target: Rs.{target_ltp:,.2f} = {target_paise:,} paise")
    print()
    
    # Find all packets
    packet_dir = 'data/raw_packets'
    packets = glob.glob(f'{packet_dir}/20251017_*.bin')
    
    print(f"Scanning {len(packets)} packets...")
    print()
    
    # Find packet with token 861384 and LTP close to target
    best_match = None
    best_diff = float('inf')
    
    for pkt_path in packets:
        with open(pkt_path, 'rb') as f:
            packet = f.read()
        
        info = decode_packet_simple(packet)
        if info and info['token'] == 861384:
            diff = abs(info['ltp'] - target_ltp)
            if diff < best_diff:
                best_diff = diff
                best_match = {
                    'path': pkt_path,
                    'info': info
                }
    
    if not best_match:
        print("❌ No packet found with token 861384")
        return
    
    print(f"Found best match:")
    print(f"  File: {os.path.basename(best_match['path'])}")
    print(f"  Token: {best_match['info']['token']}")
    print(f"  Current decoder LTP: Rs.{best_match['info']['ltp']:,.2f}")
    print(f"  Difference: Rs.{best_diff:,.2f}")
    print()
    
    # Now scan this packet for the expected value
    packet = best_match['info']['packet']
    record_start = best_match['info']['record_start']
    
    print("="*100)
    print("SCANNING FOR EXPECTED VALUE: 8,384,700 paise (Rs.83,847.00)")
    print("="*100)
    print()
    
    expected_paise = 8384700
    matches = scan_all_offsets(packet, record_start, expected_paise)
    
    if matches:
        print(f"FOUND {len(matches)} potential match(es):")
        print()
        for m in matches:
            print(f"  Offset +{m['offset']:3d}: {m['encoding']:20s} = {m['value']:>10,} paise = Rs.{m['rupees']:>10,.2f}")
        print()
        
        # Show hex dump for each match
        for m in matches:
            offset = m['offset']
            print(f"Hex dump around offset +{offset}:")
            start = max(0, offset - 16)
            end = min(200, offset + 32)
            print(hex_dump_range(packet[record_start:], start, end))
            print()
    else:
        print("❌ Expected value NOT FOUND in this packet")
        print()
        print("This suggests:")
        print("  1. The expected value 83,847 is not in this specific packet")
        print("  2. The packet might be from a different time when price was different")
        print("  3. We need to capture a packet during market hours when LTP = 83,847")
        print()
    
    # Show what values are actually in the record at various offsets
    print("="*100)
    print("SAMPLING ALL 4-BYTE VALUES IN RECORD (Big-Endian)")
    print("="*100)
    print()
    print("Offset | Signed Value  | As Rupees     | Unsigned Value")
    print("-------|---------------|---------------|---------------")
    
    for offset in range(0, min(100, len(packet) - record_start - 4), 4):
        pos = record_start + offset
        val_s = struct.unpack('>i', packet[pos:pos+4])[0]
        val_u = struct.unpack('>I', packet[pos:pos+4])[0]
        rupees = val_s / 100.0
        
        # Highlight if in reasonable range
        marker = " *" if 50000 < val_s < 200000 else ""
        
        print(f"  +{offset:3d} | {val_s:>13,} | Rs.{rupees:>12,.2f} | {val_u:>14,}{marker}")
    
    print()
    
    # Show full hex dump of first 150 bytes
    print("="*100)
    print("FULL RECORD HEX DUMP (first 150 bytes)")
    print("="*100)
    print()
    print(hex_dump_range(packet[record_start:], 0, min(150, len(packet) - record_start)))
    print()

if __name__ == '__main__':
    main()
