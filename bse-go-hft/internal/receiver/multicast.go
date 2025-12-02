// Package receiver provides multicast UDP packet reception
package receiver

import (
	"context"
	"fmt"
	"net"
	"time"

	"golang.org/x/net/ipv4"
)

// Config holds receiver configuration
type Config struct {
	MulticastIP  string
	Port         int
	BufferSize   int
	SocketRcvBuf int           // SO_RCVBUF size
	ReadTimeout  time.Duration // Socket read timeout
}

// DefaultConfig returns default receiver configuration
func DefaultConfig() Config {
	return Config{
		MulticastIP:  "239.1.2.5",
		Port:         26001,
		BufferSize:   65536,
		SocketRcvBuf: 16 * 1024 * 1024, // 16MB
		ReadTimeout:  100 * time.Millisecond,
	}
}

// PacketHandler is called for each received packet
type PacketHandler func(data []byte, length int, receiveTime time.Time)

// MulticastReceiver receives multicast UDP packets
type MulticastReceiver struct {
	config  Config
	conn    *net.UDPConn
	pconn   *ipv4.PacketConn
	handler PacketHandler

	// Statistics
	packetsReceived uint64
	bytesReceived   uint64
	errors          uint64
}

// NewMulticastReceiver creates a new multicast receiver
func NewMulticastReceiver(config Config, handler PacketHandler) *MulticastReceiver {
	return &MulticastReceiver{
		config:  config,
		handler: handler,
	}
}

// Connect joins the multicast group
func (r *MulticastReceiver) Connect() error {
	addr := fmt.Sprintf("%s:%d", r.config.MulticastIP, r.config.Port)

	// Resolve multicast address
	gaddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve address: %w", err)
	}

	// Create UDP connection
	conn, err := net.ListenMulticastUDP("udp4", nil, gaddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	r.conn = conn

	// Set socket options
	if err := r.configureSocket(); err != nil {
		conn.Close()
		return fmt.Errorf("failed to configure socket: %w", err)
	}

	// Create packet connection for multicast control
	r.pconn = ipv4.NewPacketConn(conn)

	// Enable control message for timestamps (if supported)
	if err := r.pconn.SetControlMessage(ipv4.FlagTTL, true); err != nil {
		// Not critical, continue without
	}

	return nil
}

// configureSocket sets socket options for low-latency operation
func (r *MulticastReceiver) configureSocket() error {
	// Set receive buffer size
	if err := r.conn.SetReadBuffer(r.config.SocketRcvBuf); err != nil {
		return fmt.Errorf("failed to set read buffer: %w", err)
	}

	return nil
}

// ReceiveLoop continuously receives packets until context is cancelled
func (r *MulticastReceiver) ReceiveLoop(ctx context.Context) error {
	buffer := make([]byte, r.config.BufferSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Set read deadline for non-blocking check of context
		r.conn.SetReadDeadline(time.Now().Add(r.config.ReadTimeout))

		n, _, err := r.conn.ReadFromUDP(buffer)
		receiveTime := time.Now() // Capture receive time immediately

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout is normal, check context and continue
				continue
			}
			r.errors++
			continue
		}

		if n == 0 {
			continue
		}

		// Update statistics
		r.packetsReceived++
		r.bytesReceived += uint64(n)

		// Call handler with packet data
		if r.handler != nil {
			r.handler(buffer, n, receiveTime)
		}
	}
}

// Close closes the receiver
func (r *MulticastReceiver) Close() error {
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

// Stats returns receiver statistics
type Stats struct {
	PacketsReceived uint64
	BytesReceived   uint64
	Errors          uint64
}

// GetStats returns current statistics
func (r *MulticastReceiver) GetStats() Stats {
	return Stats{
		PacketsReceived: r.packetsReceived,
		BytesReceived:   r.bytesReceived,
		Errors:          r.errors,
	}
}
