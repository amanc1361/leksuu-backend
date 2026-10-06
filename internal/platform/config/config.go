// Package config loads runtime configuration from the environment (12-factor).
// All defaults live here, in one place.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env                string
	HTTPPort           string
	DatabaseURL        string
	RedisURL           string
	JWTSecret          string
	AIProvider         string
	PronProvider       string
	PronURL            string
	PronAPIKey         string
	AIBaseURL          string
	AIModel            string
	AIAPIKey           string
	AIDisableThinking  bool
	PaymentProvider    string
	ZarinpalMerchantID string
	FreeDailyLimit     int
	GemUnlockCost      int
	PriceMonthlyToman  int
	PriceYearlyToman   int
	BillingCallbackURL string
	BillingSuccessURL  string
	BillingFailURL     string
	CORSOrigins        []string
	AdminEmails        []string
	AdminPassword      string
	AccessTTL          time.Duration
	RefreshTTL         time.Duration
	// media / object storage
	MediaDir               string
	RecordingsDir          string
	RecordingRetentionDays int
	MediaPublicURL         string
	MediaMaxBytes          int64
	S3Endpoint             string
	S3Region               string
	S3Bucket               string
	S3Prefix               string
	S3AccessKey            string
	S3SecretKey            string
	S3Secure               bool
	S3PublicRead           bool
	// live classes (WebRTC signaling + ICE)
	StunURL    string
	TurnURL    string
	TurnTLSURL string
	TurnSecret string
	TurnTTL    int
	// class reminders
	ReminderLeadMin     int
	ReminderIntervalSec int
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() Config {
	return Config{
		Env:                    env("APP_ENV", "development"),
		HTTPPort:               env("HTTP_PORT", "8080"),
		DatabaseURL:            env("DATABASE_URL", "postgres://postgres@localhost:5432/leksuu?sslmode=disable"),
		RedisURL:               env("REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:              env("JWT_SECRET", "dev-only-change-me"),
		AIProvider:             env("AI_PROVIDER", "stub"),
		PronProvider:           env("PRON_PROVIDER", "stub"),
		PronURL:                env("PRON_URL", ""),
		PronAPIKey:             env("PRON_API_KEY", ""),
		AIBaseURL:              env("AI_BASE_URL", ""),
		AIModel:                env("AI_MODEL", ""),
		AIAPIKey:               env("AI_API_KEY", ""),
		AIDisableThinking:      env("AI_DISABLE_THINKING", "true") != "false",
		PaymentProvider:        env("PAYMENT_PROVIDER", "stub"),
		ZarinpalMerchantID:     env("ZARINPAL_MERCHANT_ID", ""),
		FreeDailyLimit:         atoiDefault(env("FREE_DAILY_LIMIT", "2")),
		GemUnlockCost:          atoiDefault(env("GEM_UNLOCK_COST", "50")),
		PriceMonthlyToman:      atoiDefault(env("PRICE_MONTHLY_TOMAN", "99000")),
		PriceYearlyToman:       atoiDefault(env("PRICE_YEARLY_TOMAN", "890000")),
		BillingCallbackURL:     env("BILLING_CALLBACK_URL", "http://localhost:8080/api/v1/billing/callback"),
		BillingSuccessURL:      env("BILLING_SUCCESS_URL", "http://localhost:3000/fa/billing/success"),
		BillingFailURL:         env("BILLING_FAIL_URL", "http://localhost:3000/fa/billing/fail"),
		CORSOrigins:            splitList(env("CORS_ORIGIN", "http://localhost:3000,https://leksuu.ir,https://www.leksuu.ir")),
		AdminEmails:            splitList(env("ADMIN_EMAILS", "admin@leksuu.app")),
		AdminPassword:          env("ADMIN_PASSWORD", "leksuu-admin"),
		AccessTTL:              15 * time.Minute,
		RefreshTTL:             30 * 24 * time.Hour,
		MediaDir:               env("MEDIA_DIR", "./media-data"),
		RecordingsDir:          env("RECORDINGS_DIR", "./recordings-tmp"),
		RecordingRetentionDays: atoiDefault(env("RECORDING_RETENTION_DAYS", "0")),
		MediaPublicURL:         env("MEDIA_PUBLIC_URL", "http://localhost:8080"),
		MediaMaxBytes:          int64(atoiDefault(env("MEDIA_MAX_MB", "50"))) * 1024 * 1024,
		S3Endpoint:             env("S3_ENDPOINT", ""),
		S3Region:               env("S3_REGION", "us-east-1"),
		S3Bucket:               env("S3_BUCKET", ""),
		S3Prefix:               env("S3_PREFIX", ""),
		S3AccessKey:            env("S3_ACCESS_KEY", ""),
		S3SecretKey:            env("S3_SECRET_KEY", ""),
		S3Secure:               env("S3_SECURE", "true") != "false",
		// Default false: a bucket is private unless its owner deliberately opened
		// it, and guessing otherwise would hand every object to anyone with a URL.
		S3PublicRead: env("S3_PUBLIC_READ", "false") == "true",
		// Without at least one STUN server the browser only gathers host
		// candidates, so media never crosses two different networks — the room
		// connects but every remote tile stays black. Ship reachable defaults.
		StunURL:             env("STUN_URL", "stun:stun.cloudflare.com:3478,stun:stun.l.google.com:19302"),
		TurnURL:             env("TURN_URL", ""),
		TurnTLSURL:          env("TURN_TLS_URL", ""),
		TurnSecret:          env("TURN_SECRET", ""),
		TurnTTL:             atoiDefault(env("TURN_TTL_SECONDS", "86400")),
		ReminderLeadMin:     atoiDefault(env("REMINDER_LEAD_MIN", "15")),
		ReminderIntervalSec: atoiDefault(env("REMINDER_INTERVAL_SEC", "60")),
	}
}

// splitList parses a comma-separated env value into a trimmed, lowercased list.
func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(strings.ToLower(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func atoiDefault(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
