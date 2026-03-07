package game

import "testing"

func TestClampToArena_withinBounds(t *testing.T) {
	x, y := clampToArena(500, 350)
	if x != 500 || y != 350 {
		t.Errorf("expected (500, 350), got (%v, %v)", x, y)
	}
}

func TestClampToArena_clampLeft(t *testing.T) {
	x, _ := clampToArena(0, 350)
	if x != PlayerRadius {
		t.Errorf("expected %v, got %v", PlayerRadius, x)
	}
}

func TestClampToArena_clampRight(t *testing.T) {
	x, _ := clampToArena(ArenaWidth+100, 350)
	if x != ArenaWidth-PlayerRadius {
		t.Errorf("expected %v, got %v", ArenaWidth-PlayerRadius, x)
	}
}

func TestClampToArena_clampTop(t *testing.T) {
	_, y := clampToArena(500, 0)
	if y != PlayerRadius {
		t.Errorf("expected %v, got %v", PlayerRadius, y)
	}
}

func TestClampToArena_clampBottom(t *testing.T) {
	_, y := clampToArena(500, ArenaHeight+100)
	if y != ArenaHeight-PlayerRadius {
		t.Errorf("expected %v, got %v", ArenaHeight-PlayerRadius, y)
	}
}
