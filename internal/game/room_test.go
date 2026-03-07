package game

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRoom_JoinAssignsColorAndSpawn(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	if p.Color == "" {
		t.Error("Join must assign a colour")
	}
	// Spawn must be a valid non-zero grid position (first spawn is {1,1}).
	if p.GX == 0 && p.GY == 0 {
		t.Error("Join must assign a non-zero spawn position")
	}
}

func TestRoom_JoinAssignsDifferentColors(t *testing.T) {
	r := NewRoom("test")
	p1 := NewPlayer("p1", "test")
	p2 := NewPlayer("p2", "test")
	r.Join(p1)
	r.Join(p2)

	if p1.Color == p2.Color {
		t.Errorf("two players got the same colour: %s", p1.Color)
	}
}

func TestRoom_Empty(t *testing.T) {
	r := NewRoom("test")

	if !r.Empty() {
		t.Error("new room should be empty")
	}

	p := NewPlayer("p1", "test")
	r.Join(p)

	if r.Empty() {
		t.Error("room with a player should not be empty")
	}

	r.Leave(p)

	if !r.Empty() {
		t.Error("room should be empty after player leaves")
	}
}

func TestRoom_BroadcastState_sendsJSON(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	r.broadcastState()

	select {
	case msg := <-p.Messages():
		var state worldState
		if err := json.Unmarshal(msg, &state); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if state.Type != "state" {
			t.Errorf("expected type state, got %s", state.Type)
		}
		if len(state.Players) != 1 {
			t.Errorf("expected 1 player in state, got %d", len(state.Players))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("no state message received")
	}
}

func TestRoom_ApplyInput_adjacentMove(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	startGX := p.GX
	r.applyInput(Input{PlayerID: "p1", GX: p.GX + 1, GY: p.GY})

	if p.GX != startGX+1 {
		t.Errorf("expected GX %d, got %d", startGX+1, p.GX)
	}
}

func TestRoom_ApplyInput_rejectsDiagonal(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	startGX, startGY := p.GX, p.GY
	r.applyInput(Input{PlayerID: "p1", GX: p.GX + 1, GY: p.GY + 1})

	if p.GX != startGX || p.GY != startGY {
		t.Error("diagonal move should be rejected")
	}
}

func TestRoom_ApplyInput_rejectsTwoSteps(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	startGX := p.GX
	r.applyInput(Input{PlayerID: "p1", GX: p.GX + 2, GY: p.GY})

	if p.GX != startGX {
		t.Error("two-step move should be rejected")
	}
}

func TestRoom_ApplyInput_clampsToGrid(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	// Place player at right edge and try to move one step further right.
	p.GX = GridCols - 1
	r.applyInput(Input{PlayerID: "p1", GX: GridCols, GY: p.GY})

	if p.GX > GridCols-1 {
		t.Errorf("player exceeded right grid bound: GX=%d", p.GX)
	}
}
