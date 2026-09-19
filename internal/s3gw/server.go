package s3gw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/vrc/nimbus/internal/app"
	"github.com/vrc/nimbus/internal/domain"
)

const (
	rootID     = "root"
	maxKeysCap = 1000
)

// Server is the S3-compatible front end over a NimbusDrive *app.Services.
type Server struct {
	svc       *app.Services
	cfg       Config
	multipart *multipartTracker
}

func New(svc *app.Services, cfg Config, dataDir string) *Server {
	t := newMultipartTracker(dataDir)
	t.cleanStale()
	return &Server{svc: svc, cfg: cfg, multipart: t}
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, fmt.Sprintf("panic: %v", rec))
			}
		}()
		if err := verifySigV4(r, s.cfg.AccessKey, s.cfg.SecretKey); err != nil {
			switch {
			case errors.Is(err, errInvalidKey):
				writeS3Error(w, r, http.StatusForbidden, codeInvalidAccessKey, err.Error())
			case errors.Is(err, errBadSignature):
				writeS3Error(w, r, http.StatusForbidden, codeSignatureMismatch, err.Error())
			default:
				writeS3Error(w, r, http.StatusForbidden, codeAccessDenied, err.Error())
			}
			return
		}
		s.route(w, r)
	})
}

// route dispatches path-style requests: "/" is the service, "/bucket" is a
// bucket, "/bucket/key/with/slashes" is an object. r.URL.Path is already
// percent-decoded by net/http, so keys arrive decoded.
func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	p := strings.Trim(r.URL.Path, "/")
	if p == "" {
		if r.Method == http.MethodGet {
			s.listBuckets(w, r)
			return
		}
		writeS3Error(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
		return
	}
	bucket, rest := splitFirst(p)
	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			s.listObjects(w, r, bucket)
		case http.MethodHead:
			s.headBucket(w, r, bucket)
		case http.MethodPut:
			s.createBucket(w, r, bucket)
		default:
			writeS3Error(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
		}
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getObject(w, r, bucket, rest)
	case http.MethodHead:
		s.headObject(w, r, bucket, rest)
	case http.MethodPut:
		s.putObject(w, r, bucket, rest)
	case http.MethodPost:
		s.postObject(w, r, bucket, rest)
	case http.MethodDelete:
		s.deleteObject(w, r, bucket, rest)
	default:
		writeS3Error(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
	}
}

func (s *Server) listBuckets(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.svc.List(r.Context(), rootID)
	if err != nil {
		writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, err.Error())
		return
	}
	var out listAllMyBucketsResult
	out.Owner = s3Owner{ID: "nimbus", DisplayName: "nimbus"}
	for _, n := range nodes {
		if n.Type != domain.NodeFolder {
			continue
		}
		out.Buckets.Bucket = append(out.Buckets.Bucket, s3BucketInfo{Name: n.Name, CreationDate: iso8601(n.CreatedAt)})
	}
	if out.Buckets.Bucket == nil {
		out.Buckets.Bucket = []s3BucketInfo{}
	}
	writeXML(w, http.StatusOK, out)
}

