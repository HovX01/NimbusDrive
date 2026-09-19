// Package s3gw exposes NimbusDrive storage as an S3-compatible endpoint.
// S3 buckets map to top-level drive folders; object keys map to nested folders + file names.
package s3gw

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the gateway keypair and region, persisted in <dataDir>/s3gw.json (mode 0600).
type Config struct {
	Enabled   bool   `json:"enabled"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Region    string `json:"region"`
}

func configPath(dataDir string) string { return filepath.Join(dataDir, "s3gw.json") }

// Load reads the gateway config. A keypair is generated on first use so the
// Settings UI can display credentials before the gateway is switched on.
// enabledSeed seeds Enabled when the file does not exist yet.
func Load(dataDir string, enabledSeed bool, region string) (Config, error) {
	b, err := os.ReadFile(configPath(dataDir))
	if err != nil {
		if !os.IsNotExist(err) {
			return Config{}, fmt.Errorf("read s3 gateway config: %w", err)
		}
		cfg := Config{Enabled: enabledSeed, Region: region}
		if err := persist(dataDir, cfg); err != nil {
			return Config{}, err
		}
		return Load(dataDir, enabledSeed, region)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse s3 gateway config: %w", err)
	}
	if cfg.Region == "" {
		cfg.Region = region
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		ak, sk, err := newKeypair()
		if err != nil {
			return Config{}, err
		}
		cfg.AccessKey, cfg.SecretKey = ak, sk
		if err := persist(dataDir, cfg); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

// Save updates the persisted region and Enabled flag, keeping the current keypair.
func Save(dataDir string, cfg Config) error {
	stored, err := Load(dataDir, false, cfg.Region)
	if err != nil {
		return err
	}
	stored.Enabled, stored.Region = cfg.Enabled, cfg.Region
	return persist(dataDir, stored)
}

// Rotate replaces the keypair with a fresh one. Existing clients must be reconfigured.
func Rotate(dataDir string, cfg Config) (Config, error) {
	ak, sk, err := newKeypair()
	if err != nil {
		return Config{}, err
	}
	cfg.AccessKey, cfg.SecretKey = ak, sk
	return cfg, persist(dataDir, cfg)
}

func persist(dataDir string, cfg Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(dataDir), append(b, '\n'), 0o600)
}

func newKeypair() (accessKey, secretKey string, err error) {
	ak := make([]byte, 10)
	sk := make([]byte, 32)
	if _, err := rand.Read(ak); err != nil {
		return "", "", err
	}
	if _, err := rand.Read(sk); err != nil {
		return "", "", err
	}
	return "nimbus" + hex.EncodeToString(ak), hex.EncodeToString(sk), nil
}
