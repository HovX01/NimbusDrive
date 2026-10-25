package httpserver

import (
	"fmt"
	"net/http"

	"github.com/vrc/nimbus/internal/domain"
)

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
