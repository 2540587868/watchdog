package alerter

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ysqss/watchdog/internal/prober"
	"github.com/ysqss/watchdog/internal/statemachine"
	"github.com/ysqss/watchdog/internal/store"
	"github.com/ysqss/watchdog/internal/target"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	return s
}

func insertTarget(t *testing.T, s *store.Store, tgt *target.Target) {
	t.Helper()
	if err := s.InsertTarget(tgt); err != nil {
		t.Fatalf("insert target: %v", err)
	}
}

func newTestAlerter(client *NotifierClient, st *store.Store, tlsWarningDays int) *Alerter {
	a := New(client, st, tlsWarningDays)
	a.aggregator = NewAlertAggregator(50*time.Millisecond, a.sendBatch)
	return a
}

func TestNew_DefaultTLSWarningDays(t *testing.T) {
	s := newTestStore(t)
	a := New(nil, s, 0)
	if a.tlsWarningDays != 30 {
		t.Errorf("tlsWarningDays = %d, want 30", a.tlsWarningDays)
	}
}

func TestNew_CustomTLSWarningDays(t *testing.T) {
	s := newTestStore(t)
	a := New(nil, s, 14)
	if a.tlsWarningDays != 14 {
		t.Errorf("tlsWarningDays = %d, want 14", a.tlsWarningDays)
	}
}

func TestOnStateChange_UnhealthyAlert(t *testing.T) {
	s := newTestStore(t)
	tgt := &target.Target{
		ID:   "svc-1",
		Name: "Service1",
		Type: target.ProbeHTTP,
		URL:  "http://example.com",
	}
	insertTarget(t, s, tgt)

	var mu sync.Mutex
	var receivedBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		_ = json.Unmarshal(body, &receivedBody)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewNotifierClient(srv.URL, "test-token")
	a := newTestAlerter(client, s, 30)

	event := statemachine.StateChangeEvent{
		TargetID:  "svc-1",
		From:      target.StateHealthy,
		To:        target.StateUnhealthy,
		Result:    &prober.ProbeResult{StatusCode: 500, Latency: 100 * time.Millisecond},
		Timestamp: time.Now(),
	}

	a.OnStateChange(event)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if receivedBody == nil {
		t.Fatal("no alert was sent to the notifier server")
	}
	if receivedBody["level"] != "critical" {
		t.Errorf("alert level = %v, want critical", receivedBody["level"])
	}
}

func TestOnStateChange_HealthyAlert(t *testing.T) {
	s := newTestStore(t)
	tgt := &target.Target{
		ID:   "svc-2",
		Name: "Service2",
		Type: target.ProbeHTTP,
		URL:  "http://example.com",
	}
	insertTarget(t, s, tgt)

	var mu sync.Mutex
	var receivedBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		_ = json.Unmarshal(body, &receivedBody)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewNotifierClient(srv.URL, "test-token")
	a := newTestAlerter(client, s, 30)

	event := statemachine.StateChangeEvent{
		TargetID:  "svc-2",
		From:      target.StateUnhealthy,
		To:        target.StateHealthy,
		Result:    &prober.ProbeResult{StatusCode: 200, Latency: 50 * time.Millisecond},
		Timestamp: time.Now(),
	}

	a.OnStateChange(event)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if receivedBody == nil {
		t.Fatal("no alert was sent")
	}
	if receivedBody["level"] != "info" {
		t.Errorf("alert level = %v, want info", receivedBody["level"])
	}
}

func TestOnStateChange_SilencedTarget(t *testing.T) {
	s := newTestStore(t)
	future := time.Now().Add(1 * time.Hour)
	tgt := &target.Target{
		ID:            "svc-3",
		Name:          "Service3",
		Type:          target.ProbeHTTP,
		URL:           "http://example.com",
		SilencedUntil: &future,
	}
	insertTarget(t, s, tgt)

	var alertSent atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alertSent.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewNotifierClient(srv.URL, "test-token")
	a := newTestAlerter(client, s, 30)

	event := statemachine.StateChangeEvent{
		TargetID:  "svc-3",
		From:      target.StateHealthy,
		To:        target.StateUnhealthy,
		Timestamp: time.Now(),
	}

	a.OnStateChange(event)
	time.Sleep(200 * time.Millisecond)

	if alertSent.Load() {
		t.Error("alert should not be sent for silenced target")
	}
}

