// postgres.go manages the PostgreSQL connection pool and all database operations.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps a pgx connection pool and exposes typed query methods.
type DB struct {
	pool *pgxpool.Pool
}

// Connect opens a connection pool to the given Postgres URL and verifies connectivity.
func Connect(ctx context.Context, url string) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Migrate runs all SQL migration files in lexicographic order.
// Each file is executed as a single statement block; IF NOT EXISTS guards make
// the operation safe to re-run on every startup.
func (db *DB) Migrate(ctx context.Context) error {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		sql, err := fs.ReadFile(migrationsFS, "migrations/"+entry.Name())
		if err != nil {
			return fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		if _, err := db.pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("exec %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// Close releases all pool connections.
func (db *DB) Close() {
	db.pool.Close()
}

// NextSeedIndex reserves the next HD wallet index from the database sequence.
// This must be called before deriving the Nano address so the index is stable.
func (db *DB) NextSeedIndex(ctx context.Context) (int, error) {
	var idx int
	err := db.pool.QueryRow(ctx, "SELECT nextval('player_seed_index_seq')").Scan(&idx)
	return idx, err
}

// CreatePlayer inserts a new player row. If the nano_address already exists
// (reconnecting player) the existing row is returned unchanged.
func (db *DB) CreatePlayer(ctx context.Context, nanoAddress string, seedIndex int) (id string, err error) {
	err = db.pool.QueryRow(ctx,
		`INSERT INTO players (nano_address, seed_index)
		 VALUES ($1, $2)
		 ON CONFLICT (nano_address) DO UPDATE
		   SET created_at = players.created_at
		 RETURNING id`,
		nanoAddress, seedIndex,
	).Scan(&id)
	return id, err
}

// CreateSession inserts a new game_sessions row for the given room and player.
func (db *DB) CreateSession(ctx context.Context, roomID, playerID string) (id string, err error) {
	err = db.pool.QueryRow(ctx,
		`INSERT INTO game_sessions (room_id, player_id)
		 VALUES ($1, $2)
		 RETURNING id`,
		roomID, playerID,
	).Scan(&id)
	return id, err
}

// SettleSession marks a session as complete with the final Nano result.
func (db *DB) SettleSession(ctx context.Context, sessionID, nanoResult string) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE game_sessions
		 SET nano_result = $2, settled_at = now()
		 WHERE id = $1`,
		sessionID, nanoResult,
	)
	return err
}

// RecordTransaction appends a Nano transaction record to a session.
// direction must be one of: deposit, withdrawal, shot, donation, heal_reward.
func (db *DB) RecordTransaction(ctx context.Context, sessionID, direction, amount, blockHash string) error {
	_, err := db.pool.Exec(ctx,
		`INSERT INTO nano_transactions (session_id, direction, amount, block_hash)
		 VALUES ($1, $2, $3, NULLIF($4, ''))`,
		sessionID, direction, amount, blockHash,
	)
	return err
}

// RecordDeposit records an incoming Nano deposit with the sender's address.
// This audit trail allows refunds if a player needs to be reimbursed.
func (db *DB) RecordDeposit(ctx context.Context, sessionID, fromAddress, amountRaw, blockHash string) error {
	_, err := db.pool.Exec(ctx,
		`INSERT INTO nano_transactions (session_id, direction, amount, block_hash, from_address)
		 VALUES ($1, 'deposit', $2, NULLIF($3, ''), NULLIF($4, ''))`,
		sessionID, amountRaw, blockHash, fromAddress,
	)
	return err
}

// StoreMasterSeedFingerprint upserts the Blake2b-256 hash of the master seed
// into the settings table. This lets you verify which seed generated the wallets
// stored in the DB without ever persisting the seed itself.
func (db *DB) StoreMasterSeedFingerprint(ctx context.Context, fingerprintHex string) error {
	return db.SetSetting(ctx, "master_seed_blake2b_fingerprint", fingerprintHex)
}

// LogSession records a new WebSocket session start in the session_log table.
// playerID may be empty when running without a DB-persisted player record.
func (db *DB) LogSession(ctx context.Context, playerID, nanoAddress, roomID, team, remoteAddr, nickname string) error {
	var pid *string
	if playerID != "" {
		pid = &playerID
	}
	_, err := db.pool.Exec(ctx,
		`INSERT INTO session_log (player_id, nano_address, room_id, team, remote_addr, nickname)
		 VALUES ($1, NULLIF($2,''), $3, NULLIF($4,''), NULLIF($5,''), NULLIF($6,''))`,
		pid, nanoAddress, roomID, team, remoteAddr, nickname,
	)
	return err
}

// GetDepositSender returns the nano_ address of the most recent deposit sender
// for the given session. Returns an error if no deposit with a known sender exists.
func (db *DB) GetDepositSender(ctx context.Context, sessionID string) (string, error) {
	var addr string
	err := db.pool.QueryRow(ctx,
		`SELECT from_address FROM nano_transactions
		 WHERE session_id = $1 AND direction = 'deposit' AND from_address IS NOT NULL
		 ORDER BY created_at DESC LIMIT 1`,
		sessionID,
	).Scan(&addr)
	return addr, err
}

// Setting retrieves a single value from the settings table.
func (db *DB) Setting(ctx context.Context, key string) (string, error) {
	var value string
	err := db.pool.QueryRow(ctx,
		`SELECT value FROM settings WHERE key = $1`, key,
	).Scan(&value)
	return value, err
}

// FaucetDisableSameIPCheck returns true when the faucet_disable_same_ip_check
// setting is "true", meaning same-IP kills and heals will still earn rewards.
func (db *DB) FaucetDisableSameIPCheck(ctx context.Context) bool {
	val, err := db.Setting(ctx, "faucet_disable_same_ip_check")
	return err == nil && val == "true"
}

// RecordFaucetPayout inserts an audit row for a faucet reward that was sent on-chain.
func (db *DB) RecordFaucetPayout(ctx context.Context, reason, toAddress, ip, amount, blockHash string) error {
	_, err := db.pool.Exec(ctx,
		`INSERT INTO faucet_payouts (reason, to_address, ip, amount, block_hash)
		 VALUES ($1, $2, NULLIF($3,''), $4, NULLIF($5,''))`,
		reason, toAddress, ip, amount, blockHash,
	)
	return err
}

// FaucetPayoutsToday counts how many faucet payouts have been made to a given IP
// since midnight UTC today. Used to enforce the daily per-IP payout limit.
func (db *DB) FaucetPayoutsToday(ctx context.Context, ip string) (int, error) {
	var count int
	err := db.pool.QueryRow(ctx,
		`SELECT count(*) FROM faucet_payouts
		 WHERE ip = $1 AND paid_at >= CURRENT_DATE`,
		ip,
	).Scan(&count)
	return count, err
}

// LogAccess inserts a row into access_log and increments the access_daily counter.
// Returns the new row's ID so the caller can later fill in the country.
func (db *DB) LogAccess(ctx context.Context, ip, path string) (int64, error) {
	var id int64
	err := db.pool.QueryRow(ctx,
		`INSERT INTO access_log (ip, path) VALUES ($1, $2) RETURNING id`,
		ip, path,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	_, err = db.pool.Exec(ctx,
		`INSERT INTO access_daily (day, count) VALUES (CURRENT_DATE, 1)
		 ON CONFLICT (day) DO UPDATE SET count = access_daily.count + 1`,
	)
	return id, err
}

// SetAccessCountry fills in the country for an existing access_log row.
func (db *DB) SetAccessCountry(ctx context.Context, id int64, country string) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE access_log SET country = $2 WHERE id = $1`,
		id, country,
	)
	return err
}

// SetSetting upserts a key/value pair in the settings table.
func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := db.pool.Exec(ctx,
		`INSERT INTO settings (key, value) VALUES ($1, $2)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		key, value,
	)
	return err
}
