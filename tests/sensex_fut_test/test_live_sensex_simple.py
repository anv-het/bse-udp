"""
Simple live SENSEX test - let the pipeline run and check results
Target contracts at 14:22:23:
1. SENSEX 30 Oct Fut: 83847
2. SENSEX 83900 CE 23 Oct: 486
3. SENSEX 83800 PE 23 Oct: 340
4. SENSEX 83700 PE 23 Oct: 304
5. SENSEX 84000 CE 23 Oct: 430
"""

import sys
import os
import time
import json
import shutil
from datetime import datetime
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from main import main

# Target tokens we're interested in
TARGET_TOKENS = {
    861384: {'name': 'SENSEX 30-OCT-2025 FUT', 'expected_ltp': 83847},
    878196: {'name': 'SENSEX 83900 CE 23-OCT', 'expected_ltp': 486},
    878015: {'name': 'SENSEX 83800 PE 23-OCT', 'expected_ltp': 340},
    877845: {'name': 'SENSEX 83700 PE 23-OCT', 'expected_ltp': 304},
    877761: {'name': 'SENSEX 84000 CE 23-OCT', 'expected_ltp': 430},
}

print("="*100)
print("LIVE SENSEX MARKET DATA TEST - SIMPLIFIED")
print("="*100)
print(f"Start Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
print()
print("Target Contracts:")
for token, info in TARGET_TOKENS.items():
    print(f"  {token}: {info['name']:<30} Expected LTP: ₹{info['expected_ltp']:>8,.2f}")
print()
print("Running BSE pipeline for 30 seconds...")
print("Press Ctrl+C to stop early")
print("="*100)
print()

print()
print("Running BSE pipeline...")
print("This will run until manually stopped (Ctrl+C)")
print("Recommended: Let it run for 30-60 seconds, then stop")
print("="*100)
print()

# Note: main() runs indefinitely, so we just inform the user
print("⏳ Please run the main pipeline manually:")
print()
print("   python src/main.py")
print()
print("   Let it run for 30-60 seconds, then press Ctrl+C")
print()
print("After stopping, this script will analyze the captured data.")
print()
input("Press Enter when you're ready to analyze the results (after running main.py)...")
print()
print("="*100)
print("ANALYZING RESULTS")
print("="*100)

# Read the CSV output
csv_path = f'data/processed_csv/{datetime.now().strftime("%Y%m%d")}_quotes.csv'

if not os.path.exists(csv_path):
    print(f"❌ No CSV output found at {csv_path}")
    print("   Market might be closed or no data received")
    sys.exit(1)

# Parse CSV and find our tokens
import csv

found_tokens = {}
with open(csv_path, 'r') as f:
    reader = csv.DictReader(f)
    for row in reader:
        token = int(row['token'])
        if token in TARGET_TOKENS:
            if token not in found_tokens:
                found_tokens[token] = []
            
            ltp = float(row['ltp'])
            found_tokens[token].append({
                'timestamp': row['timestamp'],
                'ltp': ltp,
                'volume': int(row.get('volume', 0)),
                'open': float(row.get('open', 0)),
                'high': float(row.get('high', 0)),
                'low': float(row.get('low', 0)),
                'close': float(row.get('close', 0)),
            })

print()
print(f"Found {len(found_tokens)}/{len(TARGET_TOKENS)} target tokens")
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
            diff_pct = abs(latest['ltp'] - expected) / expected * 100 if expected > 0 else 0
            
            status = "✅ PASS" if diff_pct < 5 else "⚠️ WARN" if diff_pct < 10 else "❌ FAIL"
            
            print(f"{status} {info['name']:<30}")
            print(f"       Latest LTP:  ₹{latest['ltp']:>10,.2f}")
            print(f"       Expected:    ₹{expected:>10,.2f}")
            print(f"       Difference:  {diff_pct:>6.1f}%")
            print(f"       Updates:     {len(updates)}")
            print(f"       Open/High/Low: ₹{latest['open']:.2f} / ₹{latest['high']:.2f} / ₹{latest['low']:.2f}")
            print()
        else:
            print(f"❌ MISS {info['name']:<30} - NOT FOUND IN OUTPUT")
            print()
else:
    print("❌ NONE OF THE TARGET TOKENS WERE FOUND!")
    print("   Possible reasons:")
    print("   - Market is closed (BSE hours: 9:00 AM - 3:30 PM IST)")
    print("   - These contracts are not actively trading")
    print("   - Multicast feed issue")

# Copy to timestamped file
timestamp_str = datetime.now().strftime('%Y%m%d_%H%M%S')
test_csv = f'data/test_results/sensex_live_{timestamp_str}.csv'
test_json = f'data/test_results/sensex_live_{timestamp_str}.json'

if os.path.exists(csv_path):
    shutil.copy2(csv_path, test_csv)
    print(f"✅ Results saved to: {test_csv}")

json_path = f'data/processed_json/{datetime.now().strftime("%Y%m%d")}_quotes.json'
if os.path.exists(json_path):
    shutil.copy2(json_path, test_json)
    print(f"✅ Results saved to: {test_json}")

print()
print("="*100)
print("TEST COMPLETE")
print("="*100)
