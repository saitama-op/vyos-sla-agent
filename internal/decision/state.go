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
	mu             sync.RWMutex
	CurrentState   State
	FailCount      int
	SuccessCount   int
	MaxFails       int
	MaxSuccesses   int
	WANName        string
}

func NewEngine(wanName string) *Engine {
	return &Engine{
		CurrentState: StateUp,
		MaxFails:     5,
		MaxSuccesses: 20,
		WANName:      wanName,
	}
}

func (e *Engine) Evaluate(latency, loss, jitter float64, thresholdLatency, thresholdLoss, thresholdJitter float64) State {
	e.mu.Lock()
	defer e.mu.Unlock()

	isFailing := latency > thresholdLatency || loss > thresholdLoss || jitter > thresholdJitter

	if isFailing {
		e.SuccessCount = 0
		e.FailCount++
		if e.CurrentState == StateUp {
			e.CurrentState = StateDegraded
			slog.Warn("SLA Degraded", "wan", e.WANName, "latency", latency, "loss", loss)
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
