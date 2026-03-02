package game

// ActiveEffect represents an active status effect on a player.
// Defined in the game package so it can be used in the Player struct.
// The handler package manages effect logic and tick processing.
type ActiveEffect struct {
	ID       int32
	Level    int32 // 0-indexed (potion level 1 = 0)
	Duration int32 // ticks remaining (-1 = infinite)
	Ambient  bool
}
