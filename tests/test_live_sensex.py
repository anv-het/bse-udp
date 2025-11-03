"""
Test live SENSEX market data with specific tokens
Target contracts at 14:22:23 on Oct 17, 2025:
1. SENSEX 30 Oct Fut: 83847
2. SENSEX 83900 CE 23 Oct: 486
3. SENSEX 83800 PE 23 Oct: 340
4. SENSEX 83700 PE 23 Oct: 304
5. SENSEX 84000 CE 23 Oct: 430
"""

import sys
import os
import time
from datetime import datetime
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from connection import BSEMulticastConnection
from packet_receiver import PacketReceiver
from decoder import PacketDecoder
from decompressor import NFCASTDecompressor
from data_collector import MarketDataCollector
from saver import DataSaver
import json

# Target tokens
TARGET_TOKENS = {
    861384: {'name': 'SENSEX 30-OCT-2025 FUT', 'expected_ltp': 83847},
    878196: {'name': 'SENSEX 83900 CE 23-OCT', 'expected_ltp': 486},
    878015: {'name': 'SENSEX 83800 PE 23-OCT', 'expected_ltp': 340},
    877845: {'name': 'SENSEX 83700 PE 23-OCT', 'expected_ltp': 304},
    877761: {'name': 'SENSEX 84000 CE 23-OCT', 'expected_ltp': 430},
}

