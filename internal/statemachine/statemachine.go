package statemachine

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ysqss/watchdog/internal/prober"
	"github.com/ysqss/watchdog/internal/ringbuffer"
	"github.com/ysqss/watchdog/internal/target"
)

type StateChangeEvent struct {
	TargetID  string
	From      target.TargetState
	To        target.TargetState
	Result    *prober.ProbeResult
	Timestamp time.Time
}

type StateMachine struct {
	mu       sync.RWMutex
	target   *target.Target
	state    target.TargetState
	window   *ringbuffer.RingBuffer[bool]
	latency  *ringbuffer.RingBuffer[time.Duration]
	version  atomic.Int64
	onChange func(event StateChangeEvent)
}

func New(t *target.Target, onChange func(event StateChangeEvent)) *StateMachine {
	windowSize := t.GetUnhealthyThreshold()
	if windowSize < t.GetHealthyThreshold() {
		windowSize = t.GetHealthyThreshold()
	}

	return &StateMachine{
		target:   t,
		state:    target.StateUnknown,
		window:   ringbuffer.New[bool](windowSize),
		latency:  ringbuffer.New[time.Duration](30),
		onChange: onChange,
	}
}

func (sm *StateMachine) Process(result *prober.ProbeResult) (changed bool, from, to target.TargetState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.window.Push(result.Success)
	if result.Success && result.Latency > 0 {
		sm.latency.Push(result.Latency)
	}

	from = sm.state
	to = sm.evaluate()

	if to != from {
		sm.state = to
		sm.version.Add(1)

		slog.Info("state changed",
			"target", sm.target.ID,
			"from", from.String(),
			"to", to.String(),
		)

		if sm.onChange != nil {
			event := StateChangeEvent{
				TargetID:  sm.target.ID,
				From:      from,
				To:        to,
				Result:    result,
				Timestamp: time.Now(),
			}
			go sm.onChange(event)
		}
		return true, from, to
	}

	return false, from, to
}

func (sm *StateMachine) evaluate() target.TargetState {
	windowLen := sm.window.Len()

	if windowLen < sm.target.GetUnhealthyThreshold() {
		return sm.state
	}

	if sm.window.AllFail() {
		return target.StateUnhealthy
	}

	if sm.window.AllSuccess() {
		newState := target.StateHealthy

		p50 := sm.latency.P50()
		if p50 > sm.degradedThreshold() {
			newState = target.StateDegraded
		}

		return newState
	}

	if sm.state == target.StateUnhealthy {
		return target.StateUnhealthy
	}

	return sm.state
}

func (sm *StateMachine) degradedThreshold() time.Duration {
	return sm.target.GetDegradedThreshold()
}

func (sm *StateMachine) State() target.TargetState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state
}

func (sm *StateMachine) LatencyP50() time.Duration {
	return sm.latency.P50()
}

func (sm *StateMachine) LatencyP95() time.Duration {
	return sm.latency.P95()
}

func (sm *StateMachine) LatencyP99() time.Duration {
	return sm.latency.P99()
}

func (sm *StateMachine) Version() int64 {
	return sm.version.Load()
}
