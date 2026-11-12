package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pbdb/platform/internal/config"
	"github.com/pbdb/platform/internal/database"
	"github.com/pbdb/platform/internal/httpapi"
	"github.com/pbdb/platform/internal/httpapi/handlers"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/integrations"
	"github.com/pbdb/platform/internal/modules/admissions"
	"github.com/pbdb/platform/internal/modules/auth"
	"github.com/pbdb/platform/internal/modules/public"
	"github.com/pbdb/platform/internal/seed"
	"github.com/pbdb/platform/internal/storage"
	"github.com/pbdb/platform/internal/worker"
	"github.com/pbdb/platform/web"
	"github.com/riverqueue/river"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	var objectStore storage.ObjectStorage
	if cfg.StorageProvider == "local" {
		localStore, localErr := storage.NewLocalStorage(cfg.LocalStorageDir, cfg.PublicBaseURL, cfg.DocumentSecret)
		if localErr != nil {
			logger.Error("failed to initialize local object storage", "error", localErr)
			os.Exit(1)
		}
		objectStore = localStore
		logger.Info("using local disk object storage", "dir", cfg.LocalStorageDir, "public_base_url", cfg.PublicBaseURL)
	} else {
		presigner, presignErr := storage.NewPresigner(storage.Config{
			Endpoint:       cfg.S3Endpoint,
			PublicEndpoint: cfg.S3PublicEndpoint,
			Region:         cfg.S3Region,
			Bucket:         cfg.S3Bucket,
			AccessKey:      cfg.S3AccessKey,
			SecretKey:      cfg.S3SecretKey,
			UsePathStyle:   cfg.S3UsePathStyle,
		})
		if presignErr != nil {
			if errors.Is(presignErr, storage.ErrStorageNotConfigured) {
				logger.Warn("object storage not configured; document presign and export upload endpoints disabled")
			} else {
				logger.Error("failed to initialize object storage presigner", "error", presignErr)
				os.Exit(1)
			}
		} else {
			objectStore = presigner
		}
	}

	waSender := integrations.NewWhatsAppSender(cfg.WhatsAppProvider, cfg.WhatsAppGatewayURL, cfg.WhatsAppGatewayToken, logger)
	paymentGateway := integrations.NewPaymentGateway(cfg.PaymentProvider, cfg.MidtransServerKey, cfg.MidtransSandbox)

	var pool *pgxpool.Pool
	var riverClient *river.Client[pgx.Tx]
	var tenantLookup middleware.TenantLookup
	var publicStore handlers.PublicPortalStore
	var platformStore handlers.PlatformTenantStore
	var setupStore handlers.AdminSetupStore
	var paymentConfigProvider admissions.TenantPaymentConfigProvider
	var authStore auth.Store
	var admissionsStore admissions.Store

	if cfg.DatabaseURL != "" {
		pool, err = database.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
		if err != nil {
			logger.Error("failed to connect postgresql", "error", err)
			os.Exit(1)
		}
		defer pool.Close()

		migPool := pool
		if cfg.MigrationDatabaseURL != "" && cfg.MigrationDatabaseURL != cfg.DatabaseURL {
			migPool, err = database.NewPool(ctx, cfg.MigrationDatabaseURL, 4, 1)
			if err != nil {
				logger.Error("failed to connect migration postgresql pool", "error", err)
				os.Exit(1)
			}
			defer migPool.Close()
		}

		if cfg.DBAutoMigrate {
			if err := database.RunMigrations(ctx, migPool, logger); err != nil {
				logger.Error("failed to apply sql migrations", "error", err)
				os.Exit(1)
			}
		}

		pgStore := public.NewPGStore(pool)
		tenantLookup = pgStore
		publicStore = pgStore
		platformStore = pgStore
		setupStore = pgStore
		paymentConfigProvider = pgStore

		if cfg.RiverEnabled {
			if err := worker.EnsureRiverSchema(ctx, migPool); err != nil {
				logger.Error("failed to migrate river schema", "error", err)
				os.Exit(1)
			}
		}

		if err := database.GrantRuntimePermissions(ctx, migPool); err != nil {
			logger.Warn("failed to refresh pbdb_app runtime permissions (ignoring if non-superuser runtime pool)", "error", err)
		}

		if cfg.DBSeedDemo && (cfg.AppEnv != "production" || cfg.AllowDemoSeedInProd) {
			if err := seed.SeedDemoData(ctx, migPool, logger); err != nil {
				logger.Error("failed to seed demo data", "error", err)
				os.Exit(1)
			}
		}

		if cfg.RiverEnabled {
			runWorkers := cfg.Mode == "all" || cfg.Mode == "worker"
			riverClient, err = worker.NewClient(pool, worker.ClientOptions{
				Logger:   logger,
				Storage:  objectStore,
				WhatsApp: waSender,
			}, runWorkers)
			if err != nil {
				logger.Error("failed to create river client", "error", err)
				os.Exit(1)
			}
			if runWorkers {
				if err := riverClient.Start(ctx); err != nil {
					logger.Error("failed to start river worker", "error", err)
					os.Exit(1)
				}
				logger.Info("river background worker started")
			}
		}

		authStore = auth.NewPGStore(pool, riverClient)
		admissionsStore = admissions.NewPGStore(pool, riverClient)
	} else {
		if cfg.AppEnv == "production" {
			logger.Error("DATABASE_URL is required in production")
			os.Exit(1)
		}
		logger.Warn("DATABASE_URL not set; running in development in-memory demo store mode")
		demo := public.NewDemoStore()
		tenantLookup = demo
		publicStore = demo
		platformStore = demo
		setupStore = demo
		paymentConfigProvider = demo
		authStore = auth.NewDemoStore()
		demoAdm := admissions.NewSeededDemoStore()
		demoAdm.SetCatalogProvider(demo)
		admissionsStore = demoAdm
	}

	if cfg.Mode == "worker" {
		<-ctx.Done()
		shutdownRiver(riverClient, logger)
		return
	}

	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	var otpLimiter auth.RateLimiter
	var idemStore admissions.IdempotencyStore
	if pool != nil {
		otpLimiter = auth.NewPGOTPRateLimiter(pool, time.Minute, 15*time.Minute, 5)
		idemStore = admissions.NewPGIdempotencyCache(pool, 24*time.Hour)
	} else {
		otpLimiter = auth.NewOTPRateLimiter(time.Minute, 15*time.Minute, 5)
		idemStore = admissions.NewIdempotencyCache(24 * time.Hour)
	}

	authService := auth.NewService(
		authStore,
		jwtManager,
		otpLimiter,
		cfg.OTPSecret,
		cfg.RefreshTokenTTL,
		cfg.AppEnv != "production",
		logger,
	)
	admissionsService := admissions.NewService(admissionsStore, objectStore, idemStore)
	admissionsService.SetPaymentGateway(paymentGateway)
	admissionsService.SetPaymentConfigProvider(paymentConfigProvider)
	admissionsService.SetDocumentSecret(cfg.DocumentSecret)

	frontendFS, err := web.DistFS()
	if err != nil {
		logger.Error("failed to load embedded frontend assets", "error", err)
		os.Exit(1)
	}

	router := httpapi.NewRouter(httpapi.Dependencies{
		Config:            cfg,
		Pool:              pool,
		TenantLookup:      tenantLookup,
		PublicStore:       publicStore,
		PlatformStore:     platformStore,
		SetupStore:        setupStore,
		ObjectStorage:     objectStore,
		DNSResolver:       handlers.NetDNSResolver{},
		AuthService:       authService,
		AdmissionsService: admissionsService,
		JWTManager:        jwtManager,
		FrontendFS:        frontendFS,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("http server listening", "port", cfg.Port, "mode", cfg.Mode, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down gracefully")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", "error", err)
	}
	shutdownRiver(riverClient, logger)
}

func shutdownRiver(client *river.Client[pgx.Tx], logger *slog.Logger) {
	if client == nil {
		return
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Stop(stopCtx); err != nil {
		logger.Error("river worker stop error", "error", err)
	}
}

