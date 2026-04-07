package wallet

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestGenerate_seedIsHex(t *testing.T) {
	g, err := Generate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := hex.DecodeString(g.SeedHex); err != nil {
		t.Errorf("SeedHex is not valid hex: %v", err)
	}
}

func TestGenerate_seedLength(t *testing.T) {
	g, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	// 32 bytes → 64 hex characters
	if len(g.SeedHex) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(g.SeedHex))
	}
}

func TestGenerate_addressFormat(t *testing.T) {
	g, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(g.Address, "nano_") {
		t.Errorf("address must start with nano_, got %s", g.Address)
	}
	// nano_(5) + 52 key chars + 8 checksum chars = 65
	if len(g.Address) != 65 {
		t.Errorf("expected address length 65, got %d", len(g.Address))
	}
}

func TestGenerate_uniqueSeeds(t *testing.T) {
	g1, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	g2, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if g1.SeedHex == g2.SeedHex {
		t.Error("two generated seeds must not be equal")
	}
	if g1.Address == g2.Address {
		t.Error("two generated addresses must not be equal")
	}
}
