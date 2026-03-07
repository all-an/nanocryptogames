// physics.go contains grid constants and server-side move validation.
package game

const (
	CellSize = 40   // pixels per grid cell; equals player diameter
	GridCols = 25   // number of columns  (canvas width  = 1000px)
	GridRows = 17   // number of rows     (canvas height = 680px)
)

// clampToGrid keeps grid coordinates inside the valid cell range.
func clampToGrid(gx, gy int) (int, int) {
	return clampInt(gx, 0, GridCols-1), clampInt(gy, 0, GridRows-1)
}

// isAdjacentMove returns true when (gx2,gy2) is exactly one step
// (up, down, left, or right) from (gx1,gy1). Diagonal moves are not allowed.
func isAdjacentMove(gx1, gy1, gx2, gy2 int) bool {
	dx := gx2 - gx1
	dy := gy2 - gy1
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx+dy == 1
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
