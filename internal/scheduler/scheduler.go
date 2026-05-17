package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ysqss/watchdog/internal/metrics"
	"github.com/ysqss/watchdog/internal/prober"
	"github.com/ysqss/watchdog/internal/statemachine"
	"github.com/ysqss/watchdog/internal/store"
	"github.com/ysqss/watchdog/internal/target"
)

type Scheduler struct {
	mu            sync.RWMutex
	targets       map[string]*scheduledTarget
	probers       map[target.ProbeType]prober.Prober
	store         *store.Store
	onChange      func(event statemachine.StateChangeEvent)
	onProbeResult func(targetID string, result *prober.ProbeResult)

	register   chan *target.Target
	unregister chan string
	wg         sync.WaitGroup
	done       chan struct{}
}

type scheduledTarget struct {
	target *target.Target
	sm     *statemachine.StateMachine
	cancel context.CancelFunc
}

func New(st *store.Store, onChange func(event statemachine.StateChangeEvent), onProbeResult func(targetID string, result *prober.ProbeResult)) *Scheduler {
	probers := map[target.ProbeType]prober.Prober{
		target.ProbeHTTP: prober.NewHTTPProber(),
		target.ProbeTCP:  prober.NewTCPProber(),
	}

	return &Scheduler{
		targets:       make(map[string]*scheduledTarget),
		probers:       probers,
		store:         st,
		onChange:      onChange,
		onProbeResult: onProbeResult,
		register:      make(chan *target.Target, 64),
		unregister:    make(chan string, 64),
		done:          make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	go s.loop()
}

func (s *Scheduler) loop() {
	for {
		select {
		case t := <-s.register:
			s.startTarget(t)
		case id := <-s.unregister:
			s.stopTarget(id)
		case <-s.done:
			s.mu.Lock()
			for id := range s.targets {
				s.stopTargetLocked(id)
			}
			s.mu.Unlock()
			return
		}
	}
}

func (s *Scheduler) AddTarget(t *target.Target) {
	s.register <- t
}

func (s *Scheduler) RemoveTarget(id string) {
	s.unregister <- id
}

func (s *Scheduler) startTarget(t *target.Target) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.targets[t.ID]; ok {
		existing.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	sm := statemachine.New(t, s.onChange)

	st := &scheduledTarget{
		target: t,
		sm:     sm,
		cancel: cancel,
	}
	s.targets[t.ID] = st

	s.wg.Add(1)
	go s.runTarget(ctx, st)
}

func (s *Scheduler) stopTarget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopTargetLocked(id)
}

func (s *Scheduler) stopTargetLocked(id string) {
	st, ok := s.targets[id]
	if !ok {
		return
	}
	st.cancel()
	delete(s.targets, id)
}

func (s *Scheduler) runTarget(ctx context.Context, st *scheduledTarget) {
	defer s.wg.Done()

	interval := st.target.IntervalDuration()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("target probe started",
		"target", st.target.ID,
		"type", string(st.target.Type),
		"url", st.target.URL,
		"interval", interval,
	)

	for {
		select {
		case <-ticker.C:
			s.probeOnce(ctx, st)
			newInterval := adaptiveInterval(st.sm.State(), st.target.IntervalDuration())
			if newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
			}

		case <-ctx.Done():
			slog.Info("target probe stopped", "target", st.target.ID)
			return
		}
	}
}

func (s *Scheduler) probeOnce(ctx context.Context, st *scheduledTarget) {
	p, ok := s.probers[st.target.Type]
	if !ok {
		slog.Error("no prober for target type", "type", string(st.target.Type), "target", st.target.ID)
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, st.target.TimeoutDuration())
	result := p.Probe(probeCtx, st.target)
	cancel()

	st.sm.Process(result)

	if s.onProbeResult != nil {
		s.onProbeResult(st.target.ID, result)
	}

	metrics.RecordProbe(st.target.ID, string(st.target.Type), result.Success, result.Latency.Seconds())
	metrics.SetTargetState(st.target.ID, st.sm.State().String())

	s.recordResult(result)
}

func (s *Scheduler) recordResult(result *prober.ProbeResult) {
	record := &store.ProbeHistoryRecord{
		TargetID:   result.TargetID,
		Timestamp:  result.Timestamp,
		Success:    result.Success,
		StatusCode: result.StatusCode,
		LatencyMs:  result.Latency.Milliseconds(),
		Error:      result.Error,
	}
	if err := s.store.InsertProbeResult(record); err != nil {
		slog.Error("failed to record probe result", "target", result.TargetID, "error", err)
	}
}

func (s *Scheduler) GetTargetState(id string) (target.TargetState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.targets[id]
	if !ok {
		return target.StateUnknown, false
	}
	return st.sm.State(), true
}

func (s *Scheduler) GetTargetLatency(id string) (p50, p95, p99 time.Duration, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.targets[id]
	if !ok {
		return 0, 0, 0, false
	}
	return st.sm.LatencyP50(), st.sm.LatencyP95(), st.sm.LatencyP99(), true
}

func (s *Scheduler) GetAllStates() map[string]target.TargetState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	states := make(map[string]target.TargetState, len(s.targets))
	for id, st := range s.targets {
		states[id] = st.sm.State()
	}
	return states
}

func (s *Scheduler) Shutdown() {
	close(s.done)
	s.wg.Wait()
}

func (s *Scheduler) SyncTargets(targets []*target.Target) {
	s.mu.RLock()
	existing := make(map[string]bool, len(s.targets))
	for id := range s.targets {
		existing[id] = true
	}
	s.mu.RUnlock()

	newIDs := make(map[string]bool, len(targets))
	for _, t := range targets {
		newIDs[t.ID] = true
		s.AddTarget(t)
	}

	for id := range existing {
		if !newIDs[id] {
			s.RemoveTarget(id)
		}
	}
}

func adaptiveInterval(state target.TargetState, base time.Duration) time.Duration {
	switch state {
	case target.StateHealthy:
		return base * 2
	case target.StateUnhealthy:
		newInterval := base / 3
		if newInterval < 5*time.Second {
			newInterval = 5 * time.Second
		}
		return newInterval
	default:
		return base
	}
}
