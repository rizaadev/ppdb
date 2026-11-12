package database

import (
	"io/fs"
	"sort"
	"strings"
	"testing"

	sqlfiles "github.com/pbdb/platform/sql"
)

func TestEmbeddedMigrationsParseCleanly(t *testing.T) {
	entries, err := fs.ReadDir(sqlfiles.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	if len(entries) < 4 {
		t.Fatalf("expected at least 4 embedded migration files, got %d", len(entries))
	}

	for _, entry := range entries {
		raw, err := sqlfiles.MigrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		upSQL := parseGooseUpSQL(string(raw))
		if strings.TrimSpace(upSQL) == "" {
			t.Fatalf("migration %s produced empty up SQL", entry.Name())
		}
		if strings.Contains(upSQL, "DROP TABLE") {
			t.Fatalf("migration %s leaked Down SQL into Up section", entry.Name())
		}
	}
}

