package util

import (
	"sync"
)

// FloatRingBuffer is a thread-safe circular buffer for float64 metrics
type FloatRingBuffer struct {
	mu       sync.RWMutex
	data     []float64
	capacity int
	head     int
	isFull   bool
}

// NewFloatRingBuffer initializes a buffer of the given size
func NewFloatRingBuffer(size int) *FloatRingBuffer {
	return &FloatRingBuffer{
		data:     make([]float64, size),
		capacity: size,
	}
}

// Add inserts a new value into the ring buffer
func (r *FloatRingBuffer) Add(val float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[r.head] = val
	r.head = (r.head + 1) % r.capacity
	if r.head == 0 {
		r.isFull = true
	}
}

// Values returns a copy of the current values in the buffer, ordered from oldest to newest
func (r *FloatRingBuffer) Values() []float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []float64
	if r.isFull {
		result = make([]float64, 0, r.capacity)
		result = append(result, r.data[r.head:]...)
		result = append(result, r.data[:r.head]...)
	} else {
		result = make([]float64, r.head)
		copy(result, r.data[:r.head])
	}
	return result
}

// Clear resets the buffer state
func (r *FloatRingBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.head = 0
	r.isFull = false
}
