package target

import (
	"testing"
	"time"
)

func TestIntervalDuration_Default(t *testing.T) {
	tgt := &Target{Interval: 0}
	got := tgt.IntervalDuration()
	if got != 30*time.Second {
		t.Errorf("IntervalDuration() = %v, want %v", got, 30*time.Second)
	}
}

func TestIntervalDuration_Negative(t *testing.T) {
	tgt := &Target{Interval: -100}
	got := tgt.IntervalDuration()
	if got != 30*time.Second {
		t.Errorf("IntervalDuration() = %v, want %v", got, 30*time.Second)
	}
}

func TestIntervalDuration_Custom(t *testing.T) {
	tgt := &Target{Interval: 5000}
	got := tgt.IntervalDuration()
	want := 5 * time.Second
	if got != want {
		t.Errorf("IntervalDuration() = %v, want %v", got, want)
	}
}

func TestTimeoutDuration_Default(t *testing.T) {
	tgt := &Target{Timeout: 0}
	got := tgt.TimeoutDuration()
	if got != 5*time.Second {
		t.Errorf("TimeoutDuration() = %v, want %v", got, 5*time.Second)
	}
}

func TestTimeoutDuration_Negative(t *testing.T) {
	tgt := &Target{Timeout: -50}
	got := tgt.TimeoutDuration()
	if got != 5*time.Second {
		t.Errorf("TimeoutDuration() = %v, want %v", got, 5*time.Second)
	}
}

func TestTimeoutDuration_Custom(t *testing.T) {
	tgt := &Target{Timeout: 3000}
	got := tgt.TimeoutDuration()
	want := 3 * time.Second
	if got != want {
		t.Errorf("TimeoutDuration() = %v, want %v", got, want)
	}
}

func TestGetMethod_Default(t *testing.T) {
	tgt := &Target{Method: ""}
	got := tgt.GetMethod()
	if got != "GET" {
		t.Errorf("GetMethod() = %q, want %q", got, "GET")
	}
}

func TestGetMethod_Custom(t *testing.T) {
	tgt := &Target{Method: "POST"}
	got := tgt.GetMethod()
	if got != "POST" {
		t.Errorf("GetMethod() = %q, want %q", got, "POST")
	}
}

func TestGetExpectStatus_Default(t *testing.T) {
	tgt := &Target{ExpectStatus: 0}
	got := tgt.GetExpectStatus()
	if got != 200 {
		t.Errorf("GetExpectStatus() = %d, want %d", got, 200)
	}
}

func TestGetExpectStatus_Custom(t *testing.T) {
	tgt := &Target{ExpectStatus: 404}
	got := tgt.GetExpectStatus()
	if got != 404 {
		t.Errorf("GetExpectStatus() = %d, want %d", got, 404)
	}
}

func TestGetHealthyThreshold_Default(t *testing.T) {
	tgt := &Target{HealthyThreshold: 0}
	got := tgt.GetHealthyThreshold()
	if got != 3 {
		t.Errorf("GetHealthyThreshold() = %d, want %d", got, 3)
	}
}

func TestGetHealthyThreshold_Negative(t *testing.T) {
	tgt := &Target{HealthyThreshold: -1}
	got := tgt.GetHealthyThreshold()
	if got != 3 {
		t.Errorf("GetHealthyThreshold() = %d, want %d", got, 3)
	}
}

func TestGetHealthyThreshold_Custom(t *testing.T) {
	tgt := &Target{HealthyThreshold: 5}
	got := tgt.GetHealthyThreshold()
	if got != 5 {
		t.Errorf("GetHealthyThreshold() = %d, want %d", got, 5)
	}
}

func TestGetUnhealthyThreshold_Default(t *testing.T) {
	tgt := &Target{UnhealthyThreshold: 0}
	got := tgt.GetUnhealthyThreshold()
	if got != 3 {
		t.Errorf("GetUnhealthyThreshold() = %d, want %d", got, 3)
	}
}

func TestGetUnhealthyThreshold_Negative(t *testing.T) {
	tgt := &Target{UnhealthyThreshold: -2}
	got := tgt.GetUnhealthyThreshold()
	if got != 3 {
		t.Errorf("GetUnhealthyThreshold() = %d, want %d", got, 3)
	}
}

func TestGetUnhealthyThreshold_Custom(t *testing.T) {
	tgt := &Target{UnhealthyThreshold: 7}
	got := tgt.GetUnhealthyThreshold()
	if got != 7 {
		t.Errorf("GetUnhealthyThreshold() = %d, want %d", got, 7)
	}
}

func TestGetAlertLevel_Default(t *testing.T) {
	tgt := &Target{AlertLevel: ""}
	got := tgt.GetAlertLevel()
	if got != AlertCritical {
		t.Errorf("GetAlertLevel() = %q, want %q", got, AlertCritical)
	}
}

func TestGetAlertLevel_Custom(t *testing.T) {
	tgt := &Target{AlertLevel: AlertWarning}
	got := tgt.GetAlertLevel()
	if got != AlertWarning {
		t.Errorf("GetAlertLevel() = %q, want %q", got, AlertWarning)
	}
}

func TestGetDegradedThreshold_Default(t *testing.T) {
	tgt := &Target{DegradedThresholdMs: 0}
	got := tgt.GetDegradedThreshold()
	if got != 2*time.Second {
		t.Errorf("GetDegradedThreshold() = %v, want %v", got, 2*time.Second)
	}
}

func TestGetDegradedThreshold_Negative(t *testing.T) {
	tgt := &Target{DegradedThresholdMs: -500}
	got := tgt.GetDegradedThreshold()
	if got != 2*time.Second {
		t.Errorf("GetDegradedThreshold() = %v, want %v", got, 2*time.Second)
	}
}

func TestGetDegradedThreshold_Custom(t *testing.T) {
	tgt := &Target{DegradedThresholdMs: 5000}
	got := tgt.GetDegradedThreshold()
	want := 5 * time.Second
	if got != want {
		t.Errorf("GetDegradedThreshold() = %v, want %v", got, want)
	}
}

func TestIsSilenced_Nil(t *testing.T) {
	tgt := &Target{SilencedUntil: nil}
	if tgt.IsSilenced() {
		t.Error("IsSilenced() = true, want false when SilencedUntil is nil")
	}
}

func TestIsSilenced_Future(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)
	tgt := &Target{SilencedUntil: &future}
	if !tgt.IsSilenced() {
		t.Error("IsSilenced() = false, want true when SilencedUntil is in the future")
	}
}

func TestIsSilenced_Past(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	tgt := &Target{SilencedUntil: &past}
	if tgt.IsSilenced() {
		t.Error("IsSilenced() = true, want false when SilencedUntil is in the past")
	}
}

func TestTargetState_String(t *testing.T) {
	tests := []struct {
		state TargetState
		want  string
	}{
		{StateUnknown, "unknown"},
		{StateHealthy, "healthy"},
		{StateDegraded, "degraded"},
		{StateUnhealthy, "unhealthy"},
		{TargetState(99), "unknown"},
	}
	for _, tt := range tests {
		got := tt.state.String()
		if got != tt.want {
			t.Errorf("TargetState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestParseState(t *testing.T) {
	tests := []struct {
		input string
		want  TargetState
	}{
		{"healthy", StateHealthy},
		{"degraded", StateDegraded},
		{"unhealthy", StateUnhealthy},
		{"unknown", StateUnknown},
		{"", StateUnknown},
		{"invalid", StateUnknown},
	}
	for _, tt := range tests {
		got := ParseState(tt.input)
		if got != tt.want {
			t.Errorf("ParseState(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
