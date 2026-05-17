package alerter

import (
	"log/slog"
	"sync"
	"time"
)

type AlertAggregator struct {
	mu      sync.Mutex
	buffer  []*AlertMessage
	timer   *time.Timer
	window  time.Duration
	onFlush func([]*AlertMessage)
}

func NewAlertAggregator(window time.Duration, onFlush func([]*AlertMessage)) *AlertAggregator {
	return &AlertAggregator{
		window:  window,
		onFlush: onFlush,
	}
}

func (a *AlertAggregator) Enqueue(msg *AlertMessage) {
	a.mu.Lock()
	a.buffer = append(a.buffer, msg)
	count := len(a.buffer)
	a.mu.Unlock()

	if count == 1 {
		a.mu.Lock()
		a.timer = time.AfterFunc(a.window, func() {
			a.flush()
		})
		a.mu.Unlock()
	}

	slog.Info("alert enqueued", "title", msg.Title, "buffer_size", count)
}

func (a *AlertAggregator) flush() {
	a.mu.Lock()
	msgs := a.buffer
	a.buffer = nil
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
	a.mu.Unlock()

	if len(msgs) == 0 {
		return
	}

	slog.Info("flushing alerts", "count", len(msgs))
	a.onFlush(msgs)
}

func (a *AlertAggregator) Flush() {
	a.flush()
}
