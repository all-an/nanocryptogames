// reclaim recovers funds stranded in a session wallet that was derived with the
// old (broken) SHA-512 ed25519 key expansion, which produced addresses that no
// standard Nano wallet can import.
//
// It works by:
//  1. Reproducing the SHA-512 scalar that generated the on-chain address.
//  2. Signing the receive block with Blake2b nonce/challenge (what Nano's network
//     actually verifies), using the same scalar.
//  3. Sending the recovered balance to a destination address.
//
// Usage:
//
//	go run ./cmd/reclaim \
//	  -seed <64-hex-master-seed> \
//	  -index 13 \
//	  -to nano_<your_address> \
//	  [-rpc https://rpc.nano.to]
package main

import (
	"context"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"filippo.io/edwards25519"
	"github.com/allanabrahao/nanomultiplayer/internal/nano"
	"golang.org/x/crypto/blake2b"
)

func main() {
	seedHex := flag.String("seed", "", "master seed as 64 hex characters")
	index := flag.Uint("index", 0, "seed index of the stranded wallet")
	to := flag.String("to", "", "destination nano_ address")
	rpcURL := flag.String("rpc", "https://rpc.nano.to", "Nano RPC node URL")
	flag.Parse()

	if *seedHex == "" || *to == "" {
		flag.Usage()
		log.Fatal("both -seed and -to are required")
	}

	seed, err := hex.DecodeString(*seedHex)
	if err != nil || len(seed) != 32 {
		log.Fatalf("seed must be 64 hex characters (32 bytes)")
	}

	// Reproduce the SHA-512 key expansion used by the old broken code.
	// This gives us the scalar whose corresponding public key is the on-chain
	// address, even though Nano normally uses Blake2b for key expansion.
	scalar, noncePfx, pubKey, address, err := sha512Wallet(seed, uint32(*index))
	if err != nil {
		log.Fatalf("derive sha512 wallet: %v", err)
	}

	fmt.Printf("Recovered address : %s\n", address)
	fmt.Printf("Public key        : %s\n", strings.ToUpper(hex.EncodeToString(pubKey)))
	fmt.Printf("Destination       : %s\n", *to)
	fmt.Printf("RPC               : %s\n\n", *rpcURL)

	rpc := nano.NewClient(nano.Config{PrimaryURL: *rpcURL, FallbackURL: *rpcURL})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Step 1: find pending blocks.
	fmt.Println("Step 1: checking for pending blocks…")
	hashes, err := rpc.Receivable(ctx, address)
	if err != nil {
		log.Fatalf("receivable: %v", err)
	}
	fmt.Printf("         %d pending block(s)\n", len(hashes))
	if len(hashes) == 0 {
		fmt.Println("Nothing to receive.")
		return
	}

	// Step 2: receive each pending block.
	fmt.Println("Step 2: receiving pending blocks…")
	if err := receivePending(ctx, rpc, scalar, noncePfx, pubKey, address); err != nil {
		log.Fatalf("receive: %v", err)
	}
	fmt.Println("         done.")

	// Step 3: read resulting balance.
	fmt.Println("Step 3: reading balance…")
	info, err := rpc.GetAccountInfo(ctx, address)
	if err != nil {
		log.Fatalf("account info after receive: %v", err)
	}
	balance, ok := new(big.Int).SetString(info.Balance, 10)
	if !ok || balance.Sign() <= 0 {
		fmt.Println("         balance is zero after receive — nothing to send.")
		return
	}
	xno := new(big.Float).Quo(new(big.Float).SetInt(balance),
		new(big.Float).SetInt(tenToThe(30)))
	fmt.Printf("         balance = %s raw (%.6f XNO)\n\n", info.Balance, xno)

	// Step 4: send full balance to destination.
	fmt.Printf("Step 4: sending to %s…\n", *to)
	hash, err := sendAll(ctx, rpc, scalar, noncePfx, pubKey, address, *to, info)
	if err != nil {
		log.Fatalf("send: %v", err)
	}
	fmt.Printf("         sent! block hash: %s\n", hash)
	fmt.Println("\nRecovery complete.")
}

