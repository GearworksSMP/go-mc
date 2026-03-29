package cluster

import "github.com/google/uuid"

// BorderEntityUpdate is published to a server's border channel to sync entities near region boundaries.
type BorderEntityUpdate struct {
	SourceServer string         `json:"source_server"`
	Entities     []BorderEntity `json:"entities"`
}

// BorderEntity represents a single entity near a region boundary visible to adjacent servers.
type BorderEntity struct {
	EID    int32     `json:"eid"`
	TypeID int32     `json:"type_id"` // entity type registry ID
	X      float64   `json:"x"`
	Y      float64   `json:"y"`
	Z      float64   `json:"z"`
	Yaw    float32   `json:"yaw"`
	Pitch  float32   `json:"pitch"`
	Name   string    `json:"name,omitempty"` // for players
	UUID   uuid.UUID `json:"uuid"`
}
