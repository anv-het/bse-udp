"""
BSE UDP Market Data Reader - Main Application
==============================================

Phase 1-3 Complete: Full pipeline from UDP multicast to JSON/CSV output

This script:
1. Loads configuration and token map (contract master)
2. Establishes UDP multicast connection to BSE NFCAST
3. Receives packets and filters for types 2020/2021
4. **Phase 3: Decodes packet headers and base values**
5. **Phase 3: Decompresses NFCAST differential fields**
6. **Phase 3: Collects normalized quotes with symbol resolution**
7. **Phase 3: Saves to JSON/CSV with timestamps**
8. Handles graceful shutdown with comprehensive statistics

Completed:
- Phase 1: Project structure and UDP multicast connection
- Phase 2: Packet receiving, filtering, token extraction, storage
- **Phase 3: Full decoding, decompression, normalization, and output**

Future phases:
- Phase 4: BOLTPLUS authentication
- Phase 5: Contract master synchronization via API

Usage:
    python src/main.py

Requirements:
- config.json in project root
- data/tokens/token_details.json (contract master with ~29k tokens)
- Network access to BSE multicast (simulation: 226.1.0.1:11401)
- IGMPv2 multicast support on network interface
- BSE Market hours: 9:00 AM - 3:30 PM IST (Mon-Fri)

Author: BSE Integration Team
Phase: Phase 3 - COMPLETE
Date: January 2025
"""

import sys
import json
import logging
import signal
import socket
from pathlib import Path

# Add src directory to path for imports
sys.path.insert(0, str(Path(__file__).parent))

from connection import create_connection, BSEMulticastConnection
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

# Global flag for graceful shutdown
running = True


def signal_handler(signum, frame):
    """Handle Ctrl+C gracefully."""
    global running
    logger.info("\n⚠ Shutdown signal received (Ctrl+C)")
    running = False


def load_config(config_path: str = 'config.json') -> dict:
    """
    Load configuration from JSON file.
    
    Args:
        config_path: Path to configuration file
    
    Returns:
        dict: Configuration dictionary
    
    Raises:
        FileNotFoundError: If config file doesn't exist
        json.JSONDecodeError: If config file is invalid JSON
    """
    try:
        logger.info(f"Loading configuration from {config_path}...")
        with open(config_path, 'r') as f:
            config = json.load(f)
        
        # Log configuration (without sensitive data)
        multicast = config.get('multicast', {})
        logger.info(f"✓ Configuration loaded successfully")
        logger.info(f"  Environment: {multicast.get('env', 'unknown')}")
        logger.info(f"  Segment: {multicast.get('segment', 'unknown')}")
        logger.info(f"  Multicast IP: {multicast.get('ip', 'unknown')}")
        logger.info(f"  Port: {multicast.get('port', 'unknown')}")
        logger.info(f"  Buffer Size: {config.get('buffer_size', 2000)} bytes")
        
        return config
        
    except FileNotFoundError:
        logger.error(f"✗ Configuration file not found: {config_path}")
        logger.error("  Please ensure config.json exists in project root")
        raise
    except json.JSONDecodeError as e:
        logger.error(f"✗ Invalid JSON in configuration file: {e}")
        raise


def load_token_map(token_path: str = 'data/tokens/token_details.json') -> dict:
    """
    Load token mapping (contract master) from JSON file.
    
    Token map maps BSE token IDs to contract details (symbol, expiry, strike, etc.)
    Used by Phase 3 data collector to resolve tokens to symbols.
    
    Args:
        token_path: Path to token details file
    
    Returns:
        dict: Token mapping {token_id: {symbol, expiry, option_type, strike_price, ...}, ...}
    
    Raises:
        FileNotFoundError: If token file doesn't exist
        json.JSONDecodeError: If token file is invalid JSON
    """
    try:
        logger.info(f"Loading token map from {token_path}...")
        with open(token_path, 'r') as f:
            token_map = json.load(f)
        
        logger.info(f"✓ Token map loaded successfully: {len(token_map):,} tokens")
        
        # Log sample tokens for verification
        sample_tokens = list(token_map.keys())[:3]
        logger.info(f"  Sample tokens: {', '.join(sample_tokens)}")
        
        return token_map
        
    except FileNotFoundError:
        logger.error(f"✗ Token map file not found: {token_path}")
        logger.error("  Please ensure token_details.json exists in data/tokens/")
        logger.warning("  Continuing without token map (symbols will be 'UNKNOWN')")
        return {}
        
    except json.JSONDecodeError as e:
        logger.error(f"✗ Invalid JSON in token map file: {e}")
        logger.warning("  Continuing without token map (symbols will be 'UNKNOWN')")
        return {}


