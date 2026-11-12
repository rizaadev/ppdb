package admissions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pbdb/platform/internal/database"
)

// PGIdempotencyCache stores mutation responses in idempotency_keys under tenant RLS isolation.
type PGIdempotencyCache struct {
	pool *pgxpool.Pool
	ttl  time.Duration
}

// NewPGIdempotencyCache creates a PostgreSQL-backed IdempotencyStore.
func NewPGIdempotencyCache(pool *pgxpool.Pool, ttl time.Duration) *PGIdempotencyCache {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &PGIdempotencyCache{pool: pool, ttl: ttl}
}

func (c *PGIdempotencyCache) Lookup(
	ctx context.Context,
	tenantID, userID uuid.UUID,
	op, key string,
	dest any,
) (bool, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" || c.pool == nil {
		return false, nil
	}

	var raw []byte
	var found bool
	err := database.WithTenantTx(ctx, c.pool, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT response_json
			FROM idempotency_keys
			WHERE tenant_id = $1
			  AND user_id = $2
			  AND scope = $3
			  AND idempotency_key = $4
			  AND expires_at > NOW()
		`, tenantID, userID, op, trimmed).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = len(raw) > 0
		return nil
	})
	if err != nil || !found {
		return false, err
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return false, err
	}
	return true, nil
}

func (c *PGIdempotencyCache) Save(
	ctx context.Context,
	tenantID, userID uuid.UUID,
	op, key string,
	value any,
) error {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" || c.pool == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(op + ":" + trimmed))
	reqHash := hex.EncodeToString(hash[:])
	expiresAt := time.Now().UTC().Add(c.ttl)

	return database.WithTenantTx(ctx, c.pool, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO idempotency_keys (
				id, tenant_id, user_id, scope, idempotency_key,
				request_hash, status_code, response_json, expires_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, 200, $7::jsonb, $8
			)
			ON CONFLICT (tenant_id, user_id, scope, idempotency_key) DO UPDATE SET
				response_json = EXCLUDED.response_json,
				expires_at = EXCLUDED.expires_at
		`, uuid.Must(uuid.NewV7()), tenantID, userID, op, trimmed, reqHash, string(raw), expiresAt)
		return err
	})
}