func TestOnStateChange_RateLimited(t *testing.T) {
	s := newTestStore(t)
	tgt := &target.Target{
		ID:   "svc-4",
		Name: "Service4",
		Type: target.ProbeHTTP,
		URL:  "http://example.com",
	}
	insertTarget(t, s, tgt)

	var alertCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alertCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewNotifierClient(srv.URL, "test-token")
	a := newTestAlerter(client, s, 30)

	for i := 0; i < 5; i++ {
		event := statemachine.StateChangeEvent{
			TargetID:  "svc-4",
			From:      target.StateHealthy,
			To:        target.StateUnhealthy,
			Timestamp: time.Now(),
		}
		a.OnStateChange(event)
	}

	time.Sleep(200 * time.Millisecond)

	if alertCount.Load() > 3 {
		t.Errorf("sent %d alerts, expected rate limiting to cap at 3", alertCount.Load())
	}
}

func TestOnStateChange_Cooldown(t *testing.T) {
	s := newTestStore(t)
	tgt := &target.Target{
		ID:   "svc-5",
		Name: "Service5",
		Type: target.ProbeHTTP,
		URL:  "http://example.com",
	}
	insertTarget(t, s, tgt)

	var alertCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alertCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewNotifierClient(srv.URL, "test-token")
	a := newTestAlerter(client, s, 30)

	event1 := statemachine.StateChangeEvent{
		TargetID:  "svc-5",
		From:      target.StateHealthy,
		To:        target.StateUnhealthy,
		Timestamp: time.Now(),
	}
	a.OnStateChange(event1)

	time.Sleep(200 * time.Millisecond)

	if alertCount.Load() < 1 {
		t.Error("expected at least one alert to be sent")
	}

	event2 := statemachine.StateChangeEvent{
		TargetID:  "svc-5",
		From:      target.StateUnhealthy,
		To:        target.StateHealthy,
		Timestamp: time.Now(),
	}
	a.OnStateChange(event2)

	if !a.inCooldown("svc-5") {
		t.Error("target should be in cooldown after unhealthy alert")
	}
}

func TestOnStateChange_UnknownTarget(t *testing.T) {
	s := newTestStore(t)

	var alertSent atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alertSent.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewNotifierClient(srv.URL, "test-token")
	a := newTestAlerter(client, s, 30)

	event := statemachine.StateChangeEvent{
		TargetID:  "nonexistent",
		From:      target.StateHealthy,
		To:        target.StateUnhealthy,
		Timestamp: time.Now(),
	}

	a.OnStateChange(event)
	time.Sleep(200 * time.Millisecond)

	if alertSent.Load() {
		t.Error("alert should not be sent for unknown target")
	}
}

func TestOnProbeResult_TLSExpiryWarning(t *testing.T) {
	s := newTestStore(t)
	tgt := &target.Target{
		ID:   "svc-tls",
		Name: "TLSService",
		Type: target.ProbeHTTP,
		URL:  "https://example.com",
	}
	insertTarget(t, s, tgt)

	var mu sync.Mutex
	var receivedBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		_ = json.Unmarshal(body, &receivedBody)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewNotifierClient(srv.URL, "test-token")
	a := newTestAlerter(client, s, 30)

	expiry := time.Now().Add(15 * 24 * time.Hour)
	result := &prober.ProbeResult{
		TargetID:  "svc-tls",
		Success:   true,
		TLSExpiry: &expiry,
		Timestamp: time.Now(),
	}

	a.OnProbeResult("svc-tls", result)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if receivedBody == nil {
		t.Fatal("no TLS warning alert was sent")
	}
	if receivedBody["level"] != "warning" {
		t.Errorf("alert level = %v, want warning", receivedBody["level"])
	}
}

