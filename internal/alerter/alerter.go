package alerter

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ysqss/watchdog/internal/metrics"
	"github.com/ysqss/watchdog/internal/prober"
	"github.com/ysqss/watchdog/internal/statemachine"
	"github.com/ysqss/watchdog/internal/store"
	"github.com/ysqss/watchdog/internal/target"
)

type Alerter struct {
	client            *NotifierClient
	rateLimiter       *AlertRateLimiter
	aggregator        *AlertAggregator
	store             *store.Store
	cooldowns         map[string]time.Time
	tlsWarningDays    int
	mu                sync.Mutex
}

func New(client *NotifierClient, st *store.Store, tlsWarningDays int) *Alerter {
	if tlsWarningDays <= 0 {
		tlsWarningDays = 30
	}
	a := &Alerter{
		client:         client,
		rateLimiter:    NewAlertRateLimiter(3, 5*time.Minute),
		store:          st,
		cooldowns:      make(map[string]time.Time),
		tlsWarningDays: tlsWarningDays,
	}
	a.aggregator = NewAlertAggregator(30*time.Second, a.sendBatch)
	return a
}

func (a *Alerter) OnStateChange(event statemachine.StateChangeEvent) {
	t, err := a.store.GetTargetByID(event.TargetID)
	if err != nil || t == nil {
		slog.Error("failed to get target for alert", "target_id", event.TargetID, "error", err)
		return
	}

	if t.IsSilenced() {
		slog.Info("target is silenced, skipping alert", "target", t.ID)
		return
	}

	if !a.rateLimiter.Allow(event.TargetID) {
		slog.Warn("alert rate limited", "target", event.TargetID)
		return
	}

	if a.inCooldown(event.TargetID) {
		slog.Info("target in cooldown, skipping alert", "target", event.TargetID)
		return
	}

	level := AlertLevelWarning
	if event.To == target.StateUnhealthy {
		level = AlertLevelCritical
	} else if event.To == target.StateHealthy {
		level = AlertLevelInfo
	}

	content := a.buildContent(t, event)

	msg := &AlertMessage{
		Title:     fmt.Sprintf("%s %s", t.Name, stateDescription(event.To)),
		Content:   content,
		Level:     level,
		Service:   t.ID,
		Timestamp: event.Timestamp,
	}

	a.aggregator.Enqueue(msg)

	a.recordEvent(event)

	if event.To == target.StateUnhealthy {
		a.setCooldown(event.TargetID, 5*time.Minute)
	}
}

func (a *Alerter) OnProbeResult(targetID string, result *prober.ProbeResult) {
	if result == nil || result.TLSExpiry == nil {
		return
	}

	daysUntilExpiry := time.Until(*result.TLSExpiry).Hours() / 24
	if daysUntilExpiry > float64(a.tlsWarningDays) {
		return
	}

	cooldownKey := "tls:" + targetID
	if a.inCooldown(cooldownKey) {
		return
	}

	t, err := a.store.GetTargetByID(targetID)
	if err != nil || t == nil {
		return
	}

	if t.IsSilenced() {
		return
	}

	level := AlertLevelWarning
	if daysUntilExpiry <= 7 {
		level = AlertLevelCritical
	}

	content := fmt.Sprintf("## %s\n\n", t.Name)
	content += fmt.Sprintf("**Cert Expiry**: %s\n", result.TLSExpiry.Format("2006-01-02"))
	content += fmt.Sprintf("**Days Remaining**: %.0f\n", daysUntilExpiry)
	content += fmt.Sprintf("\n**Time**: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	msg := &AlertMessage{
		Title:     fmt.Sprintf("%s TLS Certificate Expiring Soon", t.Name),
		Content:   content,
		Level:     level,
		Service:   targetID,
		Timestamp: time.Now(),
	}

	a.aggregator.Enqueue(msg)
	a.setCooldown(cooldownKey, 24*time.Hour)
}

func (a *Alerter) Shutdown() {
	slog.Info("flushing pending alerts before shutdown")
	a.aggregator.Flush()
}

func (a *Alerter) buildContent(t *target.Target, event statemachine.StateChangeEvent) string {
	content := fmt.Sprintf("## %s\n\n", t.Name)
	content += fmt.Sprintf("**Status**: %s → %s\n", event.From.String(), event.To.String())

	if event.Result != nil {
		content += fmt.Sprintf("**Status Code**: %d\n", event.Result.StatusCode)
		content += fmt.Sprintf("**Latency**: %s\n", event.Result.Latency)
		if event.Result.Error != "" {
			content += fmt.Sprintf("**Error**: %s\n", event.Result.Error)
		}
	}

	content += fmt.Sprintf("\n**Time**: %s\n", event.Timestamp.Format("2006-01-02 15:04:05"))
	return content
}

func (a *Alerter) recordEvent(event statemachine.StateChangeEvent) {
	e := &store.EventRecord{
		TargetID:  event.TargetID,
		Timestamp: event.Timestamp,
		FromState: event.From,
		ToState:   event.To,
		Message:   a.stateMessage(event),
	}
	if err := a.store.InsertEvent(e); err != nil {
		slog.Error("failed to record event", "error", err)
	}
}

func (a *Alerter) stateMessage(event statemachine.StateChangeEvent) string {
	if event.Result != nil && event.Result.Error != "" {
		return event.Result.Error
	}
	return fmt.Sprintf("state changed from %s to %s", event.From.String(), event.To.String())
}

func (a *Alerter) inCooldown(targetID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.cooldowns[targetID]
	if !ok {
		return false
	}
	return time.Now().Before(t)
}

func (a *Alerter) setCooldown(targetID string, d time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cooldowns[targetID] = time.Now().Add(d)
}

func (a *Alerter) sendBatch(msgs []*AlertMessage) {
	for _, msg := range msgs {
		metrics.RecordAlert(msg.Service, string(msg.Level))
	}

	if len(msgs) == 1 {
		ctx, cancel := contextWithTimeout(10 * time.Second)
		defer cancel()
		if err := a.client.Send(ctx, msgs[0]); err != nil {
			slog.Error("failed to send alert", "error", err)
		}
		return
	}

	merged := mergeMessages(msgs)
	ctx, cancel := contextWithTimeout(10 * time.Second)
	defer cancel()
	if err := a.client.Send(ctx, merged); err != nil {
		slog.Error("failed to send aggregated alert", "error", err)
	}
}

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

func stateDescription(state target.TargetState) string {
	switch state {
	case target.StateHealthy:
		return "Recovered"
	case target.StateDegraded:
		return "Degraded"
	case target.StateUnhealthy:
		return "Unhealthy"
	default:
		return "State Changed"
	}
}

func mergeMessages(msgs []*AlertMessage) *AlertMessage {
	critical := 0
	warning := 0
	info := 0
	content := ""

	for _, m := range msgs {
		switch m.Level {
		case AlertLevelCritical:
			critical++
		case AlertLevelWarning:
			warning++
		case AlertLevelInfo:
			info++
		}
		content += fmt.Sprintf("- %s\n", m.Title)
	}

	level := AlertLevelInfo
	if critical > 0 {
		level = AlertLevelCritical
	} else if warning > 0 {
		level = AlertLevelWarning
	}

	return &AlertMessage{
		Title:     fmt.Sprintf("%d Service State Changes", len(msgs)),
		Content:   content,
		Level:     level,
		Service:   "watchdog",
		Timestamp: time.Now(),
	}
}
