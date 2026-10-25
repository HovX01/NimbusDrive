package httpserver

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/vrc/nimbus/internal/domain"
)

const maxDatabaseImportBytes = 512 << 20

func (s *Server) importDatabase(w http.ResponseWriter, r *http.Request) {
	if s.svc.DBImport == nil {
		writeErr(w, fmt.Errorf("%w: database import is not available", domain.ErrNotConfigured))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDatabaseImportBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, fmt.Errorf("%w: upload a Nimbus .sqlite backup (max 512 MiB)", domain.ErrValidation))
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, fmt.Errorf("%w: choose a .sqlite backup to import", domain.ErrValidation))
		return
	}
	defer file.Close()

	dir := filepath.Join(s.svc.DataDir, "imports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErr(w, err)
		return
	}
	tmp, err := os.CreateTemp(dir, "import-*.sqlite")
	if err != nil {
		writeErr(w, err)
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	defer tmp.Close()

	n, err := io.Copy(tmp, io.LimitReader(file, maxDatabaseImportBytes+1))
	if err != nil {
		writeErr(w, err)
		return
	}
	if n > maxDatabaseImportBytes {
		writeErr(w, fmt.Errorf("%w: backup is larger than 512 MiB", domain.ErrValidation))
		return
	}
	if err := tmp.Close(); err != nil {
		writeErr(w, err)
		return
	}

	summary, err := s.svc.DBImport.MergeDatabase(r.Context(), tmpName)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
