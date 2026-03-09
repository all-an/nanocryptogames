// physics.go contains grid constants, server-side move validation, and the
// static barrier map used in faucet mode to give players cover.
package game

import "math"

const (
	CellSize       = 40  // pixels per grid cell; equals player diameter
	GridCols       = 25  // number of columns  (canvas width  = 1000px)
	GridRows       = 17  // number of rows     (canvas height = 680px)
	MovementRadius = 5.0 // maximum Euclidean distance a player may move per action
)

// barrierCells is the fixed set of impassable grid cells in faucet mode.
// Mirrors the BARRIERS set in faucet_game.js — keep both in sync.
//
// Layout (25×17 grid):
//   3×3 blocks at the four inner corners  → big cover pillars
//   2×2 blocks flanking the centre lane   → mid-field cover
//   1×1 single cells                      → small scattered cover
var barrierCells = func() map[[2]int]bool {
	b := make(map[[2]int]bool)
	addBlock := func(col, row, size int) {
		for dy := 0; dy < size; dy++ {
			for dx := 0; dx < size; dx++ {
				b[[2]int{col + dx, row + dy}] = true
			}
		}
	}
	addBlock(3, 2, 3)   // 3×3 top-left corner
	addBlock(19, 2, 3)  // 3×3 top-right corner
	addBlock(3, 12, 3)  // 3×3 bottom-left corner
	addBlock(19, 12, 3) // 3×3 bottom-right corner
	addBlock(8, 7, 2)   // 2×2 left mid-field
	addBlock(15, 7, 2)  // 2×2 right mid-field
	addBlock(11, 2, 2)  // 2×2 centre-top
	addBlock(11, 13, 2) // 2×2 centre-bottom
	addBlock(6, 10, 1)  // 1×1 left flank
	addBlock(18, 10, 1) // 1×1 right flank
	addBlock(12, 5, 1)  // 1×1 centre
	return b
}()

// IsBarrier reports whether the given grid cell is an impassable barrier.
func IsBarrier(gx, gy int) bool {
	return barrierCells[[2]int{gx, gy}]
}

// HasLineOfSight reports whether there is an unobstructed straight line between
// two grid cells. Uses Bresenham's line algorithm; intermediate cells (not the
// start or end) that are barriers count as blockers.
func HasLineOfSight(x0, y0, x1, y1 int) bool {
	dx := iabs(x1 - x0)
	dy := iabs(y1 - y0)
	sx := isign(x1 - x0)
	sy := isign(y1 - y0)
	errVal := dx - dy
	x, y := x0, y0
	for {
		// Intermediate cells only — start (shooter) and end (target) are skipped.
		if !(x == x0 && y == y0) && !(x == x1 && y == y1) {
			if IsBarrier(x, y) {
				return false
			}
		}
		if x == x1 && y == y1 {
			return true
		}
		e2 := 2 * errVal
		if e2 > -dy {
			errVal -= dy
			x += sx
		}
		if e2 < dx {
			errVal += dx
			y += sy
		}
	}
}

func iabs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func isign(x int) int {
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	return 0
}

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
