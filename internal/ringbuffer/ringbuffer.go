package ringbuffer

import (
	"sort"
	"sync"
	"time"
)

type RingBuffer[T any] struct {
	buf    []T
	size   int
	cursor int
	full   bool
	mu     sync.Mutex
}

func New[T any](size int) *RingBuffer[T] {
	if size <= 0 {
		size = 10
	}
	return &RingBuffer[T]{
		buf:  make([]T, 0, size),
		size: size,
	}
}

func (rb *RingBuffer[T]) Push(v T) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if len(rb.buf) < rb.size {
		rb.buf = append(rb.buf, v)
	} else {
		rb.buf[rb.cursor] = v
		rb.cursor = (rb.cursor + 1) % rb.size
		if rb.cursor == 0 {
			rb.full = true
		}
	}
}

func (rb *RingBuffer[T]) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.full {
		return rb.size
	}
	return len(rb.buf)
}

func (rb *RingBuffer[T]) All() []T {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if !rb.full || len(rb.buf) < rb.size {
		result := make([]T, len(rb.buf))
		copy(result, rb.buf)
		return result
	}

	result := make([]T, rb.size)
	copy(result, rb.buf[rb.cursor:])
	copy(result[rb.size-rb.cursor:], rb.buf[:rb.cursor])
	return result
}

func (rb *RingBuffer[T]) AllSuccess() bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	items := rb.itemsLocked()
	for _, v := range items {
		b, ok := any(v).(bool)
		if !ok || !b {
			return false
		}
	}
	return len(items) > 0
}

func (rb *RingBuffer[T]) AllFail() bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	items := rb.itemsLocked()
	for _, v := range items {
		b, ok := any(v).(bool)
		if !ok || b {
			return false
		}
	}
	return len(items) > 0
}

func (rb *RingBuffer[T]) P50() time.Duration {
	return rb.Percentile(50)
}

func (rb *RingBuffer[T]) P95() time.Duration {
	return rb.Percentile(95)
}

func (rb *RingBuffer[T]) P99() time.Duration {
	return rb.Percentile(99)
}

func (rb *RingBuffer[T]) Percentile(p int) time.Duration {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	items := rb.itemsLocked()
	if len(items) == 0 {
		return 0
	}

	durations := make([]time.Duration, 0, len(items))
	for _, v := range items {
		if d, ok := any(v).(time.Duration); ok {
			durations = append(durations, d)
		}
	}
	if len(durations) == 0 {
		return 0
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	idx := float64(p) / 100.0 * float64(len(durations)-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= len(durations) {
		return durations[len(durations)-1]
	}
	frac := idx - float64(lo)
	return time.Duration(float64(durations[lo]) + frac*float64(durations[hi]-durations[lo]))
}

func (rb *RingBuffer[T]) itemsLocked() []T {
	if !rb.full || len(rb.buf) < rb.size {
		result := make([]T, len(rb.buf))
		copy(result, rb.buf)
		return result
	}

	result := make([]T, rb.size)
	copy(result, rb.buf[rb.cursor:])
	copy(result[rb.size-rb.cursor:], rb.buf[:rb.cursor])
	return result
}
