// wallet.go implements Nano HD wallet key derivation and address encoding.
// Nano uses Blake2b + ed25519, with a custom base32 address format.
package nano

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// nanoAlphabet is Nano's base32 character set (excludes 0, 2, l, v to avoid ambiguity).
const nanoAlphabet = "13456789abcdefghijkmnopqrstuwxyz"

// DeriveKeypair derives an ed25519 key pair from a master seed and player index.
// Account key = Blake2b-256(seed || uint32_be(index)).
func DeriveKeypair(seed []byte, index uint32) (pubKey ed25519.PublicKey, privKey ed25519.PrivateKey, err error) {
	h, err := blake2b.New256(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("blake2b init: %w", err)
	}

	idx := make([]byte, 4)
	binary.BigEndian.PutUint32(idx, index)
	h.Write(seed)
	h.Write(idx)
	accountKey := h.Sum(nil)

	// ed25519 uses the account key as its seed.
	privKey = ed25519.NewKeyFromSeed(accountKey)
	pubKey = privKey.Public().(ed25519.PublicKey)
	return pubKey, privKey, nil
}

// AddressFromPublicKey encodes a 32-byte ed25519 public key as a nano_ address.
// Format: "nano_" + base32(pubkey, 4 padding bits) + base32(checksum, 0 padding bits)
func AddressFromPublicKey(pubKey []byte) (string, error) {
	if len(pubKey) != 32 {
		return "", fmt.Errorf("public key must be 32 bytes, got %d", len(pubKey))
	}

	// 256 bits + 4 padding bits = 260 bits = 52 base32 chars.
	keyEncoded := nanoBase32Encode(pubKey, 4)

	// Checksum: Blake2b with 5-byte output of the public key, bytes reversed.
	h, err := blake2b.New(5, nil)
	if err != nil {
		return "", fmt.Errorf("blake2b checksum: %w", err)
	}
	h.Write(pubKey)
	checksum := h.Sum(nil)
	for i, j := 0, len(checksum)-1; i < j; i, j = i+1, j-1 {
		checksum[i], checksum[j] = checksum[j], checksum[i]
	}
	// 40 bits = 8 base32 chars.
	checksumEncoded := nanoBase32Encode(checksum, 0)

	return "nano_" + keyEncoded + checksumEncoded, nil
}

// DeriveAddress is the combined helper used throughout the application.
// It returns the nano_ address for the given master seed and player index.
func DeriveAddress(seed []byte, index uint32) (string, error) {
	pubKey, _, err := DeriveKeypair(seed, index)
	if err != nil {
		return "", err
	}
	return AddressFromPublicKey(pubKey)
}

// PublicKeyFromAddress extracts the 32-byte ed25519 public key from a nano_ address.
// It is the inverse of AddressFromPublicKey and is used to build send block link fields.
func PublicKeyFromAddress(address string) ([]byte, error) {
	if !strings.HasPrefix(address, "nano_") {
		return nil, fmt.Errorf("address must start with nano_")
	}
	body := strings.TrimPrefix(address, "nano_")
	if len(body) != 60 {
		return nil, fmt.Errorf("invalid address: expected 60 chars after nano_, got %d", len(body))
	}
	// First 52 chars encode the 256-bit public key with 4 leading padding bits.
	return nanoBase32Decode(body[:52], 4)
}

// nanoBase32Decode is the inverse of nanoBase32Encode.
// It strips leadingZeroBits padding bits from the decoded bit stream.
func nanoBase32Decode(s string, leadingZeroBits int) ([]byte, error) {
	totalBits := len(s) * 5
	dataBits := totalBits - leadingZeroBits
	data := make([]byte, dataBits/8)

	for i, c := range s {
		idx := strings.IndexRune(nanoAlphabet, c)
		if idx < 0 {
			return nil, fmt.Errorf("invalid character %q in Nano address", c)
		}
		for j := range 5 {
			paddedPos := i*5 + j
			if paddedPos < leadingZeroBits {
				continue
			}
			dataPos := paddedPos - leadingZeroBits
			byteIdx := dataPos / 8
			bitIdx := 7 - (dataPos % 8)
			bit := (idx >> (4 - j)) & 1
			data[byteIdx] |= byte(bit) << bitIdx
		}
	}
	return data, nil
}

// nanoBase32Encode converts bytes to Nano's base32 string.
// leadingZeroBits pads the MSB end of the bit stream so that totalBits is a multiple of 5.
func nanoBase32Encode(data []byte, leadingZeroBits int) string {
	totalBits := leadingZeroBits + len(data)*8
	numChars := totalBits / 5

	var sb strings.Builder
	sb.Grow(numChars)

	for i := range numChars {
		var val int
		for j := range 5 {
			// Position in the padded bit stream.
			paddedPos := i*5 + j
			if paddedPos >= leadingZeroBits {
				// Map back to an actual data bit.
				dataPos := paddedPos - leadingZeroBits
				byteIdx := dataPos / 8
				bitIdx := 7 - (dataPos % 8) // MSB first
				val |= int((data[byteIdx]>>bitIdx)&1) << (4 - j)
			}
			// Bits in the padding region contribute 0 — no action needed.
		}
		sb.WriteByte(nanoAlphabet[val])
	}
	return sb.String()
}
