// rpc.go is an HTTP client for the Nano RPC protocol.
// It tries the primary node first and falls back to the secondary on any error.
package nano

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const rpcTimeout = 60 * time.Second

// Config holds the primary and fallback Nano RPC node URLs.
type Config struct {
	PrimaryURL  string // e.g. https://nanoslo.0x.no
	FallbackURL string // e.g. https://node.somenano.com
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

// GenerateWork requests proof-of-work from the node for the given hash or public key hex.
func (c *Client) GenerateWork(ctx context.Context, hash string) (string, error) {
	var r struct {
		Work  string `json:"work"`
		Error rpcErrorField `json:"error"`
	}
	if err := c.call(ctx, map[string]string{
		"action": "work_generate",
		"hash":   hash,
	}, &r); err != nil {
		return "", err
	}
	if r.Error != "" {
		return "", fmt.Errorf("rpc work_generate: %s", r.Error)
	}
	return r.Work, nil
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
