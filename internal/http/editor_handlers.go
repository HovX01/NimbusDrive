package httpserver

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/vrc/nimbus/internal/domain"
)

func (s *Server) createEditProject(w http.ResponseWriter, r *http.Request) {
	project, err := s.svc.CreateEditProject(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"project": project})
}

func (s *Server) listEditProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.svc.ListEditProjects(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if projects == nil {
		projects = []domain.EditProject{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *Server) getEditProject(w http.ResponseWriter, r *http.Request) {
	project, err := s.svc.GetEditProject(r.Context(), chi.URLParam(r, "projectID"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": project})
}

func (s *Server) saveEditTimeline(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TimelineJSON string `json:"timeline_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	if err := s.svc.SaveEditTimeline(r.Context(), chi.URLParam(r, "projectID"), body.TimelineJSON); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) deleteEditProject(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.DeleteEditProject(r.Context(), chi.URLParam(r, "projectID")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) startEditExport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Preset string `json:"preset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	jobID, err := s.svc.StartEditExport(r.Context(), chi.URLParam(r, "projectID"), body.Preset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

func (s *Server) editExportStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.svc.GetEditExportStatus(chi.URLParam(r, "jobID"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) probeFile(w http.ResponseWriter, r *http.Request) {
	probe, err := s.svc.ProbeFile(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, probe)
}

func (s *Server) proxyStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.svc.ProxyStatus(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) serveEditorProxy(w http.ResponseWriter, r *http.Request) {
	path, err := s.svc.EditorProxyPath(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", "inline; filename=\""+url.PathEscape(filepath.Base(path))+"\"")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, path)
}

func (s *Server) getKeyframes(w http.ResponseWriter, r *http.Request) {
	start, _ := strconv.ParseFloat(r.URL.Query().Get("start"), 64)
	end, _ := strconv.ParseFloat(r.URL.Query().Get("end"), 64)
	items, err := s.svc.ProbeKeyframes(r.Context(), chi.URLParam(r, "id"), start, end)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keyframes": items})
}

func (s *Server) importCaptions(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	defer file.Close()
	captions, err := s.svc.ImportProjectCaptions(r.Context(), chi.URLParam(r, "projectID"), file)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"captions": captions})
}

func (s *Server) exportCaptions(w http.ResponseWriter, r *http.Request) {
	srt, err := s.svc.ExportProjectCaptions(r.Context(), chi.URLParam(r, "projectID"))
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-subrip; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"captions.srt\"")
	_, _ = w.Write([]byte(srt))
}
