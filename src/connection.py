"""
BSE UDP Multicast Connection Module
====================================

Handles UDP multicast socket setup and connection to BSE NFCAST feed.

Key Features:
- IGMPv2 multicast group join
- Configurable buffer size (2000 bytes recommended by BSE)
- Error handling for socket operations
- Support for both simulation and production environments

Protocol Details:
- Transport: UDP (unreliable, self-contained packets)
- Multicast Protocol: IGMPv2
- Buffer Size: 2000 bytes (per BSE specification)
- Byte Order: Big-endian (network byte order)

Multicast IPs:
- Simulation Equity NFCAST: 226.1.0.1:11401
- Production Equity NFCAST: 227.0.0.21:12996
"""

import socket
import struct
import logging
from typing import Optional

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class BSEMulticastConnection:
    """
    Manages UDP multicast connection to BSE NFCAST feed.
    
    This class handles:
    - Socket creation and configuration
    - Multicast group joining (IGMPv2)
    - Buffer size management
    - Error handling and logging
    """
    
    def __init__(self, multicast_ip: str, port: int, buffer_size: int = 2000):
        """
        Initialize BSE multicast connection parameters.
        
        Args:
            multicast_ip: Multicast group IP address (e.g., "226.1.0.1")
            port: UDP port number (e.g., 11401)
            buffer_size: Socket receive buffer size in bytes (default: 2000)
        
        Note:
            Buffer size of 2000 bytes is recommended by BSE for optimal performance.
        """
        self.multicast_ip = multicast_ip
        self.port = port
        self.buffer_size = buffer_size
        self.socket: Optional[socket.socket] = None
        
        logger.info(f"Initialized BSE connection parameters: {multicast_ip}:{port}")
    
    def connect(self) -> socket.socket:
        """
        Establish UDP multicast connection to BSE NFCAST feed.
        
        Steps:
        1. Create UDP socket with IPPROTO_UDP
        2. Set SO_REUSEADDR to allow multiple listeners
        3. Bind to multicast port (any interface)
        4. Join multicast group using IP_ADD_MEMBERSHIP
        5. Set receive buffer size (SO_RCVBUF)
        
        Returns:
            socket.socket: Connected UDP socket ready to receive data
        
        Raises:
            OSError: If socket creation or configuration fails
            ValueError: If invalid multicast IP or port
        
        Example:
            >>> conn = BSEMulticastConnection("226.1.0.1", 11401)
            >>> sock = conn.connect()
            >>> data, addr = sock.recvfrom(2000)
        """
        try:
            # Step 1: Create UDP socket
            logger.info("Creating UDP socket...")
            self.socket = socket.socket(
                socket.AF_INET,      # IPv4
                socket.SOCK_DGRAM,   # UDP
                socket.IPPROTO_UDP   # UDP protocol
            )
            
            # Step 2: Set socket options - allow address reuse
            # This allows multiple programs to listen on same multicast group
            logger.info("Setting SO_REUSEADDR option...")
            self.socket.setsockopt(
                socket.SOL_SOCKET,
                socket.SO_REUSEADDR,
                1
            )
            
            # Step 3: Bind to multicast port on all interfaces
            # Empty string '' means bind to all available network interfaces
            bind_address = ('', self.port)
            logger.info(f"Binding to port {self.port} on all interfaces...")
            self.socket.bind(bind_address)
            
            # Step 4: Join multicast group using IGMPv2
            # mreq structure: [4 bytes multicast IP][4 bytes interface IP]
            # socket.INADDR_ANY (0.0.0.0) means use default interface
            logger.info(f"Joining multicast group {self.multicast_ip}...")
            mreq = struct.pack(
                '4sl',  # Format: 4 bytes + long (interface)
                socket.inet_aton(self.multicast_ip),  # Multicast group IP
                socket.INADDR_ANY  # Use default network interface
            )
            self.socket.setsockopt(
                socket.IPPROTO_IP,
                socket.IP_ADD_MEMBERSHIP,
                mreq
            )
            
            # Step 5: Set receive buffer size
            # BSE recommends 2000 bytes for optimal packet reception
            logger.info(f"Setting receive buffer size to {self.buffer_size} bytes...")
            self.socket.setsockopt(
                socket.SOL_SOCKET,
                socket.SO_RCVBUF,
                self.buffer_size
            )
            
            # Step 6: Set socket timeout to allow Ctrl+C to work
            # Timeout of 1 second allows checking for shutdown signals
            logger.info("Setting socket timeout to 1 second...")
            self.socket.settimeout(1.0)
            
            logger.info(f"✓ Successfully connected to BSE NFCAST: {self.multicast_ip}:{self.port}")
            logger.info(f"✓ Multicast group joined using IGMPv2 protocol")
            logger.info(f"✓ Ready to receive market data packets (buffer: {self.buffer_size} bytes)")
            logger.info(f"✓ Socket timeout: 1.0 seconds (allows graceful shutdown)")
            
            return self.socket
            
        except socket.error as e:
            logger.error(f"✗ Socket error during connection: {e}")
            if self.socket:
                self.socket.close()
            raise
        except Exception as e:
            logger.error(f"✗ Unexpected error during connection: {e}")
            if self.socket:
                self.socket.close()
            raise
    
    def disconnect(self):
        """
        Close the multicast socket connection.
        
        Properly leaves the multicast group and closes the socket.
        Should be called during cleanup or shutdown.
        """
        if self.socket:
            try:
                logger.info(f"Disconnecting from {self.multicast_ip}:{self.port}...")
                
                # Leave multicast group
                mreq = struct.pack(
                    '4sl',
                    socket.inet_aton(self.multicast_ip),
                    socket.INADDR_ANY
                )
                self.socket.setsockopt(
                    socket.IPPROTO_IP,
                    socket.IP_DROP_MEMBERSHIP,
                    mreq
                )
                
                # Close socket
                self.socket.close()
                logger.info("✓ Disconnected successfully")
                
            except Exception as e:
                logger.error(f"Error during disconnect: {e}")
            finally:
                self.socket = None
    
    def __enter__(self):
        """Context manager entry - establish connection."""
        return self.connect()
    
    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit - cleanup connection."""
        self.disconnect()


def create_connection(config: dict) -> BSEMulticastConnection:
    """
    Factory function to create BSE connection from configuration.
    
    Args:
        config: Configuration dictionary with 'multicast' section
    
    Returns:
        BSEMulticastConnection: Configured connection object
    
    Example:
        >>> config = {'multicast': {'ip': '239.1.2.5', 'port': 26002}}
        >>> conn = create_connection(config)
        >>> sock = conn.connect()
    """
    multicast_config = config.get('multicast', {})
    
    return BSEMulticastConnection(
        multicast_ip=multicast_config.get('ip', '239.1.2.5'),
        port=multicast_config.get('port', 26002),
        buffer_size=config.get('buffer_size', 2000)
    )


# Example usage (for testing this module independently)
if __name__ == '__main__':
    # Test connection setup
    print("Testing BSE Multicast Connection...")
    
    # Simulation environment configuration
    test_config = {
        'multicast': {
            'ip': '239.1.2.5',
            'port': 26002
        },
        'buffer_size': 2000
    }
    
    # Create connection
    connection = create_connection(test_config)
    
    # Test connection (will timeout if no BSE network available)
    try:
        sock = connection.connect()
        print("\n✓ Connection test successful!")
        print(f"Socket ready to receive on {test_config['multicast']['ip']}:{test_config['multicast']['port']}")
        connection.disconnect()
    except Exception as e:
        print(f"\n✗ Connection test failed: {e}")
        print("Note: This is expected if not connected to BSE network")
