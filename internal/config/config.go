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
	Port            string
	DataDir         string
	DatabaseURL     string
	JWTSecret       string
	AccessSecret    string
	Public          bool
	JWTTTLHours     int
	TelegramAPIID   int
	TelegramAPIHash string
	ChunkSize       int
	CORSOrigins     []string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		Port:            getenv("PORT", "8080"),
		DataDir:         getenv("DATA_DIR", "./data"),
		DatabaseURL:     getenv("DATABASE_URL", "sqlite://./data/nimbus.db"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		AccessSecret:    strings.TrimSpace(os.Getenv("NIMBUS_ACCESS_SECRET")),
		Public:          getenvBool("NIMBUS_PUBLIC", false),
		JWTTTLHours:     getenvInt("JWT_TTL_HOURS", 72),
		TelegramAPIHash: strings.TrimSpace(os.Getenv("TELEGRAM_API_HASH")),
		ChunkSize:       getenvInt("CHUNK_SIZE", 8*1024*1024),
		CORSOrigins:     splitCSV(getenv("CORS_ORIGINS", "http://localhost:5173")),
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

	if cfg.ChunkSize < 64*1024 {
		return Config{}, fmt.Errorf("CHUNK_SIZE must be at least 64KiB")
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
