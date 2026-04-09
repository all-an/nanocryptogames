// farm.go contains database operations for the Nano Faucet Multiplayer Farm.
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// FarmAccount holds the persisted data for an Farm player account.
type FarmAccount struct {
	ID           string
	Username     string
	PasswordHash string
	Email        *string // optional; nil when not provided
	Color        string  // CSS hex color chosen by the player; empty means use palette
	SeedIndex    int
	NanoAddress  string
	CreatedAt    time.Time
}

// CreateFarmAccount inserts a new Farm player account and returns the created row.
// email may be nil to store NULL. Returns an error if the username or email is taken.
func (db *DB) CreateFarmAccount(ctx context.Context, username, passwordHash string, email *string, seedIndex int, nanoAddress string) (*FarmAccount, error) {
	a := &FarmAccount{}
	err := db.pool.QueryRow(ctx,
		`INSERT INTO farm_accounts (username, password_hash, email, seed_index, nano_address)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, username, password_hash, email, color, seed_index, nano_address, created_at`,
		username, passwordHash, email, seedIndex, nanoAddress,
	).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.Email, &a.Color, &a.SeedIndex, &a.NanoAddress, &a.CreatedAt)
	return a, err
}

// GetFarmAccountByUsername fetches an account by username.
// Returns pgx.ErrNoRows when not found.
func (db *DB) GetFarmAccountByUsername(ctx context.Context, username string) (*FarmAccount, error) {
	a := &FarmAccount{}
	err := db.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, email, color, seed_index, nano_address, created_at
		 FROM farm_accounts WHERE username = $1`,
		username,
	).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.Email, &a.Color, &a.SeedIndex, &a.NanoAddress, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// GetFarmAccountByID fetches an account by its UUID primary key.
// Returns pgx.ErrNoRows when not found.
func (db *DB) GetFarmAccountByID(ctx context.Context, id string) (*FarmAccount, error) {
	a := &FarmAccount{}
	err := db.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, email, color, seed_index, nano_address, created_at
		 FROM farm_accounts WHERE id = $1`,
		id,
	).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.Email, &a.Color, &a.SeedIndex, &a.NanoAddress, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// UpdateFarmAccount updates the username, email, and display color for an account.
// email may be nil to store NULL. color should be a CSS hex string (e.g. "#4A90D9").
// Returns an error on unique-constraint violation (duplicate username or email).
func (db *DB) UpdateFarmAccount(ctx context.Context, accountID, username string, email *string, color string) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE farm_accounts SET username = $1, email = $2, color = $3 WHERE id = $4`,
		username, email, color, accountID,
	)
	return err
}

// CreateFarmSession stores a new session token associated with an account.
func (db *DB) CreateFarmSession(ctx context.Context, token, accountID string) error {
	_, err := db.pool.Exec(ctx,
		`INSERT INTO farm_sessions (token, account_id) VALUES ($1, $2)`,
		token, accountID,
	)
	return err
}

// GetFarmSession looks up the account ID for a session token and refreshes last_seen.
// Returns pgx.ErrNoRows when the token is not found or has not been active within 7 days.
func (db *DB) GetFarmSession(ctx context.Context, token string) (string, error) {
	var accountID string
	err := db.pool.QueryRow(ctx,
		`UPDATE farm_sessions
		 SET last_seen = now()
		 WHERE token = $1 AND last_seen > now() - interval '7 days'
		 RETURNING account_id`,
		token,
	).Scan(&accountID)
	if err == pgx.ErrNoRows {
		return "", pgx.ErrNoRows
	}
	return accountID, err
}

// DeleteFarmSession removes a session token (player logout).
func (db *DB) DeleteFarmSession(ctx context.Context, token string) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM farm_sessions WHERE token = $1`, token)
	return err
}
