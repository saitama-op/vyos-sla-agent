package exporter

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	LatencyGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sla_latency_ms",
			Help: "Current average latency in milliseconds",
		},
		[]string{"wan"},
	)
	LossGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sla_loss_percent",
			Help: "Current packet loss percentage",
		},
		[]string{"wan"},
	)
	StateGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sla_state",
			Help: "Current operational state (0=UP, 1=DEGRADED, 2=DOWN)",
		},
		[]string{"wan"},
	)
)

func InitPrometheus() {
	prometheus.MustRegister(LatencyGauge)
	prometheus.MustRegister(LossGauge)
	prometheus.MustRegister(StateGauge)
}
