package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	ProbesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "watchdog_probes_total",
			Help: "Total number of probe attempts",
		},
		[]string{"target", "type"},
	)

	ProbesSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "watchdog_probes_success_total",
			Help: "Total number of successful probes",
		},
		[]string{"target", "type"},
	)

	ProbesFailed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "watchdog_probes_failed_total",
			Help: "Total number of failed probes",
		},
		[]string{"target", "type"},
	)

	ProbeDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "watchdog_probe_duration_seconds",
			Help:    "Probe duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"target", "type"},
	)

	TargetState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "watchdog_target_state",
			Help: "Current target state: 0=unknown, 1=healthy, 2=degraded, 3=unhealthy",
		},
		[]string{"target"},
	)

	AlertsSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "watchdog_alerts_sent_total",
			Help: "Total number of alerts sent",
		},
		[]string{"target", "level"},
	)
)

func stateValue(state string) float64 {
	switch state {
	case "healthy":
		return 1
	case "degraded":
		return 2
	case "unhealthy":
		return 3
	default:
		return 0
	}
}

func SetTargetState(targetID, state string) {
	TargetState.WithLabelValues(targetID).Set(stateValue(state))
}

func RecordProbe(targetID, probeType string, success bool, durationSeconds float64) {
	ProbesTotal.WithLabelValues(targetID, probeType).Inc()
	if success {
		ProbesSuccess.WithLabelValues(targetID, probeType).Inc()
	} else {
		ProbesFailed.WithLabelValues(targetID, probeType).Inc()
	}
	ProbeDurationSeconds.WithLabelValues(targetID, probeType).Observe(durationSeconds)
}

func RecordAlert(targetID, level string) {
	AlertsSent.WithLabelValues(targetID, level).Inc()
}

func init() {
	prometheus.MustRegister(
		ProbesTotal,
		ProbesSuccess,
		ProbesFailed,
		ProbeDurationSeconds,
		TargetState,
		AlertsSent,
	)
}
