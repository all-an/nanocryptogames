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
	"time"
)

const rpcTimeout = 10 * time.Second

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
		Error   string `json:"error"`
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
		Error          string `json:"error"`
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
func (c *Client) Receivable(ctx context.Context, address string) ([]string, error) {
	var r struct {
		Blocks any    `json:"blocks"`
		Error  string `json:"error"`
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

	// blocks is "" when empty or a map[hash]amount when non-empty.
	m, ok := r.Blocks.(map[string]any)
	if !ok {
		return nil, nil
	}
	hashes := make([]string, 0, len(m))
	for hash := range m {
		hashes = append(hashes, hash)
	}
	return hashes, nil
}

// BlockInfo returns amount and source info for a given block hash.
// Used to determine the value of a pending receive block.
func (c *Client) BlockInfo(ctx context.Context, blockHash string) (amount string, err error) {
	var r struct {
		Amount string `json:"amount"`
		Error  string `json:"error"`
	}
	if err := c.call(ctx, map[string]string{
		"action": "block_info",
		"hash":   blockHash,
	}, &r); err != nil {
		return "", err
	}
	if r.Error != "" {
		return "", fmt.Errorf("rpc block_info: %s", r.Error)
	}
	return r.Amount, nil
}

// GenerateWork requests proof-of-work from the node for the given hash or public key hex.
func (c *Client) GenerateWork(ctx context.Context, hash string) (string, error) {
	var r struct {
		Work  string `json:"work"`
		Error string `json:"error"`
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
		Error string `json:"error"`
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

	raw, primaryErr := c.post(ctx, c.cfg.PrimaryURL, body)
	if primaryErr != nil {
		if c.cfg.FallbackURL == "" {
			return primaryErr
		}
		raw, err = c.post(ctx, c.cfg.FallbackURL, body)
		if err != nil {
			return fmt.Errorf("primary: %w; fallback: %w", primaryErr, err)
		}
	}

	return json.Unmarshal(raw, dst)
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
