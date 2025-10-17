"""
Quick check of today's SENSEX data from existing CSV
Compares against expected values from 14:22:23
"""

import csv
import os
from datetime import datetime

# Target tokens
TARGET_TOKENS = {
    861384: {'name': 'SENSEX 30-OCT-2025 FUT', 'expected_ltp': 83847},
    878196: {'name': 'SENSEX 83900 CE 23-OCT', 'expected_ltp': 486},
    878015: {'name': 'SENSEX 83800 PE 23-OCT', 'expected_ltp': 340},
    877845: {'name': 'SENSEX 83700 PE 23-OCT', 'expected_ltp': 304},
    877761: {'name': 'SENSEX 84000 CE 23-OCT', 'expected_ltp': 430},
}

print("="*100)
print("CHECKING TODAY'S SENSEX DATA")
print("="*100)
print()

# Read today's CSV
csv_path = f'data/processed_csv/{datetime.now().strftime("%Y%m%d")}_quotes.csv'

if not os.path.exists(csv_path):
    print(f"❌ No CSV found at: {csv_path}")
    print("   Run the main pipeline first: python src/main.py")
    exit(1)

print(f"Reading: {csv_path}")
print()

# Parse CSV
found_tokens = {}
with open(csv_path, 'r') as f:
    reader = csv.DictReader(f)
    for row in reader:
        token = int(row['token'])
        if token in TARGET_TOKENS:
            if token not in found_tokens:
                found_tokens[token] = []
            
            try:
                ltp = float(row['ltp'])
                found_tokens[token].append({
                    'timestamp': row['timestamp'],
                    'ltp': ltp,
                    'volume': int(float(row.get('volume', 0))),
                    'open': float(row.get('open', 0)),
                    'high': float(row.get('high', 0)),
                    'low': float(row.get('low', 0)),
                    'close': float(row.get('close', 0)),
                })
            except (ValueError, KeyError) as e:
                continue

print(f"Found {len(found_tokens)}/{len(TARGET_TOKENS)} target tokens in CSV")
print()

# Display results
if found_tokens:
    print("RESULTS:")
    print("-"*100)
    
    for token in TARGET_TOKENS.keys():
        info = TARGET_TOKENS[token]
        
        if token in found_tokens:
            updates = found_tokens[token]
            latest = updates[-1]
            expected = info['expected_ltp']
            
            if expected > 0:
                diff_pct = abs(latest['ltp'] - expected) / expected * 100
                status = "✅" if diff_pct < 5 else "⚠️" if diff_pct < 10 else "❌"
            else:
                diff_pct = 0
                status = "⚠️"
            
            print(f"{status} Token {token}: {info['name']:<30}")
            print(f"   Latest LTP:  ₹{latest['ltp']:>10,.2f}")
            print(f"   Expected:    ₹{expected:>10,.2f}")
            print(f"   Difference:  {diff_pct:>6.1f}%")
            print(f"   Updates:     {len(updates)}")
            if latest['open'] > 0:
                print(f"   OHLC: Open=₹{latest['open']:.2f}, High=₹{latest['high']:.2f}, Low=₹{latest['low']:.2f}, Close=₹{latest['close']:.2f}")
            print(f"   Volume:      {latest['volume']:,}")
            print()
        else:
            print(f"❌ Token {token}: {info['name']:<30} - NOT FOUND")
            print()
else:
    print("❌ NONE OF THE TARGET TOKENS WERE FOUND!")

print("="*100)
