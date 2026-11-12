package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/pbdb/platform/internal/config"
	"github.com/pbdb/platform/internal/database"
	"github.com/pbdb/platform/internal/seed"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	migURL := cfg.MigrationDatabaseURL
	if migURL == "" {
		migURL = cfg.DatabaseURL
	}
	if migURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL or MIGRATION_DATABASE_URL is required to run database migrations or seeding")
		os.Exit(1)
	}

	action := "all"
	if len(os.Args) > 1 {
		action = strings.ToLower(strings.TrimSpace(os.Args[1]))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := database.NewPool(ctx, migURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		logger.Error("connect postgresql", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	switch action {
	case "up":
		err = database.RunMigrations(ctx, pool, logger)
	case "seed":
		if cfg.AppEnv == "production" && !cfg.AllowDemoSeedInProd {
			fmt.Fprintln(os.Stderr, "refusing to run demo seed in production without ALLOW_DEMO_SEED_IN_PROD=true")
			os.Exit(2)
		}
		err = seed.SeedDemoData(ctx, pool, logger)
	case "all", "up-and-seed":
		if err = database.RunMigrations(ctx, pool, logger); err == nil {
			if cfg.AppEnv != "production" || cfg.AllowDemoSeedInProd {
				err = seed.SeedDemoData(ctx, pool, logger)
			} else {
				logger.Info("skipping demo data seed in production environment")
			}
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q: use up, seed, or all\n", action)
		os.Exit(2)
	}

	if err != nil {
		logger.Error("migration command failed", "action", action, "error", err)
		os.Exit(1)
	}
	logger.Info("migration command completed", "action", action)
}

