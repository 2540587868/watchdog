package statemachine

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/ysqss/watchdog/internal/prober"
	"github.com/ysqss/watchdog/internal/target"
)

func newTestTarget() *target.Target {
	return &target.Target{
		ID:                  "test",
		HealthyThreshold:    3,
		UnhealthyThreshold:  3,
		DegradedThresholdMs: 2000,
	}
}

func successResult() *prober.ProbeResult {
	return &prober.ProbeResult{
		TargetID:  "test",
		Success:   true,
		Latency:   50 * time.Millisecond,
		Timestamp: time.Now(),
	}
}

func failResult() *prober.ProbeResult {
	return &prober.ProbeResult{
		TargetID:  "test",
		Success:   false,
		Error:     "connection refused",
		Timestamp: time.Now(),
	}
}

func slowResult(latency time.Duration) *prober.ProbeResult {
	return &prober.ProbeResult{
		TargetID:  "test",
		Success:   true,
		Latency:   latency,
		Timestamp: time.Now(),
	}
}

func TestNew_InitialState(t *testing.T) {
	sm := New(newTestTarget(), nil)
	if sm.State() != target.StateUnknown {
		t.Errorf("initial state = %v, want %v", sm.State(), target.StateUnknown)
	}
}

func TestNew_WindowSize(t *testing.T) {
	tgt := &target.Target{
		ID:                 "test",
		HealthyThreshold:   5,
		UnhealthyThreshold: 3,
	}
	sm := New(tgt, nil)
	if sm.window == nil {
		t.Fatal("window is nil")
	}
	if sm.window.Len() != 0 {
		t.Errorf("window Len() = %d, want 0", sm.window.Len())
	}
}

func TestProcess_UnknownToHealthy(t *testing.T) {
	sm := New(newTestTarget(), nil)

	sm.Process(successResult())
	sm.Process(successResult())

	if sm.State() != target.StateUnknown {
		t.Errorf("state after 2 successes = %v, want %v (not enough data)", sm.State(), target.StateUnknown)
	}

	changed, from, to := sm.Process(successResult())
	if !changed {
		t.Error("expected state change on 3rd success")
	}
	if from != target.StateUnknown {
		t.Errorf("from = %v, want %v", from, target.StateUnknown)
	}
	if to != target.StateHealthy {
		t.Errorf("to = %v, want %v", to, target.StateHealthy)
	}
}

func TestProcess_HealthyToUnhealthy(t *testing.T) {
	sm := New(newTestTarget(), nil)

	sm.Process(successResult())
	sm.Process(successResult())
	sm.Process(successResult())

	if sm.State() != target.StateHealthy {
		t.Fatalf("state before failures = %v, want %v", sm.State(), target.StateHealthy)
	}

	sm.Process(failResult())
	sm.Process(failResult())

	changed, from, to := sm.Process(failResult())
	if !changed {
		t.Error("expected state change on 3rd failure")
	}
	if from != target.StateHealthy {
		t.Errorf("from = %v, want %v", from, target.StateHealthy)
	}
	if to != target.StateUnhealthy {
		t.Errorf("to = %v, want %v", to, target.StateUnhealthy)
	}
}

func TestProcess_UnhealthyToHealthy(t *testing.T) {
	sm := New(newTestTarget(), nil)

	sm.Process(failResult())
	sm.Process(failResult())
	sm.Process(failResult())

	if sm.State() != target.StateUnhealthy {
		t.Fatalf("state before recoveries = %v, want %v", sm.State(), target.StateUnhealthy)
	}

	sm.Process(successResult())
	sm.Process(successResult())
	changed, from, to := sm.Process(successResult())
	if !changed {
		t.Error("expected state change on 3rd success")
	}
	if from != target.StateUnhealthy {
		t.Errorf("from = %v, want %v", from, target.StateUnhealthy)
	}
	if to != target.StateHealthy {
		t.Errorf("to = %v, want %v", to, target.StateHealthy)
	}
}

func TestProcess_DegradedState(t *testing.T) {
	tgt := newTestTarget()
	tgt.DegradedThresholdMs = 100
	sm := New(tgt, nil)

	sm.Process(slowResult(50 * time.Millisecond))
	sm.Process(slowResult(50 * time.Millisecond))
	sm.Process(slowResult(50 * time.Millisecond))

	if sm.State() != target.StateHealthy {
		t.Fatalf("state with low latency = %v, want %v", sm.State(), target.StateHealthy)
	}

	sm.Process(slowResult(200 * time.Millisecond))
	sm.Process(slowResult(200 * time.Millisecond))
	changed, from, to := sm.Process(slowResult(200 * time.Millisecond))
	if !changed {
		t.Error("expected state change with high latency")
	}
	if from != target.StateHealthy {
		t.Errorf("from = %v, want %v", from, target.StateHealthy)
	}
	if to != target.StateDegraded {
		t.Errorf("to = %v, want %v", to, target.StateDegraded)
	}
}

