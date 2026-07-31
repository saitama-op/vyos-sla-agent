package metrics

import (
	"math"
	"sync"
)

// RFC3550Jitter maintains state for standard VoIP jitter calculations
type RFC3550Jitter struct {
	mu          sync.Mutex
	lastRTT     float64
	currentJ    float64
	initialized bool
}

// NewRFC3550Jitter creates a new initialized jitter calculator
func NewRFC3550Jitter() *RFC3550Jitter {
	return &RFC3550Jitter{}
}

// AddSample inputs the latest RTT in milliseconds and returns the updated Jitter
func (j *RFC3550Jitter) AddSample(rttMs float64) float64 {
	j.mu.Lock()
	defer j.mu.Unlock()

	if !j.initialized {
		j.lastRTT = rttMs
		j.currentJ = 0.0
		j.initialized = true
		return 0.0
	}

	// Calculate absolute difference between this RTT and the last RTT
	delta := math.Abs(rttMs - j.lastRTT)
	
	// RFC 3550 Jitter Formula: J(i) = J(i-1) + (|D(i-1, i)| - J(i-1)) / 16
	j.currentJ = j.currentJ + ((delta - j.currentJ) / 16.0)
	
	j.lastRTT = rttMs
	return j.currentJ
}

// Current returns the current jitter value without adding a new sample
func (j *RFC3550Jitter) Current() float64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.currentJ
}
