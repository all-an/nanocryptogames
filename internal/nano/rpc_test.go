package nano

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockNode starts a test HTTP server that responds with the given JSON payload.
func mockNode(t *testing.T, response any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}

func TestGetBalance_success(t *testing.T) {
	srv := mockNode(t, map[string]string{"balance": "1000000", "pending": "500"})
	defer srv.Close()

	client := NewClient(Config{PrimaryURL: srv.URL})
	bal, err := client.GetBalance(context.Background(), "nano_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bal.Balance != "1000000" {
		t.Errorf("expected balance 1000000, got %s", bal.Balance)
	}
	if bal.Pending != "500" {
		t.Errorf("expected pending 500, got %s", bal.Pending)
	}
}

func TestGetBalance_nodeError(t *testing.T) {
	srv := mockNode(t, map[string]string{"error": "Account not found"})
	defer srv.Close()

	client := NewClient(Config{PrimaryURL: srv.URL})
	_, err := client.GetBalance(context.Background(), "nano_test")
	if err == nil {
		t.Error("expected error for node error response")
	}
}

func TestGetBalance_fallback(t *testing.T) {
	// Primary always fails with HTTP 500.
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer primary.Close()

	// Fallback returns a valid response.
	fallback := mockNode(t, map[string]string{"balance": "999", "pending": "0"})
	defer fallback.Close()

	client := NewClient(Config{PrimaryURL: primary.URL, FallbackURL: fallback.URL})
	bal, err := client.GetBalance(context.Background(), "nano_test")
	if err != nil {
		t.Fatalf("fallback should have succeeded: %v", err)
	}
	if bal.Balance != "999" {
		t.Errorf("expected balance 999, got %s", bal.Balance)
	}
}

func TestGetAccountInfo_success(t *testing.T) {
	srv := mockNode(t, map[string]string{
		"frontier":       "ABCD1234",
		"balance":        "2000000",
		"representative": "nano_3t6k35gi95xu6tergt6p69ck76ogmitsa8mnijtpxm9fkcm736xtoncuohr3",
	})
	defer srv.Close()

	client := NewClient(Config{PrimaryURL: srv.URL})
	info, err := client.GetAccountInfo(context.Background(), "nano_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Frontier != "ABCD1234" {
		t.Errorf("expected frontier ABCD1234, got %s", info.Frontier)
	}
	if info.Balance != "2000000" {
		t.Errorf("expected balance 2000000, got %s", info.Balance)
	}
}

func TestReceivable_empty(t *testing.T) {
	// Node returns empty string for blocks when nothing is pending.
	srv := mockNode(t, map[string]any{"blocks": ""})
	defer srv.Close()

	client := NewClient(Config{PrimaryURL: srv.URL})
	hashes, err := client.Receivable(context.Background(), "nano_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hashes) != 0 {
		t.Errorf("expected 0 hashes, got %d", len(hashes))
	}
}

func TestReceivable_withBlocks(t *testing.T) {
	srv := mockNode(t, map[string]any{
		"blocks": map[string]string{
			"HASH1": "1000000000000000000000000000000",
			"HASH2": "500000000000000000000000000000",
		},
	})
	defer srv.Close()

	client := NewClient(Config{PrimaryURL: srv.URL})
	hashes, err := client.Receivable(context.Background(), "nano_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hashes) != 2 {
		t.Errorf("expected 2 hashes, got %d", len(hashes))
	}
}

func TestBlockInfo_success(t *testing.T) {
	srv := mockNode(t, map[string]string{
		"amount":        "5000000",
		"block_account": "nano_3sender111",
	})
	defer srv.Close()

	client := NewClient(Config{PrimaryURL: srv.URL})
	details, err := client.BlockInfo(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if details.Amount != "5000000" {
		t.Errorf("expected amount 5000000, got %s", details.Amount)
	}
	if details.Account != "nano_3sender111" {
		t.Errorf("expected account nano_3sender111, got %s", details.Account)
	}
}

func TestGenerateWork_success(t *testing.T) {
	srv := mockNode(t, map[string]string{"work": "abc123def456"})
	defer srv.Close()

	client := NewClient(Config{PrimaryURL: srv.URL})
	work, err := client.GenerateWork(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if work != "abc123def456" {
		t.Errorf("expected work abc123def456, got %s", work)
	}
}

func TestGenerateWork_numericError(t *testing.T) {
	// When the node returns a numeric 403 error, GenerateWork falls back to CPU mining
	// rather than propagating the error. Verify it handles the numeric error field
	// without panicking, and returns a context error when the deadline expires.
	srv := mockNode(t, map[string]any{"error": 403, "message": "Access Denied"})
	defer srv.Close()

	client := NewClient(Config{PrimaryURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := client.GenerateWork(ctx, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil {
		t.Fatal("expected context deadline error, but got nil")
	}
}