func (s *Server) headBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if _, err := s.bucketNode(r.Context(), bucket); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeS3Error(w, r, http.StatusNotFound, codeNoSuchBucket, "bucket does not exist")
			return
		}
		writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, err.Error())
		return
	}
	w.Header().Set("X-Amz-Bucket-Region", s.cfg.Region)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) createBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if err := validateBucketName(bucket); err != nil {
		writeS3Error(w, r, http.StatusBadRequest, codeInvalidBucketName, err.Error())
		return
	}
	if _, err := s.svc.Nodes.FindChildByName(r.Context(), rootID, bucket); err == nil {
		writeS3Error(w, r, http.StatusConflict, codeBucketAlreadyOwned, "bucket already exists")
		return
	} else if !errors.Is(err, domain.ErrNotFound) {
		writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, err.Error())
		return
	}
	if _, err := s.svc.Mkdir(r.Context(), rootID, bucket); err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
	w.Header().Set("Location", "/"+bucket)
	w.Header().Set("X-Amz-Bucket-Region", s.cfg.Region)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) listObjects(w http.ResponseWriter, r *http.Request, bucket string) {
	ctx := r.Context()
	q := r.URL.Query()
	if _, ok := q["uploads"]; ok {
		// Abandoned-upload sweep; we do not track uploads across restarts.
		writeXML(w, http.StatusOK, listMultipartUploadsResult{Bucket: bucket})
		return
	}
	if _, ok := q["location"]; ok {
		writeXML(w, http.StatusOK, locationConstraintResult{Region: s.cfg.Region})
		return
	}
	bNode, err := s.bucketNode(ctx, bucket)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeS3Error(w, r, http.StatusNotFound, codeNoSuchBucket, "bucket does not exist")
			return
		}
		writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, err.Error())
		return
	}

	prefix := q.Get("prefix")
	delimiter := q.Get("delimiter")
	maxKeys := clampInt(intQueryDefault(q.Get("max-keys"), maxKeysCap), 1, maxKeysCap)
	marker := q.Get("marker")
	if token := q.Get("continuation-token"); token != "" {
		marker = token
	}
	encodeURL := strings.EqualFold(q.Get("encoding-type"), "url")
	enc := func(v string) string {
		if encodeURL {
			return uriEncode(v)
		}
		return v
	}

	result := listBucketResult{
		Name:         bucket,
		Prefix:       prefix,
		Delimiter:    delimiter,
		MaxKeys:      maxKeys,
		EncodingType: q.Get("encoding-type"),
		Contents:     []s3Object{},
	}

	// Resolve the folder the prefix points at. folderKey is the deepest folder
	// implied by the prefix; literal is the remaining name filter (used only
	// when the prefix does not end with the delimiter).
	folderKey, literal := splitPrefix(prefix)
	listNode, err := s.resolveFolder(ctx, bNode.ID, folderKey)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeXML(w, http.StatusOK, result) // prefix matches nothing
			return
		}
		writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, err.Error())
		return
	}

	var objects []s3Object
	var common []s3CommonPrefix
	if delimiter != "" {
		children, err := s.svc.List(ctx, listNode.ID)
		if err != nil {
			writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, err.Error())
			return
		}
		for _, c := range children {
			if literal != "" && !strings.HasPrefix(c.Name, literal) {
				continue
			}
			if c.Type == domain.NodeFolder {
				common = append(common, s3CommonPrefix{Prefix: enc(joinKey(folderKey, c.Name) + "/")})
				continue
			}
			if c.Status != domain.StatusReady {
				continue
			}
			objects = append(objects, s3Object{
				Key:          enc(joinKey(folderKey, c.Name)),
				LastModified: iso8601(c.UpdatedAt),
				ETag:         etag(c),
				Size:         c.Size,
				StorageClass: "STANDARD",
			})
		}
	} else {
		if err := s.walk(ctx, listNode.ID, folderKey, func(key string, n domain.Node) {
			if !strings.HasPrefix(key, prefix) {
				return
			}
			objects = append(objects, s3Object{
				Key:          enc(key),
				LastModified: iso8601(n.UpdatedAt),
				ETag:         etag(n),
				Size:         n.Size,
				StorageClass: "STANDARD",
			})
		}); err != nil {
			writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, err.Error())
			return
		}
	}

	sortObjects(objects)
	sortCommon(common)

	if marker != "" {
		objects = filterAfter(objects, marker)
		common = filterCommonAfter(common, marker)
	}
	truncated := false
	if len(objects) > maxKeys {
		objects = objects[:maxKeys]
		truncated = true
	}
	result.Contents = objects
	result.CommonPrefixes = common
	result.IsTruncated = truncated
	result.KeyCount = len(objects) + len(common)
	result.Marker = q.Get("marker")
	result.ContinuationToken = q.Get("continuation-token")
	if q.Get("list-type") == "2" {
		result.Marker = ""
	} else {
		result.ContinuationToken = ""
	}
	if truncated && len(objects) > 0 {
		result.NextMarker = objects[len(objects)-1].Key
		result.NextContinuationToken = objects[len(objects)-1].Key
	}
	writeXML(w, http.StatusOK, result)
}

func (s *Server) getObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	node, err := s.lookupObject(r.Context(), bucket, key)
	if err != nil {
		writeObjectError(w, r, err)
		return
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady {
		writeS3Error(w, r, http.StatusNotFound, codeNoSuchKey, "key does not exist")
		return
	}
	w.Header().Set("Content-Type", downloadContentType(node.MimeType, node.Name))
	w.Header().Set("ETag", etag(node))
	w.Header().Set("Last-Modified", node.UpdatedAt.UTC().Format(http.TimeFormat))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("X-Amz-Storage-Class", "STANDARD")

	start, end, ok, err := parseObjectRange(r.Header.Get("Range"), node.Size)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", node.Size))
		writeS3Error(w, r, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "invalid range")
		return
	}
	if !ok {
		w.Header().Set("Content-Length", strconv.FormatInt(node.Size, 10))
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = s.svc.Download(r.Context(), node.ID, w)
		}
		return
	}
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, node.Size))
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.WriteHeader(http.StatusPartialContent)
	if r.Method == http.MethodGet {
		_, _ = s.svc.DownloadRange(r.Context(), node.ID, w, start, end)
	}
}

