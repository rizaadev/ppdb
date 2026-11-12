package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDevJWTSecret = "dev-insecure-jwt-secret-change-in-production-32bytes"
	defaultDevOTPSecret = "dev-insecure-otp-pepper"
	defaultDevWebhook   = "dev-insecure-payment-webhook-secret"
)

// Config holds runtime settings loaded from environment variables.
type Config struct {
	AppEnv               string
	Port                 string
	Mode                 string
	DatabaseURL          string
	MigrationDatabaseURL string
	BaseDomain           string
	PublicBaseURL        string
	DevFallbackSlug      string
	TrustedProxyCIDRs    []string
	DBMaxConns           int32
	DBMinConns           int32
	RiverEnabled         bool
	LogRequests          bool
	JWTSecret            string
	OTPSecret            string
	DocumentSecret       string
	AccessTokenTTL       time.Duration
	RefreshTokenTTL      time.Duration
	StorageProvider      string
	LocalStorageDir      string
	S3Endpoint           string
	S3PublicEndpoint     string
	S3Region             string
	S3Bucket             string
	S3AccessKey          string
	S3SecretKey          string
	S3UsePathStyle       bool
	PaymentWebhookSecret string
	PaymentProvider      string
	MidtransServerKey    string
	MidtransClientKey    string
	MidtransSandbox      bool
	WhatsAppProvider     string
	WhatsAppGatewayURL   string
	WhatsAppGatewayToken string
	DBAutoMigrate        bool
	DBSeedDemo           bool
	AllowDemoSeedInProd  bool
	AllowMockInProd      bool
}