def test_live_sensex():
    """Test with live market data for specific SENSEX contracts"""
    
    print("="*100)
    print("LIVE SENSEX MARKET DATA TEST")
    print("="*100)
    print(f"Start Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print()
    print("Target Contracts:")
    for token, info in TARGET_TOKENS.items():
        print(f"  {token}: {info['name']:<30} Expected LTP: ₹{info['expected_ltp']:>8,.2f}")
    print()
    
    # Initialize components
    config_path = os.path.join(os.path.dirname(__file__), '..', 'config.json')
    with open(config_path, 'r') as f:
        config = json.load(f)
    
    # Create filtered token map with only target tokens
    token_map = {}
    full_token_path = os.path.join(os.path.dirname(__file__), '..', 'data', 'tokens', 'token_details.json')
    with open(full_token_path, 'r') as f:
        full_map = json.load(f)
        for token_str in TARGET_TOKENS.keys():
            if str(token_str) in full_map:
                token_map[str(token_str)] = full_map[str(token_str)]
    
    print(f"Loaded {len(token_map)} target tokens from token master")
    print()
    
    # Initialize pipeline
    connection = BSEMulticastConnection(
        multicast_ip=config['multicast']['ip'],
        port=config['multicast']['port'],
        buffer_size=config.get('buffer_size', 2048)
    )
    sock = connection.connect()
    receiver = PacketReceiver(sock, config, token_map)
    decoder = PacketDecoder()
    decompressor = NFCASTDecompressor()
    collector = MarketDataCollector(token_map)
    
    # Create timestamped output filename
    timestamp_str = datetime.now().strftime('%Y%m%d_%H%M%S')
    
    # Create test_results directory if it doesn't exist
    test_results_dir = os.path.join(os.path.dirname(__file__), '..', 'data', 'test_results')
    os.makedirs(test_results_dir, exist_ok=True)
    
    csv_path = os.path.join(test_results_dir, f'sensex_live_{timestamp_str}.csv')
    json_path = os.path.join(test_results_dir, f'sensex_live_{timestamp_str}.json')
    
    # Use standard DataSaver (it will create daily files)
    # We'll move/copy the files after with timestamp
    saver = DataSaver(output_dir=os.path.join(os.path.dirname(__file__), '..', 'data'))
    
    print(f"Output will be saved to:")
    print(f"  Daily CSV:  data/processed_csv/{datetime.now().strftime('%Y%m%d')}_quotes.csv")
    print(f"  Daily JSON: data/processed_json/{datetime.now().strftime('%Y%m%d')}_quotes.json")
    print(f"  (Will copy to test_results/{timestamp_str} after)")
    print()
    
    # Track found tokens
    found_tokens = {}
    packets_processed = 0
    start_time = time.time()
    max_duration = 30  # Run for 30 seconds max
    
    print("Listening for packets... (Ctrl+C to stop)")
    print("-"*100)
    
    try:
        while time.time() - start_time < max_duration:
            # Receive packet
            packet_data = receiver.receive_packet()
            if not packet_data:
                continue
            
            packets_processed += 1
            
            # Decode packet
            decoded = decoder.decode_packet(packet_data)
            if not decoded or 'records' not in decoded:
                continue
            
            # Process each record
            for record in decoded['records']:
                token = record.get('token')
                
                # Only process target tokens
                if token not in TARGET_TOKENS:
                    continue
                
                # Decompress (if needed)
                # For now, just use base LTP from decoder
                ltp_paise = record.get('ltp', 0)
                ltp_rupees = ltp_paise / 100.0
                
                # Collect market data
                quote = collector.collect_quote(record, decoded['header'])
                
                if quote:
                    # Save to CSV/JSON
                    saver.save_quote(quote)
                    
                    # Track this token
                    if token not in found_tokens:
                        found_tokens[token] = []
                    
                    found_tokens[token].append({
                        'timestamp': datetime.now(),
                        'ltp': ltp_rupees,
                        'volume': record.get('volume', 0),
                        'trades': record.get('num_trades', 0)
                    })
                    
                    # Print update
                    expected = TARGET_TOKENS[token]['expected_ltp']
                    diff_pct = abs(ltp_rupees - expected) / expected * 100 if expected > 0 else 0
                    status = "✅" if diff_pct < 5 else "⚠️" if diff_pct < 10 else "❌"
                    
                    print(f"{status} Token {token}: {TARGET_TOKENS[token]['name']:<30} "
                          f"LTP: ₹{ltp_rupees:>8,.2f} (Expected: ₹{expected:>8,.2f}, "
                          f"Diff: {diff_pct:>5.1f}%)")
            
            # Show progress every 10 packets
            if packets_processed % 10 == 0:
                print(f"  ... {packets_processed} packets processed, "
                      f"{len(found_tokens)}/{len(TARGET_TOKENS)} tokens found ...")
            
            # Stop if we found all tokens at least once
            if len(found_tokens) >= len(TARGET_TOKENS):
                updates_per_token = min(len(updates) for updates in found_tokens.values())
                if updates_per_token >= 3:  # Got at least 3 updates for each
                    print()
                    print("✅ All target tokens found with multiple updates!")
                    break
    
    except KeyboardInterrupt:
        print("\n\n⚠️  Interrupted by user")
    
    finally:
        connection.close()
    
    # Copy output files with timestamp
    import shutil
    daily_csv = os.path.join(os.path.dirname(__file__), '..', 'data', 'processed_csv', f'{datetime.now().strftime("%Y%m%d")}_quotes.csv')
    daily_json = os.path.join(os.path.dirname(__file__), '..', 'data', 'processed_json', f'{datetime.now().strftime("%Y%m%d")}_quotes.json')
    
    if os.path.exists(daily_csv):
        shutil.copy2(daily_csv, csv_path)
        print(f"\n✅ CSV copied to: {csv_path}")
    
    if os.path.exists(daily_json):
        shutil.copy2(daily_json, json_path)
        print(f"✅ JSON copied to: {json_path}")
    
    # Summary
    print()
    print("="*100)
    print("SUMMARY")
    print("="*100)
    print(f"Duration: {time.time() - start_time:.1f} seconds")
    print(f"Packets processed: {packets_processed}")
    print(f"Tokens found: {len(found_tokens)}/{len(TARGET_TOKENS)}")
    print()
    
    if found_tokens:
        print("Final Values:")
        print("-"*100)
        for token in TARGET_TOKENS.keys():
            if token in found_tokens:
                updates = found_tokens[token]
                latest = updates[-1]
                expected = TARGET_TOKENS[token]['expected_ltp']
                diff_pct = abs(latest['ltp'] - expected) / expected * 100 if expected > 0 else 0
                
                status = "✅ PASS" if diff_pct < 5 else "⚠️ WARN" if diff_pct < 10 else "❌ FAIL"
                
                print(f"{status} {TARGET_TOKENS[token]['name']:<30}")
                print(f"       Latest LTP: ₹{latest['ltp']:>8,.2f}")
                print(f"       Expected:   ₹{expected:>8,.2f}")
                print(f"       Difference: {diff_pct:>5.1f}%")
                print(f"       Updates:    {len(updates)}")
                print()
            else:
                print(f"❌ MISS {TARGET_TOKENS[token]['name']:<30} - NOT FOUND")
                print()
    else:
        print("❌ No target tokens found in captured packets!")
        print("   Possible reasons:")
        print("   - Market is closed")
        print("   - No trading activity for these contracts")
        print("   - Incorrect multicast configuration")
    
    print(f"\nOutput saved to:")
    print(f"  {csv_path}")
    print(f"  {json_path}")

if __name__ == '__main__':
    test_live_sensex()