func (s *Server) headObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	s.getObject(w, r, bucket, key)
}

func (s *Server) putObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	q := r.URL.Query()
	if uploadID := q.Get("uploadId"); uploadID != "" && q.Get("partNumber") != "" {
		s.uploadPart(w, r, uploadID, q.Get("partNumber"))
		return
	}
	if key == "" {
		writeS3Error(w, r, http.StatusBadRequest, codeInvalidRequest, "object key required")
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if strings.HasSuffix(key, "/") {
		bNode, err := s.bucketNode(r.Context(), bucket)
		if err != nil {
			writeObjectError(w, r, err)
			return
		}
		folder, err := s.ensureFolderPath(r.Context(), bNode.ID, strings.TrimSuffix(key, "/"))
		if err != nil {
			writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
			return
		}
		w.Header().Set("ETag", etag(folder))
		w.WriteHeader(http.StatusOK)
		return
	}
	node, err := s.storeObject(r.Context(), bucket, key, contentType, chunkedBody(r))
	if err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
	w.Header().Set("ETag", etag(node))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) postObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	q := r.URL.Query()
	if _, ok := q["uploads"]; ok {
		uploadID, err := s.multipart.create(bucket, key, r.Header.Get("Content-Type"))
		if err != nil {
			writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, err.Error())
			return
		}
		writeXML(w, http.StatusOK, initiateMultipartResult{Bucket: bucket, Key: key, UploadID: uploadID})
		return
	}
	if uploadID := q.Get("uploadId"); uploadID != "" {
		s.completeMultipart(w, r, bucket, key, uploadID)
		return
	}
	writeS3Error(w, r, http.StatusBadRequest, codeInvalidRequest, "unsupported POST")
}

func (s *Server) deleteObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	q := r.URL.Query()
	if uploadID := q.Get("uploadId"); uploadID != "" {
		s.multipart.abort(uploadID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if node, err := s.lookupObject(r.Context(), bucket, key); err == nil {
		_ = s.svc.Delete(r.Context(), node.ID)
	} else if !errors.Is(err, domain.ErrNotFound) {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) uploadPart(w http.ResponseWriter, r *http.Request, uploadID, partNoRaw string) {
	partNo, err := strconv.Atoi(strings.TrimSpace(partNoRaw))
	if err != nil || partNo < 1 || partNo > 10000 {
		writeS3Error(w, r, http.StatusBadRequest, codeInvalidPart, "part number must be 1..10000")
		return
	}
	md5Hex, err := s.multipart.putPart(uploadID, partNo, chunkedBody(r))
	if err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
	w.Header().Set("ETag", `"`+md5Hex+`"`)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) completeMultipart(w http.ResponseWriter, r *http.Request, bucket, key, uploadID string) {
	var req completeMultipartRequest
	if err := decodeXMLBody(r, &req); err != nil {
		writeS3Error(w, r, http.StatusBadRequest, codeMalformedXML, err.Error())
		return
	}
	reader, cleanup, err := s.multipart.assemble(uploadID, req.Parts)
	if err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
	defer cleanup()
	// Prefer the content type from the initiate request; the complete request's
	// own Content-Type describes its XML body, not the object.
	contentType := "application/octet-stream"
	if up, gerr := s.multipart.get(uploadID); gerr == nil && up.ContentType != "" {
		contentType = up.ContentType
	}
	node, err := s.storeObject(r.Context(), bucket, key, contentType, reader)
	if err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
	writeXML(w, http.StatusOK, completeMultipartResult{
		Location: "/" + bucket + "/" + key,
		Bucket:   bucket,
		Key:      key,
		ETag:     etag(node),
	})
}

// storeObject creates or replaces the object at key, streaming bytes straight
// into the drive's chunked storage backend.
func (s *Server) storeObject(ctx context.Context, bucket, key, contentType string, reader io.Reader) (domain.Node, error) {
	bNode, err := s.bucketNode(ctx, bucket)
	if err != nil {
		return domain.Node{}, err
	}
	folderPath, name := splitKey(key)
	folder, err := s.ensureFolderPath(ctx, bNode.ID, folderPath)
	if err != nil {
		return domain.Node{}, err
	}
	// S3 PUT overwrites; the drive renames duplicates, so trash the incumbent first.
	if existing, err := s.svc.Nodes.FindChildByName(ctx, folder.ID, name); err == nil {
		_ = s.svc.Delete(ctx, existing.ID)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Node{}, err
	}
	return s.svc.UploadNoDedup(ctx, folder.ID, name, contentType, reader)
}
