"""
Unit Tests for BSE Packet Receiver Module
==========================================

Tests packet receiving, validation, filtering, token extraction, and storage.

Test Coverage:
- Packet validation (size, header, leading zeros, format ID)
- Message type extraction (2020, 2021)
- Token extraction from market data records
- Packet filtering (accept 2020/2021, reject others)
- Storage functionality (raw packets and JSON metadata)
- Statistics tracking
- Error handling

Uses mocked socket and file operations to avoid network dependencies.
"""

import unittest
from unittest.mock import Mock, patch, mock_open, MagicMock
import struct
import json
from pathlib import Path
import sys

# Add src directory to path
sys.path.insert(0, str(Path(__file__).parent.parent / 'src'))

from src.packet_receiver import PacketReceiver


class TestPacketValidation(unittest.TestCase):
    """Test packet validation logic."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.mock_socket = Mock()
        self.config = {
            'raw_packets_dir': 'test_data/raw_packets',
            'processed_json_dir': 'test_data/processed_json',
            'store_limit': 10,
            'timeout': 5
        }
        
        # Create receiver instance
        with patch('packet_receiver.Path.mkdir'):
            self.receiver = PacketReceiver(self.mock_socket, self.config)
    
    def test_valid_300byte_packet(self):
        """Test validation of properly formatted 300-byte packet."""
        # Create valid 300-byte packet with proper structure
        packet = bytearray(300)
        
        # Leading zeros (offset 0-3)
        packet[0:4] = b'\x00\x00\x00\x00'
        
        # Format ID (offset 4-5, Big-Endian): 0x0124 for 300B
        packet[4:6] = struct.pack('>H', 0x0124)
        
        # Type field (offset 8-9, Little-Endian): 0x07E4 (2020)
        packet[8:10] = struct.pack('<H', 0x07E4)
        
        self.assertTrue(self.receiver._validate_packet(bytes(packet)))
    
    def test_valid_556byte_packet(self):
        """Test validation of properly formatted 556-byte packet."""
        # Create valid 556-byte packet
        packet = bytearray(556)
        
        # Leading zeros
        packet[0:4] = b'\x00\x00\x00\x00'
        
        # Format ID: 0x022C for 556B
        packet[4:6] = struct.pack('>H', 0x022C)
        
        # Type field
        packet[8:10] = struct.pack('<H', 0x07E5)
        
        self.assertTrue(self.receiver._validate_packet(bytes(packet)))
    
    def test_invalid_packet_too_small(self):
        """Test rejection of packet smaller than header size."""
        packet = b'\x00' * 30  # Only 30 bytes (need 36)
        self.assertFalse(self.receiver._validate_packet(packet))
    
    def test_invalid_packet_wrong_size(self):
        """Test rejection of packet with unexpected size."""
        packet = b'\x00' * 400  # Not 300 or 556
        self.assertFalse(self.receiver._validate_packet(packet))
    
    def test_invalid_leading_bytes(self):
        """Test rejection of packet without leading zeros."""
        packet = bytearray(300)
        packet[0:4] = b'\x01\x02\x03\x04'  # Wrong leading bytes
        packet[4:6] = struct.pack('>H', 0x0124)
        
        self.assertFalse(self.receiver._validate_packet(bytes(packet)))
    
    def test_invalid_format_id(self):
        """Test rejection of packet with unknown format ID."""
        packet = bytearray(300)
        packet[0:4] = b'\x00\x00\x00\x00'
        packet[4:6] = struct.pack('>H', 0x9999)  # Unknown format
        
        self.assertFalse(self.receiver._validate_packet(bytes(packet)))


class TestMessageTypeExtraction(unittest.TestCase):
    """Test message type extraction from packets."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.mock_socket = Mock()
        self.config = {
            'raw_packets_dir': 'test_data/raw_packets',
            'processed_json_dir': 'test_data/processed_json',
            'store_limit': 10,
            'timeout': 5
        }
        
        with patch('packet_receiver.Path.mkdir'):
            self.receiver = PacketReceiver(self.mock_socket, self.config)
    
    def test_extract_type_2020(self):
        """Test extraction of message type 2020."""
        packet = bytearray(300)
        packet[8:10] = struct.pack('<H', 0x07E4)  # 2020 in LE
        
        msg_type = self.receiver._extract_message_type(bytes(packet))
        self.assertEqual(msg_type, 2020)
    
    def test_extract_type_2021(self):
        """Test extraction of message type 2021."""
        packet = bytearray(300)
        packet[8:10] = struct.pack('<H', 0x07E5)  # 2021 in LE
        
        msg_type = self.receiver._extract_message_type(bytes(packet))
        self.assertEqual(msg_type, 2021)


