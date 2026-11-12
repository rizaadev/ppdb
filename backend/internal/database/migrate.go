package database

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	sqlfiles "github.com/pbdb/platform/sql"
)

const migrationLockKey int64 = 7482019412

// RunMigrations applies pending SQL schema migrations embedded from sql/migrations idempotently.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	if pool == nil {
		return fmt.Errorf("database pool is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection for migrations: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockKey)
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	entries, err := fs.ReadDir(sqlfiles.MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version := entry.Name()

		var applied bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if applied {
			continue
		}

		raw, err := sqlfiles.MigrationsFS.ReadFile("migrations/" + version)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		upSQL := parseGooseUpSQL(string(raw))
		if strings.TrimSpace(upSQL) == "" {
			return fmt.Errorf("migration %s has empty up SQL", version)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration tx %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, upSQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("execute migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}
		logger.Info("applied sql migration", "version", version)
	}

	if err := GrantRuntimePermissions(ctx, pool); err != nil {
		return fmt.Errorf("grant runtime permissions: %w", err)
	}
	return nil
}

func parseGooseUpSQL(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	var out strings.Builder
	inUp := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "-- +goose Up":
			inUp = true
			continue
		case trimmed == "-- +goose Down":
			inUp = false
			return strings.TrimSpace(out.String())
		case strings.HasPrefix(trimmed, "-- +goose "):
			continue
		}
		if inUp {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return strings.TrimSpace(out.String())
}

