package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RLSRoleName is the non-superuser database role assumed inside tenant transactions so
// PostgreSQL Row-Level Security policies are enforced even if the pool connects as owner/postgres.
const RLSRoleName = "pbdb_app"

// NewPool initializes a resource-conscious pgx connection pool suitable for VPS deployments.
func NewPool(ctx context.Context, databaseURL string, maxConns, minConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgresql: %w", err)
	}

	return pool, nil
}

// WithTenantTx executes fn inside a transaction bound to tenantID and drops privileges to pbdb_app
// so PostgreSQL RLS policies isolate tenant rows strictly in the database kernel.
func WithTenantTx(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, fn func(tx pgx.Tx) error) error {
	if tenantID == uuid.Nil {
		return fmt.Errorf("tenant_id is required for RLS-scoped transaction")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := ApplyTenantRLS(ctx, tx, tenantID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ApplyTenantRLS switches the current transaction role to pbdb_app (if available) and sets
// app.current_tenant_id transaction-locally so PostgreSQL RLS policies filter strictly to tenantID.
func ApplyTenantRLS(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) error {
	if tenantID == uuid.Nil {
		return fmt.Errorf("tenant_id is required for RLS context")
	}
	_, err := tx.Exec(ctx, `
		SELECT
			CASE
				WHEN EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pbdb_app')
				     AND pg_has_role(session_user, 'pbdb_app', 'MEMBER')
				THEN set_config('role', 'pbdb_app', true)
				ELSE current_user
			END,
			set_config('app.bypass_rls', 'off', true),
			set_config('app.current_tenant_id', $1, true)
	`, tenantID.String())
	return err
}

// WithBypassRLSTx runs a transaction explicitly marked for platform/cross-tenant system operations
// (such as platform admin tenant listing or resolving a payment gateway webhook external_ref to its tenant).
func WithBypassRLSTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		SELECT
			CASE
				WHEN EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pbdb_app')
				     AND pg_has_role(session_user, 'pbdb_app', 'MEMBER')
				THEN set_config('role', 'pbdb_app', true)
				ELSE current_user
			END,
			set_config('app.current_tenant_id', '', true),
			set_config('app.bypass_rls', 'on', true)
	`); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GrantRuntimePermissions ensures the non-superuser pbdb_app role has DML access to all tables
// and sequences created during migrations and River queue initialization.
func GrantRuntimePermissions(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}
	_, err := pool.Exec(ctx, `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pbdb_app') THEN
				GRANT USAGE ON SCHEMA public TO pbdb_app;
				GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO pbdb_app;
				GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO pbdb_app;
				BEGIN
					EXECUTE format('GRANT pbdb_app TO %I', current_user);
				EXCEPTION WHEN OTHERS THEN
					NULL;
				END;
			END IF;
		END $$;
	`)
	return err
}
