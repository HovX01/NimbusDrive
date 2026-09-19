package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Config struct {
	Enabled      bool   `json:"enabled"`
	Endpoint     string `json:"endpoint"`
	Region       string `json:"region"`
	Bucket       string `json:"bucket"`
	Prefix       string `json:"prefix"`
	AccessKeyID  string `json:"access_key_id"`
	SecretKey    string `json:"secret_key,omitempty"`
	SessionToken string `json:"session_token,omitempty"`
	UseSSL       bool   `json:"use_ssl"`
	PathStyle    bool   `json:"path_style"`
}

func configPath(dataDir string) string {
	return filepath.Join(dataDir, "backup_s3.json")
}

func LoadConfig(dataDir string) (S3Config, error) {
	b, err := os.ReadFile(configPath(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return S3Config{Region: "us-east-1", UseSSL: true}, nil
		}
		return S3Config{}, err
	}
	var cfg S3Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return S3Config{}, err
	}
	return cfg, nil
}

func SaveConfig(dataDir string, cfg S3Config) error {
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	cfg.Prefix = strings.Trim(strings.TrimSpace(cfg.Prefix), "/")
	cfg.AccessKeyID = strings.TrimSpace(cfg.AccessKeyID)
	cfg.Region = strings.TrimSpace(cfg.Region)
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Enabled && (cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretKey == "") {
		return fmt.Errorf("endpoint, bucket, access key ID, and secret key are required when enabled")
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
		return os.WriteFile(configPath(dataDir), append(b, '\n'), 0o600)
}

func TestConnection(ctx context.Context, cfg S3Config) error {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretKey, cfg.SessionToken),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return fmt.Errorf("s3 client: %w", err)
	}
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return fmt.Errorf("s3 bucket check: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket %q does not exist", cfg.Bucket)
	}
	return nil
}