func TestOnProbeResult_TLSExpiryCritical(t *testing.T) {
	s := newTestStore(t)
	tgt := &target.Target{
		ID:   "svc-tls2",
		Name: "TLSService2",
		Type: target.ProbeHTTP,
		URL:  "https://example.com",
	}
	insertTarget(t, s, tgt)

	var mu sync.Mutex
	var receivedBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		_ = json.Unmarshal(body, &receivedBody)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewNotifierClient(srv.URL, "test-token")
	a := newTestAlerter(client, s, 30)

	expiry := time.Now().Add(5 * 24 * time.Hour)
	result := &prober.ProbeResult{
		TargetID:  "svc-tls2",
		Success:   true,
		TLSExpiry: &expiry,
		Timestamp: time.Now(),
	}

	a.OnProbeResult("svc-tls2", result)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if receivedBody == nil {
		t.Fatal("no TLS critical alert was sent")
	}
	if receivedBody["level"] != "critical" {
		t.Errorf("alert level = %v, want critical", receivedBody["level"])
	}
}

func TestOnProbeResult_TLSNotExpiring(t *testing.T) {
	s := newTestStore(t)
	tgt := &target.Target{
		ID:   "svc-tls3",
		Name: "TLSService3",
		Type: target.ProbeHTTP,
		URL:  "https://example.com",
	}
	insertTarget(t, s, tgt)

	var alertSent atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alertSent.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewNotifierClient(srv.URL, "test-token")
	a := newTestAlerter(client, s, 30)

	expiry := time.Now().Add(90 * 24 * time.Hour)
	result := &prober.ProbeResult{
		TargetID:  "svc-tls3",
		Success:   true,
		TLSExpiry: &expiry,
		Timestamp: time.Now(),
	}

	a.OnProbeResult("svc-tls3", result)
	time.Sleep(200 * time.Millisecond)

	if alertSent.Load() {
		t.Error("alert should not be sent when TLS is not expiring soon")
	}
}

func TestOnProbeResult_NilResult(t *testing.T) {
	s := newTestStore(t)
	a := New(nil, s, 30)

	a.OnProbeResult("any", nil)
}

func TestOnProbeResult_NilTLSExpiry(t *testing.T) {
	s := newTestStore(t)
	a := New(nil, s, 30)

	result := &prober.ProbeResult{
		TargetID:  "any",
		Success:   true,
		TLSExpiry: nil,
	}

	a.OnProbeResult("any", result)
}

func TestOnProbeResult_TLSCooldown(t *testing.T) {
	s := newTestStore(t)
	tgt := &target.Target{
		ID:   "svc-tls4",
		Name: "TLSService4",
		Type: target.ProbeHTTP,
		URL:  "https://example.com",
	}
	insertTarget(t, s, tgt)

	var alertCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alertCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewNotifierClient(srv.URL, "test-token")
	a := newTestAlerter(client, s, 30)

	expiry := time.Now().Add(15 * 24 * time.Hour)
	result := &prober.ProbeResult{
		TargetID:  "svc-tls4",
		Success:   true,
		TLSExpiry: &expiry,
	}

	a.OnProbeResult("svc-tls4", result)
	time.Sleep(200 * time.Millisecond)

	firstCount := alertCount.Load()

	a.OnProbeResult("svc-tls4", result)
	time.Sleep(200 * time.Millisecond)

	if firstCount != 1 {
		t.Errorf("first call sent %d alerts, want 1", firstCount)
	}
	if alertCount.Load() != 1 {
		t.Errorf("sent %d alerts after second call, want 1 (second should be cooldown-blocked)", alertCount.Load())
	}
}

func TestAlertRateLimiter_Allow(t *testing.T) {
	rl := NewAlertRateLimiter(3, 5*time.Minute)

	if !rl.Allow("target-1") {
		t.Error("first alert should be allowed")
	}
	if !rl.Allow("target-1") {
		t.Error("second alert should be allowed")
	}
	if !rl.Allow("target-1") {
		t.Error("third alert should be allowed")
	}
	if rl.Allow("target-1") {
		t.Error("fourth alert should be rate limited")
	}
}

