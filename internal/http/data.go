package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vrc/nimbus/internal/app"
	"github.com/vrc/nimbus/internal/dataquery"
	"github.com/vrc/nimbus/internal/domain"
)

func (s *Server) listDataCollections(w http.ResponseWriter, r *http.Request) {
	items, err := s.svc.ListCollections(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if items == nil {
		items = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) queryData(w http.ResponseWriter, r *http.Request) {
	collection := chi.URLParam(r, "collection")
	q, err := dataquery.Parse(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	rows, err := s.svc.QueryData(r.Context(), collection, q)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": app.DataRowsJSON(rows)})
}

func (s *Server) insertData(w http.ResponseWriter, r *http.Request) {
	collection := chi.URLParam(r, "collection")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	row, err := s.svc.InsertData(r.Context(), collection, body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, app.DataRowJSON(row))
}

func (s *Server) getDataRow(w http.ResponseWriter, r *http.Request) {
	collection := chi.URLParam(r, "collection")
	id := chi.URLParam(r, "id")
	row, err := s.svc.GetData(r.Context(), collection, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.DataRowJSON(row))
}

func (s *Server) updateData(w http.ResponseWriter, r *http.Request) {
	collection := chi.URLParam(r, "collection")
	id := chi.URLParam(r, "id")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	row, err := s.svc.UpdateData(r.Context(), collection, id, body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.DataRowJSON(row))
}

func (s *Server) deleteData(w http.ResponseWriter, r *http.Request) {
	collection := chi.URLParam(r, "collection")
	id := chi.URLParam(r, "id")
	if err := s.svc.DeleteData(r.Context(), collection, id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
