package game

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRoom_JoinAssignsColorAndSpawn(t *testing.T) {
	r := NewRoom("test", "paid")
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
	r := NewRoom("test", "paid")
	p1 := NewPlayer("p1", "test")
	p2 := NewPlayer("p2", "test")
	r.Join(p1)
	r.Join(p2)

	if p1.Color == p2.Color {
		t.Errorf("two players got the same colour: %s", p1.Color)
	}
}

func TestRoom_Empty(t *testing.T) {
	r := NewRoom("test", "paid")
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
	r := NewRoom("test", "paid")
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
	r := NewRoom("test", "paid")
	p := NewPlayer("p1", "test")
	r.Join(p)
	p.GX, p.GY = 5, 5 // place in centre so GX+5 stays within grid

	// Move 5 squares right — exactly at the radius boundary.
	target := p.GX + 5
	r.applyInput(Input{PlayerID: "p1", GX: target, GY: p.GY})
	if p.GX != target {
		t.Errorf("move within radius should be accepted: expected GX=%d got %d", target, p.GX)
	}
}

func TestRoom_ApplyInput_beyondRadius(t *testing.T) {
	r := NewRoom("test", "paid")
	p := NewPlayer("p1", "test")
	r.Join(p)
	p.GX, p.GY = 5, 5

	startGX := p.GX
	r.applyInput(Input{PlayerID: "p1", GX: p.GX + 6, GY: p.GY})
	if p.GX != startGX {
		t.Error("move beyond radius should be rejected")
	}
}

func TestRoom_ApplyInput_diagonalWithinRadius(t *testing.T) {
	r := NewRoom("test", "paid")
	p := NewPlayer("p1", "test")
	r.Join(p)
	p.GX, p.GY = 5, 5

	// (3,4) offset = Euclidean distance 5.0 — exactly on the boundary.
	startGX := p.GX
	r.applyInput(Input{PlayerID: "p1", GX: p.GX + 3, GY: p.GY + 4})
	if p.GX != startGX+3 {
		t.Errorf("(3,4) diagonal move should be accepted (distance=5.0)")
	}
}

func TestRoom_ApplyInput_sameCell(t *testing.T) {
	r := NewRoom("test", "paid")
	p := NewPlayer("p1", "test")
	r.Join(p)

	startGX, startGY := p.GX, p.GY
	r.applyInput(Input{PlayerID: "p1", GX: p.GX, GY: p.GY})
	if p.GX != startGX || p.GY != startGY {
		t.Error("same-cell move should be rejected")
	}
}

func TestRoom_ApplyInput_clampsToGrid(t *testing.T) {
	r := NewRoom("test", "paid")
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
	r := NewRoom("test", "paid")
	shooter := NewPlayer("shooter", "test")
	target  := NewPlayer("target", "test")
	r.Join(shooter)
	r.Join(target)

	shooter.Team = "red"
	target.Team  = "blue"
	shooter.GX, shooter.GY = 5, 5
	target.GX,  target.GY  = 6, 5

	r.applyInput(Input{Action: "shoot", PlayerID: "shooter", TargetID: "target"})

	if target.Health != 50 {
		t.Errorf("first hit should reduce health to 50, got %d", target.Health)
	}
}

func TestRoom_ApplyShoot_secondHitKills(t *testing.T) {
	r := NewRoom("test", "paid")
	shooter := NewPlayer("shooter", "test")
	target  := NewPlayer("target", "test")
	r.Join(shooter)
	r.Join(target)

	shooter.Team = "red"
	target.Team  = "blue"
	shooter.GX, shooter.GY = 5, 5
	target.GX,  target.GY  = 6, 5
	target.Health = 50 // already incapacitated

	r.applyInput(Input{Action: "shoot", PlayerID: "shooter", TargetID: "target"})

	if target.Health != 0 {
		t.Errorf("second hit should reduce health to 0, got %d", target.Health)
	}
}

func TestRoom_ApplyShoot_deadShooterCannotShoot(t *testing.T) {
	r := NewRoom("test", "paid")
	shooter := NewPlayer("shooter", "test")
	target  := NewPlayer("target", "test")
	r.Join(shooter)
	r.Join(target)

	shooter.Team = "red"
	target.Team  = "blue"
	shooter.GX, shooter.GY = 5, 5
	target.GX,  target.GY  = 6, 5
	shooter.Health = 0 // shooter is dead

	startHealth := target.Health
	r.applyInput(Input{Action: "shoot", PlayerID: "shooter", TargetID: "target"})

	if target.Health != startHealth {
		t.Error("dead shooter should not be able to shoot")
	}
}

