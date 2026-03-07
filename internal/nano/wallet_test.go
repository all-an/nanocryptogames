package nano

import (
	"strings"
	"testing"
)

// knownSeed is a test-only seed — never use for real funds.
var knownSeed = []byte{
	0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
	0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
	0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
	0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
}

func TestDeriveKeypair_deterministicOutput(t *testing.T) {
	pub1, priv1, err := DeriveKeypair(knownSeed, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pub2, priv2, _ := DeriveKeypair(knownSeed, 0)

	if string(pub1) != string(pub2) {
		t.Error("public key must be deterministic for same seed+index")
	}
	if string(priv1) != string(priv2) {
		t.Error("private key must be deterministic for same seed+index")
	}
}

func TestDeriveKeypair_differentIndexProducesDifferentKeys(t *testing.T) {
	pub0, _, _ := DeriveKeypair(knownSeed, 0)
	pub1, _, _ := DeriveKeypair(knownSeed, 1)

	if string(pub0) == string(pub1) {
		t.Error("different indices must produce different public keys")
	}
}

func TestDeriveKeypair_publicKeyLength(t *testing.T) {
	pub, _, err := DeriveKeypair(knownSeed, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) != 32 {
		t.Errorf("expected 32-byte public key, got %d", len(pub))
	}
}

func TestAddressFromPublicKey_format(t *testing.T) {
	pub, _, _ := DeriveKeypair(knownSeed, 0)
	addr, err := AddressFromPublicKey(pub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(addr, "nano_") {
		t.Errorf("address must start with nano_, got %s", addr)
	}
	// nano_ (5) + 52 key chars + 8 checksum chars = 65 total
	if len(addr) != 65 {
		t.Errorf("expected length 65, got %d: %s", len(addr), addr)
	}
}

func TestAddressFromPublicKey_deterministicOutput(t *testing.T) {
	pub, _, _ := DeriveKeypair(knownSeed, 0)
	addr1, _ := AddressFromPublicKey(pub)
	addr2, _ := AddressFromPublicKey(pub)

	if addr1 != addr2 {
		t.Error("address encoding must be deterministic")
	}
}

func TestDeriveAddress_differentIndexProducesDifferentAddresses(t *testing.T) {
	addr0, _ := DeriveAddress(knownSeed, 0)
	addr1, _ := DeriveAddress(knownSeed, 1)

	if addr0 == addr1 {
		t.Error("different indices must produce different addresses")
	}
}

func TestAddressFromPublicKey_invalidLength(t *testing.T) {
	_, err := AddressFromPublicKey([]byte{0x01, 0x02})
	if err == nil {
		t.Error("expected error for wrong-length public key")
	}
}

func TestNanoBase32Encode_onlyUsesAlphabet(t *testing.T) {
	pub, _, _ := DeriveKeypair(knownSeed, 0)
	addr, _ := AddressFromPublicKey(pub)
	body := strings.TrimPrefix(addr, "nano_")

	for i, c := range body {
		if !strings.ContainsRune(nanoAlphabet, c) {
			t.Errorf("invalid char %q at position %d", c, i)
		}
	}
}
