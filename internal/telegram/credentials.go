package telegram

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type storedCreds struct {
	APIID   int    `json:"api_id"`
	APIHash string `json:"api_hash"`
}

func (c *Client) credsPath() string {
	return filepath.Join(c.dataDir, "telegram_api.json")
}

func (c *Client) loadStoredCreds() (int, string, bool) {
	b, err := os.ReadFile(c.credsPath())
	if err != nil {
		return 0, "", false
	}
	var s storedCreds
	if err := json.Unmarshal(b, &s); err != nil {
		return 0, "", false
	}
	if s.APIID == 0 || strings.TrimSpace(s.APIHash) == "" {
		return 0, "", false
	}
	return s.APIID, strings.TrimSpace(s.APIHash), true
}

func (c *Client) saveCreds(apiID int, apiHash string) error {
	if apiID <= 0 || strings.TrimSpace(apiHash) == "" {
		return fmt.Errorf("api_id and api_hash are required")
	}
	b, err := json.MarshalIndent(storedCreds{APIID: apiID, APIHash: strings.TrimSpace(apiHash)}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.credsPath(), append(b, '\n'), 0o600)
}

func (c *Client) IsConfigured() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.apiID > 0 && c.apiHash != ""
}