func TestRoom_ApplyShoot_outOfRangeRejected(t *testing.T) {
	r := NewRoom("test", "paid")
	shooter := NewPlayer("shooter", "test")
	target  := NewPlayer("target", "test")
	r.Join(shooter)
	r.Join(target)

	shooter.Team = "red"
	target.Team  = "blue"
	shooter.GX, shooter.GY = 0, 0
	target.GX,  target.GY  = 10, 0

	startHealth := target.Health
	r.applyInput(Input{Action: "shoot", PlayerID: "shooter", TargetID: "target"})

	if target.Health != startHealth {
		t.Error("out-of-range shot should be rejected")
	}
}

func TestRoom_ApplyShoot_teammateCannotBeShot(t *testing.T) {
	r := NewRoom("test", "paid")
	shooter := NewPlayer("shooter", "test")
	target  := NewPlayer("target", "test")
	r.Join(shooter)
	r.Join(target)

	shooter.Team = "red"
	target.Team  = "red" // same team
	shooter.GX, shooter.GY = 5, 5
	target.GX,  target.GY  = 6, 5

	startHealth := target.Health
	r.applyInput(Input{Action: "shoot", PlayerID: "shooter", TargetID: "target"})

	if target.Health != startHealth {
		t.Error("shooting a teammate should be rejected")
	}
}

func TestRoom_ApplyMove_deadPlayerCannotMove(t *testing.T) {
	r := NewRoom("test", "paid")
	p := NewPlayer("p1", "test")
	r.Join(p)
	p.GX, p.GY = 5, 5

	p.Health = 0 // dead
	startGX := p.GX
	r.applyInput(Input{PlayerID: "p1", GX: p.GX + 1, GY: p.GY})

	if p.GX != startGX {
		t.Error("dead player should not be able to move")
	}
}

func TestRoom_ApplyHelp_adjacentTeammateRestoresHealth(t *testing.T) {
	r := NewRoom("test", "paid")
	helper := NewPlayer("helper", "test")
	target := NewPlayer("target", "test")
	r.Join(helper)
	r.Join(target)

	helper.Team = "blue"
	target.Team = "blue" // same team
	helper.GX, helper.GY = 5, 5
	target.GX, target.GY = 6, 5 // one cell apart — adjacent
	target.Health = 50           // incapacitated

	r.applyInput(Input{Action: "help", PlayerID: "helper", TargetID: "target"})

	if target.Health != 100 {
		t.Errorf("medical help should restore health to 100, got %d", target.Health)
	}
}

func TestRoom_ApplyHelp_notAdjacentRejected(t *testing.T) {
	r := NewRoom("test", "paid")
	helper := NewPlayer("helper", "test")
	target := NewPlayer("target", "test")
	r.Join(helper)
	r.Join(target)

	helper.Team = "blue"
	target.Team = "blue"
	helper.GX, helper.GY = 5, 5
	target.GX, target.GY = 8, 5 // 3 cells away — not adjacent
	target.Health = 50

	r.applyInput(Input{Action: "help", PlayerID: "helper", TargetID: "target"})

	if target.Health != 50 {
		t.Error("help from non-adjacent position should be rejected")
	}
}

func TestRoom_ApplyHelp_healthyTargetRejected(t *testing.T) {
	r := NewRoom("test", "paid")
	helper := NewPlayer("helper", "test")
	target := NewPlayer("target", "test")
	r.Join(helper)
	r.Join(target)

	helper.Team = "blue"
	target.Team = "blue"
	helper.GX, helper.GY = 5, 5
	target.GX, target.GY = 6, 5
	// target.Health is 100 — not incapacitated

	r.applyInput(Input{Action: "help", PlayerID: "helper", TargetID: "target"})

	if target.Health != 100 {
		t.Error("help on a healthy player should be rejected (no-op)")
	}
}

func TestRoom_ApplyHelp_woundedHelperCanHelp(t *testing.T) {
	r := NewRoom("test", "paid")
	helper := NewPlayer("helper", "test")
	target := NewPlayer("target", "test")
	r.Join(helper)
	r.Join(target)

	helper.Team = "blue"
	target.Team = "blue"
	helper.GX, helper.GY = 5, 5
	target.GX, target.GY = 6, 5
	helper.Health = 50 // helper is wounded but still alive — should be able to help
	target.Health = 50

	r.applyInput(Input{Action: "help", PlayerID: "helper", TargetID: "target"})

	if target.Health != 100 {
		t.Error("wounded helper should be able to give medical help")
	}
}

func TestRoom_ApplyHelp_enemyCannotBeHelped(t *testing.T) {
	r := NewRoom("test", "paid")
	helper := NewPlayer("helper", "test")
	target := NewPlayer("target", "test")
	r.Join(helper)
	r.Join(target)

	helper.Team = "red"
	target.Team = "blue" // different team — enemy
	helper.GX, helper.GY = 5, 5
	target.GX, target.GY = 6, 5
	target.Health = 50

	r.applyInput(Input{Action: "help", PlayerID: "helper", TargetID: "target"})

	if target.Health != 50 {
		t.Error("enemy player should not be able to receive medical help")
	}
}
