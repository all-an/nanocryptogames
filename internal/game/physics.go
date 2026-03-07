// physics.go contains arena constants and server-side movement logic.
package game

const (
	ArenaWidth   = 1000.0
	ArenaHeight  = 700.0
	PlayerRadius = 20.0
	MoveSpeed    = 5.0 // pixels per tick at 20 TPS = 100 px/s
)

// clampToArena ensures a position stays within arena bounds
// accounting for the player circle radius.
func clampToArena(x, y float64) (float64, float64) {
	return clamp(x, PlayerRadius, ArenaWidth-PlayerRadius),
		clamp(y, PlayerRadius, ArenaHeight-PlayerRadius)
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
