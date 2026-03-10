// rpc.go is an HTTP client for the Nano RPC protocol.
// It tries the primary node first and falls back to the secondary on any error.
package nano

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/blake2b"
)

const rpcTimeout = 60 * time.Second

// Config holds the primary and fallback Nano RPC node URLs.
type Config struct {
	PrimaryURL  string // e.g. https://rpc.nano.to
	FallbackURL string // e.g. https://rpc.nano.to
	APIKey      string // optional nano.to paid API key (removes rate limits)
}

// Client is an HTTP client for the Nano RPC protocol with automatic fallback.
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient creates a Client with the given node configuration.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: rpcTimeout},
	}
}

// AccountBalance holds the confirmed and pending balances in raw units.
type AccountBalance struct {
	Balance string
	Pending string
}

// AccountInfo holds the account state required to create new blocks.
type AccountInfo struct {
	Frontier       string // hex hash of the latest block
	Balance        string // confirmed balance in raw
	Representative string // nano_ representative address
}

// GetBalance returns the confirmed and pending balance for a nano_ address.
func (c *Client) GetBalance(ctx context.Context, address string) (*AccountBalance, error) {
	var r struct {
		Balance string `json:"balance"`
		Pending string `json:"pending"`
		Error   rpcErrorField `json:"error"`
	}
	if err := c.call(ctx, map[string]string{
		"action":  "account_balance",
		"account": address,
	}, &r); err != nil {
		return nil, err
	}
	if r.Error != "" {
		return nil, fmt.Errorf("rpc account_balance: %s", r.Error)
	}
	return &AccountBalance{Balance: r.Balance, Pending: r.Pending}, nil
}

// GetAccountInfo returns the frontier, balance, and representative for an account.
// Returns an error containing "Account not found" for unopened accounts.
func (c *Client) GetAccountInfo(ctx context.Context, address string) (*AccountInfo, error) {
	var r struct {
		Frontier       string `json:"frontier"`
		Balance        string `json:"balance"`
		Representative string `json:"representative"`
		Error          rpcErrorField `json:"error"`
	}
	if err := c.call(ctx, map[string]string{
		"action":  "account_info",
		"account": address,
	}, &r); err != nil {
		return nil, err
	}
	if r.Error != "" {
		return nil, fmt.Errorf("rpc account_info: %s", r.Error)
	}
	return &AccountInfo{
		Frontier:       r.Frontier,
		Balance:        r.Balance,
		Representative: r.Representative,
	}, nil
}

// Receivable returns the hashes of unconfirmed incoming blocks for an address.
// Handles both response formats that different nodes use:
//   - map format: {"blocks": {"hash": "amount", ...}}
//   - array format: {"blocks": ["hash1", "hash2", ...]}
func (c *Client) Receivable(ctx context.Context, address string) ([]string, error) {
	var r struct {
		Blocks any    `json:"blocks"`
		Error  rpcErrorField `json:"error"`
	}
	if err := c.call(ctx, map[string]any{
		"action":  "receivable",
		"account": address,
		"count":   "10",
	}, &r); err != nil {
		return nil, err
	}
	if r.Error != "" {
		return nil, fmt.Errorf("rpc receivable: %s", r.Error)
	}

	switch v := r.Blocks.(type) {
	case map[string]any:
		// Map format: {"hash": "amount", ...}
		hashes := make([]string, 0, len(v))
		for hash := range v {
			hashes = append(hashes, hash)
		}
		return hashes, nil
	case []any:
		// Array format: ["hash1", "hash2", ...]
		hashes := make([]string, 0, len(v))
		for _, h := range v {
			if s, ok := h.(string); ok {
				hashes = append(hashes, s)
			}
		}
		return hashes, nil
	default:
		// Empty string or unexpected type — no pending blocks.
		return nil, nil
	}
}

// BlockDetails holds the amount and sender account for a Nano block.
type BlockDetails struct {
	Amount  string // transaction amount in raw units
	Account string // nano_ address that created (sent) this block
}

// BlockInfo returns the amount and sender account for a given block hash.
// Used to determine the value and origin of a pending receive block.
func (c *Client) BlockInfo(ctx context.Context, blockHash string) (*BlockDetails, error) {
	var r struct {
		Amount  string `json:"amount"`
		Account string `json:"block_account"`
		Error   rpcErrorField `json:"error"`
	}
	if err := c.call(ctx, map[string]string{
		"action": "block_info",
		"hash":   blockHash,
	}, &r); err != nil {
		return nil, err
	}
	if r.Error != "" {
		return nil, fmt.Errorf("rpc block_info: %s", r.Error)
	}
	return &BlockDetails{Amount: r.Amount, Account: r.Account}, nil
}