// sha512Wallet derives the wallet that was produced by the old broken code.
// The old DeriveKeypair used Go's ed25519.NewKeyFromSeed which expands the
// account key with SHA-512 instead of Blake2b.
func sha512Wallet(seed []byte, index uint32) (scalar *edwards25519.Scalar, noncePfx, pubKey []byte, address string, err error) {
	// Account key = Blake2b-256(seed || uint32_be(index)) — this part was always correct.
	h, _ := blake2b.New256(nil)
	idx := make([]byte, 4)
	binary.BigEndian.PutUint32(idx, index)
	h.Write(seed)
	h.Write(idx)
	accountKey := h.Sum(nil)

	// Old code: expanded = SHA-512(accountKey), then ed25519.NewKeyFromSeed.
	// We replicate the SHA-512 expansion to recover the same scalar.
	hash := sha512.New()
	hash.Write(accountKey)
	expanded := hash.Sum(nil) // 64 bytes

	// SetBytesWithClamping applies the same clamping as Go's ed25519 package.
	scalar, err = edwards25519.NewScalar().SetBytesWithClamping(expanded[:32])
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("scalar: %w", err)
	}
	noncePfx = make([]byte, 32)
	copy(noncePfx, expanded[32:64])

	// Public key = scalar * BasePoint (same curve math, hash choice doesn't matter).
	pubKey = new(edwards25519.Point).ScalarBaseMult(scalar).Bytes()
	address, err = nano.AddressFromPublicKey(pubKey)
	return
}

// receivePending receives all pending blocks using Blake2b signing with the SHA-512 scalar.
func receivePending(ctx context.Context, rpc *nano.Client, scalar *edwards25519.Scalar, noncePfx, pubKey []byte, address string) error {
	hashes, err := rpc.Receivable(ctx, address)
	if err != nil {
		return fmt.Errorf("receivable: %w", err)
	}
	if len(hashes) == 0 {
		return nil
	}

	info, err := rpc.GetAccountInfo(ctx, address)
	isNew := err != nil && strings.Contains(err.Error(), "Account not found")
	if err != nil && !isNew {
		return fmt.Errorf("account info: %w", err)
	}

	for _, hash := range hashes {
		if err := receiveOne(ctx, rpc, scalar, noncePfx, pubKey, address, info, hash, isNew); err != nil {
			return fmt.Errorf("receive %s: %w", hash[:8], err)
		}
		if isNew {
			info, err = rpc.GetAccountInfo(ctx, address)
			if err != nil {
				return fmt.Errorf("account info after open: %w", err)
			}
			isNew = false
		}
	}
	return nil
}

func receiveOne(ctx context.Context, rpc *nano.Client, scalar *edwards25519.Scalar, noncePfx, pubKey []byte, address string, info *nano.AccountInfo, pendingHash string, isNew bool) error {
	var frontier string
	var currentBalance *big.Int
	var representative string

	if isNew {
		frontier = strings.Repeat("0", 64)
		currentBalance = big.NewInt(0)
		representative = nano.DefaultRepresentative
	} else {
		frontier = info.Frontier
		currentBalance, _ = new(big.Int).SetString(info.Balance, 10)
		representative = info.Representative
	}

	details, err := rpc.BlockInfo(ctx, pendingHash)
	if err != nil {
		return fmt.Errorf("block info: %w", err)
	}
	amount, ok := new(big.Int).SetString(details.Amount, 10)
	if !ok {
		return fmt.Errorf("invalid block amount: %s", details.Amount)
	}
	newBalance := new(big.Int).Add(currentBalance, amount)

	var workInput string
	if isNew {
		workInput = strings.ToUpper(hex.EncodeToString(pubKey))
	} else {
		workInput = frontier
	}
	work, err := rpc.GenerateWork(ctx, workInput)
	if err != nil {
		return fmt.Errorf("work: %w", err)
	}

	pendingBytes, err := hex.DecodeString(pendingHash)
	if err != nil {
		return fmt.Errorf("decode pending hash: %w", err)
	}

	blockHash, err := stateBlockHash(pubKey, frontier, representative, newBalance, pendingBytes)
	if err != nil {
		return err
	}

	sig, err := nanoSignWith(scalar, noncePfx, pubKey, blockHash)
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}

	fmt.Printf("         receiving %s raw (block %s)\n", details.Amount, pendingHash[:8])
	_, err = rpc.ProcessBlock(ctx, "receive", map[string]string{
		"type":           "state",
		"account":        address,
		"previous":       frontier,
		"representative": representative,
		"balance":        newBalance.String(),
		"link":           pendingHash,
		"signature":      strings.ToUpper(hex.EncodeToString(sig)),
		"work":           work,
	})
	return err
}

