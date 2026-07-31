package decision

import (
	"log/slog"
	"sync"
)

type State string

const (
	StateUp       State = "UP"
	StateDegraded State = "DEGRADED"
	StateDown     State = "DOWN"
)

type Engine struct {
	mu           sync.RWMutex
	CurrentState State
	FailCount    int
	SuccessCount int
	MaxFails     int
	MaxSuccesses int
	WANName      string
}

func NewEngine(wanName string) *Engine {
	return &Engine{
		CurrentState: StateUp,
		MaxFails:     5,
		MaxSuccesses: 20,
		WANName:      wanName,
	}
}

// Evaluate now accepts tcpLatency and tcpLoss alongside the ICMP metrics
func (e *Engine) Evaluate(latency, loss, jitter, tcpLatency, tcpLoss float64, thresholdLatency, thresholdLoss, thresholdJitter float64) State {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 1. Check if ICMP metrics breach thresholds
	icmpFails := latency > thresholdLatency || loss > thresholdLoss || jitter > thresholdJitter
	
	// 2. Check if TCP metrics breach thresholds (we check tcpLatency > 0 to ensure TCP probing is actually active)
	tcpFails := (tcpLatency > 0 && tcpLatency > thresholdLatency) || tcpLoss > thresholdLoss

	// 3. Link is failing if EITHER protocol degrades
	isFailing := icmpFails || tcpFails

	if isFailing {
		e.SuccessCount = 0
		e.FailCount++
		
		if e.CurrentState == StateUp {
			e.CurrentState = StateDegraded
			// Updated to log both ICMP and TCP metrics for easier debugging
			slog.Warn("SLA Degraded", 
				"wan", e.WANName, 
				"icmp_lat", latency, 
				"icmp_loss", loss,
				"tcp_lat", tcpLatency,
				"tcp_loss", tcpLoss,
			)
		} else if e.CurrentState == StateDegraded && e.FailCount >= e.MaxFails {
			e.CurrentState = StateDown
			slog.Error("SLA Down", "wan", e.WANName)
		}
	} else {
		e.FailCount = 0
		if e.CurrentState != StateUp {
			e.SuccessCount++
			if e.SuccessCount >= e.MaxSuccesses {
				e.CurrentState = StateUp
				slog.Info("SLA Recovered", "wan", e.WANName)
			}
		}
	}
	return e.CurrentState
}
