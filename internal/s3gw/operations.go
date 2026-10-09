package s3gw

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/vrc/nimbus/internal/domain"
)

func (s *Server) copyObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	source, err := url.Parse("/" + strings.TrimPrefix(r.Header.Get("X-Amz-Copy-Source"), "/"))
	if err != nil || source.RawQuery != "" || source.Fragment != "" {
		writeS3Error(w, r, http.StatusBadRequest, codeInvalidRequest, "invalid copy source or unsupported version")
		return
	}
	sourceBucket, sourceKey := splitFirst(strings.TrimPrefix(source.Path, "/"))
	node, err := s.lookupObject(r.Context(), sourceBucket, sourceKey)
	if err != nil {
		writeObjectError(w, r, err)
		return
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady {
		writeS3Error(w, r, http.StatusNotFound, codeNoSuchKey, "source key does not exist")
		return
	}
	contentType := node.MimeType
	if r.Header.Get("X-Amz-Metadata-Directive") == "REPLACE" {
		contentType = r.Header.Get("Content-Type")
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	go func() {
		_, err := s.svc.Download(r.Context(), node.ID, writer)
		_ = writer.CloseWithError(err)
	}()
	copied, err := s.storeObject(r.Context(), bucket, key, contentType, reader)
	if err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
	writeXML(w, http.StatusOK, copyObjectResult{LastModified: iso8601(copied.UpdatedAt), ETag: etag(copied)})
}

func (s *Server) deleteKey(ctx context.Context, bucket, key string) error {
	node, err := s.lookupObject(ctx, bucket, key)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if node.Type != domain.NodeFile {
		return nil
	}
	return s.svc.Delete(ctx, node.ID)
}

func (s *Server) deleteObjects(w http.ResponseWriter, r *http.Request, bucket string) {
	if _, err := bucketNodeOf(r.Context(), s.svc, bucket); err != nil {
		writeS3Error(w, r, errorStatus(err), codeNoSuchBucket, err.Error())
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req deleteObjectsRequest
	if err := decodeXMLBody(r, &req); err != nil {
		writeS3Error(w, r, http.StatusBadRequest, codeMalformedXML, err.Error())
		return
	}
	if len(req.Objects) == 0 || len(req.Objects) > 1000 {
		writeS3Error(w, r, http.StatusBadRequest, codeInvalidRequest, "delete requires 1..1000 keys")
		return
	}
	result := deleteObjectsResult{}
	for _, obj := range req.Objects {
		var err error
		if obj.Key == "" || obj.VersionID != "" {
			err = domain.ErrValidation
		} else {
			err = s.deleteKey(r.Context(), bucket, obj.Key)
		}
		if err != nil {
			result.Errors = append(result.Errors, deleteObjectError{Key: obj.Key, Code: errorCode(err), Message: err.Error()})
		} else if !req.Quiet {
			result.Deleted = append(result.Deleted, deletedObject{Key: obj.Key})
		}
	}
	writeXML(w, http.StatusOK, result)
}

func (s *Server) deleteBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	node, err := bucketNodeOf(r.Context(), s.svc, bucket)
	if err != nil {
		writeS3Error(w, r, errorStatus(err), codeNoSuchBucket, err.Error())
		return
	}
	var hasFiles func(string) (bool, error)
	hasFiles = func(id string) (bool, error) {
		children, err := s.svc.Nodes.ListChildren(r.Context(), id)
		if err != nil {
			return false, err
		}
		for _, child := range children {
			if child.Type == domain.NodeFile {
				return true, nil
			}
			if found, err := hasFiles(child.ID); err != nil || found {
				return found, err
			}
		}
		return false, nil
	}
	nonempty, err := hasFiles(node.ID)
	if err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
	s.multipart.mu.Lock()
	for _, up := range s.multipart.uploads {
		if up.Bucket == bucket {
			nonempty = true
			break
		}
	}
	s.multipart.mu.Unlock()
	if nonempty {
		writeS3Error(w, r, http.StatusConflict, "BucketNotEmpty", "bucket is not empty")
		return
	}
	if err := s.svc.Delete(r.Context(), node.ID); err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listParts(w http.ResponseWriter, r *http.Request, bucket, key, uploadID string) {
	up, err := s.multipart.getForObject(uploadID, bucket, key)
	if err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
	q := r.URL.Query()
	marker := intQueryDefault(q.Get("part-number-marker"), 0)
	maxParts := clampInt(intQueryDefault(q.Get("max-parts"), 1000), 0, 1000)
	result := listPartsResult{Bucket: bucket, Key: key, UploadID: uploadID, PartNumberMarker: marker, MaxParts: maxParts, StorageClass: "STANDARD"}
	up.mu.Lock()
	for no, part := range up.parts {
		if no > marker {
			result.Parts = append(result.Parts, s3Part{PartNumber: no, Size: part.Size, ETag: `"` + part.MD5 + `"`, LastModified: iso8601(part.Modified)})
		}
	}
	up.mu.Unlock()
	sort.Slice(result.Parts, func(i, j int) bool { return result.Parts[i].PartNumber < result.Parts[j].PartNumber })
	if maxParts > 0 && len(result.Parts) > maxParts {
		result.Parts = result.Parts[:maxParts]
		result.IsTruncated = true
		result.NextPartNumberMarker = result.Parts[len(result.Parts)-1].PartNumber
	} else if maxParts == 0 {
		result.Parts = nil
	}
	writeXML(w, http.StatusOK, result)
}

func (s *Server) listMultipartUploads(w http.ResponseWriter, r *http.Request, bucket string) {
	q := r.URL.Query()
	result := listMultipartUploadsResult{Bucket: bucket, KeyMarker: q.Get("key-marker"), UploadIDMarker: q.Get("upload-id-marker"), Prefix: q.Get("prefix"), MaxUploads: clampInt(intQueryDefault(q.Get("max-uploads"), 1000), 0, 1000)}
	s.multipart.mu.Lock()
	for id, up := range s.multipart.uploads {
		if up.Bucket == bucket && strings.HasPrefix(up.Key, result.Prefix) && (up.Key > result.KeyMarker || (up.Key == result.KeyMarker && result.UploadIDMarker != "" && id > result.UploadIDMarker)) {
			result.Uploads = append(result.Uploads, s3Upload{Key: up.Key, UploadID: id, Initiated: iso8601(up.Initiated), StorageClass: "STANDARD"})
		}
	}
	s.multipart.mu.Unlock()
	sort.Slice(result.Uploads, func(i, j int) bool {
		a, b := result.Uploads[i], result.Uploads[j]
		return a.Key < b.Key || (a.Key == b.Key && a.UploadID < b.UploadID)
	})
	if result.MaxUploads > 0 && len(result.Uploads) > result.MaxUploads {
		result.Uploads = result.Uploads[:result.MaxUploads]
		result.IsTruncated = true
		last := result.Uploads[len(result.Uploads)-1]
		result.NextKeyMarker, result.NextUploadIDMarker = last.Key, last.UploadID
	} else if result.MaxUploads == 0 {
		result.Uploads = nil
	}
	writeXML(w, http.StatusOK, result)
}
