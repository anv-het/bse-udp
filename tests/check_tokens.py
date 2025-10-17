"""
Quick check of token values in different packets
"""

import struct
import os

packet_dir = 'd:\\bse\\data\\raw_packets'
packet_files = sorted([f for f in os.listdir(packet_dir) if 'type2020' in f])[:10]  # First 10

print("TOKENS AT OFFSET 36 (Little-Endian):")
print("="*80)

for pf in packet_files:
    path = os.path.join(packet_dir, pf)
    with open(path, 'rb') as f:
        data = f.read()
    
    token = struct.unpack('<I', data[36:40])[0]
    print(f"{pf[:30]:<30} Token: {token:>8}")

print("\n" + "="*80)
print("From CSV, expected tokens:")
print("  878192, 878196, 878199, 878202, 878203, 878207...")
