"""
BSE UDP Market Data Reader - Dual Feed Implementation
======================================================

Supports simultaneous connections to:
- Port 26001: Equity Cash (CM) - ~4,689 stocks
- Port 26002: Derivatives (F&O) - ~40,000 contracts

Features:
- Configurable enable/disable for each segment via config.json
- Separate CSV files for each segment (YYYYMMDD_CM_quotes.csv, YYYYMMDD_FO_quotes.csv)
- Unified token mapping from BhavCopy + Contract Master CSVs
- NO dependency on token_details.json
- Proper Ctrl+C termination on Windows

Configuration (config.json):
    "segments": {
        "cm_enabled": true,   // Enable Equity Cash feed (port 26001)
        "fo_enabled": true    // Enable Derivatives feed (port 26002)
    }

Usage:
    python src/main.py

Author: BSE Integration Team
Date: November 2025
"""

import sys
import json
import logging
import signal
import socket
import threading
import time
from pathlib import Path
from typing import Dict, Optional

# Add src directory to path for imports
sys.path.insert(0, str(Path(__file__).parent))

from connection import BSEMulticastConnection
from packet_receiver import PacketReceiver

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler('bse_reader.log'),
        logging.StreamHandler()
    ]
)
logger = logging.getLogger(__name__)

# Global shutdown event for thread coordination
shutdown_event = threading.Event()


def signal_handler(signum, frame):
    """Handle Ctrl+C gracefully - works on Windows."""
    logger.info("\n⚠ Shutdown signal received (Ctrl+C)")
    shutdown_event.set()  # Signal all threads to stop
    # Force exit after a short delay if threads don't stop
    def force_exit():
        time.sleep(2)
        if not shutdown_event.is_set():
            return
        logger.info("Force exiting...")
        import os
        os._exit(0)
    threading.Thread(target=force_exit, daemon=True).start()


def load_config(config_path: str = 'config.json') -> dict:
    """Load configuration from JSON file."""
    try:
        logger.info(f"Loading configuration from {config_path}...")
        with open(config_path, 'r') as f:
            config = json.load(f)
        
        # Log segment configuration
        segments = config.get('segments', {})
        cm_enabled = segments.get('cm_enabled', False)
        fo_enabled = segments.get('fo_enabled', False)
        
        logger.info(f"✓ Configuration loaded successfully")
        logger.info(f"  Segments enabled:")
        logger.info(f"    CM (Equity Cash, port 26001): {'✅ ENABLED' if cm_enabled else '❌ DISABLED'}")
        logger.info(f"    FO (Derivatives, port 26002): {'✅ ENABLED' if fo_enabled else '❌ DISABLED'}")
        
        return config
        
    except FileNotFoundError:
        logger.error(f"✗ Configuration file not found: {config_path}")
        raise
    except json.JSONDecodeError as e:
        logger.error(f"✗ Invalid JSON in configuration file: {e}")
        raise


def load_token_map_unified(use_api: bool = True, keep_days: int = 2) -> dict:
    """
    Load unified token mapping from BSE daily CSV files.
    
    Uses BSETokenMapper to combine:
    - BhavCopy CSV (Equity Cash, port 26001) - ~4,689 stocks
    - Contract Master CSV (F&O, port 26002) - ~40,000 derivatives
    """
    try:
        from token_mapper import BSETokenMapper
        
        logger.info("=" * 80)
        logger.info("📥 LOADING UNIFIED TOKEN MAPPER (No JSON dependency)")
        logger.info("=" * 80)
        
        mapper = BSETokenMapper()
        
        if use_api:
            logger.info("Updating token mappings from BSE API...")
            success = mapper.update_all(keep_days=keep_days)
            
            if not success:
                logger.warning("⚠️  API fetch failed, trying cached files...")
                mapper.load_from_cache()
        else:
            logger.info("Loading from cached CSV files (API disabled)...")
            mapper.load_from_cache()
        
        # Build token_map dict
        token_map = {}
        
        # Add Equity tokens
        for token_str, contract in mapper.equity_fetcher.contracts.items():
            token_map[token_str] = {
                'symbol': contract['symbol'],
                'segment': 'EQ',
                'company_name': contract.get('company_name', ''),
                'isin': contract.get('isin', '')
            }
        
        # Add F&O tokens
        for token_str, contract in mapper.contract_manager.contracts.items():
            token_map[token_str] = {
                'symbol': contract['symbol'],
                'expiry': contract.get('expiry', ''),
                'option_type': contract.get('option_type', ''),
                'strike_price': contract.get('strike', ''),
                'lot_size': contract.get('lot_size', ''),
                'instrument_name': contract.get('instrument_name', ''),
                'full_name': contract.get('full_name', ''),
                'contract_type': contract.get('contract_type', ''),
                'segment': 'FO'
            }
        
        logger.info(f"✅ Unified token map loaded: {len(token_map):,} total contracts")
        logger.info(f"   Equity Cash: {mapper.stats['equity_tokens']:,} stocks")
        logger.info(f"   Derivatives: {mapper.stats['fo_tokens']:,} F&O contracts")
        logger.info("=" * 80)
        
        return token_map
        
    except Exception as e:
        logger.error(f"❌ Failed to load unified token map: {e}")
        logger.error("Falling back to legacy F&O-only loader...")
        return load_token_map_legacy(use_api)


