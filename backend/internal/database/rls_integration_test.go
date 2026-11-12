package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresRLSEnforcementAndIsolation(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping live PostgreSQL RLS integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := NewPool(ctx, dbURL, 4, 1)
	if err != nil {
		t.Fatalf("connect postgresql: %v", err)
	}
	defer pool.Close()

	if err := RunMigrations(ctx, pool, nil); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if err := GrantRuntimePermissions(ctx, pool); err != nil {
		t.Fatalf("GrantRuntimePermissions: %v", err)
	}

	// 1. Verify pbdb_app role exists and cannot bypass RLS.
	var rolSuper, rolBypass bool
	if err := pool.QueryRow(ctx, `
		SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = $1
	`, RLSRoleName).Scan(&rolSuper, &rolBypass); err != nil {
		t.Fatalf("inspect pbdb_app role: %v", err)
	}
	if rolSuper || rolBypass {
		t.Fatalf("expected non-superuser NOBYPASSRLS role %s, got rolsuper=%v rolbypassrls=%v", RLSRoleName, rolSuper, rolBypass)
	}

	// 2. Verify all 17 tenant-scoped tables have ENABLE + FORCE ROW LEVEL SECURITY.
	expectedTables := []string{
		"units",
		"user_tenant_roles",
		"admission_periods",
		"admission_tracks",
		"track_requirements",
		"applications",
		"applicant_profiles",
		"applicant_guardians",
		"applicant_achievements",
		"applicant_documents",
		"payment_transactions",
		"assessment_schedules",
		"assessment_scores",
		"status_audit_logs",
		"notification_logs",
		"export_jobs",
		"idempotency_keys",
	}
	for _, tbl := range expectedTables {
		var rlsEnabled, rlsForced bool
		if err := pool.QueryRow(ctx, `
			SELECT relrowsecurity, relforcerowsecurity
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $1
		`, tbl).Scan(&rlsEnabled, &rlsForced); err != nil {
			t.Fatalf("inspect pg_class for %s: %v", tbl, err)
		}
		if !rlsEnabled || !rlsForced {
			t.Fatalf("table %s missing ENABLE/FORCE RLS (enabled=%v forced=%v)", tbl, rlsEnabled, rlsForced)
		}
	}

	// 3. Create two isolated temporary tenants + units via WithTenantTx, and clean up afterwards.
	tenantA := uuid.Must(uuid.NewV7())
	tenantB := uuid.Must(uuid.NewV7())
	unitA := uuid.Must(uuid.NewV7())
	unitB := uuid.Must(uuid.NewV7())
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM units WHERE tenant_id IN ($1, $2)`, tenantA, tenantB)
		_, _ = pool.Exec(context.Background(), `DELETE FROM tenants WHERE id IN ($1, $2)`, tenantA, tenantB)
	}()

	if err := WithBypassRLSTx(ctx, pool, func(tx pgx.Tx) error {
		for _, item := range []struct {
			id   uuid.UUID
			slug string
			name string
		}{
			{tenantA, "rls-a-" + suffix, "RLS Tenant A"},
			{tenantB, "rls-b-" + suffix, "RLS Tenant B"},
		} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO tenants (id, slug, name, institution_type, terminology_preset)
				VALUES ($1, $2, $3, 'school', 'spmb')
			`, item.id, item.slug, item.name); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("insert test tenants: %v", err)
	}

	if err := WithTenantTx(ctx, pool, tenantA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO units (id, tenant_id, code, name, education_level)
			VALUES ($1, $2, 'UNIT-A', 'Unit Alpha', 'sma_ma')
		`, unitA, tenantA)
		return err
	}); err != nil {
		t.Fatalf("insert unitA under WithTenantTx: %v", err)
	}

	if err := WithTenantTx(ctx, pool, tenantB, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO units (id, tenant_id, code, name, education_level)
			VALUES ($1, $2, 'UNIT-B', 'Unit Beta', 'smp_mts')
		`, unitB, tenantB)
		return err
	}); err != nil {
		t.Fatalf("insert unitB under WithTenantTx: %v", err)
	}

	// 4. Inside WithTenantTx(tenantA): SELECT sees only unitA; INSERT/UPDATE into tenantB fails RLS!
	if err := WithTenantTx(ctx, pool, tenantA, func(tx pgx.Tx) error {
		var currentRole string
		if err := tx.QueryRow(ctx, `SELECT current_user`).Scan(&currentRole); err != nil {
			return err
		}
		if currentRole != RLSRoleName {
			t.Fatalf("expected active transaction role %s, got %s", RLSRoleName, currentRole)
		}

		var countAll, countB int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM units`).Scan(&countAll); err != nil {
			return err
		}
		if countAll != 1 {
			t.Fatalf("expected WithTenantTx(tenantA) to see 1 unit row, got %d", countAll)
		}
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM units WHERE id = $1`, unitB).Scan(&countB); err != nil {
			return err
		}
		if countB != 0 {
			t.Fatalf("cross-tenant leak: saw unitB (%d rows) from tenantA transaction", countB)
		}

		tag, err := tx.Exec(ctx, `UPDATE units SET name = 'TAMPERED' WHERE id = $1`, unitB)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 0 {
			t.Fatalf("cross-tenant UPDATE affected %d rows!", tag.RowsAffected())
		}
		return nil
	}); err != nil {
		t.Fatalf("WithTenantTx(tenantA) read/update assertions: %v", err)
	}

	insertHijackErr := WithTenantTx(ctx, pool, tenantA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO units (id, tenant_id, code, name, education_level)
			VALUES ($1, $2, 'HIJACK', 'Hijack Unit', 'sd_mi')
		`, uuid.Must(uuid.NewV7()), tenantB)
		return err
	})
	var pgErr *pgconn.PgError
	if !errors.As(insertHijackErr, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("expected PostgreSQL RLS 42501 error inserting into tenantB from tenantA tx, got: %v", insertHijackErr)
	}

	// 5. Dropping to pbdb_app with empty app.current_tenant_id hides all tenant rows across core tables.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fail-closed test tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		SELECT set_config('role', $1, true),
		       set_config('app.bypass_rls', 'off', true),
		       set_config('app.current_tenant_id', '', true)
	`, RLSRoleName); err != nil {
		t.Fatalf("configure fail-closed role: %v", err)
	}
	for _, tbl := range []string{"units", "applications", "payment_transactions", "applicant_documents", "user_tenant_roles"} {
		var visible int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM `+tbl).Scan(&visible); err != nil {
			t.Fatalf("fail-closed query on %s: %v", tbl, err)
		}
		if visible != 0 {
			t.Fatalf("expected 0 visible rows on %s without tenant context, got %d", tbl, visible)
		}
	}
	_ = tx.Rollback(ctx)

	// 6. WithBypassRLSTx can inspect rows across both tenants cleanly.
	var totalTestUnits int
	if err := WithBypassRLSTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COUNT(*) FROM units WHERE id IN ($1, $2)`, unitA, unitB).Scan(&totalTestUnits)
	}); err != nil {
		t.Fatalf("WithBypassRLSTx: %v", err)
	}
	if totalTestUnits != 2 {
		t.Fatalf("expected WithBypassRLSTx to see both units (2), got %d", totalTestUnits)
	}
}
