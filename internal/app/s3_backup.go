package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3BackupConfig struct {
	Endpoint       string `json:"endpoint"`
	Bucket         string `json:"bucket"`
	Region         string `json:"region"`
	Prefix         string `json:"prefix"`
	AccessKeyID    string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SSL            bool   `json:"ssl"`
	PathStyle      bool   `json:"path_style"`
	Enabled        bool   `json:"enabled"`
}

type S3BackupSettings struct {
	S3BackupConfig
	SecretKeySet bool `json:"secret_key_set"`
}

var s3BackupMu sync.Mutex

func (s *Services) GetS3BackupConfig() (S3BackupSettings, error) {
	cfg, err := s.loadS3BackupConfig()
	if err != nil { return S3BackupSettings{}, err }
	return redactS3BackupConfig(cfg), nil
}

func (s *Services) SaveS3BackupConfig(input S3BackupConfig) (S3BackupSettings, error) {
	s3BackupMu.Lock()
	defer s3BackupMu.Unlock()
	old, err := s.loadS3BackupConfigLocked()
	if err != nil { return S3BackupSettings{}, err }
	if strings.TrimSpace(input.SecretAccessKey) == "" { input.SecretAccessKey = old.SecretAccessKey }
	if err := validateS3BackupConfig(input); err != nil { return S3BackupSettings{}, err }
	path := filepath.Join(s.DataDir, "s3-backup.json")
	data, err := json.MarshalIndent(input, "", "  ")
	if err != nil { return S3BackupSettings{}, err }
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil { return S3BackupSettings{}, fmt.Errorf("save S3 backup config: %w", err) }
	_ = os.Chmod(path, 0o600)
	return redactS3BackupConfig(input), nil
}

func (s *Services) TestS3Backup(ctx context.Context) error {
	cfg, err := s.loadS3BackupConfig()
	if err != nil { return err }
	if err := validateS3BackupConfig(cfg); err != nil { return err }
	loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(cfg.Region), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""))}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil { return fmt.Errorf("configure S3 client: %w", err) }
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.PathStyle
		o.BaseEndpoint = aws.String(cfg.Endpoint)
	})
	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(cfg.Bucket)})
	if err != nil { return fmt.Errorf("S3 connectivity test failed: %w", err) }
	return nil
}

func (s *Services) loadS3BackupConfig() (S3BackupConfig, error) {
	s3BackupMu.Lock()
	defer s3BackupMu.Unlock()
	return s.loadS3BackupConfigLocked()
}

func (s *Services) loadS3BackupConfigLocked() (S3BackupConfig, error) {
	path := filepath.Join(s.DataDir, "s3-backup.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) { return S3BackupConfig{SSL: true}, nil }
	if err != nil { return S3BackupConfig{}, fmt.Errorf("read S3 backup config: %w", err) }
	var cfg S3BackupConfig
	if err := json.Unmarshal(data, &cfg); err != nil { return S3BackupConfig{}, fmt.Errorf("parse S3 backup config: %w", err) }
	return cfg, nil
}

func redactS3BackupConfig(cfg S3BackupConfig) S3BackupSettings {
	return S3BackupSettings{S3BackupConfig: S3BackupConfig{Endpoint: cfg.Endpoint, Bucket: cfg.Bucket, Region: cfg.Region, Prefix: cfg.Prefix, AccessKeyID: cfg.AccessKeyID, SSL: cfg.SSL, PathStyle: cfg.PathStyle, Enabled: cfg.Enabled}, SecretKeySet: cfg.SecretAccessKey != ""}
}

func validateS3BackupConfig(cfg S3BackupConfig) error {
	if !cfg.Enabled && strings.TrimSpace(cfg.Endpoint) == "" && strings.TrimSpace(cfg.Bucket) == "" { return nil }
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.Bucket) == "" || strings.TrimSpace(cfg.Region) == "" || strings.TrimSpace(cfg.AccessKeyID) == "" || strings.TrimSpace(cfg.SecretAccessKey) == "" { return fmt.Errorf("S3 endpoint, bucket, region, access key ID, and secret key are required when configured") }
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") { return fmt.Errorf("S3 endpoint must be an http or https URL") }
	if cfg.SSL && u.Scheme != "https" { return fmt.Errorf("SSL is enabled but S3 endpoint is not https") }
	if !cfg.SSL && u.Scheme != "http" { return fmt.Errorf("SSL is disabled but S3 endpoint is not http") }
	return nil
}
