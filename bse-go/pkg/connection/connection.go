package connection

import (
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/net/ipv4"
)

type BSEConnection struct {
	multicastIP string
	port        int
	bufferSize  int
	conn        *net.UDPConn
	stopChan    chan struct{}
}

func NewBSEConnection(multicastIP string, port, bufferSize int) *BSEConnection {
	ip := net.ParseIP(multicastIP)
	if ip == nil {
		log.Fatalf("Invalid multicast IP: %s", multicastIP)
	}
	return &BSEConnection{
		multicastIP: multicastIP,
		port:        port,
		bufferSize:  bufferSize,
		stopChan:    make(chan struct{}),
	}
}

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
	log.Printf("Joining multicast group %s...", c.multicastIP)
	group := net.ParseIP(c.multicastIP)
	if group == nil {
		conn.Close()
		return fmt.Errorf("invalid multicast IP: %s", c.multicastIP)
	}

	// Create ipv4 packet connection for multicast operations
	p := ipv4.NewPacketConn(conn)

	// Join the multicast group
	if err := p.JoinGroup(nil, &net.UDPAddr{IP: group, Port: c.port}); err != nil {
		log.Printf("Warning: failed to join multicast group: %v", err)
		// Continue anyway - some systems don't require explicit joining
	}

	// Set buffer size
	if err := conn.SetReadBuffer(c.bufferSize); err != nil {
		log.Printf("Warning: failed to set read buffer: %v", err)
	}

	// Set timeout
	if err := conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		conn.Close()
		return fmt.Errorf("failed to set timeout: %w", err)
	}

	c.conn = conn
	log.Printf("✓ Connected to BSE NFCAST: %s:%d", c.multicastIP, c.port)
	return nil
}

func (c *BSEConnection) ReceiveLoop(packets chan<- []byte) {
	defer close(packets)

	buffer := make([]byte, c.bufferSize)
	timeoutCount := 0

	for {
		// Check for stop signal
		select {
		case <-c.stopChan:
			log.Println("🛑 ReceiveLoop stopping...")
			return
		default:
		}

		n, _, err := c.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				timeoutCount++
				if timeoutCount >= 30 {
					log.Println("⏱️ Still waiting for packets...")
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
				log.Printf("Read error: %v", err)
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
			log.Println("Warning: packet channel full, dropping packet")
		}
	}
}

func (c *BSEConnection) Stop() {
	close(c.stopChan)
}

func (c *BSEConnection) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}
