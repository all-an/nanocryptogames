// rpg.go contains database operations for the Nano Faucet Multiplayer RPG.
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// RPGAccount holds the persisted data for an RPG player account.
type RPGAccount struct {
	ID           string
	Username     string
	PasswordHash string
	SeedIndex    int
	NanoAddress  string
	CreatedAt    time.Time
}

// CreateRPGAccount inserts a new RPG player account and returns the created row.
// Returns an error (typically a unique constraint violation) if the username is taken.
func (db *DB) CreateRPGAccount(ctx context.Context, username, passwordHash string, seedIndex int, nanoAddress string) (*RPGAccount, error) {
	a := &RPGAccount{}
	err := db.pool.QueryRow(ctx,
		`INSERT INTO rpg_accounts (username, password_hash, seed_index, nano_address)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, username, password_hash, seed_index, nano_address, created_at`,
		username, passwordHash, seedIndex, nanoAddress,
	).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.SeedIndex, &a.NanoAddress, &a.CreatedAt)
	return a, err
}

// GetRPGAccountByUsername fetches an account by username.
// Returns pgx.ErrNoRows when not found.
func (db *DB) GetRPGAccountByUsername(ctx context.Context, username string) (*RPGAccount, error) {
	a := &RPGAccount{}
	err := db.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, seed_index, nano_address, created_at
		 FROM rpg_accounts WHERE username = $1`,
		username,
	).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.SeedIndex, &a.NanoAddress, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// GetRPGAccountByID fetches an account by its UUID primary key.
// Returns pgx.ErrNoRows when not found.
func (db *DB) GetRPGAccountByID(ctx context.Context, id string) (*RPGAccount, error) {
	a := &RPGAccount{}
	err := db.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, seed_index, nano_address, created_at
		 FROM rpg_accounts WHERE id = $1`,
		id,
	).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.SeedIndex, &a.NanoAddress, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// CreateRPGSession stores a new session token associated with an account.
func (db *DB) CreateRPGSession(ctx context.Context, token, accountID string) error {
	_, err := db.pool.Exec(ctx,
		`INSERT INTO rpg_sessions (token, account_id) VALUES ($1, $2)`,
		token, accountID,
	)
	return err
}

// GetRPGSession looks up the account ID for a session token and refreshes last_seen.
// Returns pgx.ErrNoRows when the token is not found or has not been active within 7 days.
func (db *DB) GetRPGSession(ctx context.Context, token string) (string, error) {
	var accountID string
	err := db.pool.QueryRow(ctx,
		`UPDATE rpg_sessions
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

// DeleteRPGSession removes a session token (player logout).
func (db *DB) DeleteRPGSession(ctx context.Context, token string) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM rpg_sessions WHERE token = $1`, token)
	return err
}
