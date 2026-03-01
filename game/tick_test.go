package game

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestTickLoop20TPS(t *testing.T) {
	var count atomic.Int64
	handler := TickHandlerFunc(func(tick int64) {
		count.Add(1)
	})

	loop := NewTickLoop(handler)
	ctx, cancel := context.WithTimeout(context.Background(), 520*time.Millisecond)
	defer cancel()

	loop.Run(ctx)

	ticks := count.Load()
	// 520ms / 50ms = 10.4 ticks expected. Allow 8-12 for timing jitter.
	if ticks < 8 || ticks > 12 {
		t.Errorf("Expected ~10 ticks in 520ms, got %d", ticks)
	}
	t.Logf("Ticks in 520ms: %d", ticks)
}

func TestTickLoopMultipleHandlers(t *testing.T) {
	var count1, count2 atomic.Int64
	h1 := TickHandlerFunc(func(tick int64) { count1.Add(1) })
	h2 := TickHandlerFunc(func(tick int64) { count2.Add(1) })

	loop := NewTickLoop(h1, h2)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	loop.Run(ctx)

	if count1.Load() != count2.Load() {
		t.Errorf("Handlers called different number of times: %d vs %d", count1.Load(), count2.Load())
	}
}