func TestProcess_NoChangeBeforeThreshold(t *testing.T) {
	sm := New(newTestTarget(), nil)

	sm.Process(failResult())
	sm.Process(failResult())

	if sm.State() != target.StateUnknown {
		t.Errorf("state before threshold = %v, want %v", sm.State(), target.StateUnknown)
	}
}

func TestProcess_MixedResultsStayInCurrentState(t *testing.T) {
	sm := New(newTestTarget(), nil)

	sm.Process(successResult())
	sm.Process(successResult())
	sm.Process(successResult())

	if sm.State() != target.StateHealthy {
		t.Fatalf("state = %v, want %v", sm.State(), target.StateHealthy)
	}

	sm.Process(successResult())
	sm.Process(failResult())
	sm.Process(successResult())

	if sm.State() != target.StateHealthy {
		t.Errorf("state with mixed results = %v, want %v (stays healthy)", sm.State(), target.StateHealthy)
	}
}

func TestProcess_OnChangeCallback(t *testing.T) {
	var called atomic.Int32
	tgt := newTestTarget()
	sm := New(tgt, func(event StateChangeEvent) {
		called.Add(1)
	})

	sm.Process(successResult())
	sm.Process(successResult())
	sm.Process(successResult())

	time.Sleep(50 * time.Millisecond)
	if called.Load() != 1 {
		t.Errorf("onChange called %d times, want 1", called.Load())
	}
}

func TestProcess_VersionIncrement(t *testing.T) {
	sm := New(newTestTarget(), nil)
	initialVersion := sm.Version()

	sm.Process(successResult())
	sm.Process(successResult())
	sm.Process(successResult())

	if sm.Version() <= initialVersion {
		t.Errorf("version = %d, want > %d after state change", sm.Version(), initialVersion)
	}
}

func TestProcess_NoVersionIncrementWithoutChange(t *testing.T) {
	sm := New(newTestTarget(), nil)

	sm.Process(failResult())
	v1 := sm.Version()

	sm.Process(failResult())
	v2 := sm.Version()

	if v2 != v1 {
		t.Errorf("version changed from %d to %d without state change", v1, v2)
	}
}

func TestProcess_UnhealthyStaysOnMixedResults(t *testing.T) {
	sm := New(newTestTarget(), nil)

	sm.Process(failResult())
	sm.Process(failResult())
	sm.Process(failResult())

	if sm.State() != target.StateUnhealthy {
		t.Fatalf("state = %v, want %v", sm.State(), target.StateUnhealthy)
	}

	sm.Process(successResult())
	sm.Process(failResult())
	sm.Process(successResult())

	if sm.State() != target.StateUnhealthy {
		t.Errorf("state with mixed results from unhealthy = %v, want %v", sm.State(), target.StateUnhealthy)
	}
}

func TestProcess_DefaultDegradedThreshold(t *testing.T) {
	tgt := &target.Target{
		ID:                 "test",
		HealthyThreshold:   3,
		UnhealthyThreshold: 3,
	}
	sm := New(tgt, nil)

	sm.Process(slowResult(3 * time.Second))
	sm.Process(slowResult(3 * time.Second))
	changed, _, to := sm.Process(slowResult(3 * time.Second))
	if !changed {
		t.Error("expected state change with latency > default 2s threshold")
	}
	if to != target.StateDegraded {
		t.Errorf("to = %v, want %v", to, target.StateDegraded)
	}
}

func TestLatencyP50(t *testing.T) {
	sm := New(newTestTarget(), nil)
	sm.Process(successResult())

	if sm.LatencyP50() <= 0 {
		t.Error("LatencyP50() should be > 0 after a successful probe")
	}
}

func TestLatencyP95(t *testing.T) {
	sm := New(newTestTarget(), nil)
	for i := 0; i < 10; i++ {
		sm.Process(successResult())
	}

	if sm.LatencyP95() <= 0 {
		t.Error("LatencyP95() should be > 0 after successful probes")
	}
}

func TestLatencyP99(t *testing.T) {
	sm := New(newTestTarget(), nil)
	for i := 0; i < 10; i++ {
		sm.Process(successResult())
	}

	if sm.LatencyP99() <= 0 {
		t.Error("LatencyP99() should be > 0 after successful probes")
	}
}
