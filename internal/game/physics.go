// physics.go contains grid constants and server-side move validation.
package game

import "math"

const (
	CellSize       = 40  // pixels per grid cell; equals player diameter
	GridCols       = 25  // number of columns  (canvas width  = 1000px)
	GridRows       = 17  // number of rows     (canvas height = 680px)
	MovementRadius = 5.0 // maximum Euclidean distance a player may move per action
)

// isValidMove returns true when (gx2,gy2) is within MovementRadius of (gx1,gy1)
// and is not the same cell. Euclidean distance gives a natural circular reach area.
func isValidMove(gx1, gy1, gx2, gy2 int) bool {
	if gx1 == gx2 && gy1 == gy2 {
		return false // standing still is not a move
	}
	dx := float64(gx2 - gx1)
	dy := float64(gy2 - gy1)
	return math.Sqrt(dx*dx+dy*dy) <= MovementRadius
}

// clampToGrid keeps grid coordinates inside the valid cell range.
func clampToGrid(gx, gy int) (int, int) {
	return clampInt(gx, 0, GridCols-1), clampInt(gy, 0, GridRows-1)
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
