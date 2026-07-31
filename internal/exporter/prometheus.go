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
        JitterGauge = prometheus.NewGaugeVec(
                prometheus.GaugeOpts{
                        Name: "sla_jitter_ms",
                        Help: "Current max jitter in milliseconds",
                },
                []string{"wan"},
        )
        TCPLatencyGauge = prometheus.NewGaugeVec(
                prometheus.GaugeOpts{
                        Name: "sla_tcp_latency_ms",
                        Help: "Current TCP handshake latency in milliseconds",
                },
                []string{"wan"},
        )
        TCPLossGauge = prometheus.NewGaugeVec(
                prometheus.GaugeOpts{
                        Name: "sla_tcp_loss_percent",
                        Help: "Current TCP handshake loss percentage",
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
        prometheus.MustRegister(JitterGauge)
        prometheus.MustRegister(TCPLatencyGauge)
        prometheus.MustRegister(TCPLossGauge)
        prometheus.MustRegister(StateGauge)
}
