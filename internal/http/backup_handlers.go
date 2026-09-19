package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vrc/nimbus/internal/backup"
	"github.com/vrc/nimbus/internal/domain"
)

func (s *Server) getBackupSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := backup.LoadConfig(s.svc.DataDir)
	if err != nil {
		writeErr(w, err)
		return
	}
	resp := map[string]any{
		"enabled":                cfg.Enabled,
		"endpoint":               cfg.Endpoint,
		"region":                 cfg.Region,
		"bucket":                 cfg.Bucket,
		"prefix":                 cfg.Prefix,
		"use_ssl":                cfg.UseSSL,
		"path_style":             cfg.PathStyle,
		"credentials_configured": cfg.AccessKeyID != "",
	}
	if cfg.AccessKeyID != "" && len(cfg.AccessKeyID) > 4 {
		resp["access_key_id_hint"] = cfg.AccessKeyID[:4] + "****"
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) saveBackupSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled      bool   `json:"enabled"`
		Endpoint     string `json:"endpoint"`
		Region       string `json:"region"`
		Bucket       string `json:"bucket"`
		Prefix       string `json:"prefix"`
		AccessKeyID  string `json:"access_key_id"`
		SecretKey    string `json:"secret_key"`
		SessionToken string `json:"session_token"`
		UseSSL       *bool  `json:"use_ssl"`
		PathStyle    *bool  `json:"path_style"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	cfg, err := backup.LoadConfig(s.svc.DataDir)
	if err != nil {
		writeErr(w, err)
		return
	}
	cfg.Enabled = body.Enabled
	cfg.Endpoint = body.Endpoint
	cfg.Bucket = body.Bucket
	cfg.Prefix = body.Prefix
	cfg.AccessKeyID = body.AccessKeyID
	if body.Region != "" {
		cfg.Region = body.Region
	}
	if body.SecretKey != "" {
		cfg.SecretKey = body.SecretKey
	}
	if body.SessionToken != "" {
		cfg.SessionToken = body.SessionToken
	}
	if body.UseSSL != nil {
		cfg.UseSSL = *body.UseSSL
	}
	if body.PathStyle != nil {
		cfg.PathStyle = *body.PathStyle
	}
	if err := backup.SaveConfig(s.svc.DataDir, cfg); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) testBackupSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := backup.LoadConfig(s.svc.DataDir)
	if err != nil || !cfg.Enabled {
		writeErr(w, fmt.Errorf("%w: backup not configured", domain.ErrNotConfigured))
		return
	}
	if err := backup.TestConnection(r.Context(), cfg); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "connection successful"})
}

func (s *Server) startBackup(w http.ResponseWriter, r *http.Request) {
	if s.svc.Backup == nil {
		writeErr(w, domain.ErrNotConfigured)
		return
	}
	var req backup.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	id, err := s.svc.Backup.Start(r.Context(), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": id, "status": "queued"})
}

func (s *Server) backupStatus(w http.ResponseWriter, r *http.Request) {
	if s.svc.Backup == nil {
		writeErr(w, domain.ErrNotConfigured)
		return
	}
	st, ok := s.svc.Backup.Status(chi.URLParam(r, "jobID"))
	if !ok {
		writeErr(w, domain.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, st)
}
