package game

import (
	"context"
	"time"
)

const TickDuration = 50 * time.Millisecond // 20 TPS

// TickHandler is called every game tick.
type TickHandler interface {
	Tick(tick int64)
}

// TickHandlerFunc adapts a function to the TickHandler interface.
type TickHandlerFunc func(tick int64)

func (f TickHandlerFunc) Tick(tick int64) { f(tick) }

// TickLoop runs game logic at 20 ticks per second.
type TickLoop struct {
	handlers []TickHandler
}

// NewTickLoop creates a TickLoop with the given handlers.
func NewTickLoop(handlers ...TickHandler) *TickLoop {
	return &TickLoop{handlers: handlers}
}

// AddHandler appends a handler to the tick loop. Not safe to call after Run.
func (t *TickLoop) AddHandler(h TickHandler) {
	t.handlers = append(t.handlers, h)
}

// Run starts the tick loop, blocking until ctx is cancelled.
// Returns the total number of ticks executed.
func (t *TickLoop) Run(ctx context.Context) int64 {
	ticker := time.NewTicker(TickDuration)
	defer ticker.Stop()

	var tick int64
	for {
		select {
		case <-ctx.Done():
			return tick
		case <-ticker.C:
			for _, h := range t.handlers {
				h.Tick(tick)
			}
			tick++
		}
	}
}
