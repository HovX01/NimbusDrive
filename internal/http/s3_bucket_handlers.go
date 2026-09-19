package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vrc/nimbus/internal/domain"
	"github.com/vrc/nimbus/internal/s3gw"
)

func (s *Server) listS3Buckets(w http.ResponseWriter, r *http.Request) {
	buckets, err := s3gw.ListBuckets(r.Context(), s.svc)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": buckets})
}

func (s *Server) createS3Bucket(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	bucket, err := s3gw.CreateBucket(r.Context(), s.svc, body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, bucket)
}

func (s *Server) listS3BucketObjects(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	objects, err := s3gw.ListBucketObjects(r.Context(), s.svc, bucket)
	if err != nil {
		writeErr(w, err)
		return
	}
	if objects == nil {
		objects = []s3gw.ObjectInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bucket": bucket,
		"items":  objects,
		"upload": "rclone copy ./file.txt nimbus:" + bucket + "/",
	})
}
