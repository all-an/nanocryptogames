package games

import (
	"math"
	"testing"
)

func TestClampToGrid_withinBounds(t *testing.T) {
	gx, gy := clampToGrid(5, 5)
	if gx != 5 || gy != 5 {
		t.Errorf("expected (5,5), got (%d,%d)", gx, gy)
	}
}

func TestClampToGrid_clampLeft(t *testing.T) {
	gx, _ := clampToGrid(-1, 5)
	if gx != 0 {
		t.Errorf("expected 0, got %d", gx)
	}
}

func TestClampToGrid_clampRight(t *testing.T) {
	gx, _ := clampToGrid(GridCols+10, 5)
	if gx != GridCols-1 {
		t.Errorf("expected %d, got %d", GridCols-1, gx)
	}
}

func TestClampToGrid_clampTop(t *testing.T) {
	_, gy := clampToGrid(5, -1)
	if gy != 0 {
		t.Errorf("expected 0, got %d", gy)
	}
}

func TestClampToGrid_clampBottom(t *testing.T) {
	_, gy := clampToGrid(5, GridRows+10)
	if gy != GridRows-1 {
		t.Errorf("expected %d, got %d", GridRows-1, gy)
	}
}

func TestIsValidMove_sameCell(t *testing.T) {
	if isValidMove(5, 5, 5, 5) {
		t.Error("same cell should not be a valid move")
	}
}

func TestIsValidMove_adjacentCells(t *testing.T) {
	cases := [][4]int{
		{5, 5, 6, 5}, // right
		{5, 5, 4, 5}, // left
		{5, 5, 5, 6}, // down
		{5, 5, 5, 4}, // up
	}
	for _, c := range cases {
		if !isValidMove(c[0], c[1], c[2], c[3]) {
			t.Errorf("adjacent move should be valid: (%d,%d)→(%d,%d)", c[0], c[1], c[2], c[3])
		}
	}
}

func TestIsValidMove_diagonalWithinRadius(t *testing.T) {
	if !isValidMove(5, 5, 6, 6) {
		t.Error("diagonal one step should be valid within radius 5")
	}
}

func TestIsValidMove_exactRadius(t *testing.T) {
	if !isValidMove(5, 5, 10, 5) {
		t.Error("move of exactly radius 5 should be valid")
	}
}

func TestIsValidMove_justBeyondRadius(t *testing.T) {
	if isValidMove(5, 5, 11, 5) {
		t.Error("move of 6 squares should be invalid (beyond radius 5)")
	}
}

func TestIsValidMove_diagonalBeyondRadius(t *testing.T) {
	dist := math.Sqrt(float64(4*4 + 4*4))
	if dist <= MovementRadius {
		t.Fatalf("test assumption wrong: dist=%.2f should be > %.1f", dist, MovementRadius)
	}
	if isValidMove(5, 5, 9, 9) {
		t.Error("diagonal (4,4) is beyond radius 5 and should be invalid")
	}
}

func TestIsValidMove_diagonalWithinRadius5(t *testing.T) {
	dist := math.Sqrt(float64(3*3 + 4*4))
	if math.Abs(dist-5.0) > 0.001 {
		t.Fatalf("test assumption wrong: dist=%.4f should be 5.0", dist)
	}
	if !isValidMove(5, 5, 8, 9) {
		t.Error("(3,4) diagonal is exactly radius 5 and should be valid")
	}
}
