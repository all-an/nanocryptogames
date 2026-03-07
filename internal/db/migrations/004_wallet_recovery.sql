-- 004_wallet_recovery.sql
-- Self-contained recovery reference for all player session wallets.
--
-- RECOVERY INSTRUCTIONS
-- ─────────────────────
-- Every temporary address is derived from a single master seed using the
-- standard Nano HD wallet algorithm:
--
--   account_key = Blake2b-256( master_seed_bytes(32) || seed_index_uint32_be(4) )
--   private_key = ed25519.NewKeyFromSeed(account_key)
--   address     = nano_<base32(pubkey)><base32(checksum)>
--
-- To recover any address:
--   1. Open https://nault.cc (or any standard Nano HD wallet)
--   2. "Import wallet" → paste your NANO_MASTER_SEED (64 hex chars)
--   3. Navigate to account number = seed_index (0-based)
--   That account is the player's temporary wallet.
--
-- The master_seed_blake2b_fingerprint in the settings table lets you verify
-- you have the correct master seed without ever storing the seed itself.

CREATE OR REPLACE VIEW wallet_recovery AS
SELECT
    p.id                  AS player_id,
    p.nano_address,
    p.seed_index,
    p.created_at          AS wallet_created_at,
    'Blake2b-256(NANO_MASTER_SEED || uint32_be(seed_index)) → ed25519 private key. '
    'Open nault.cc, import NANO_MASTER_SEED, go to account #seed_index.'
                          AS recovery_method
FROM players p
ORDER BY p.seed_index;

-- Document the derivation standard for future reference.
INSERT INTO settings (key, value) VALUES
    ('derivation_standard',
     'Nano HD: account_key = Blake2b-256(seed_32bytes || index_uint32_be); ed25519 private key from account_key'),
    ('derivation_tool',
     'https://nault.cc — import NANO_MASTER_SEED as wallet seed, use seed_index as account number'),
    ('master_seed_blake2b_fingerprint',
     '')   -- populated at startup by the server; lets you verify the seed without exposing it
ON CONFLICT (key) DO NOTHING;
