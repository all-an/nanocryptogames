package game

import "testing"

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

func TestIsAdjacentMove_validDirections(t *testing.T) {
	cases := [][4]int{
		{5, 5, 6, 5}, // right
		{5, 5, 4, 5}, // left
		{5, 5, 5, 6}, // down
		{5, 5, 5, 4}, // up
	}
	for _, c := range cases {
		if !isAdjacentMove(c[0], c[1], c[2], c[3]) {
			t.Errorf("expected adjacent: (%d,%d)→(%d,%d)", c[0], c[1], c[2], c[3])
		}
	}
}

func TestIsAdjacentMove_diagonal(t *testing.T) {
	if isAdjacentMove(5, 5, 6, 6) {
		t.Error("diagonal should not be adjacent")
	}
}

func TestIsAdjacentMove_sameCell(t *testing.T) {
	if isAdjacentMove(5, 5, 5, 5) {
		t.Error("same cell should not be adjacent")
	}
}

func TestIsAdjacentMove_twoSteps(t *testing.T) {
	if isAdjacentMove(5, 5, 7, 5) {
		t.Error("two steps should not be adjacent")
	}
}
