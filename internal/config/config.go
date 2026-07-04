package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type AggregationTarget struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	BaseURL         string `json:"baseUrl"`
	Token           string `json:"token"`
	ConnectionMode  string `json:"connectionMode,omitempty"`
	ReverseUsername string `json:"reverseUsername,omitempty"`
	ReversePassword string `json:"reversePassword,omitempty"`
}

type Config struct {
	Environment         string
	HTTPAddr            string
	DatabaseDSN         string
	MigrationsDir       string
	StaticDir           string
	AutoMigrate         bool
	NewAPIBaseURL       string
	NewAPIAdminToken    string
	NewAPIAdminUserID   int
	NewAPIQuotaPerUSD   float64
	AggregationTargets  []AggregationTarget
	EncryptionKey       string
	AuthEnabled         bool
	RegistrationEnabled bool
	BootstrapAdmin      string
	BootstrapPassword   string
	SessionTTL          time.Duration
	CookieSecure        bool
	WorkerEnabled       bool
	HealthCheckEvery    time.Duration
	UsageSyncEvery      time.Duration
}

func Load() Config {
	newAPIBaseURL := getEnv("KEYHUB_NEWAPI_BASE_URL", "http://new-api:3000")
	newAPIAdminToken := os.Getenv("KEYHUB_NEWAPI_ADMIN_TOKEN")
	return Config{
		Environment:         getEnv("KEYHUB_ENV", "development"),
		HTTPAddr:            getEnv("KEYHUB_HTTP_ADDR", ":8080"),
		DatabaseDSN:         getEnv("KEYHUB_DATABASE_DSN", "keyhub:keyhub@tcp(127.0.0.1:3306)/keyhub?parseTime=true&loc=Local&charset=utf8mb4,utf8"),
		MigrationsDir:       getEnv("KEYHUB_MIGRATIONS_DIR", "migrations"),
		StaticDir:           getEnv("KEYHUB_STATIC_DIR", "web/dist"),
		AutoMigrate:         getBoolEnv("KEYHUB_AUTO_MIGRATE", true),
		NewAPIBaseURL:       newAPIBaseURL,
		NewAPIAdminToken:    newAPIAdminToken,
		NewAPIAdminUserID:   getIntEnv("KEYHUB_NEWAPI_ADMIN_USER_ID", 0),
		NewAPIQuotaPerUSD:   getFloatEnv("KEYHUB_NEWAPI_QUOTA_PER_USD", 500000),
		AggregationTargets:  parseAggregationTargets(os.Getenv("KEYHUB_AGGREGATION_TARGETS_JSON"), newAPIBaseURL, newAPIAdminToken),
		EncryptionKey:       os.Getenv("KEYHUB_ENCRYPTION_KEY"),
		AuthEnabled:         getBoolEnv("KEYHUB_AUTH_ENABLED", true),
		RegistrationEnabled: getBoolEnv("KEYHUB_REGISTRATION_ENABLED", true),
		BootstrapAdmin:      getEnv("KEYHUB_BOOTSTRAP_ADMIN_USER", "admin"),
		BootstrapPassword:   os.Getenv("KEYHUB_BOOTSTRAP_ADMIN_PASSWORD"),
		SessionTTL:          getDurationEnv("KEYHUB_SESSION_TTL", 24*time.Hour),
		CookieSecure:        getBoolEnv("KEYHUB_COOKIE_SECURE", false),
		WorkerEnabled:       getBoolEnv("KEYHUB_WORKER_ENABLED", false),
		HealthCheckEvery:    getDurationEnv("KEYHUB_HEALTH_CHECK_INTERVAL", 30*time.Minute),
		UsageSyncEvery:      getDurationEnv("KEYHUB_USAGE_SYNC_INTERVAL", 15*time.Minute),
	}
}

func (c Config) IsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(c.Environment), "production")
}

func (c Config) Validate() error {
	if c.SessionTTL <= 0 {
		return fmt.Errorf("KEYHUB_SESSION_TTL must be greater than 0")
	}
	if !c.IsProduction() {
		return nil
	}
	if strings.TrimSpace(c.EncryptionKey) == "" || strings.TrimSpace(c.EncryptionKey) == "replace-with-32-byte-base64-key" {
		return fmt.Errorf("KEYHUB_ENCRYPTION_KEY must be set to a real secret in production")
	}
	if !c.AuthEnabled {
		return fmt.Errorf("KEYHUB_AUTH_ENABLED must be true in production")
	}
	if strings.TrimSpace(c.BootstrapPassword) == "change-me-now" {
		return fmt.Errorf("KEYHUB_BOOTSTRAP_ADMIN_PASSWORD must not use the example value in production")
	}
	if strings.TrimSpace(c.NewAPIAdminToken) == "" && len(c.AggregationTargets) == 0 {
		return fmt.Errorf("KEYHUB_AGGREGATION_TARGETS_JSON or KEYHUB_NEWAPI_ADMIN_TOKEN is required in production")
	}
	if strings.TrimSpace(c.NewAPIAdminToken) != "" && c.NewAPIAdminUserID <= 0 {
		return fmt.Errorf("KEYHUB_NEWAPI_ADMIN_USER_ID is required in production")
	}
	return nil
}

func parseAggregationTargets(raw string, fallbackBaseURL string, fallbackToken string) []AggregationTarget {
	raw = strings.TrimSpace(raw)
	targets := make([]AggregationTarget, 0)
	if raw != "" {
		var parsed []AggregationTarget
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			targets = append(targets, parsed...)
		}
	}
	if len(targets) == 0 && strings.TrimSpace(fallbackBaseURL) != "" && strings.TrimSpace(fallbackToken) != "" {
		targets = append(targets, AggregationTarget{
			Code:    "default",
			Name:    "Default new-api",
			BaseURL: fallbackBaseURL,
			Token:   fallbackToken,
		})
	}

	cleaned := make([]AggregationTarget, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		target.Code = strings.TrimSpace(target.Code)
		target.Name = strings.TrimSpace(target.Name)
		target.BaseURL = strings.TrimRight(strings.TrimSpace(target.BaseURL), "/")
		target.Token = strings.TrimSpace(target.Token)
		target.ConnectionMode = strings.TrimSpace(target.ConnectionMode)
		target.ReverseUsername = strings.TrimSpace(target.ReverseUsername)
		target.ReversePassword = strings.TrimSpace(target.ReversePassword)
		if target.Code == "" || target.BaseURL == "" || target.Token == "" {
			continue
		}
		if target.Name == "" {
			target.Name = target.Code
		}
		if _, ok := seen[target.Code]; ok {
			continue
		}
		seen[target.Code] = struct{}{}
		cleaned = append(cleaned, target)
	}
	return cleaned
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getBoolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getIntEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getFloatEnv(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err == nil {
		return parsed
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}