def load_token_map_legacy(use_api: bool = True) -> dict:
    """LEGACY: Load token mapping from BSE daily CSV (F&O only)."""
    try:
        from contract_manager import BSEContractManager
        
        logger.info("=" * 80)
        logger.info("📥 LOADING BSE CONTRACT MASTER (F&O Only - Legacy)")
        logger.info("=" * 80)
        
        api_url = "http://192.168.102.166:2060/v1/sftp-files"
        manager = BSEContractManager(api_url)
        
        if use_api:
            manager.update_contracts()
        
        fetcher = manager.load_latest_contracts()
        
        token_map = {}
        for token_str, contract in fetcher.contracts.items():
            token_map[token_str] = {
                'symbol': contract['symbol'],
                'expiry': contract['expiry'],
                'option_type': contract['option_type'],
                'strike_price': contract['strike'],
                'lot_size': contract['lot_size'],
                'instrument_name': contract['instrument_name'],
                'full_name': contract['full_name'],
                'contract_type': contract['contract_type'],
                'segment': 'FO'
            }
        
        logger.info(f"✅ Token map loaded: {len(token_map):,} F&O contracts")
        logger.info("=" * 80)
        
        return token_map
        
    except Exception as e:
        logger.error(f"❌ Failed to load F&O contracts: {e}")
        return {}


def create_segment_connection(config: dict, segment: str) -> BSEMulticastConnection:
    """
    Create connection for a specific segment.
    
    Args:
        config: Configuration dictionary
        segment: 'CM' for Equity Cash, 'FO' for Derivatives
    
    Returns:
        BSEMulticastConnection configured for the segment
    """
    if segment == 'CM':
        mc_config = config.get('multicast_cm', {})
    else:  # 'FO'
        mc_config = config.get('multicast_fo', config.get('multicast', {}))
    
    return BSEMulticastConnection(
        multicast_ip=mc_config.get('ip', '239.1.2.5'),
        port=mc_config.get('port', 26001 if segment == 'CM' else 26002),
        buffer_size=config.get('buffer_size', 2048)
    )


def run_segment_receiver(
    config: dict,
    token_map: dict,
    segment: str,
    connection: BSEMulticastConnection
):
    """
    Run packet receiver for a specific segment in a thread.
    
    Args:
        config: Configuration dictionary
        token_map: Token mapping dictionary
        segment: 'CM' or 'FO'
        connection: BSEMulticastConnection for this segment
    """
    segment_name = "Equity Cash" if segment == 'CM' else "Derivatives (F&O)"
    port = 26001 if segment == 'CM' else 26002
    
    try:
        logger.info(f"[{segment}] Starting {segment_name} receiver on port {port}...")
        
        # Connect
        sock = connection.connect()
        
        logger.info(f"[{segment}] ✅ Connected to 239.1.2.5:{port}")
        
        # Initialize receiver with segment
        receiver_config = {
            'raw_packets_dir': 'data/raw_packets',
            'processed_json_dir': 'data/processed_json',
            'store_limit': config.get('store_limit', 100),
            'timeout': config.get('timeout', 30)
        }
        
        # Filter token map for this segment
        segment_token_map = {
            k: v for k, v in token_map.items()
            if v.get('segment') == ('EQ' if segment == 'CM' else 'FO')
        }
        
        # If segment filter results in empty map, use full map
        if not segment_token_map:
            segment_token_map = token_map
            logger.warning(f"[{segment}] Using full token map (no segment-specific tokens found)")
        else:
            logger.info(f"[{segment}] Using {len(segment_token_map):,} tokens for {segment}")
        
        receiver = PacketReceiver(sock, receiver_config, segment_token_map, segment=segment)
        
        logger.info(f"[{segment}] 📦 Receiver initialized")
        logger.info(f"[{segment}] 📁 CSV output: data/processed_csv/YYYYMMDD_{segment}_quotes.csv")
        
        # Run receive loop with shutdown check
        while not shutdown_event.is_set():
            try:
                # Receive with short timeout to check shutdown_event frequently
                sock.settimeout(0.5)
                packet, addr = sock.recvfrom(2000)
                receiver._process_packet(packet, addr)
                receiver.stats['packets_received'] += 1
                receiver.stats['bytes_received'] += len(packet)
                
                # Log every 10 packets
                if receiver.stats['packets_received'] % 10 == 0:
                    logger.info(
                        f"[{segment}] 📦 Packets: {receiver.stats['packets_received']}, "
                        f"Valid: {receiver.stats['packets_valid']}, "
                        f"Type 2020: {receiver.stats['packets_2020']}"
                    )
            except socket.timeout:
                continue  # Check shutdown_event and retry
            except Exception as e:
                if not shutdown_event.is_set():
                    logger.error(f"[{segment}] Receive error: {e}")
                break
        
        logger.info(f"[{segment}] 🛑 Receiver stopped (shutdown requested)")
        
    except Exception as e:
        logger.error(f"[{segment}] ❌ Error in receiver: {e}")
    finally:
        if connection:
            connection.disconnect()
            logger.info(f"[{segment}] 🔌 Disconnected")