// GenerateWork requests proof-of-work for the given hash or public key hex.
// It races a remote RPC call (GPU peers) against local CPU mining and returns
// whichever finishes first, so a slow or unresponsive node never adds latency.
func (c *Client) GenerateWork(ctx context.Context, hash string) (string, error) {
	type result struct {
		work string
		err  error
	}

	// Shared cancellable context — winner cancels the loser.
	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan result, 2)

	// Remote attempt: GPU peers via RPC node (fast when available).
	go func() {
		var r struct {
			Work  string        `json:"work"`
			Error rpcErrorField `json:"error"`
		}
		err := c.call(raceCtx, map[string]any{
			"action":    "work_generate",
			"hash":      hash,
			"use_peers": true,
		}, &r)
		if err == nil && r.Error == "" && r.Work != "" {
			ch <- result{r.Work, nil}
		} else if err != nil {
			log.Printf("nano: remote work_generate failed: %v", err)
		} else if r.Error != "" {
			log.Printf("nano: remote work_generate RPC error: %s", r.Error)
		} else {
			log.Printf("nano: remote work_generate returned empty work")
		}
	}()

	// Local CPU mining: always works, ~2-5 s at send difficulty.
	go func() {
		work, err := generateWorkCPU(raceCtx, hash)
		ch <- result{work, err}
	}()

	// Return whichever result arrives first.
	select {
	case res := <-ch:
		cancel() // stop the other goroutine
		return res.work, res.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// generateWorkCPU mines Nano proof-of-work locally.
// It finds an 8-byte nonce N such that blake2b_64(N_LE || hashBytes) >= threshold.
// The standard send/change difficulty requires ~16M iterations on average (~1-3s on modern hardware).
func generateWorkCPU(ctx context.Context, hashHex string) (string, error) {
	hashBytes, err := hex.DecodeString(hashHex)
	if err != nil {
		return "", fmt.Errorf("decode hash: %w", err)
	}

	const threshold = uint64(0xfffffff800000000) // epoch-2 send/change difficulty

	h, err := blake2b.New(8, nil)
	if err != nil {
		return "", fmt.Errorf("blake2b init: %w", err)
	}

	// Start from a random nonce so concurrent calls don't duplicate effort.
	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return "", fmt.Errorf("rand seed: %w", err)
	}
	nonce := binary.LittleEndian.Uint64(seed[:])

	var nonceLE [8]byte
	for i := 0; ; i++ {
		// Check for context cancellation every 50k iterations to stay responsive.
		if i%50000 == 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			default:
			}
		}

		binary.LittleEndian.PutUint64(nonceLE[:], nonce)
		h.Reset()
		h.Write(nonceLE[:])
		h.Write(hashBytes)
		digest := h.Sum(nil)
		if binary.LittleEndian.Uint64(digest) >= threshold {
			// The Nano RPC work field is the nonce as a 16-char big-endian hex uint64,
			// matching the format returned by work_generate.
			return fmt.Sprintf("%016x", nonce), nil
		}
		nonce++
	}
}

// ProcessBlock submits a signed, worked block to the network.
// subtype must be "send", "receive", or "change".
// Returns the confirmed block hash on success.
func (c *Client) ProcessBlock(ctx context.Context, subtype string, block map[string]string) (string, error) {
	var r struct {
		Hash  string `json:"hash"`
		Error rpcErrorField `json:"error"`
	}
	if err := c.call(ctx, map[string]any{
		"action":     "process",
		"json_block": "true",
		"subtype":    subtype,
		"block":      block,
	}, &r); err != nil {
		return "", err
	}
	if r.Error != "" {
		return "", fmt.Errorf("rpc process: %s", r.Error)
	}
	return r.Hash, nil
}

// call marshals payload, posts to the primary node (with fallback), and unmarshals into dst.
func (c *Client) call(ctx context.Context, payload any, dst any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	var raw []byte
	var lastErr error

	// Try primary, then fallback.
	urls := []string{c.cfg.PrimaryURL}
	if c.cfg.FallbackURL != "" && c.cfg.FallbackURL != c.cfg.PrimaryURL {
		urls = append(urls, c.cfg.FallbackURL)
	}

	for _, url := range urls {
		for attempt := 0; attempt < 3; attempt++ {
			raw, lastErr = c.post(ctx, url, body)
			if lastErr == nil {
				// Success at transport level. Now verify it's JSON.
				if len(raw) > 0 && raw[0] == '<' {
					lastErr = fmt.Errorf("node returned HTML instead of JSON")
					continue // Try next attempt/url
				}
				if err := json.Unmarshal(raw, dst); err != nil {
					lastErr = fmt.Errorf("unmarshal: %w", err)
					continue // Try next attempt/url
				}
				return nil // Success!
			}

			// If it's a rate limit (429) or server error (5xx), wait and retry.
			if strings.Contains(lastErr.Error(), "429") || strings.Contains(lastErr.Error(), "5") {
				delay := time.Duration(attempt+1) * time.Second
				log.Printf("RPC [%s] attempt %d failed: %v — retrying in %v", url, attempt+1, lastErr, delay)
				time.Sleep(delay)
				continue
			}
			// For other errors (DNS, timeout), try next URL immediately.
			break
		}
	}

	return fmt.Errorf("all nodes failed: %w", lastErr)
}

// post sends a JSON POST to url and returns the raw response body.
func (c *Client) post(ctx context.Context, url string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", c.cfg.APIKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// rpcErrorField handles Nano node responses where the "error" field might be a string or a number.
type rpcErrorField string

func (e *rpcErrorField) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v == nil {
		*e = ""
		return nil
	}
	*e = rpcErrorField(fmt.Sprintf("%v", v))
	return nil
}
