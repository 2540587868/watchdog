package ringbuffer

import (
	"testing"
	"time"
)

func TestNew_DefaultSize(t *testing.T) {
	rb := New[int](0)
	if rb == nil {
		t.Fatal("New(0) returned nil")
	}
	if rb.size != 10 {
		t.Errorf("New(0).size = %d, want 10", rb.size)
	}
}

func TestNew_NegativeSize(t *testing.T) {
	rb := New[int](-5)
	if rb.size != 10 {
		t.Errorf("New(-5).size = %d, want 10", rb.size)
	}
}

func TestNew_ValidSize(t *testing.T) {
	rb := New[int](5)
	if rb.size != 5 {
		t.Errorf("New(5).size = %d, want 5", rb.size)
	}
}

func TestPush_AndLen(t *testing.T) {
	rb := New[int](3)
	if rb.Len() != 0 {
		t.Errorf("Len() = %d, want 0", rb.Len())
	}

	rb.Push(1)
	rb.Push(2)
	if rb.Len() != 2 {
		t.Errorf("Len() = %d, want 2", rb.Len())
	}

	rb.Push(3)
	if rb.Len() != 3 {
		t.Errorf("Len() = %d, want 3", rb.Len())
	}
}

func TestPush_Wrapping(t *testing.T) {
	rb := New[int](3)
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Push(4)
	rb.Push(5)
	rb.Push(6)

	if rb.Len() != 3 {
		t.Errorf("Len() after wrap = %d, want 3", rb.Len())
	}

	all := rb.All()
	if len(all) != 3 {
		t.Fatalf("All() length = %d, want 3", len(all))
	}

	expected := []int{4, 5, 6}
	for i, v := range expected {
		if all[i] != v {
			t.Errorf("All()[%d] = %d, want %d", i, all[i], v)
		}
	}
}

func TestAll_Empty(t *testing.T) {
	rb := New[int](5)
	all := rb.All()
	if len(all) != 0 {
		t.Errorf("All() length = %d, want 0", len(all))
	}
}

func TestAll_Partial(t *testing.T) {
	rb := New[int](5)
	rb.Push(10)
	rb.Push(20)
	all := rb.All()
	if len(all) != 2 {
		t.Fatalf("All() length = %d, want 2", len(all))
	}
	if all[0] != 10 || all[1] != 20 {
		t.Errorf("All() = %v, want [10 20]", all)
	}
}

func TestAll_Full(t *testing.T) {
	rb := New[int](3)
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	all := rb.All()
	expected := []int{1, 2, 3}
	for i, v := range expected {
		if all[i] != v {
			t.Errorf("All()[%d] = %d, want %d", i, all[i], v)
		}
	}
}

func TestAll_Wrapped(t *testing.T) {
	rb := New[int](3)
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Push(4)
	rb.Push(5)
	all := rb.All()
	expected := []int{4, 5, 3}
	for i, v := range expected {
		if all[i] != v {
			t.Errorf("All()[%d] = %d, want %d", i, all[i], v)
		}
	}
}

func TestAll_WrappedTwice(t *testing.T) {
	rb := New[int](3)
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Push(4)
	rb.Push(5)
	rb.Push(6)
	all := rb.All()
	expected := []int{4, 5, 6}
	for i, v := range expected {
		if all[i] != v {
			t.Errorf("All()[%d] = %d, want %d", i, all[i], v)
		}
	}
}

func TestAllSuccess_AllTrue(t *testing.T) {
	rb := New[bool](3)
	rb.Push(true)
	rb.Push(true)
	rb.Push(true)
	if !rb.AllSuccess() {
		t.Error("AllSuccess() = false, want true")
	}
}

func TestAllSuccess_Mixed(t *testing.T) {
	rb := New[bool](3)
	rb.Push(true)
	rb.Push(false)
	rb.Push(true)
	if rb.AllSuccess() {
		t.Error("AllSuccess() = true, want false")
	}
}

func TestAllSuccess_Empty(t *testing.T) {
	rb := New[bool](3)
	if rb.AllSuccess() {
		t.Error("AllSuccess() on empty buffer = true, want false")
	}
}

