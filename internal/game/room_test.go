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

func TestRoom_ApplyInput_withinRadius(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	// Move 5 squares right — exactly at the radius boundary.
	target := p.GX + 5
	r.applyInput(Input{PlayerID: "p1", GX: target, GY: p.GY})
	if p.GX != target {
		t.Errorf("move within radius should be accepted: expected GX=%d got %d", target, p.GX)
	}
}

func TestRoom_ApplyInput_beyondRadius(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	startGX := p.GX
	r.applyInput(Input{PlayerID: "p1", GX: p.GX + 6, GY: p.GY})
	if p.GX != startGX {
		t.Error("move beyond radius should be rejected")
	}
}

func TestRoom_ApplyInput_diagonalWithinRadius(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	// (3,4) offset = Euclidean distance 5.0 — exactly on the boundary.
	r.applyInput(Input{PlayerID: "p1", GX: p.GX + 3, GY: p.GY + 4})
	if p.GX != spawnPoints[0][0]+3 {
		t.Errorf("(3,4) diagonal move should be accepted (distance=5.0)")
	}
}

func TestRoom_ApplyInput_sameCell(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	startGX, startGY := p.GX, p.GY
	r.applyInput(Input{PlayerID: "p1", GX: p.GX, GY: p.GY})
	if p.GX != startGX || p.GY != startGY {
		t.Error("same-cell move should be rejected")
	}
}

func TestRoom_ApplyInput_clampsToGrid(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	// Place player near right edge, move within radius but past grid boundary.
	p.GX = GridCols - 2
	r.applyInput(Input{PlayerID: "p1", GX: GridCols + 3, GY: p.GY})
	if p.GX > GridCols-1 {
		t.Errorf("player exceeded right grid bound: GX=%d", p.GX)
	}
}

func TestRoom_ApplyShoot_firstHitIncapacitates(t *testing.T) {
	r := NewRoom("test")
	shooter := NewPlayer("shooter", "test")
	target  := NewPlayer("target", "test")
	r.Join(shooter)
	r.Join(target)

	// Place them adjacent so the shot is in range.
	shooter.GX, shooter.GY = 5, 5
	target.GX,  target.GY  = 6, 5

	r.applyInput(Input{Action: "shoot", PlayerID: "shooter", TargetID: "target"})

	if target.Health != 50 {
		t.Errorf("first hit should reduce health to 50, got %d", target.Health)
	}
}

func TestRoom_ApplyShoot_secondHitKills(t *testing.T) {
	r := NewRoom("test")
	shooter := NewPlayer("shooter", "test")
	target  := NewPlayer("target", "test")
	r.Join(shooter)
	r.Join(target)

	shooter.GX, shooter.GY = 5, 5
	target.GX,  target.GY  = 6, 5
	target.Health = 50 // already incapacitated

	// Shooter is healthy (100); target is incapacitated (50) — second shot kills.
	r.applyInput(Input{Action: "shoot", PlayerID: "shooter", TargetID: "target"})

	if target.Health != 0 {
		t.Errorf("second hit should reduce health to 0, got %d", target.Health)
	}
}

func TestRoom_ApplyShoot_incapacitatedShooterCannotShoot(t *testing.T) {
	r := NewRoom("test")
	shooter := NewPlayer("shooter", "test")
	target  := NewPlayer("target", "test")
	r.Join(shooter)
	r.Join(target)

	shooter.GX, shooter.GY = 5, 5
	target.GX,  target.GY  = 6, 5
	shooter.Health = 50 // shooter is incapacitated

	startHealth := target.Health
	r.applyInput(Input{Action: "shoot", PlayerID: "shooter", TargetID: "target"})

	if target.Health != startHealth {
		t.Error("incapacitated shooter should not be able to shoot")
	}
}

func TestRoom_ApplyShoot_outOfRangeRejected(t *testing.T) {
	r := NewRoom("test")
	shooter := NewPlayer("shooter", "test")
	target  := NewPlayer("target", "test")
	r.Join(shooter)
	r.Join(target)

	// Place 10 cells apart — well beyond movement radius of 5.
	shooter.GX, shooter.GY = 0, 0
	target.GX,  target.GY  = 10, 0

	startHealth := target.Health
	r.applyInput(Input{Action: "shoot", PlayerID: "shooter", TargetID: "target"})

	if target.Health != startHealth {
		t.Error("out-of-range shot should be rejected")
	}
}

func TestRoom_ApplyMove_incapacitatedPlayerCannotMove(t *testing.T) {
	r := NewRoom("test")
	p := NewPlayer("p1", "test")
	r.Join(p)

	p.Health = 50 // incapacitated
	startGX := p.GX
	r.applyInput(Input{PlayerID: "p1", GX: p.GX + 1, GY: p.GY})

	if p.GX != startGX {
		t.Error("incapacitated player should not be able to move")
	}
}
