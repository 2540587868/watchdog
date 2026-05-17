package alerter

import (
	"sync"
	"time"
)

type AlertRateLimiter struct {
	mu       sync.Mutex
	windows  map[string][]time.Time
	maxPer   int
	interval time.Duration
}

func NewAlertRateLimiter(maxPer int, interval time.Duration) *AlertRateLimiter {
	return &AlertRateLimiter{
		windows:  make(map[string][]time.Time),
		maxPer:   maxPer,
		interval: interval,
	}
}

func (r *AlertRateLimiter) Allow(targetID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.interval)

	timestamps := r.windows[targetID]
	filtered := make([]time.Time, 0, len(timestamps))
	for _, t := range timestamps {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}

	if len(filtered) >= r.maxPer {
		r.windows[targetID] = filtered
		return false
	}

	filtered = append(filtered, now)
	r.windows[targetID] = filtered
	return true
}
