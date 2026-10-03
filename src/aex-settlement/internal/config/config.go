package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port               string
	Environment        string
	StoreType          string
	MongoURI           string
	MongoDB            string
	NatsURL            string
	NatsStreamReplicas int
	WebhookSecret      string

	// Settlement resumer and balance op retention.
	ResumeInterval     time.Duration
	ResumeGrace        time.Duration
	PendingAlertAge    time.Duration
	BalanceOpRetention time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:               getEnv("PORT", "8080"),
		Environment:        getEnv("ENVIRONMENT", "development"),
		StoreType:          getEnv("STORE_TYPE", "mongo"),
		MongoURI:           getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:            getEnv("MONGO_DB", "aex"),
		NatsURL:            getEnv("NATS_URL", ""),
		NatsStreamReplicas: getEnvInt("NATS_STREAM_REPLICAS", 1),
		WebhookSecret:      getEnv("WEBHOOK_SECRET", ""),
	}

	durations := []struct {
		key  string
		def  time.Duration
		dest *time.Duration
	}{
		{"SETTLEMENT_RESUME_INTERVAL", 30 * time.Second, &cfg.ResumeInterval},
		{"SETTLEMENT_RESUME_GRACE", time.Minute, &cfg.ResumeGrace},
		{"SETTLEMENT_PENDING_ALERT_AGE", 15 * time.Minute, &cfg.PendingAlertAge},
		{"SETTLEMENT_OP_RETENTION", 30 * 24 * time.Hour, &cfg.BalanceOpRetention},
	}
	for _, d := range durations {
		v, err := getEnvDuration(d.key, d.def)
		if err != nil {
			return nil, err
		}
		*d.dest = v
	}

	// A stranded settlement must be picked up by the resumer well before it
	// becomes too old to replay (half the op retention), or it can only be
	// reconciled by hand.
	if replayLimit := cfg.BalanceOpRetention / 2; replayLimit <= 2*cfg.ResumeGrace+cfg.ResumeInterval ||
		replayLimit <= cfg.PendingAlertAge {
		return nil, fmt.Errorf(
			"SETTLEMENT_OP_RETENTION/2 (%s) must exceed both 2*SETTLEMENT_RESUME_GRACE+SETTLEMENT_RESUME_INTERVAL (%s) and SETTLEMENT_PENDING_ALERT_AGE (%s)",
			replayLimit, 2*cfg.ResumeGrace+cfg.ResumeInterval, cfg.PendingAlertAge)
	}

	return cfg, nil
}

// getEnvDuration parses a positive Go duration (e.g. "30s", "720h"). An
// invalid value is an error rather than a silent default: these settings
// bound how long a settlement can stay unpaid.
func getEnvDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration, got %q", key, value)
	}
	return d, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