func sendAll(ctx context.Context, rpc *nano.Client, scalar *edwards25519.Scalar, noncePfx, pubKey []byte, address, toAddress string, info *nano.AccountInfo) (string, error) {
	balance, _ := new(big.Int).SetString(info.Balance, 10)

	destPub, err := nano.PublicKeyFromAddress(toAddress)
	if err != nil {
		return "", fmt.Errorf("destination address: %w", err)
	}
	newBalance := big.NewInt(0)

	work, err := rpc.GenerateWork(ctx, info.Frontier)
	if err != nil {
		return "", fmt.Errorf("work: %w", err)
	}

	blockHash, err := stateBlockHash(pubKey, info.Frontier, info.Representative, newBalance, destPub)
	if err != nil {
		return "", err
	}
	sig, err := nanoSignWith(scalar, noncePfx, pubKey, blockHash)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}

	_ = balance
	return rpc.ProcessBlock(ctx, "send", map[string]string{
		"type":           "state",
		"account":        address,
		"previous":       info.Frontier,
		"representative": info.Representative,
		"balance":        "0",
		"link":           hex.EncodeToString(destPub),
		"signature":      strings.ToUpper(hex.EncodeToString(sig)),
		"work":           work,
	})
}

// nanoSignWith signs using Blake2b nonce/challenge with the provided scalar.
// The scalar may have been derived by either SHA-512 or Blake2b expansion —
// what matters for Nano network verification is only that scalar*B == pubKey.
func nanoSignWith(scalar *edwards25519.Scalar, noncePfx, pubKey, message []byte) ([]byte, error) {
	nh, err := blake2b.New512(nil)
	if err != nil {
		return nil, err
	}
	nh.Write(noncePfx)
	nh.Write(message)
	r, err := edwards25519.NewScalar().SetUniformBytes(nh.Sum(nil))
	if err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}

	R := new(edwards25519.Point).ScalarBaseMult(r)
	Rbytes := R.Bytes()

	ch, err := blake2b.New512(nil)
	if err != nil {
		return nil, err
	}
	ch.Write(Rbytes)
	ch.Write(pubKey)
	ch.Write(message)
	k, err := edwards25519.NewScalar().SetUniformBytes(ch.Sum(nil))
	if err != nil {
		return nil, fmt.Errorf("challenge: %w", err)
	}

	s := edwards25519.NewScalar().MultiplyAdd(k, scalar, r)
	sig := make([]byte, 64)
	copy(sig[:32], Rbytes)
	copy(sig[32:], s.Bytes())
	return sig, nil
}

// stateBlockHash computes the Blake2b-256 hash of a Nano state block.
func stateBlockHash(accountPub []byte, previous, representative string, balance *big.Int, link []byte) ([]byte, error) {
	preamble := append(make([]byte, 31), 0x06)
	prevBytes, err := hex.DecodeString(previous)
	if err != nil {
		return nil, fmt.Errorf("decode previous: %w", err)
	}
	repPub, err := nano.PublicKeyFromAddress(representative)
	if err != nil {
		return nil, fmt.Errorf("representative key: %w", err)
	}
	balanceBytes := make([]byte, 16)
	balance.FillBytes(balanceBytes)

	h, err := blake2b.New256(nil)
	if err != nil {
		return nil, err
	}
	h.Write(preamble)
	h.Write(accountPub)
	h.Write(prevBytes)
	h.Write(repPub)
	h.Write(balanceBytes)
	h.Write(link)
	return h.Sum(nil), nil
}

func tenToThe(exp int) *big.Int {
	result := big.NewInt(1)
	ten := big.NewInt(10)
	for range exp {
		result.Mul(result, ten)
	}
	return result
}
