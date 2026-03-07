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
	if p.X == 0 && p.Y == 0 {
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

func TestRoom_Submit_applyInput(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	r.applyInput(Input{PlayerID: "p1", DX: 1, DY: 0})

	if p.Vx != 1 || p.Vy != 0 {
		t.Errorf("expected Vx=1 Vy=0, got Vx=%v Vy=%v", p.Vx, p.Vy)
	}
}

func TestRoom_ApplyVelocities_movesPlayer(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	startX := p.X
	p.Vx = 1

	r.applyVelocities()

	if p.X <= startX {
		t.Errorf("player should have moved right: startX=%v newX=%v", startX, p.X)
	}
}

func TestRoom_ApplyVelocities_clampsToArena(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	// Push hard against the right wall.
	p.X = ArenaWidth
	p.Vx = 1
	r.applyVelocities()

	if p.X > ArenaWidth-PlayerRadius {
		t.Errorf("player exceeded arena right bound: x=%v", p.X)
	}
}