// Load reads runtime configuration from environment variables with safe defaults for local development
// and strict fail-fast validation when APP_ENV=production.
func Load() (Config, error) {
	maxConns, err := parseInt32Env("DB_MAX_CONNS", 15)
	if err != nil {
		return Config{}, err
	}
	minConns, err := parseInt32Env("DB_MIN_CONNS", 2)
	if err != nil {
		return Config{}, err
	}
	accessTTL, err := parseDurationEnv("ACCESS_TOKEN_TTL", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	refreshTTL, err := parseDurationEnv("REFRESH_TOKEN_TTL", 14*24*time.Hour)
	if err != nil {
		return Config{}, err
	}

	appEnv := strings.ToLower(getEnv("APP_ENV", "development"))
	defaultStorage := "s3"
	if appEnv != "production" && strings.TrimSpace(os.Getenv("S3_ENDPOINT")) == "" {
		defaultStorage = "local"
	}

	trustedCSV := getEnv("TRUSTED_PROXY_CIDRS", "127.0.0.1/32,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16")
	trustedCIDRs, err := parseCIDRList(trustedCSV)
	if err != nil {
		return Config{}, fmt.Errorf("invalid TRUSTED_PROXY_CIDRS: %w", err)
	}

	cfg := Config{
		AppEnv:               appEnv,
		Port:                 getEnv("PORT", "8080"),
		Mode:                 strings.ToLower(getEnv("APP_MODE", "all")),
		DatabaseURL:          strings.TrimSpace(os.Getenv("DATABASE_URL")),
		MigrationDatabaseURL: strings.TrimSpace(os.Getenv("MIGRATION_DATABASE_URL")),
		BaseDomain:           strings.ToLower(getEnv("BASE_DOMAIN", "localhost")),
		PublicBaseURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/"),
		DevFallbackSlug:      strings.ToLower(getEnv("DEV_FALLBACK_TENANT_SLUG", "demo-spmb")),
		TrustedProxyCIDRs:    trustedCIDRs,
		DBMaxConns:           maxConns,
		DBMinConns:           minConns,
		RiverEnabled:         parseBoolEnv("RIVER_ENABLED", true),
		LogRequests:          parseBoolEnv("LOG_REQUESTS", true),
		JWTSecret:            strings.TrimSpace(os.Getenv("JWT_SECRET")),
		OTPSecret:            strings.TrimSpace(os.Getenv("OTP_SECRET")),
		DocumentSecret:       strings.TrimSpace(os.Getenv("DOCUMENT_SIGNING_SECRET")),
		AccessTokenTTL:       accessTTL,
		RefreshTokenTTL:      refreshTTL,
		StorageProvider:      strings.ToLower(getEnv("STORAGE_PROVIDER", defaultStorage)),
		LocalStorageDir:      getEnv("LOCAL_STORAGE_DIR", "./var/storage"),
		S3Endpoint:           strings.TrimSpace(os.Getenv("S3_ENDPOINT")),
		S3PublicEndpoint:     strings.TrimSpace(os.Getenv("S3_PUBLIC_ENDPOINT")),
		S3Region:             getEnv("S3_REGION", "auto"),
		S3Bucket:             strings.TrimSpace(os.Getenv("S3_BUCKET")),
		S3AccessKey:          strings.TrimSpace(os.Getenv("S3_ACCESS_KEY")),
		S3SecretKey:          strings.TrimSpace(os.Getenv("S3_SECRET_KEY")),
		S3UsePathStyle:       parseBoolEnv("S3_USE_PATH_STYLE", true),
		PaymentWebhookSecret: strings.TrimSpace(os.Getenv("PAYMENT_WEBHOOK_SECRET")),
		PaymentProvider:      strings.ToLower(getEnv("PAYMENT_GATEWAY_PROVIDER", defaultPaymentProvider(appEnv))),
		MidtransServerKey:    strings.TrimSpace(os.Getenv("PAYMENT_MIDTRANS_SERVER_KEY")),
		MidtransClientKey:    strings.TrimSpace(os.Getenv("PAYMENT_MIDTRANS_CLIENT_KEY")),
		MidtransSandbox:      parseBoolEnv("PAYMENT_MIDTRANS_SANDBOX", appEnv != "production"),
		WhatsAppProvider:     strings.ToLower(getEnv("WHATSAPP_GATEWAY_PROVIDER", defaultWhatsAppProvider(appEnv))),
		WhatsAppGatewayURL:   strings.TrimSpace(os.Getenv("WHATSAPP_GATEWAY_URL")),
		WhatsAppGatewayToken: strings.TrimSpace(os.Getenv("WHATSAPP_GATEWAY_TOKEN")),
		DBAutoMigrate:        parseBoolEnv("DB_AUTO_MIGRATE", true),
		DBSeedDemo:           parseBoolEnv("DB_SEED_DEMO", false),
		AllowDemoSeedInProd:  parseBoolEnv("ALLOW_DEMO_SEED_IN_PROD", false),
		AllowMockInProd:      parseBoolEnv("ALLOW_MOCK_PROVIDERS_IN_PROD", false),
	}

	switch cfg.Mode {
	case "all", "api", "worker":
	default:
		return Config{}, fmt.Errorf("invalid APP_MODE %q: must be all, api, or worker", cfg.Mode)
	}

	switch cfg.StorageProvider {
	case "s3", "local":
	default:
		return Config{}, fmt.Errorf("invalid STORAGE_PROVIDER %q: must be s3 or local", cfg.StorageProvider)
	}

	if cfg.AppEnv == "production" {
		if cfg.DatabaseURL == "" {
			return Config{}, fmt.Errorf("DATABASE_URL is required when APP_ENV=production")
		}
		if len(cfg.JWTSecret) < 32 || cfg.JWTSecret == defaultDevJWTSecret {
			return Config{}, fmt.Errorf("JWT_SECRET must be a unique production secret of at least 32 characters")
		}
		if len(cfg.OTPSecret) < 16 || cfg.OTPSecret == defaultDevOTPSecret {
			return Config{}, fmt.Errorf("OTP_SECRET must be a unique production secret of at least 16 characters")
		}
		if cfg.PaymentWebhookSecret == defaultDevWebhook {
			return Config{}, fmt.Errorf("PAYMENT_WEBHOOK_SECRET cannot use development default in production")
		}
		if cfg.DBSeedDemo && !cfg.AllowDemoSeedInProd {
			return Config{}, fmt.Errorf("DB_SEED_DEMO=true is blocked in production unless ALLOW_DEMO_SEED_IN_PROD=true")
		}
		if cfg.StorageProvider == "s3" {
			if cfg.S3Endpoint == "" || cfg.S3Bucket == "" || cfg.S3AccessKey == "" || cfg.S3SecretKey == "" {
				return Config{}, fmt.Errorf("S3_ENDPOINT, S3_BUCKET, S3_ACCESS_KEY, and S3_SECRET_KEY are required when STORAGE_PROVIDER=s3 in production")
			}
			if cfg.S3AccessKey == "minioadmin" || cfg.S3SecretKey == "minioadmin" {
				return Config{}, fmt.Errorf("default minioadmin S3 credentials must be replaced in production")
			}
		}
		if !cfg.AllowMockInProd {
			if cfg.PaymentProvider == "mock" {
				return Config{}, fmt.Errorf("PAYMENT_GATEWAY_PROVIDER=mock is disallowed in production (use midtrans_snap or manual)")
			}
			if cfg.WhatsAppProvider == "log" {
				return Config{}, fmt.Errorf("WHATSAPP_GATEWAY_PROVIDER=log is disallowed in production (use fonnte or http)")
			}
		}
	} else {
		if cfg.JWTSecret == "" {
			cfg.JWTSecret = defaultDevJWTSecret
		}
		if cfg.OTPSecret == "" {
			cfg.OTPSecret = defaultDevOTPSecret
		}
		if cfg.PaymentWebhookSecret == "" {
			cfg.PaymentWebhookSecret = defaultDevWebhook
		}
		if cfg.StorageProvider == "s3" {
			if cfg.S3Endpoint == "" {
				cfg.S3Endpoint = "http://localhost:9000"
			}
			if cfg.S3Bucket == "" {
				cfg.S3Bucket = "pbdb-private-docs"
			}
			if cfg.S3AccessKey == "" {
				cfg.S3AccessKey = "minioadmin"
			}
			if cfg.S3SecretKey == "" {
				cfg.S3SecretKey = "minioadmin"
			}
		}
	}

	if cfg.DocumentSecret == "" {
		cfg.DocumentSecret = cfg.JWTSecret
	}
	if cfg.PublicBaseURL == "" {
		if cfg.AppEnv == "production" && cfg.BaseDomain != "" && cfg.BaseDomain != "localhost" {
			cfg.PublicBaseURL = "https://" + cfg.BaseDomain
		} else {
			cfg.PublicBaseURL = "http://localhost:" + cfg.Port
		}
	}

	return cfg, nil
}

func defaultPaymentProvider(appEnv string) string {
	if appEnv == "production" {
		return "midtrans_snap"
	}
	return "mock"
}

func defaultWhatsAppProvider(appEnv string) string {
	if appEnv == "production" {
		return "fonnte"
	}
	return "log"
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parseCIDRList(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if !strings.Contains(trimmed, "/") {
			ip := net.ParseIP(trimmed)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP/CIDR %q", trimmed)
			}
			if ip.To4() != nil {
				trimmed += "/32"
			} else {
				trimmed += "/128"
			}
		} else if _, _, err := net.ParseCIDR(trimmed); err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", trimmed, err)
		}
		out = append(out, trimmed)
	}
	return out, nil
}

func parseInt32Env(key string, fallback int32) (int32, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q: %w", key, raw, err)
	}
	return int32(v), nil
}

func parseDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q: %w", key, raw, err)
	}
	return d, nil
}

func parseBoolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return b
}
