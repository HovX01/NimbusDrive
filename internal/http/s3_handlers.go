package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/vrc/nimbus/internal/domain"
	"github.com/vrc/nimbus/internal/s3gw"
)

// s3Endpoint builds the gateway URL clients should point at, by swapping the API
// port for the gateway port on the request host.
func (s *Server) s3Endpoint(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if s.s3Port != "" {
		if h, _, ok := strings.Cut(host, ":"); ok {
			host = h
		}
		host = host + ":" + s.s3Port
	}
	return scheme + "://" + host
}

func (s *Server) getS3Settings(w http.ResponseWriter, r *http.Request) {
	cfg, err := s3gw.Load(s.svc.DataDir, false, "us-east-1")
	if err != nil {
		writeErr(w, err)
		return
	}
	endpoint := s.s3Endpoint(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":    cfg.Enabled,
		"region":     cfg.Region,
		"access_key": cfg.AccessKey,
		"secret_key": cfg.SecretKey,
		"endpoint":   endpoint,
		"rclone": fmt.Sprintf(
			"[nimbus]\ntype = s3\nprovider = Other\nenv_auth = false\naccess_key_id = %s\nsecret_access_key = %s\nregion = %s\nendpoint = %s\npath_style_address = true",
			cfg.AccessKey, cfg.SecretKey, cfg.Region, endpoint,
		),
	})
}

func (s *Server) saveS3Settings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Region     string `json:"region"`
		Regenerate bool   `json:"regenerate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	cfg, err := s3gw.Load(s.svc.DataDir, false, "us-east-1")
	if err != nil {
		writeErr(w, err)
		return
	}
	if region := strings.TrimSpace(body.Region); region != "" {
		cfg.Region = region
	}
	if body.Regenerate {
		cfg, err = s3gw.Rotate(s.svc.DataDir, cfg)
		if err != nil {
			writeErr(w, err)
			return
		}
	} else if err := s3gw.Save(s.svc.DataDir, cfg); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":    cfg.Enabled,
		"region":     cfg.Region,
		"access_key": cfg.AccessKey,
		"secret_key": cfg.SecretKey,
		"endpoint":   s.s3Endpoint(r),
	})
}
