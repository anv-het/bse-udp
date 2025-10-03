"""
BSE Connection Module Tests
============================

Tests for UDP multicast connection functionality.

Test Coverage:
- Socket creation and configuration
- Multicast group joining
- Buffer size configuration
- Error handling
- Connection cleanup

Note: These tests may fail if not connected to BSE network.
Some tests use mock objects to avoid network dependencies.

Author: BSE Integration Team
Phase: Phase 1 - Connection Testing
"""

import unittest
import socket
import struct
from unittest.mock import Mock, patch, MagicMock
import sys
from pathlib import Path

# Add src to path
sys.path.insert(0, str(Path(__file__).parent.parent / 'src'))

from src.connection import BSEMulticastConnection, create_connection


class TestBSEMulticastConnection(unittest.TestCase):
    """Test cases for BSEMulticastConnection class."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.test_ip = '226.1.0.1'
        self.test_port = 11401
        self.test_buffer = 2000
    
    def test_initialization(self):
        """Test connection object initialization."""
        conn = BSEMulticastConnection(self.test_ip, self.test_port, self.test_buffer)
        
        self.assertEqual(conn.multicast_ip, self.test_ip)
        self.assertEqual(conn.port, self.test_port)
        self.assertEqual(conn.buffer_size, self.test_buffer)
        self.assertIsNone(conn.socket)
    
    def test_initialization_defaults(self):
        """Test initialization with default buffer size."""
        conn = BSEMulticastConnection(self.test_ip, self.test_port)
        
        self.assertEqual(conn.buffer_size, 2000)  # Default
    
    @patch('socket.socket')
    def test_connect_success(self, mock_socket):
        """Test successful connection establishment."""
        # Mock socket object
        mock_sock_instance = MagicMock()
        mock_socket.return_value = mock_sock_instance
        
        # Create connection and connect
        conn = BSEMulticastConnection(self.test_ip, self.test_port)
        result = conn.connect()
        
        # Verify socket was created with correct parameters
        mock_socket.assert_called_once_with(
            socket.AF_INET,
            socket.SOCK_DGRAM,
            socket.IPPROTO_UDP
        )
        
        # Verify socket options were set
        self.assertTrue(mock_sock_instance.setsockopt.called)
        
        # Verify bind was called
        mock_sock_instance.bind.assert_called_once_with(('', self.test_port))
        
        # Verify result is the socket
        self.assertEqual(result, mock_sock_instance)
    
    @patch('socket.socket')
    def test_connect_socket_error(self, mock_socket):
        """Test connection failure handling."""
        # Mock socket to raise error
        mock_socket.side_effect = socket.error("Network unreachable")
        
        conn = BSEMulticastConnection(self.test_ip, self.test_port)
        
        # Should raise socket.error
        with self.assertRaises(socket.error):
            conn.connect()
    
    @patch('socket.socket')
    def test_disconnect(self, mock_socket):
        """Test disconnection cleanup."""
        mock_sock_instance = MagicMock()
        mock_socket.return_value = mock_sock_instance
        
        conn = BSEMulticastConnection(self.test_ip, self.test_port)
        conn.connect()
        conn.disconnect()
        
        # Verify close was called
        mock_sock_instance.close.assert_called_once()
        self.assertIsNone(conn.socket)
    
    @patch('socket.socket')
    def test_context_manager(self, mock_socket):
        """Test context manager protocol."""
        mock_sock_instance = MagicMock()
        mock_socket.return_value = mock_sock_instance
        
        conn = BSEMulticastConnection(self.test_ip, self.test_port)
        
        with conn as sock:
            self.assertEqual(sock, mock_sock_instance)
        
        # Verify cleanup happened
        mock_sock_instance.close.assert_called_once()


class TestConnectionFactory(unittest.TestCase):
    """Test cases for connection factory function."""
    
    def test_create_connection_from_config(self):
        """Test creating connection from config dictionary."""
        config = {
            'multicast': {
                'ip': '226.1.0.1',
                'port': 11401
            },
            'buffer_size': 2000
        }
        
        conn = create_connection(config)
        
        self.assertIsInstance(conn, BSEMulticastConnection)
        self.assertEqual(conn.multicast_ip, '226.1.0.1')
        self.assertEqual(conn.port, 11401)
        self.assertEqual(conn.buffer_size, 2000)
    
    def test_create_connection_defaults(self):
        """Test factory function with missing config values."""
        config = {}
        
        conn = create_connection(config)
        
        # Should use defaults
        self.assertEqual(conn.multicast_ip, '226.1.0.1')
        self.assertEqual(conn.port, 11401)
        self.assertEqual(conn.buffer_size, 2000)


class TestMulticastParameters(unittest.TestCase):
    """Test multicast-specific parameters."""
    
    def test_mreq_structure(self):
        """Test multicast request structure format."""
        multicast_ip = '226.1.0.1'
        
        # Test struct.pack format used for IP_ADD_MEMBERSHIP
        mreq = struct.pack(
            '4sl',
            socket.inet_aton(multicast_ip),
            socket.INADDR_ANY
        )
        
        # Verify length (4 bytes IP + 4 bytes interface = 8 bytes)
        self.assertEqual(len(mreq), 8)
    
    def test_inet_aton_conversion(self):
        """Test IP address conversion."""
        test_ip = '226.1.0.1'
        
        # Convert to binary
        binary_ip = socket.inet_aton(test_ip)
        
        # Verify length and type
        self.assertEqual(len(binary_ip), 4)
        self.assertIsInstance(binary_ip, bytes)


# Test execution
if __name__ == '__main__':
    print("="*70)
    print("Running BSE Connection Tests")
    print("="*70)
    print()
    
    # Run tests
    unittest.main(verbosity=2)
