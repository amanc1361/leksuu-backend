// Package config loads runtime configuration from the environment (12-factor).
// All defaults live here, in one place.
package config

import (
	"errors"
	"fmt"
	"net/url"
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
	CoverImageAPIKey   string
	CoverImageModel    string
	LocalizationURL    string
	LocalizationToken  string
	AIDisableThinking  bool
	PaymentProvider    string
	ZarinpalMerchantID string
	ZarinpalSandbox    bool
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
	MediaCDNBaseURL        string
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
		CoverImageAPIKey:       env("COVER_IMAGE_API_KEY", ""),
		CoverImageModel:        env("COVER_IMAGE_MODEL", "gpt-image-2.5-flare"),
		LocalizationURL:        env("LOCALIZATION_ADMIN_URL", ""),
		LocalizationToken:      env("LOCALIZATION_ADMIN_TOKEN", ""),
		AIDisableThinking:      env("AI_DISABLE_THINKING", "true") != "false",
		PaymentProvider:        env("PAYMENT_PROVIDER", "stub"),
		ZarinpalMerchantID:     env("ZARINPAL_MERCHANT_ID", ""),
		ZarinpalSandbox:        env("ZARINPAL_SANDBOX", "") == "true",
		FreeDailyLimit:         atoiDefault(env("FREE_DAILY_LIMIT", "2")),
		GemUnlockCost:          atoiDefault(env("GEM_UNLOCK_COST", "50")),
		PriceMonthlyToman:      atoiDefault(env("PRICE_MONTHLY_TOMAN", "99000")),
		PriceYearlyToman:       atoiDefault(env("PRICE_YEARLY_TOMAN", "890000")),
		BillingCallbackURL:     env("BILLING_CALLBACK_URL", "http://localhost:8080/api/v1/billing/callback"),
		BillingSuccessURL:      env("BILLING_SUCCESS_URL", "http://localhost:3000/fa/billing/success"),
		BillingFailURL:         env("BILLING_FAIL_URL", "http://localhost:3000/fa/billing/fail"),
		CORSOrigins:            splitList(env("CORS_ORIGIN", "http://localhost:3000,https://leksuu.ir,https://www.leksuu.ir")),
		AdminEmails:            splitList(env("ADMIN_EMAILS", "admin@leksuu.app")),
		AdminPassword:          env("ADMIN_PASSWORD", DefaultAdminPassword),
		AccessTTL:              15 * time.Minute,
		RefreshTTL:             30 * 24 * time.Hour,
		MediaDir:               env("MEDIA_DIR", "./media-data"),
		RecordingsDir:          env("RECORDINGS_DIR", "./recordings-tmp"),
		RecordingRetentionDays: atoiDefault(env("RECORDING_RETENTION_DAYS", "0")),
		MediaPublicURL:         env("MEDIA_PUBLIC_URL", "http://localhost:8080"),
		MediaCDNBaseURL:        env("MEDIA_CDN_BASE_URL", ""),
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

// DefaultAdminPassword is the development bootstrap password. Production never
// creates an account with it (see app.New).
const DefaultAdminPassword = "leksuu-admin"

// IsProduction reports whether this process is the production deployment.
func (c Config) IsProduction() bool { return strings.EqualFold(strings.TrimSpace(c.Env), "production") }

// placeholderSecrets are example values shipped in docs and compose files.
var placeholderSecrets = map[string]bool{
	"": true, "dev-only-change-me": true, "change-me-in-prod": true, "change_me": true, "changeme": true, "change-me": true,
}

// Validate refuses a configuration that is unsafe to run in production: tokens
// signed with a published example secret could be forged by anyone. Other
// development defaults are neutralized at wiring time instead (simulated
// payments are disabled, the default admin password is never used).
func (c Config) Validate() error {
	if !c.IsProduction() {
		return nil
	}
	if c.S3PublicRead {
		return errors.New("config: S3_PUBLIC_READ must be false for licensed Film and Music delivery")
	}
	if c.MediaCDNBaseURL != "" {
		u, err := url.Parse(c.MediaCDNBaseURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("config: MEDIA_CDN_BASE_URL must be an https origin with no path or query")
		}
	}
	// A real gateway must send paying users back to this site, not localhost.
	if strings.EqualFold(c.PaymentProvider, "zarinpal") {
		for name, u := range map[string]string{
			"BILLING_CALLBACK_URL": c.BillingCallbackURL,
			"BILLING_SUCCESS_URL":  c.BillingSuccessURL,
			"BILLING_FAIL_URL":     c.BillingFailURL,
		} {
			if !strings.HasPrefix(u, "https://") || strings.Contains(u, "localhost") || strings.Contains(u, "127.0.0.1") {
				return fmt.Errorf("config: %s must be this site's public https address when PAYMENT_PROVIDER=zarinpal (got %q)", name, u)
			}
		}
		if strings.TrimSpace(c.ZarinpalMerchantID) == "" {
			return errors.New("config: ZARINPAL_MERCHANT_ID is required when PAYMENT_PROVIDER=zarinpal")
		}
	}
	if placeholderSecrets[strings.ToLower(strings.TrimSpace(c.JWTSecret))] {
		return errors.New("config: JWT_SECRET is empty or a placeholder; set a long random secret for production")
	}
	return nil
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