func TestAllSuccess_AllFalse(t *testing.T) {
	rb := New[bool](3)
	rb.Push(false)
	rb.Push(false)
	if rb.AllSuccess() {
		t.Error("AllSuccess() with all false = true, want false")
	}
}

func TestAllFail_AllFalse(t *testing.T) {
	rb := New[bool](3)
	rb.Push(false)
	rb.Push(false)
	rb.Push(false)
	if !rb.AllFail() {
		t.Error("AllFail() = false, want true")
	}
}

func TestAllFail_Mixed(t *testing.T) {
	rb := New[bool](3)
	rb.Push(false)
	rb.Push(true)
	rb.Push(false)
	if rb.AllFail() {
		t.Error("AllFail() = true, want false")
	}
}

func TestAllFail_Empty(t *testing.T) {
	rb := New[bool](3)
	if rb.AllFail() {
		t.Error("AllFail() on empty buffer = true, want false")
	}
}

func TestAllFail_AllTrue(t *testing.T) {
	rb := New[bool](3)
	rb.Push(true)
	rb.Push(true)
	if rb.AllFail() {
		t.Error("AllFail() with all true = true, want false")
	}
}

func TestPercentile_Empty(t *testing.T) {
	rb := New[time.Duration](5)
	got := rb.Percentile(50)
	if got != 0 {
		t.Errorf("Percentile(50) on empty = %v, want 0", got)
	}
}

func TestPercentile_Single(t *testing.T) {
	rb := New[time.Duration](5)
	rb.Push(100 * time.Millisecond)
	got := rb.Percentile(50)
	if got != 100*time.Millisecond {
		t.Errorf("Percentile(50) = %v, want %v", got, 100*time.Millisecond)
	}
}

func TestPercentile_P50(t *testing.T) {
	rb := New[time.Duration](10)
	rb.Push(10 * time.Millisecond)
	rb.Push(20 * time.Millisecond)
	rb.Push(30 * time.Millisecond)
	rb.Push(40 * time.Millisecond)
	rb.Push(50 * time.Millisecond)

	p50 := rb.P50()
	if p50 < 20*time.Millisecond || p50 > 40*time.Millisecond {
		t.Errorf("P50() = %v, want between 20ms and 40ms", p50)
	}
}

func TestPercentile_P95(t *testing.T) {
	rb := New[time.Duration](10)
	for i := 1; i <= 10; i++ {
		rb.Push(time.Duration(i) * 10 * time.Millisecond)
	}

	p95 := rb.P95()
	if p95 < 80*time.Millisecond {
		t.Errorf("P95() = %v, want >= 80ms", p95)
	}
}

func TestPercentile_P99(t *testing.T) {
	rb := New[time.Duration](10)
	for i := 1; i <= 10; i++ {
		rb.Push(time.Duration(i) * 10 * time.Millisecond)
	}

	p99 := rb.P99()
	if p99 < 80*time.Millisecond {
		t.Errorf("P99() = %v, want >= 80ms", p99)
	}
}

func TestPercentile_100(t *testing.T) {
	rb := New[time.Duration](5)
	rb.Push(10 * time.Millisecond)
	rb.Push(20 * time.Millisecond)
	rb.Push(30 * time.Millisecond)

	p100 := rb.Percentile(100)
	if p100 != 30*time.Millisecond {
		t.Errorf("Percentile(100) = %v, want %v", p100, 30*time.Millisecond)
	}
}

func TestPercentile_WrappedBuffer(t *testing.T) {
	rb := New[time.Duration](3)
	rb.Push(10 * time.Millisecond)
	rb.Push(20 * time.Millisecond)
	rb.Push(30 * time.Millisecond)
	rb.Push(40 * time.Millisecond)

	all := rb.All()
	if len(all) != 3 {
		t.Fatalf("All() length = %d, want 3", len(all))
	}

	p50 := rb.P50()
	if p50 < 20*time.Millisecond || p50 > 40*time.Millisecond {
		t.Errorf("P50() after wrap = %v, want between 20ms and 40ms", p50)
	}
}

func TestAll_ReturnsCopy(t *testing.T) {
	rb := New[int](3)
	rb.Push(1)
	rb.Push(2)
	all := rb.All()
	all[0] = 99
	original := rb.All()
	if original[0] == 99 {
		t.Error("All() did not return a copy; modifying the result affected the buffer")
	}
}
