// ed25519.go implements Nano's Blake2b variant of ed25519 signing.
//
// Nano's node uses a fork of ed25519-donna that replaces SHA-512 with Blake2b-512
// for all internal hash operations (key expansion, nonce, and challenge). This
// makes Nano signatures incompatible with Go's standard crypto/ed25519 package.
//
// Signing equation (same structure as RFC 8032, different hash):
//
//	R = r*B
//	s = r + Blake2b512(R || pubKey || msg) * scalar  (mod l)
//
// Verification (done by the Nano network):
//
//	s*B == R + Blake2b512(R || pubKey || msg) * P   (where P = scalar*B)
package nano

import (
	"fmt"

	"filippo.io/edwards25519"
	"golang.org/x/crypto/blake2b"
)

// nanoExpandKey expands a 32-byte Nano account key using Blake2b-512,
// returning the clamped scalar and the 32-byte nonce prefix for signing.
// This replaces the SHA-512 expansion defined in RFC 8032 §5.1.5.
func nanoExpandKey(accountKey []byte) (*edwards25519.Scalar, []byte, error) {
	h, err := blake2b.New512(nil)
	if err != nil {
		return nil, nil, err
	}
	h.Write(accountKey)
	expanded := h.Sum(nil) // 64 bytes: scalar material (32) + nonce prefix (32)

	// SetBytesWithClamping applies RFC 8032 clamping then reduces mod l.
	// Scalar multiplication is invariant under mod-l reduction, so this is
	// equivalent to using the raw clamped value directly.
	scalar, err := edwards25519.NewScalar().SetBytesWithClamping(expanded[:32])
	if err != nil {
		return nil, nil, fmt.Errorf("expand scalar: %w", err)
	}
	noncePfx := make([]byte, 32)
	copy(noncePfx, expanded[32:64])
	return scalar, noncePfx, nil
}

// nanoSign produces a Nano-compatible ed25519 signature.
// It accepts any scalar (regardless of how it was derived), which allows
// recovery of wallets whose keys were generated with a different expansion
// function (e.g. SHA-512) as long as the scalar matches the on-chain public key.
func nanoSign(scalar *edwards25519.Scalar, noncePfx, publicKey, message []byte) ([]byte, error) {
	// Nonce: r = Blake2b-512(noncePfx || message) reduced mod l.
	nh, err := blake2b.New512(nil)
	if err != nil {
		return nil, err
	}
	nh.Write(noncePfx)
	nh.Write(message)
	r, err := edwards25519.NewScalar().SetUniformBytes(nh.Sum(nil))
	if err != nil {
		return nil, fmt.Errorf("nonce scalar: %w", err)
	}

	// R = r * B
	R := new(edwards25519.Point).ScalarBaseMult(r)
	Rbytes := R.Bytes()

	// Challenge: k = Blake2b-512(R || publicKey || message) reduced mod l.
	ch, err := blake2b.New512(nil)
	if err != nil {
		return nil, err
	}
	ch.Write(Rbytes)
	ch.Write(publicKey)
	ch.Write(message)
	k, err := edwards25519.NewScalar().SetUniformBytes(ch.Sum(nil))
	if err != nil {
		return nil, fmt.Errorf("challenge scalar: %w", err)
	}

	// s = r + k * scalar  (mod l)
	s := edwards25519.NewScalar().MultiplyAdd(k, scalar, r)

	sig := make([]byte, 64)
	copy(sig[:32], Rbytes)
	copy(sig[32:], s.Bytes())
	return sig, nil
}
