package probe

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/saitama-op/vyos-sla-agent/internal/config"
	"github.com/saitama-op/vyos-sla-agent/internal/decision"
	"github.com/saitama-op/vyos-sla-agent/internal/exporter"
	"github.com/saitama-op/vyos-sla-agent/internal/metrics"
	"github.com/saitama-op/vyos-sla-agent/internal/util"
	"github.com/saitama-op/vyos-sla-agent/internal/vyos"
)

type Worker struct {
	wan           config.WAN
	engine        *decision.Engine
	vyosCtrl      *vyos.Controller
	
	// ICMP Buffers
	latencyBuf    *util.FloatRingBuffer
	lossBuf       *util.FloatRingBuffer
	jitterBuf     *util.FloatRingBuffer
	jitterCalc    *metrics.RFC3550Jitter
	
	// TCP Buffers
	tcpLatencyBuf *util.FloatRingBuffer
	tcpLossBuf    *util.FloatRingBuffer
}

// NewWorker initializes a concurrent probe worker for a specific WAN interface
func NewWorker(wan config.WAN, engine *decision.Engine, vyosCtrl *vyos.Controller) *Worker {

	// Enforce safe defaults if they are missing from config.yaml
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
		wan:           wan,
		engine:        engine,
		vyosCtrl:      vyosCtrl,
		latencyBuf:    util.NewFloatRingBuffer(latencySamples),
		lossBuf:       util.NewFloatRingBuffer(lossSamples),
		jitterBuf:     util.NewFloatRingBuffer(jitterSamples),
		jitterCalc:    metrics.NewRFC3550Jitter(),
		tcpLatencyBuf: util.NewFloatRingBuffer(latencySamples),
		tcpLossBuf:    util.NewFloatRingBuffer(lossSamples),
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

	slog.Info("Started WAN worker", "wan", w.wan.Name, "icmp_targets", len(w.wan.Targets), "tcp_targets", len(w.wan.TCPTargets))

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
	// ==========================================
	// 1. ICMP PROBING (Fan-out)
	// ==========================================
	icmpTargetCount := len(w.wan.Targets)
	icmpResults := make(chan probeResult, icmpTargetCount)
	var wg sync.WaitGroup

	for _, target := range w.wan.Targets {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			rtt, err := MeasureRTT(w.wan.Interface, t, seq)
			icmpResults <- probeResult{target: t, rtt: rtt, err: err}
		}(target)
	}

	// ==========================================
	// 2. TCP PROBING (Fan-out)
	// ==========================================
	tcpTargetCount := len(w.wan.TCPTargets)
	tcpResults := make(chan probeResult, tcpTargetCount)

	for _, target := range w.wan.TCPTargets {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			rtt, err := SendTCPProbe(w.wan.Interface, t) // Calls the new tcp.go function
			tcpResults <- probeResult{target: t, rtt: rtt, err: err}
		}(target)
	}

	// Wait for ALL probes (ICMP and TCP) in this cycle to complete
	wg.Wait()
	close(icmpResults)
	close(tcpResults)

	// ==========================================
	// 3. AGGREGATE ICMP RESULTS (Fan-in)
	// ==========================================
	var totalICMPLatency time.Duration
	failedICMPProbes := 0

	for res := range icmpResults {
		if res.err != nil {
			failedICMPProbes++
			continue
		}
		totalICMPLatency += res.rtt
	}

	successfulICMPProbes := icmpTargetCount - failedICMPProbes
	cycleLossPercent := (float64(failedICMPProbes) / float64(icmpTargetCount)) * 100.0
	var cycleLatencyMs float64

	if successfulICMPProbes > 0 {
		cycleLatencyMs = (float64(totalICMPLatency) / float64(time.Millisecond)) / float64(successfulICMPProbes)
	} else {
		cycleLatencyMs = w.wan.Threshold.Latency * 1.5 // Penalty cap
	}

	cycleJitter := w.jitterCalc.AddSample(cycleLatencyMs)

	// ==========================================
	// 4. AGGREGATE TCP RESULTS (Fan-in)
	// ==========================================
	var totalTCPLatency time.Duration
	failedTCPProbes := 0
	var tcpCycleLossPercent float64
	var tcpCycleLatencyMs float64

	if tcpTargetCount > 0 {
		for res := range tcpResults {
			if res.err != nil {
				failedTCPProbes++
				continue
			}
			totalTCPLatency += res.rtt
		}

		successfulTCPProbes := tcpTargetCount - failedTCPProbes
		tcpCycleLossPercent = (float64(failedTCPProbes) / float64(tcpTargetCount)) * 100.0

		if successfulTCPProbes > 0 {
			tcpCycleLatencyMs = (float64(totalTCPLatency) / float64(time.Millisecond)) / float64(successfulTCPProbes)
		} else {
			tcpCycleLatencyMs = w.wan.Threshold.Latency * 1.5 // Penalty cap
		}
	}

	// ==========================================
	// 5. UPDATE RING BUFFERS
	// ==========================================
	w.latencyBuf.Add(cycleLatencyMs)
	w.lossBuf.Add(cycleLossPercent)
	w.jitterBuf.Add(cycleJitter)
	
	if tcpTargetCount > 0 {
		w.tcpLatencyBuf.Add(tcpCycleLatencyMs)
		w.tcpLossBuf.Add(tcpCycleLossPercent)
	}

	// ==========================================
	// 6. CALCULATE ROLLING METRICS
	// ==========================================
	rollingLatency := metrics.Percentile95(w.latencyBuf.Values())
	rollingLoss := metrics.Average(w.lossBuf.Values())
	rollingJitter := metrics.Average(w.jitterBuf.Values())
	
	rollingTCPLatency := 0.0
	rollingTCPLoss := 0.0
	if tcpTargetCount > 0 {
		rollingTCPLatency = metrics.Percentile95(w.tcpLatencyBuf.Values())
		rollingTCPLoss = metrics.Average(w.tcpLossBuf.Values())
	}

	// ==========================================
	// 7. EVALUATE STATE MACHINE
	// ==========================================
	previousState := w.engine.CurrentState
	
	// NOTE: You will need to update your engine.Evaluate() signature in internal/decision/engine.go
	// to accept rollingTCPLatency and rollingTCPLoss as shown here.
	currentState := w.engine.Evaluate(
		rollingLatency, rollingLoss, rollingJitter, 
		rollingTCPLatency, rollingTCPLoss, 
		w.wan.Threshold.Latency, w.wan.Threshold.Loss, w.wan.Threshold.Jitter,
	)

	// ==========================================
	// 8. EXECUTE ACTIVE SD-WAN MITIGATION
	// ==========================================
	if w.vyosCtrl != nil {
		if currentState == decision.StateDown && previousState != decision.StateDown {
			slog.Warn("SLA Failure threshold reached. Executing on_down commands.", "wan", w.wan.Name)
			if len(w.wan.OnDown) > 0 {
				if err := w.vyosCtrl.ExecuteTransaction(w.wan.OnDown); err != nil {
					slog.Error("VyOS on_down transaction failed", "error", err, "wan", w.wan.Name)
				}
			}
		} else if currentState == decision.StateUp && previousState != decision.StateUp {
			slog.Info("SLA Recovered. Executing on_up commands.", "wan", w.wan.Name)
			if len(w.wan.OnUp) > 0 {
				if err := w.vyosCtrl.ExecuteTransaction(w.wan.OnUp); err != nil {
					slog.Error("VyOS on_up transaction failed", "error", err, "wan", w.wan.Name)
				}
			}
		}
	}

	// ==========================================
	// 9. EXPORT TO PROMETHEUS
	// ==========================================
	exporter.LatencyGauge.WithLabelValues(w.wan.Name).Set(rollingLatency)
	exporter.LossGauge.WithLabelValues(w.wan.Name).Set(rollingLoss)
	exporter.JitterGauge.WithLabelValues(w.wan.Name).Set(rollingJitter)
	exporter.TCPLatencyGauge.WithLabelValues(w.wan.Name).Set(rollingTCPLatency)
	exporter.TCPLossGauge.WithLabelValues(w.wan.Name).Set(rollingTCPLoss)

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
		"r_tcp_lat", rollingTCPLatency,
		"r_tcp_loss", rollingTCPLoss,
		"state", currentState,
	)
}
