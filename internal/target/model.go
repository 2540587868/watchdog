package target

import "time"

type ProbeType string

const (
	ProbeHTTP ProbeType = "http"
	ProbeTCP  ProbeType = "tcp"
)

type TargetState int

const (
	StateUnknown   TargetState = iota
	StateHealthy
	StateDegraded
	StateUnhealthy
)

func (s TargetState) String() string {
	switch s {
	case StateHealthy:
		return "healthy"
	case StateDegraded:
		return "degraded"
	case StateUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

func ParseState(s string) TargetState {
	switch s {
	case "healthy":
		return StateHealthy
	case "degraded":
		return StateDegraded
	case "unhealthy":
		return StateUnhealthy
	default:
		return StateUnknown
	}
}

type AlertLevel string

const (
	AlertCritical AlertLevel = "critical"
	AlertWarning  AlertLevel = "warning"
	AlertInfo     AlertLevel = "info"
)

type Target struct {
	ID       string     `json:"id" yaml:"id"`
	Name     string     `json:"name" yaml:"name"`
	Type     ProbeType  `json:"type" yaml:"type"`
	URL      string     `json:"url" yaml:"url"`
	Interval int        `json:"interval_ms" yaml:"interval_ms"`
	Timeout  int        `json:"timeout_ms" yaml:"timeout_ms"`

	Method         string            `json:"method,omitempty" yaml:"method,omitempty"`
	Headers        map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	ExpectStatus   int               `json:"expect_status,omitempty" yaml:"expect_status,omitempty"`
	ExpectBody     string            `json:"expect_body,omitempty" yaml:"expect_body,omitempty"`
	TLSSkipVerify  bool              `json:"tls_skip_verify,omitempty" yaml:"tls_skip_verify,omitempty"`

	HealthyThreshold   int        `json:"healthy_threshold,omitempty" yaml:"healthy_threshold,omitempty"`
	UnhealthyThreshold int        `json:"unhealthy_threshold,omitempty" yaml:"unhealthy_threshold,omitempty"`
	DegradedThresholdMs int       `json:"degraded_threshold_ms,omitempty" yaml:"degraded_threshold_ms,omitempty"`
	AlertLevel         AlertLevel `json:"alert_level,omitempty" yaml:"alert_level,omitempty"`

	SilencedUntil *time.Time         `json:"silenced_until,omitempty" yaml:"silenced_until,omitempty"`
	Labels        map[string]string  `json:"labels,omitempty" yaml:"labels,omitempty"`

	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

func (t *Target) IntervalDuration() time.Duration {
	if t.Interval <= 0 {
		return 30 * time.Second
	}
	return time.Duration(t.Interval) * time.Millisecond
}

func (t *Target) TimeoutDuration() time.Duration {
	if t.Timeout <= 0 {
		return 5 * time.Second
	}
	return time.Duration(t.Timeout) * time.Millisecond
}

func (t *Target) GetMethod() string {
	if t.Method == "" {
		return "GET"
	}
	return t.Method
}

func (t *Target) GetExpectStatus() int {
	if t.ExpectStatus == 0 {
		return 200
	}
	return t.ExpectStatus
}

func (t *Target) GetHealthyThreshold() int {
	if t.HealthyThreshold <= 0 {
		return 3
	}
	return t.HealthyThreshold
}

func (t *Target) GetUnhealthyThreshold() int {
	if t.UnhealthyThreshold <= 0 {
		return 3
	}
	return t.UnhealthyThreshold
}

func (t *Target) GetAlertLevel() AlertLevel {
	if t.AlertLevel == "" {
		return AlertCritical
	}
	return t.AlertLevel
}

func (t *Target) GetDegradedThreshold() time.Duration {
	if t.DegradedThresholdMs <= 0 {
		return 2 * time.Second
	}
	return time.Duration(t.DegradedThresholdMs) * time.Millisecond
}

func (t *Target) IsSilenced() bool {
	if t.SilencedUntil == nil {
		return false
	}
	return time.Now().Before(*t.SilencedUntil)
}
