"""
Analyze data around offset 99 where we found a possible LTP value
"""

import struct

with open('d:\\bse\\data\\raw_packets\\20251017_102330_714968_type2020_packet.bin', 'rb') as f:
    data = f.read()

print("="*100)
print("ANALYZING DATA AROUND OFFSET 99 (Possible LTP Location)")
print("="*100)
print()

# Offset 99 has a value around 8.4M paise
# Record starts at offset 36
# So offset 99 is at position 99-36 = 63 bytes into the record

record_start = 36
ltp_offset_in_record = 99 - record_start

print(f"Record starts at: {record_start}")
print(f"LTP found at: {99}")
print(f"LTP offset within record: {ltp_offset_in_record} bytes")
print()

# Show bytes around this area
print("Bytes 36-110 (hex):")
chunk = data[36:110]
for i in range(0, len(chunk), 16):
    offset_label = f"{36+i:03d}"
    hex_bytes = ' '.join(f"{b:02x}" for b in chunk[i:i+16])
    print(f"{offset_label}: {hex_bytes}")

print()
print("="*100)
print("PARSING AROUND OFFSET 99")
print("="*100)
print()

# Parse values around offset 99
# Try 8 bytes before (offset 91) as potential LTQ
# Try 4 bytes at 99 as LTP

# At offset 91 (8 bytes before LTP)
if 99 - 8 >= 36:
    ltq_offset = 99 - 8
    ltq_le = struct.unpack('<q', data[ltq_offset:ltq_offset+8])[0]
    ltq_be = struct.unpack('>q', data[ltq_offset:ltq_offset+8])[0]
    print(f"Offset {ltq_offset} (8 bytes before LTP) - Possible LTQ:")
    print(f"  Little-Endian: {ltq_le:,}")
    print(f"  Big-Endian:    {ltq_be:,}")
    print()

# At offset 95 (4 bytes before LTP)
close_offset = 99 - 4
close_le = struct.unpack('<i', data[close_offset:close_offset+4])[0]
close_be = struct.unpack('>i', data[close_offset:close_offset+4])[0]
print(f"Offset {close_offset} (4 bytes before LTP) - Possible Close Rate:")
print(f"  Little-Endian: {close_le:,} paise = ₹{close_le/100:,.2f}")
print(f"  Big-Endian:    {close_be:,} paise = ₹{close_be/100:,.2f}")
print()

# At offset 99 (LTP)
ltp_offset = 99
ltp_le = struct.unpack('<i', data[ltp_offset:ltp_offset+4])[0]
ltp_be = struct.unpack('>i', data[ltp_offset:ltp_offset+4])[0]
print(f"Offset {ltp_offset} - LTP:")
print(f"  Little-Endian: {ltp_le:,} paise = ₹{ltp_le/100:,.2f}")
print(f"  Big-Endian:    {ltp_be:,} paise = ₹{ltp_be/100:,.2f} ← Expected ~₹85,563")
print()

# Check what's at the beginning of the record
print("="*100)
print("FIRST 76 BYTES OF RECORD")
print("="*100)
print()

token = struct.unpack('<I', data[36:40])[0]
print(f"Token (offset 36-39, LE): {token}")
print()

# If offset 99 is LTP, then:
# - Offset 95 is Close Rate (4 bytes)
# - Offset 91 is LTQ (8 bytes)
# - Offset 83 might be something else (8 bytes before LTQ)

# Work backwards from offset 91 (LTQ start)
ltq_start = 91
bytes_before_ltq = ltq_start - record_start
print(f"Bytes from record start to LTQ: {bytes_before_ltq}")
print(f"Bytes from record start to Close: {close_offset - record_start}")
print(f"Bytes from record start to LTP: {ltp_offset - record_start}")
print()

print("This suggests:")
print(f"  - Uncompressed section is {ltp_offset - record_start + 4} bytes (including LTP)")
print(f"  - Token at +0")
print(f"  - Close Rate at +{close_offset - record_start}")
print(f"  - LTQ at +{ltq_start - record_start}")
print(f"  - LTP at +{ltp_offset - record_start}")
