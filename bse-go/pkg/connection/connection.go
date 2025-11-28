package connection

import (
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/net/ipv4"
)

// BSEConnection manages UDP multicast connection to BSE NFCAST feed
type BSEConnection struct {
	multicastIP string
	port        int
	bufferSize  int
	segment     string // "CM" or "FO"
	conn        *net.UDPConn
	stopChan    chan struct{}
}

// NewBSEConnection creates a new BSE multicast connection
func NewBSEConnection(multicastIP string, port, bufferSize int, segment string) *BSEConnection {
	ip := net.ParseIP(multicastIP)
	if ip == nil {
		log.Fatalf("Invalid multicast IP: %s", multicastIP)
	}
	return &BSEConnection{
		multicastIP: multicastIP,
		port:        port,
		bufferSize:  bufferSize,
		segment:     segment,
		stopChan:    make(chan struct{}),
	}
}

// Connect establishes UDP multicast connection to BSE NFCAST feed
func (c *BSEConnection) Connect() error {
	// Create UDP socket
	addr := net.UDPAddr{
		IP:   net.IPv4zero,
		Port: c.port,
	}

	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		return fmt.Errorf("failed to create UDP socket: %w", err)
	}

	// Join multicast group using ipv4 package for proper IGMPv2 support
	log.Printf("[%s] Joining multicast group %s:%d...", c.segment, c.multicastIP, c.port)
	group := net.ParseIP(c.multicastIP)
	if group == nil {
		conn.Close()
		return fmt.Errorf("invalid multicast IP: %s", c.multicastIP)
	}

	// Create ipv4 packet connection for multicast operations
	p := ipv4.NewPacketConn(conn)

	// Join the multicast group
	if err := p.JoinGroup(nil, &net.UDPAddr{IP: group, Port: c.port}); err != nil {
		log.Printf("[%s] Warning: failed to join multicast group: %v", c.segment, err)
		// Continue anyway - some systems don't require explicit joining
	}

	// Set buffer size
	if err := conn.SetReadBuffer(c.bufferSize); err != nil {
		log.Printf("[%s] Warning: failed to set read buffer: %v", c.segment, err)
	}

	// Set timeout (1 second to allow Ctrl+C handling)
	if err := conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		conn.Close()
		return fmt.Errorf("failed to set timeout: %w", err)
	}

	c.conn = conn
	log.Printf("[%s] ✓ Connected to BSE NFCAST: %s:%d", c.segment, c.multicastIP, c.port)
	return nil
}

// ReceiveLoop continuously receives packets and sends them to the channel
func (c *BSEConnection) ReceiveLoop(packets chan<- []byte) {
	defer close(packets)

	buffer := make([]byte, c.bufferSize)
	timeoutCount := 0

	for {
		// Check for stop signal
		select {
		case <-c.stopChan:
			log.Printf("[%s] 🛑 ReceiveLoop stopping...", c.segment)
			return
		default:
		}

		n, _, err := c.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				timeoutCount++
				if timeoutCount >= 30 {
					log.Printf("[%s] ⏱️ Still waiting for packets...", c.segment)
					timeoutCount = 0
				}
				// Reset timeout
				c.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
				continue
			}
			// Check if we're stopping
			select {
			case <-c.stopChan:
				return
			default:
				log.Printf("[%s] Read error: %v", c.segment, err)
				return
			}
		}

		timeoutCount = 0

		// Copy packet data
		packet := make([]byte, n)
		copy(packet, buffer[:n])

		// Try to send, but don't block if stopping
		select {
		case packets <- packet:
		case <-c.stopChan:
			return
		default:
			log.Printf("[%s] Warning: packet channel full, dropping packet", c.segment)
		}
	}
}

// GetSegment returns the segment identifier
func (c *BSEConnection) GetSegment() string {
	return c.segment
}

// Stop signals the connection to stop receiving
func (c *BSEConnection) Stop() {
	close(c.stopChan)
}

// Close closes the UDP connection
func (c *BSEConnection) Close() {
	if c.conn != nil {
		c.conn.Close()
		log.Printf("[%s] 🔌 Disconnected", c.segment)
	}
}
