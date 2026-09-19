package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                string
	DataDir             string
	WebDir              string
	DatabaseURL         string
	JWTSecret           string
	AccessSecret        string
	APIKey              string
	Public              bool
	JWTTTLHours         int
	TelegramAPIID       int
	TelegramAPIHash     string
	TelegramBotAPIURL   string
	TelegramBotToken    string
	TelegramStorageChatID string
	ChunkSize           int
	CORSOrigins         []string
	PublicBaseURL       string
	TikTokClientKey     string
	TikTokSecret        string
	MetaAppID           string
	MetaAppSecret       string
	S3Enabled           bool
	S3Port              string
	S3Region            string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		Port:            getenv("PORT", "8080"),
		DataDir:         getenv("DATA_DIR", "./data"),
		WebDir:          strings.TrimSpace(os.Getenv("WEB_DIR")),
		DatabaseURL:     getenv("DATABASE_URL", "sqlite://./data/nimbus.db"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		AccessSecret:    strings.TrimSpace(os.Getenv("NIMBUS_ACCESS_SECRET")),
		APIKey:          strings.TrimSpace(os.Getenv("NIMBUS_API_KEY")),
		Public:          getenvBool("NIMBUS_PUBLIC", false),
		JWTTTLHours:     getenvInt("JWT_TTL_HOURS", 72),
		TelegramAPIHash:     strings.TrimSpace(os.Getenv("TELEGRAM_API_HASH")),
		TelegramBotAPIURL:   strings.TrimRight(strings.TrimSpace(os.Getenv("TELEGRAM_BOT_API_URL")), "/"),
		TelegramBotToken:    strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramStorageChatID: strings.TrimSpace(os.Getenv("TELEGRAM_STORAGE_CHAT_ID")),
		ChunkSize:           getenvInt("CHUNK_SIZE", 8*1024*1024),
		CORSOrigins:     splitCSV(getenv("CORS_ORIGINS", "http://localhost:5173")),
		PublicBaseURL:   strings.TrimRight(strings.TrimSpace(os.Getenv("NIMBUS_PUBLIC_BASE_URL")), "/"),
		TikTokClientKey: strings.TrimSpace(os.Getenv("TIKTOK_CLIENT_KEY")),
		TikTokSecret:    strings.TrimSpace(os.Getenv("TIKTOK_CLIENT_SECRET")),
		MetaAppID:       strings.TrimSpace(os.Getenv("META_APP_ID")),
		MetaAppSecret:   strings.TrimSpace(os.Getenv("META_APP_SECRET")),
		S3Enabled:       getenvBool("NIMBUS_S3_ENABLED", false),
		S3Port:          getenv("NIMBUS_S3_PORT", "9091"),
		S3Region:        strings.TrimSpace(os.Getenv("NIMBUS_S3_REGION")),
	}

	if id := strings.TrimSpace(os.Getenv("TELEGRAM_API_ID")); id != "" {
		n, err := strconv.Atoi(id)
		if err != nil {
			return Config{}, fmt.Errorf("TELEGRAM_API_ID: %w", err)
		}
		cfg.TelegramAPIID = n
	}

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return Config{}, fmt.Errorf("DATA_DIR: %w", err)
	}

	// Auto-create JWT secret (Telegram-Drive style: no manual .env dance required).
	if cfg.JWTSecret == "" || cfg.JWTSecret == "change-me-to-a-long-random-string" {
		secret, err := loadOrCreateSecretFile(cfg.DataDir, "jwt.secret")
		if err != nil {
			return Config{}, err
		}
		cfg.JWTSecret = secret
	}

	// Access secret gates /auth/resume and all API routes (prevents open resume).
	if cfg.AccessSecret == "" {
		secret, err := loadOrCreateSecretFile(cfg.DataDir, "access.secret")
		if err != nil {
			return Config{}, err
		}
		cfg.AccessSecret = secret
	}

	// API key for programmatic upload/import/download (Cloudinary-style dev API).
	if cfg.APIKey == "" {
		secret, err := loadOrCreateSecretFile(cfg.DataDir, "api.key")
		if err != nil {
			return Config{}, err
		}
		cfg.APIKey = secret
	}

		if cfg.ChunkSize < 64*1024 {
		return Config{}, fmt.Errorf("CHUNK_SIZE must be at least 64KiB")
	}

	if cfg.TelegramBotAPIURL != "" {
		if botChunk := getenvInt("TELEGRAM_BOT_CHUNK_SIZE", 0); botChunk > 0 {
			maxBotChunk := 800 * 1024 * 1024
			if botChunk > maxBotChunk {
				botChunk = maxBotChunk
			}
			if botChunk >= 64*1024 {
				cfg.ChunkSize = botChunk
			}
		}
	}

	if cfg.S3Region == "" {
		cfg.S3Region = "us-east-1"
	}

	return cfg, nil
}

func loadOrCreateSecretFile(dataDir, name string) (string, error) {
	path := filepath.Join(dataDir, name)
	if b, err := os.ReadFile(path); err == nil {
		s := strings.TrimSpace(string(b))
		if s != "" {
			return s, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	s := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(s+"\n"), 0o600); err != nil {
		return "", err
	}
	return s, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getenvBool(k string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
