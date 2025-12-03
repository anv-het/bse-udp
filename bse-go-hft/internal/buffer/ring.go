// Package buffer provides lock-free ring buffer for HFT applications
package buffer

import (
	"sync/atomic"
)

const (
	// DefaultRingSize is the default number of slots (power of 2)
	// 16384 slots for ZERO packet drops at high throughput
	// 16384 slots @ ~1700 bytes = ~28MB per feed
	DefaultRingSize = 1 << 14 // 16384 slots (was 4096)

	// DefaultPacketSize is the maximum packet size
	// BSE packets are typically 36 header + 6*264 records = 1620 bytes max
	// Using 1700 for safety margin (reduced from 2048)
	DefaultPacketSize = 1700
)

// RingBuffer is a lock-free Single-Producer Single-Consumer queue
// Optimized for high-throughput, low-latency packet processing
type RingBuffer struct {
	buffer []Slot        // Pre-allocated packet slots
	head   atomic.Uint64 // Write position (producer)
	tail   atomic.Uint64 // Read position (consumer)
	mask   uint64        // For fast modulo (size - 1)
	size   int           // Buffer size

	// Statistics
	pushCount atomic.Uint64
	popCount  atomic.Uint64
	dropCount atomic.Uint64
}

// Slot represents a single buffer slot
type Slot struct {
	Data   []byte
	Length int
}

// NewRingBuffer creates a pre-allocated ring buffer
// Size must be a power of 2 for efficient modulo operation
func NewRingBuffer(size, packetSize int) *RingBuffer {
	if size <= 0 {
		size = DefaultRingSize
	}
	if packetSize <= 0 {
		packetSize = DefaultPacketSize
	}

	// Ensure size is power of 2
	if size&(size-1) != 0 {
		// Round up to next power of 2
		size = nextPowerOf2(size)
	}

	rb := &RingBuffer{
		buffer: make([]Slot, size),
		mask:   uint64(size - 1),
		size:   size,
	}

	// Pre-allocate all buffers to avoid runtime allocations
	for i := 0; i < size; i++ {
		rb.buffer[i].Data = make([]byte, packetSize)
	}

	return rb
}

// nextPowerOf2 returns the next power of 2 >= n
func nextPowerOf2(n int) int {
	if n <= 1 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n + 1
}

// TryPush attempts to push data without blocking
// Returns false if buffer is full (data is dropped)
func (rb *RingBuffer) TryPush(data []byte, length int) bool {
	head := rb.head.Load()
	tail := rb.tail.Load()

	// Check if full (head has caught up to tail)
	if head-tail >= uint64(rb.size) {
		rb.dropCount.Add(1)
		return false
	}

	// Calculate slot index
	slot := head & rb.mask

	// Copy data to pre-allocated slot
	copy(rb.buffer[slot].Data[:length], data[:length])
	rb.buffer[slot].Length = length

	// Memory barrier + publish (Store with release semantics)
	rb.head.Store(head + 1)
	rb.pushCount.Add(1)

	return true
}

// TryPop attempts to pop data without blocking
// Returns nil, 0, false if buffer is empty
func (rb *RingBuffer) TryPop() ([]byte, int, bool) {
	tail := rb.tail.Load()
	head := rb.head.Load()

	// Check if empty
	if tail >= head {
		return nil, 0, false
	}

	// Calculate slot index
	slot := tail & rb.mask
	data := rb.buffer[slot].Data
	length := rb.buffer[slot].Length

	// Advance tail (Store with release semantics)
	rb.tail.Store(tail + 1)
	rb.popCount.Add(1)

	return data, length, true
}

// Size returns the buffer capacity
func (rb *RingBuffer) Size() int {
	return rb.size
}

// Len returns the current number of items in the buffer
func (rb *RingBuffer) Len() int {
	return int(rb.head.Load() - rb.tail.Load())
}

// IsEmpty returns true if buffer is empty
func (rb *RingBuffer) IsEmpty() bool {
	return rb.tail.Load() >= rb.head.Load()
}

// IsFull returns true if buffer is full
func (rb *RingBuffer) IsFull() bool {
	return rb.head.Load()-rb.tail.Load() >= uint64(rb.size)
}

// Stats returns buffer statistics
type Stats struct {
	PushCount  uint64
	PopCount   uint64
	DropCount  uint64
	CurrentLen int
	Capacity   int
}

// GetStats returns current buffer statistics
func (rb *RingBuffer) GetStats() Stats {
	return Stats{
		PushCount:  rb.pushCount.Load(),
		PopCount:   rb.popCount.Load(),
		DropCount:  rb.dropCount.Load(),
		CurrentLen: rb.Len(),
		Capacity:   rb.size,
	}
}

// Reset clears the buffer and statistics
func (rb *RingBuffer) Reset() {
	rb.head.Store(0)
	rb.tail.Store(0)
	rb.pushCount.Store(0)
	rb.popCount.Store(0)
	rb.dropCount.Store(0)
}
