package httpserver

import (
	"fmt"
	"net"
	"net/http"

	"github.com/vrc/nimbus/internal/domain"
)

func (s *Server) localTelegramBackup(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackBackupRequest(r) && !secureEqual(s.svc.BackupSecret, r.Header.Get("X-Nimbus-Backup")) {
		writeErr(w, domain.ErrUnauthorized)
		return
	}
	s.startTelegramBackup(w, r)
}

// isLoopbackBackupRequest accepts a direct localhost call from `nimbus backup`.
// Proxied requests carry forwarding headers and must present the backup secret.
func isLoopbackBackupRequest(r *http.Request) bool {
	if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Real-IP") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) startTelegramBackup(w http.ResponseWriter, r *http.Request) {
	if s.svc.DBBackup == nil {
		writeErr(w, fmt.Errorf("%w: telegram database backup is disabled", domain.ErrNotConfigured))
		return
	}
	entry, err := s.svc.DBBackup.RunOnce(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"status":     "uploaded",
		"name":       entry.Name,
		"message_id": entry.MessageID,
		"channel_id": entry.ChannelID,
		"size":       entry.Size,
		"sha256":     entry.SHA256,
		"created_at": entry.CreatedAt,
	})
}

func (s *Server) listTelegramBackups(w http.ResponseWriter, r *http.Request) {
	if s.svc.DBBackup == nil {
		writeErr(w, fmt.Errorf("%w: telegram database backup is disabled", domain.ErrNotConfigured))
		return
	}
	backups, err := s.svc.DBBackup.List()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": backups})
}
