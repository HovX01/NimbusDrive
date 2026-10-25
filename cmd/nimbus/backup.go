package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/vrc/nimbus/internal/config"
)

// runBackupCommand triggers the local backup endpoint. Coolify scheduled tasks
// can run `/app/nimbus backup` without embedding secrets in the command.
func runBackupCommand(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("NIMBUS_BACKUP_URL")), "/")
	if base == "" {
		base = "http://127.0.0.1:" + cfg.Port
	}
	body, err := requestBackup(ctx, base, cfg.AccessSecret, cfg.APIKey)
	if err != nil {
		return err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err == nil {
		fmt.Println(pretty.String())
		return nil
	}
	fmt.Println(strings.TrimSpace(string(body)))
	return nil
}

func requestBackup(ctx context.Context, base, accessSecret, apiKey string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/backups/telegram", nil)
	if err != nil {
		return nil, err
	}
	if accessSecret != "" {
		req.Header.Set("X-Nimbus-Access", accessSecret)
	}
	if apiKey != "" {
		req.Header.Set("X-Nimbus-Key", apiKey)
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backup request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read backup response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backup failed (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}