def receive_packets(sock, buffer_size: int = 2000):
    """
    Main packet receive loop (Phase 1: logging only).
    
    In Phase 1, this function:
    - Receives UDP packets continuously
    - Logs packet length and address
    - Does NOT process/decode packets yet (future phase)
    
    Args:
        sock: Connected UDP socket
        buffer_size: Size of receive buffer (default: 2000 bytes)
    
    Note:
        This is a placeholder loop. Future phases will add:
        - Packet validation and parsing
        - Data extraction and normalization
        - JSON/CSV output
    """
    global running
    
    logger.info("\n" + "="*70)
    logger.info("📡 Starting packet receive loop...")
    logger.info("="*70)
    logger.info("Phase 1: Logging packet information only (no processing)")
    logger.info("Press Ctrl+C to stop\n")
    
    packet_count = 0
    total_bytes = 0
    
    try:
        while running:
            try:
                # Receive packet from UDP socket
                # recvfrom() returns (data, address)
                packet, address = sock.recvfrom(buffer_size)
                
                packet_count += 1
                packet_size = len(packet)
                total_bytes += packet_size
                
                # Log packet information (Phase 1: basic info only)
                if packet_count % 10 == 0:  # Log every 10th packet to reduce noise
                    logger.info(
                        f"📦 Packet #{packet_count}: "
                        f"Size={packet_size} bytes, "
                        f"From={address[0]}:{address[1]}, "
                        f"Total={total_bytes:,} bytes received"
                    )
                
                # Phase 1: No processing yet - just receive and log
                # Future phases will add:
                # - decoded_data = decoder.decode_packet(packet)
                # - quotes = data_collector.collect_quotes(decoded_data)
                # - saver.save_json(quotes)
                # - saver.save_csv(quotes)
                
            except socket.timeout:
                # This shouldn't happen (no timeout set), but handle anyway
                logger.warning("⏱ Socket timeout - waiting for packets...")
                continue
                
            except socket.error as e:
                logger.error(f"✗ Socket error: {e}")
                break
                
    except KeyboardInterrupt:
        # Handled by signal handler
        pass
    
    finally:
        # Log final statistics
        logger.info("\n" + "="*70)
        logger.info("📊 Session Statistics:")
        logger.info("="*70)
        logger.info(f"Total Packets Received: {packet_count:,}")
        logger.info(f"Total Bytes Received: {total_bytes:,}")
        if packet_count > 0:
            logger.info(f"Average Packet Size: {total_bytes / packet_count:.1f} bytes")
        logger.info("="*70)


def main():
    """
    Main application entry point - Phase 1-3 Complete Implementation.
    
    Workflow:
    1. Load configuration and token map (contract master)
    2. Create and establish UDP multicast connection
    3. Initialize packet receiver with Phase 3 pipeline
    4. Enter receive loop (decode → decompress → collect → save)
    5. Handle graceful shutdown with comprehensive statistics
    """
    # Register signal handler for Ctrl+C
    signal.signal(signal.SIGINT, signal_handler)
    
    logger.info("\n" + "="*70)
    logger.info("🚀 BSE UDP Market Data Reader - Phase 1-3 COMPLETE")
    logger.info("="*70)
    logger.info("Purpose: Full pipeline from UDP multicast to JSON/CSV output")
    logger.info("Status: Phase 3 - Decode, Decompress, Normalize, Save")
    logger.info("="*70 + "\n")
    
    connection = None
    sock = None
    
    try:
        # Step 1: Load configuration
        config = load_config('config.json')
        
        # Step 2: Load token map (contract master for symbol resolution)
        token_map = load_token_map('data/tokens/token_details.json')
        
        # Step 3: Create connection object
        logger.info("\n📡 Creating BSE multicast connection...")
        connection = create_connection(config)
        
        # Step 4: Establish connection
        logger.info("\n🔌 Establishing UDP multicast connection...")
        sock = connection.connect()
        
        # Success! Connection established
        logger.info("\n" + "="*70)
        logger.info("✅ CONNECTION ESTABLISHED TO BSE NFCAST")
        logger.info("="*70)
        logger.info(f"✓ Connected to: {config['multicast']['ip']}:{config['multicast']['port']}")
        logger.info(f"✓ Segment: {config['multicast']['segment']}")
        logger.info(f"✓ Environment: {config['multicast']['env']}")
        logger.info(f"✓ Buffer Size: {config.get('buffer_size', 2000)} bytes")
        logger.info(f"✓ Protocol: IGMPv2 multicast")
        logger.info(f"✓ Token Map: {len(token_map):,} tokens loaded")
        logger.info("="*70 + "\n")
        
        # Step 5: Initialize packet receiver with Phase 3 pipeline
        logger.info("📦 Initializing packet receiver with Phase 3 pipeline...")
        receiver_config = {
            'raw_packets_dir': 'data/raw_packets',
            'processed_json_dir': 'data/processed_json',
            'store_limit': config.get('store_limit', 100),
            'timeout': config.get('timeout', 30)
        }
        receiver = PacketReceiver(sock, receiver_config, token_map)
        logger.info("✓ Packet receiver initialized")
        logger.info("✓ Phase 3 pipeline enabled: decode → decompress → collect → save\n")
        
        # Step 6: Enter receive loop
        # This will:
        # - Receive packets continuously
        # - Filter for types 2020/2021
        # - Decode headers + base values
        # - Decompress differential fields
        # - Collect normalized quotes
        # - Save to JSON/CSV
        # - Store raw packets + metadata (Phase 2 compatibility)
        receiver.receive_loop()
        
    except FileNotFoundError:
        logger.error("\n❌ Configuration file not found")
        logger.error("Please create config.json in project root")
        return 1
        
    except Exception as e:
        logger.error(f"\n❌ Application error: {e}")
        logger.exception("Full error traceback:")
        return 1
        
    finally:
        # Cleanup: Disconnect from multicast
        if connection:
            logger.info("\n🔌 Cleaning up connection...")
            connection.disconnect()
        
        logger.info("\n" + "="*70)
        logger.info("👋 BSE UDP Reader shutdown complete")
        logger.info("="*70 + "\n")
    
    return 0


if __name__ == '__main__':
    # Run main application
    exit_code = main()
    sys.exit(exit_code)
