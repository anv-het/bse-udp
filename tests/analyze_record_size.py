import struct

# Read packet
with open(r'd:\bse\data\raw_packets\20251006_115104_524083_type2020_packet.bin', 'rb') as f:
    data = f.read()

print(f'Total packet size: {len(data)} bytes')
print(f'Header size: 36 bytes')
print(f'Data section: {len(data) - 36} bytes')
print(f'\nPossible record sizes:')
for rec_size in [64, 66, 88, 132]:
    num_records = (len(data) - 36) / rec_size
    print(f'  {rec_size} bytes/record: {num_records:.2f} records')

# Check if it's 8 records of 66 bytes each
rec_size = 66
num_records = (len(data) - 36) // rec_size
print(f'\n✓ Using {rec_size} bytes/record: {num_records} complete records')
print(f'  Total: 36 header + {num_records * rec_size} data = {36 + num_records * rec_size} bytes')

# Parse first 3 records with 66-byte size
print(f'\n=== First 3 Records (66 bytes each) ===')
for i in range(3):
    offset = 36 + (i * rec_size)
    rec = data[offset:offset+rec_size]
    if len(rec) < 4:
        break
    
    token = struct.unpack('<I', rec[0:4])[0]
    print(f'\nRecord {i+1} (offset {offset}):')
    print(f'  Token (LE): {token}')
    
    # Show next few fields
    if len(rec) >= 20:
        print(f'  Offset 4-7 (LE uint32): {struct.unpack("<I", rec[4:8])[0]}')
        print(f'  Offset 8-11 (LE uint32): {struct.unpack("<I", rec[8:12])[0]}')
        print(f'  Offset 12-15 (LE uint32): {struct.unpack("<I", rec[12:16])[0]}')
        print(f'  Raw hex [0:20]: {rec[:20].hex()}')
