package probe

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"net"
	"github.com/saitama-op/vyos-sla-agent/internal/config"
	"github.com/saitama-op/vyos-sla-agent/internal/decision"
	"github.com/saitama-op/vyos-sla-agent/internal/exporter"
	"github.com/saitama-op/vyos-sla-agent/internal/metrics"
	"github.com/saitama-op/vyos-sla-agent/internal/util"
	"github.com/saitama-op/vyos-sla-agent/internal/vyos"
)

type Worker struct {
	wan           config.WAN
	conn          net.PacketConn
	engine        *decision.Engine
	vyosCtrl      *vyos.Controller
	latencyBuf    *util.FloatRingBuffer
	lossBuf       *util.FloatRingBuffer
	jitterBuf     *util.FloatRingBuffer
	jitterCalc    *metrics.RFC3550Jitter
}

// NewWorker initializes a concurrent probe worker for a specific WAN interface
func NewWorker(wan config.WAN, conn net.PacketConn, engine *decision.Engine, vyosCtrl *vyos.Controller) *Worker {
	// Initialize 20-sample ring buffers as per architecture spec
	latencySamples := wan.Threshold.LatencySamples
	if latencySamples <= 0 {
		latencySamples = 20
	}
	
	lossSamples := wan.Threshold.LossSamples
	if lossSamples <= 0 {
		lossSamples = 20
	}
	
	jitterSamples := wan.Threshold.JitterSamples
	if jitterSamples <= 0 {
		jitterSamples = 20
	}
	return &Worker{
		wan:        wan,
		conn:       conn,
		engine:     engine,
		vyosCtrl:   vyosCtrl,
		latencyBuf: util.NewFloatRingBuffer(int(latencySamples)), // Defaults to 20 if mapped from config
		lossBuf:    util.NewFloatRingBuffer(int(lossSamples)),
		jitterBuf:  util.NewFloatRingBuffer(int(jitterSamples)),
		jitterCalc: metrics.NewRFC3550Jitter(),
	}
}

type probeResult struct {
	target string
	rtt    time.Duration
	err    error
}

// Run starts the concurrent probing loop
func (w *Worker) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	seq := 0

	slog.Info("Started WAN worker", "wan", w.wan.Name, "targets", len(w.wan.Targets))

	for {
		select {
		case <-ctx.Done():
			slog.Info("Stopping worker", "wan", w.wan.Name)
			return
		case <-ticker.C:
			seq++
			w.executeProbeCycle(seq)
		}
	}
}

func (w *Worker) executeProbeCycle(seq int) {
	targetCount := len(w.wan.Targets)
	results := make(chan probeResult, targetCount)
	var wg sync.WaitGroup

	// 1. Fan-out: Launch goroutines for simultaneous probing
	for _, target := range w.wan.Targets {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			rtt, err := MeasureRTT(w.conn, t, seq)
			results <- probeResult{target: t, rtt: rtt, err: err}
		}(target)
	}

	// 2. Wait for all probes in this cycle to complete (or hit the 2s timeout)
	wg.Wait()
	close(results)

	// 3. Fan-in: Aggregate cycle results
	var totalLatency time.Duration
	failedProbes := 0

	for res := range results {
		if res.err != nil {
			failedProbes++
			continue
		}
		totalLatency += res.rtt
	}

	successfulProbes := targetCount - failedProbes
	cycleLossPercent := (float64(failedProbes) / float64(targetCount)) * 100.0
	var cycleLatencyMs float64

	if successfulProbes > 0 {
		cycleLatencyMs = (float64(totalLatency) / float64(time.Millisecond)) / float64(successfulProbes)
	} else {
		// If 100% loss, cap latency to threshold + penalty to ensure it penalizes the average
		cycleLatencyMs = w.wan.Threshold.Latency * 1.5 
	}

	// Calculate Jitter
	cycleJitter := w.jitterCalc.AddSample(cycleLatencyMs)

	// 4. Update Ring Buffers
	w.latencyBuf.Add(cycleLatencyMs)
	w.lossBuf.Add(cycleLossPercent)
	w.jitterBuf.Add(cycleJitter)

	// 5. Calculate rolling metrics for decision engine
	rollingLatency := metrics.Percentile95(w.latencyBuf.Values()) // Using 95th percentile for latency
	rollingLoss := metrics.Average(w.lossBuf.Values())
	rollingJitter := metrics.Average(w.jitterBuf.Values())

	// 6. Evaluate state machine
	previousState := w.engine.CurrentState
	currentState := w.engine.Evaluate(rollingLatency, rollingLoss, rollingJitter, w.wan.Threshold.Latency, w.wan.Threshold.Loss, w.wan.Threshold.Jitter)

	// 7. Execute Active SD-WAN Mitigation (VyOS Controller)
	if w.vyosCtrl != nil {
		if currentState == decision.StateDown && previousState != decision.StateDown {
			slog.Warn("SLA Failure threshold reached. Disabling WAN interface route.", "wan", w.wan.Name)
			_ = w.vyosCtrl.DisableInterface(w.wan.Interface) // Example action
		} else if currentState == decision.StateUp && previousState != decision.StateUp {
			slog.Info("SLA Recovered. Restoring WAN interface route.", "wan", w.wan.Name)
			_ = w.vyosCtrl.EnableInterface(w.wan.Interface) // Example action
		}
	}

	// 8. Export to Prometheus
	exporter.LatencyGauge.WithLabelValues(w.wan.Name).Set(rollingLatency)
	exporter.LossGauge.WithLabelValues(w.wan.Name).Set(rollingLoss)
	
	stateVal := 0.0
	if currentState == decision.StateDegraded {
		stateVal = 1.0
	} else if currentState == decision.StateDown {
		stateVal = 2.0
	}
	exporter.StateGauge.WithLabelValues(w.wan.Name).Set(stateVal)

	slog.Debug("Cycle complete", 
		"wan", w.wan.Name, 
		"r_latency", rollingLatency, 
		"r_loss", rollingLoss, 
		"r_jitter", rollingJitter,
		"state", currentState,
	)
}