func TestAlertRateLimiter_DifferentTargets(t *testing.T) {
	rl := NewAlertRateLimiter(2, 5*time.Minute)

	if !rl.Allow("target-a") {
		t.Error("target-a first alert should be allowed")
	}
	if !rl.Allow("target-a") {
		t.Error("target-a second alert should be allowed")
	}
	if !rl.Allow("target-b") {
		t.Error("target-b first alert should be allowed (independent target)")
	}
	if rl.Allow("target-a") {
		t.Error("target-a third alert should be rate limited")
	}
}

func TestAlertAggregator_Flush(t *testing.T) {
	var mu sync.Mutex
	var flushed []*AlertMessage

	agg := NewAlertAggregator(50*time.Millisecond, func(msgs []*AlertMessage) {
		mu.Lock()
		flushed = append(flushed, msgs...)
		mu.Unlock()
	})

	agg.Enqueue(&AlertMessage{Title: "test1", Level: AlertLevelWarning, Service: "s1"})
	agg.Enqueue(&AlertMessage{Title: "test2", Level: AlertLevelCritical, Service: "s2"})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 2 {
		t.Errorf("flushed %d messages, want 2", len(flushed))
	}
}

func TestMergeMessages(t *testing.T) {
	msgs := []*AlertMessage{
		{Title: "svc1 down", Level: AlertLevelCritical, Service: "svc1"},
		{Title: "svc2 slow", Level: AlertLevelWarning, Service: "svc2"},
		{Title: "svc3 ok", Level: AlertLevelInfo, Service: "svc3"},
	}

	merged := mergeMessages(msgs)
	if merged.Level != AlertLevelCritical {
		t.Errorf("merged level = %v, want critical (highest)", merged.Level)
	}
	if merged.Service != "watchdog" {
		t.Errorf("merged service = %v, want watchdog", merged.Service)
	}
}

func TestMergeMessages_OnlyWarnings(t *testing.T) {
	msgs := []*AlertMessage{
		{Title: "svc1 slow", Level: AlertLevelWarning, Service: "svc1"},
		{Title: "svc2 slow", Level: AlertLevelWarning, Service: "svc2"},
	}

	merged := mergeMessages(msgs)
	if merged.Level != AlertLevelWarning {
		t.Errorf("merged level = %v, want warning", merged.Level)
	}
}

func TestMergeMessages_OnlyInfo(t *testing.T) {
	msgs := []*AlertMessage{
		{Title: "svc1 ok", Level: AlertLevelInfo, Service: "svc1"},
	}

	merged := mergeMessages(msgs)
	if merged.Level != AlertLevelInfo {
		t.Errorf("merged level = %v, want info", merged.Level)
	}
}

func TestStateDescription(t *testing.T) {
	tests := []struct {
		state target.TargetState
		want  string
	}{
		{target.StateHealthy, "Recovered"},
		{target.StateDegraded, "Degraded"},
		{target.StateUnhealthy, "Unhealthy"},
		{target.StateUnknown, "State Changed"},
	}
	for _, tt := range tests {
		got := stateDescription(tt.state)
		if got != tt.want {
			t.Errorf("stateDescription(%v) = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestInCooldown_NotSet(t *testing.T) {
	s := newTestStore(t)
	a := New(nil, s, 30)
	if a.inCooldown("any-target") {
		t.Error("inCooldown should be false for unset target")
	}
}

func TestSetCooldown_AndInCooldown(t *testing.T) {
	s := newTestStore(t)
	a := New(nil, s, 30)
	a.setCooldown("target-1", 5*time.Minute)
	if !a.inCooldown("target-1") {
		t.Error("inCooldown should be true after setCooldown")
	}
}

func TestCooldown_Expiry(t *testing.T) {
	s := newTestStore(t)
	a := New(nil, s, 30)
	a.setCooldown("target-2", 1*time.Nanosecond)
	time.Sleep(10 * time.Millisecond)
	if a.inCooldown("target-2") {
		t.Error("inCooldown should be false after cooldown expires")
	}
}
