package server

import "github.com/prometheus/client_golang/prometheus"

var (
	http3CompanionUp = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "navidrome",
		Subsystem: "http3",
		Name:      "companion_up",
		Help:      "Whether the tokio-quiche HTTP/3 companion is ready to accept traffic.",
	})
	http3CompanionRestarts = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "navidrome",
		Subsystem: "http3",
		Name:      "companion_restarts_total",
		Help:      "Number of unexpected tokio-quiche HTTP/3 companion restarts.",
	})
	http3BridgeRejected = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "navidrome",
		Subsystem: "http3",
		Name:      "bridge_rejected_total",
		Help:      "Number of requests rejected by the private HTTP/3 bridge.",
	}, []string{"reason"})
)

func init() {
	prometheus.MustRegister(http3CompanionUp, http3CompanionRestarts, http3BridgeRejected)
}
