package cluster

import "sync/atomic"

const eidsPerServer = 134_217_728 // 2^27 = ~134M entity IDs per server

// EIDAllocator provides range-based entity ID allocation to prevent collisions
// across servers. Server index 0 gets EIDs [1, 134M), server 1 gets [134M, 268M), etc.
type EIDAllocator struct {
	next atomic.Int32
	max  int32
}

// NewEIDAllocator creates an allocator for the given server index (0-based).
func NewEIDAllocator(serverIndex int) *EIDAllocator {
	base := int32(serverIndex*eidsPerServer) + 1 // +1 because EID 0 is reserved
	if serverIndex == 0 {
		base = 1
	}
	a := &EIDAllocator{
		max: int32((serverIndex + 1) * eidsPerServer),
	}
	a.next.Store(base)
	return a
}

// Next returns the next entity ID in this server's range.
func (a *EIDAllocator) Next() int32 {
	id := a.next.Add(1) - 1
	if id >= a.max {
		panic("cluster: entity ID range exhausted")
	}
	return id
}

// AllocFunc returns a function compatible with game.PlayerManager.EIDAllocFunc.
func (a *EIDAllocator) AllocFunc() func() int32 {
	return a.Next
}