def main():
    """
    Main application entry point - Dual Feed Implementation.
    
    Workflow:
    1. Load configuration
    2. Check which segments are enabled
    3. Load unified token map
    4. Create connections for enabled segments
    5. Start receiver threads
    6. Wait for shutdown signal
    """
    # Register signal handler for Ctrl+C (works on Windows)
    signal.signal(signal.SIGINT, signal_handler)
    try:
        signal.signal(signal.SIGBREAK, signal_handler)  # Windows-specific
    except AttributeError:
        pass  # Not on Windows
    
    logger.info("\n" + "=" * 70)
    logger.info("🚀 BSE UDP Market Data Reader - DUAL FEED")
    logger.info("=" * 70)
    logger.info("Purpose: Simultaneous CM + F&O data collection")
    logger.info("Outputs: YYYYMMDD_CM_quotes.csv, YYYYMMDD_FO_quotes.csv")
    logger.info("=" * 70 + "\n")
    
    connections = []
    threads = []
    
    try:
        # Step 1: Load configuration
        config = load_config('config.json')
        
        # Step 2: Check enabled segments
        segments = config.get('segments', {})
        cm_enabled = segments.get('cm_enabled', False)
        fo_enabled = segments.get('fo_enabled', False)
        
        if not cm_enabled and not fo_enabled:
            logger.error("❌ No segments enabled! Enable cm_enabled and/or fo_enabled in config.json")
            return 1
        
        # Step 3: Load unified token map
        keep_days = config.get('data_management', {}).get('keep_days', 2)
        token_map = load_token_map_unified(use_api=True, keep_days=keep_days)
        
        if not token_map:
            logger.error("❌ Failed to load token map!")
            return 1
        
        # Step 4: Create connections for enabled segments
        logger.info("\n" + "=" * 70)
        logger.info("📡 CREATING CONNECTIONS")
        logger.info("=" * 70)
        
        if cm_enabled:
            cm_connection = create_segment_connection(config, 'CM')
            connections.append(('CM', cm_connection))
            logger.info("✅ CM connection prepared (port 26001)")
        
        if fo_enabled:
            fo_connection = create_segment_connection(config, 'FO')
            connections.append(('FO', fo_connection))
            logger.info("✅ FO connection prepared (port 26002)")
        
        # Step 5: Start receiver threads (NOT daemon - for proper cleanup)
        logger.info("\n" + "=" * 70)
        logger.info("🚀 STARTING RECEIVERS")
        logger.info("=" * 70)
        
        for segment, connection in connections:
            thread = threading.Thread(
                target=run_segment_receiver,
                args=(config, token_map, segment, connection),
                name=f"Receiver-{segment}",
                daemon=False  # Non-daemon for proper cleanup
            )
            threads.append(thread)
            thread.start()
            logger.info(f"✅ Started {segment} receiver thread")
        
        logger.info("\n" + "=" * 70)
        logger.info("✅ ALL RECEIVERS RUNNING")
        logger.info("=" * 70)
        logger.info(f"Active threads: {len(threads)}")
        logger.info("Press Ctrl+C to stop all receivers")
        logger.info("=" * 70 + "\n")
        
        # Step 6: Wait for shutdown signal or threads to finish
        while not shutdown_event.is_set():
            # Check if all threads are dead
            if all(not t.is_alive() for t in threads):
                logger.info("All threads stopped")
                break
            time.sleep(0.5)  # Check every 500ms
        
        # Signal shutdown to all threads
        shutdown_event.set()
        
        # Wait for threads to finish (with timeout)
        logger.info("\n⏳ Waiting for threads to finish...")
        for thread in threads:
            thread.join(timeout=2.0)
            if thread.is_alive():
                logger.warning(f"Thread {thread.name} did not stop in time")
        
    except FileNotFoundError:
        logger.error("\n❌ Configuration file not found")
        return 1
    except Exception as e:
        logger.error(f"\n❌ Application error: {e}")
        logger.exception("Full error traceback:")
        return 1
    finally:
        # Cleanup
        shutdown_event.set()
        
        logger.info("\n🔌 Cleaning up connections...")
        for segment, connection in connections:
            try:
                connection.disconnect()
            except:
                pass
        
        logger.info("\n" + "=" * 70)
        logger.info("👋 BSE UDP Reader shutdown complete")
        logger.info("=" * 70 + "\n")
    
    return 0


if __name__ == '__main__':
    exit_code = main()
    sys.exit(exit_code)
