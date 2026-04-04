package shooter

import "testing"

func TestNewPlayer_defaults(t *testing.T) {
	p := NewPlayer("id1", "room1")

	if p.ID != "id1" {
		t.Errorf("expected ID id1, got %s", p.ID)
	}
	if p.Health != 99 {
		t.Errorf("expected Health 99, got %d", p.Health)
	}
	if p.Messages() == nil {
		t.Error("Messages channel must not be nil")
	}
}

func TestPlayer_IsAlive(t *testing.T) {
	p := NewPlayer("id1", "room1")

	if !p.IsAlive() {
		t.Error("new player should be alive")
	}

	p.Health = 0
	if p.IsAlive() {
		t.Error("player with 0 health should not be alive")
	}
}

func TestPlayer_Send_buffers_message(t *testing.T) {
	p := NewPlayer("id1", "room1")

	p.Send([]byte("hello"))

	select {
	case got := <-p.Messages():
		if string(got) != "hello" {
			t.Errorf("expected hello, got %s", got)
		}
	default:
		t.Error("expected a message in the channel")
	}
}

func TestPlayer_Send_dropsWhenFull(t *testing.T) {
	p := NewPlayer("id1", "room1")

	// Fill the buffer (capacity 64) then add one more — must not block.
	for i := 0; i < 65; i++ {
		p.Send([]byte("x"))
	}
}