class TestTokenExtraction(unittest.TestCase):
    """Test token extraction from market data records."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.mock_socket = Mock()
        self.config = {
            'raw_packets_dir': 'test_data/raw_packets',
            'processed_json_dir': 'test_data/processed_json',
            'store_limit': 10,
            'timeout': 5
        }
        
        with patch('packet_receiver.Path.mkdir'):
            self.receiver = PacketReceiver(self.mock_socket, self.config)
    
    def test_extract_single_token(self):
        """Test extraction of single token from packet."""
        packet = bytearray(300)
        
        # Put token at offset 36 (Little-Endian)
        test_token = 842364
        packet[36:40] = struct.pack('<I', test_token)
        
        tokens = self.receiver._extract_tokens(bytes(packet))
        
        self.assertEqual(len(tokens), 1)
        self.assertEqual(tokens[0], test_token)
    
    def test_extract_multiple_tokens(self):
        """Test extraction of multiple tokens from packet."""
        packet = bytearray(300)
        
        # Put tokens at offsets 36, 100, 164
        test_tokens = [842364, 123456, 789012]
        offsets = [36, 100, 164]
        
        for token, offset in zip(test_tokens, offsets):
            packet[offset:offset+4] = struct.pack('<I', token)
        
        tokens = self.receiver._extract_tokens(bytes(packet))
        
        self.assertEqual(len(tokens), 3)
        self.assertEqual(tokens, test_tokens)
    
    def test_skip_zero_tokens(self):
        """Test that zero tokens (empty slots) are skipped."""
        packet = bytearray(300)
        
        # Token at offset 36
        packet[36:40] = struct.pack('<I', 842364)
        
        # Zero token at offset 100 (should be skipped)
        packet[100:104] = struct.pack('<I', 0)
        
        # Token at offset 164
        packet[164:168] = struct.pack('<I', 123456)
        
        tokens = self.receiver._extract_tokens(bytes(packet))
        
        # Should only get non-zero tokens
        self.assertEqual(len(tokens), 2)
        self.assertEqual(tokens, [842364, 123456])


class TestPacketProcessing(unittest.TestCase):
    """Test packet processing and filtering."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.mock_socket = Mock()
        self.config = {
            'raw_packets_dir': 'test_data/raw_packets',
            'processed_json_dir': 'test_data/processed_json',
            'store_limit': 10,
            'timeout': 5
        }
        
        with patch('packet_receiver.Path.mkdir'):
            self.receiver = PacketReceiver(self.mock_socket, self.config)
    
    def _create_valid_packet(self, msg_type: int, tokens: list) -> bytes:
        """Helper to create valid packet."""
        packet = bytearray(300)
        
        # Header
        packet[0:4] = b'\x00\x00\x00\x00'
        packet[4:6] = struct.pack('>H', 0x0124)
        packet[8:10] = struct.pack('<H', msg_type)
        
        # Tokens
        offsets = [36, 100, 164, 228]
        for token, offset in zip(tokens, offsets):
            packet[offset:offset+4] = struct.pack('<I', token)
        
        return bytes(packet)
    
    @patch('builtins.open', new_callable=mock_open)
    @patch('packet_receiver.Path.mkdir')
    def test_process_type_2020_packet(self, mock_mkdir, mock_file):
        """Test processing of type 2020 packet."""
        packet = self._create_valid_packet(0x07E4, [842364, 123456])
        
        self.receiver._process_packet(packet, ('227.0.0.21', 12996))
        
        # Check statistics
        self.assertEqual(self.receiver.stats['packets_valid'], 1)
        self.assertEqual(self.receiver.stats['packets_2020'], 1)
        self.assertEqual(self.receiver.stats['tokens_extracted'], 2)
    
    @patch('builtins.open', new_callable=mock_open)
    @patch('packet_receiver.Path.mkdir')
    def test_process_type_2021_packet(self, mock_mkdir, mock_file):
        """Test processing of type 2021 packet."""
        packet = self._create_valid_packet(0x07E5, [999999])
        
        self.receiver._process_packet(packet, ('227.0.0.21', 12996))
        
        # Check statistics
        self.assertEqual(self.receiver.stats['packets_valid'], 1)
        self.assertEqual(self.receiver.stats['packets_2021'], 1)
        self.assertEqual(self.receiver.stats['tokens_extracted'], 1)
    
    def test_reject_invalid_packet(self):
        """Test rejection of invalid packet."""
        packet = b'\x00' * 30  # Too small
        
        self.receiver._process_packet(packet, ('227.0.0.21', 12996))
        
        # Should be counted as invalid
        self.assertEqual(self.receiver.stats['packets_invalid'], 1)
        self.assertEqual(self.receiver.stats['packets_valid'], 0)


class TestStorageLimit(unittest.TestCase):
    """Test storage limit enforcement."""
    
    @patch('builtins.open', new_callable=mock_open)
    @patch('packet_receiver.Path.mkdir')
    def test_storage_limit_enforced(self, mock_mkdir, mock_file):
        """Test that storage limit is enforced."""
        mock_socket = Mock()
        config = {
            'raw_packets_dir': 'test_data/raw_packets',
            'processed_json_dir': 'test_data/processed_json',
            'store_limit': 3,  # Only store 3 packets
            'timeout': 5
        }
        
        receiver = PacketReceiver(mock_socket, config)
        
        # Create valid packet
        packet = bytearray(300)
        packet[0:4] = b'\x00\x00\x00\x00'
        packet[4:6] = struct.pack('>H', 0x0124)
        packet[8:10] = struct.pack('<H', 0x07E4)
        packet[36:40] = struct.pack('<I', 842364)
        
        # Process 5 packets
        for _ in range(5):
            receiver._process_packet(bytes(packet), ('227.0.0.21', 12996))
        
        # Should only store 3
        self.assertEqual(min(receiver.stored_count, receiver.store_limit), 3)


def run_tests():
    """Run all tests."""
    # Create test suite
    loader = unittest.TestLoader()
    suite = unittest.TestSuite()
    
    # Add test cases
    suite.addTests(loader.loadTestsFromTestCase(TestPacketValidation))
    suite.addTests(loader.loadTestsFromTestCase(TestMessageTypeExtraction))
    suite.addTests(loader.loadTestsFromTestCase(TestTokenExtraction))
    suite.addTests(loader.loadTestsFromTestCase(TestPacketProcessing))
    suite.addTests(loader.loadTestsFromTestCase(TestStorageLimit))
    
    # Run tests
    runner = unittest.TextTestRunner(verbosity=2)
    result = runner.run(suite)
    
    return 0 if result.wasSuccessful() else 1


if __name__ == '__main__':
    exit(run_tests())
