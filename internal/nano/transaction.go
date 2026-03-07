// transaction.go implements Nano state block creation, signing, and submission.
// A full send or receive requires: account info → work → block hash → signature → process.
package nano

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// DefaultRepresentative is used when opening new accounts.
// Points to a well-known, reliable representative (Nano Foundation).
const DefaultRepresentative = "nano_3t6k35gi95xu6tergt6p69ck76ogmitsa8mnijtpxm9fkcm736xtoncuohr3"

// stateBlockPreamble is the 32-byte preamble for Nano state blocks (31 × 0x00 + 0x06).
var stateBlockPreamble = append(make([]byte, 31), 0x06)

// Wallet holds the key material for a single derived HD account.
type Wallet struct {
	Address    string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// DeriveWallet returns a Wallet for the given master seed and player index.
func DeriveWallet(seed []byte, index uint32) (*Wallet, error) {
	pub, priv, err := DeriveKeypair(seed, index)
	if err != nil {
		return nil, err
	}
	addr, err := AddressFromPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return &Wallet{Address: addr, PublicKey: pub, PrivateKey: priv}, nil
}

// Send creates, signs, and submits a state block to transfer amountRaw (in raw units)
// from wallet to toAddress. Returns the confirmed block hash.
func Send(ctx context.Context, rpc *Client, wallet *Wallet, toAddress, amountRaw string) (string, error) {
	info, err := rpc.GetAccountInfo(ctx, wallet.Address)
	if err != nil {
		return "", fmt.Errorf("account info: %w", err)
	}

	current, ok := new(big.Int).SetString(info.Balance, 10)
	if !ok {
		return "", fmt.Errorf("invalid balance from node: %s", info.Balance)
	}
	amount, ok := new(big.Int).SetString(amountRaw, 10)
	if !ok {
		return "", fmt.Errorf("invalid amount: %s", amountRaw)
	}
	if current.Cmp(amount) < 0 {
		return "", fmt.Errorf("insufficient balance: have %s raw, want %s raw", info.Balance, amountRaw)
	}
	newBalance := new(big.Int).Sub(current, amount)

	// Link field for send blocks is the destination account's public key bytes.
	destPub, err := PublicKeyFromAddress(toAddress)
	if err != nil {
		return "", fmt.Errorf("destination address: %w", err)
	}

	work, err := rpc.GenerateWork(ctx, info.Frontier)
	if err != nil {
		return "", fmt.Errorf("work: %w", err)
	}

	blockHash, err := stateBlockHash(wallet.PublicKey, info.Frontier, info.Representative, newBalance, destPub)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(wallet.PrivateKey, blockHash)

	return rpc.ProcessBlock(ctx, "send", map[string]string{
		"type":           "state",
		"account":        wallet.Address,
		"previous":       info.Frontier,
		"representative": info.Representative,
		"balance":        newBalance.String(),
		"link":           hex.EncodeToString(destPub),
		"signature":      strings.ToUpper(hex.EncodeToString(sig)),
		"work":           work,
	})
}

// ReceivePending receives all unconfirmed incoming blocks for the wallet account.
// Safe to call on unopened (brand-new) accounts.
func ReceivePending(ctx context.Context, rpc *Client, wallet *Wallet) error {
	hashes, err := rpc.Receivable(ctx, wallet.Address)
	if err != nil {
		return fmt.Errorf("receivable: %w", err)
	}
	if len(hashes) == 0 {
		return nil
	}

	info, err := rpc.GetAccountInfo(ctx, wallet.Address)
	isNew := err != nil && strings.Contains(err.Error(), "Account not found")
	if err != nil && !isNew {
		return fmt.Errorf("account info: %w", err)
	}

	for _, hash := range hashes {
		if err := receiveBlock(ctx, rpc, wallet, info, hash, isNew); err != nil {
			return fmt.Errorf("receive %s: %w", hash[:8], err)
		}
		// After the first receive the account is open; refresh state.
		if isNew {
			info, err = rpc.GetAccountInfo(ctx, wallet.Address)
			if err != nil {
				return fmt.Errorf("account info after open: %w", err)
			}
			isNew = false
		}
	}
	return nil
}

// receiveBlock builds and submits a single receive state block.
func receiveBlock(ctx context.Context, rpc *Client, wallet *Wallet, info *AccountInfo, pendingHash string, isNew bool) error {
	var frontier string
	var currentBalance *big.Int
	var representative string

	if isNew {
		frontier = strings.Repeat("0", 64)
		currentBalance = big.NewInt(0)
		representative = DefaultRepresentative
	} else {
		frontier = info.Frontier
		currentBalance, _ = new(big.Int).SetString(info.Balance, 10)
		representative = info.Representative
	}

	details, err := rpc.BlockInfo(ctx, pendingHash)
	if err != nil {
		return fmt.Errorf("block info: %w", err)
	}
	amountInt, ok := new(big.Int).SetString(details.Amount, 10)
	if !ok {
		return fmt.Errorf("invalid block amount: %s", details.Amount)
	}
	newBalance := new(big.Int).Add(currentBalance, amountInt)

	// Work input: account public key for new accounts, frontier hash for existing ones.
	var workInput string
	if isNew {
		workInput = strings.ToUpper(hex.EncodeToString(wallet.PublicKey))
	} else {
		workInput = frontier
	}
	work, err := rpc.GenerateWork(ctx, workInput)
	if err != nil {
		return fmt.Errorf("work: %w", err)
	}

	// Link field for receive blocks is the source block hash as raw bytes.
	pendingBytes, err := hex.DecodeString(pendingHash)
	if err != nil {
		return fmt.Errorf("decode pending hash: %w", err)
	}

	blockHash, err := stateBlockHash(wallet.PublicKey, frontier, representative, newBalance, pendingBytes)
	if err != nil {
		return err
	}
	sig := ed25519.Sign(wallet.PrivateKey, blockHash)

	_, err = rpc.ProcessBlock(ctx, "receive", map[string]string{
		"type":           "state",
		"account":        wallet.Address,
		"previous":       frontier,
		"representative": representative,
		"balance":        newBalance.String(),
		"link":           pendingHash,
		"signature":      strings.ToUpper(hex.EncodeToString(sig)),
		"work":           work,
	})
	return err
}

// stateBlockHash computes the Blake2b-256 hash of a Nano state block.
// Hash input: preamble(32) || account(32) || previous(32) || representative(32) || balance(16) || link(32)
func stateBlockHash(accountPub []byte, previous, representative string, balance *big.Int, link []byte) ([]byte, error) {
	prevBytes, err := hex.DecodeString(previous)
	if err != nil {
		return nil, fmt.Errorf("decode previous block hash: %w", err)
	}

	repPub, err := PublicKeyFromAddress(representative)
	if err != nil {
		return nil, fmt.Errorf("representative public key: %w", err)
	}

	// Balance as 16-byte big-endian uint128.
	balanceBytes := make([]byte, 16)
	balance.FillBytes(balanceBytes)

	h, err := blake2b.New256(nil)
	if err != nil {
		return nil, err
	}
	h.Write(stateBlockPreamble) // 32 bytes
	h.Write(accountPub)         // 32 bytes
	h.Write(prevBytes)          // 32 bytes
	h.Write(repPub)             // 32 bytes
	h.Write(balanceBytes)       // 16 bytes
	h.Write(link)               // 32 bytes

	return h.Sum(nil), nil
}
