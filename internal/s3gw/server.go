package s3gw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/google/uuid"
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
	dataDir   string
	multipart *multipartTracker
	namespace *sync.RWMutex
}

func New(svc *app.Services, cfg Config, dataDir string) *Server {
	t := newMultipartTracker(dataDir)
	t.cleanStale()
	return &Server{svc: svc, cfg: cfg, dataDir: dataDir, multipart: t, namespace: &sync.RWMutex{}}
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, fmt.Sprintf("panic: %v", rec))
			}
		}()
		cfg, err := Load(s.dataDir, s.cfg.Enabled, s.cfg.Region)
		if err != nil {
			writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, "cannot load S3 credentials")
			return
		}
		defer func() {
			if r.Body != nil {
				r.Body.Close()
			}
		}()
		if err := verifySigV4(r, cfg.AccessKey, cfg.SecretKey); err != nil {
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
		current := *s
		current.cfg = cfg
		current.route(w, r)
	})
}

// route dispatches path-style requests: "/" is the service, "/bucket" is a
// bucket, "/bucket/key/with/slashes" is an object. r.URL.Path is already
// percent-decoded by net/http, so keys arrive decoded.
func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		if r.Method == http.MethodGet {
			s.listBuckets(w, r)
			return
		}
		writeS3Error(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
		return
	}
	bucket, rest := splitFirst(p)
	allowed := ""
	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			allowed = "list-type prefix delimiter max-keys marker continuation-token start-after encoding-type fetch-owner location uploads key-marker upload-id-marker max-uploads"
		case http.MethodPost:
			allowed = "delete"
		}
	} else {
		switch r.Method {
		case http.MethodGet:
			allowed = "uploadId part-number-marker max-parts response-content-type response-content-disposition response-content-language response-content-encoding response-cache-control response-expires"
		case http.MethodPut:
			allowed = "uploadId partNumber"
		case http.MethodPost:
			allowed = "uploads uploadId"
		case http.MethodDelete:
			allowed = "uploadId"
		}
	}
	for query := range r.URL.Query() {
		if query != "x-id" && !strings.HasPrefix(query, "X-Amz-") && !slices.Contains(strings.Fields(allowed), query) {
			writeS3Error(w, r, http.StatusNotImplemented, "NotImplemented", "S3 query operation not supported: "+query)
			return
		}
	}
	// Bucket removal must exclude writes between its emptiness check and deletion.
	if rest == "" && r.Method == http.MethodDelete {
		s.namespace.Lock()
		defer s.namespace.Unlock()
	} else {
		s.namespace.RLock()
		defer s.namespace.RUnlock()
	}
	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			s.listObjects(w, r, bucket)
		case http.MethodHead:
			s.headBucket(w, r, bucket)
		case http.MethodPut:
			s.createBucket(w, r, bucket)
		case http.MethodPost:
			if _, ok := r.URL.Query()["delete"]; ok {
				s.deleteObjects(w, r, bucket)
				return
			}
			writeS3Error(w, r, http.StatusBadRequest, codeInvalidRequest, "unsupported POST")
		case http.MethodDelete:
			s.deleteBucket(w, r, bucket)
		default:
			writeS3Error(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
		}
		return
	}
	if _, err := bucketNodeOf(r.Context(), s.svc, bucket); err != nil {
		writeS3Error(w, r, errorStatus(err), codeNoSuchBucket, err.Error())
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
	if _, err := bucketNodeOf(r.Context(), s.svc, bucket); err != nil {
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
	if err := ValidateBucketName(bucket); err != nil {
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
	bNode, err := bucketNodeOf(ctx, s.svc, bucket)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeS3Error(w, r, http.StatusNotFound, codeNoSuchBucket, "bucket does not exist")
			return
		}
		writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, err.Error())
		return
	}
	if _, ok := q["uploads"]; ok {
		s.listMultipartUploads(w, r, bucket)
		return
	}
	if _, ok := q["location"]; ok {
		writeXML(w, http.StatusOK, locationConstraintResult{Region: s.cfg.Region})
		return
	}

	prefix := q.Get("prefix")
	delimiter := q.Get("delimiter")
	maxKeys := clampInt(intQueryDefault(q.Get("max-keys"), maxKeysCap), 0, maxKeysCap)
	marker := q.Get("marker")
	if q.Get("list-type") == "2" {
		marker = q.Get("start-after")
	}
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
		Prefix:       enc(prefix),
		Delimiter:    enc(delimiter),
		MaxKeys:      maxKeys,
		EncodingType: q.Get("encoding-type"),
		Contents:     []s3Object{},
	}
	if q.Get("list-type") == "2" {
		result.ContinuationToken = q.Get("continuation-token")
		result.StartAfter = enc(q.Get("start-after"))
	} else {
		result.Marker = enc(q.Get("marker"))
	}

	// Resolve the folder the prefix points at. folderKey is the deepest folder
	// implied by the prefix; literal is the remaining name filter (used only
	// when the prefix does not end with the delimiter).
	folderKey, _ := splitPrefix(prefix)
	listNode, err := s.resolveFolder(ctx, bNode.ID, folderKey)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeXML(w, http.StatusOK, result) // prefix matches nothing
			return
		}
		writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, err.Error())
		return
	}

	type entry struct {
		key    string
		node   domain.Node
		common bool
	}
	var entries []entry
	prefixes := map[string]bool{}
	if err := walkTree(ctx, s.svc, listNode.ID, folderKey, func(key string, n domain.Node) {
		if !strings.HasPrefix(key, prefix) {
			return
		}
		isCommon := false
		if delimiter != "" {
			if i := strings.Index(strings.TrimPrefix(key, prefix), delimiter); i >= 0 {
				key = key[:len(prefix)+i+len(delimiter)]
				if prefixes[key] {
					return
				}
				prefixes[key] = true
				isCommon = true
			}
		}
		if key > marker {
			entries = append(entries, entry{key: key, node: n, common: isCommon})
		}
	}); err != nil {
		writeS3Error(w, r, http.StatusInternalServerError, codeInternalError, err.Error())
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	if maxKeys > 0 && len(entries) > maxKeys {
		result.IsTruncated = true
		entries = entries[:maxKeys]
		lastKey := entries[len(entries)-1].key
		if q.Get("list-type") == "2" {
			result.NextContinuationToken = lastKey
		} else {
			result.NextMarker = enc(lastKey)
		}
	} else if maxKeys == 0 {
		entries = nil
	}
	for _, e := range entries {
		if e.common {
			result.CommonPrefixes = append(result.CommonPrefixes, s3CommonPrefix{Prefix: enc(e.key)})
		} else {
			object := s3Object{
				Key: enc(e.key), LastModified: iso8601(e.node.UpdatedAt), ETag: etag(e.node),
				Size: e.node.Size, StorageClass: "STANDARD",
			}
			if q.Get("list-type") != "2" || q.Get("fetch-owner") == "true" {
				object.Owner = &s3Owner{ID: "nimbus", DisplayName: "nimbus"}
			}
			result.Contents = append(result.Contents, object)
		}
	}
	result.KeyCount = len(entries)
	writeXML(w, http.StatusOK, result)
}

func (s *Server) getObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	if uploadID := r.URL.Query().Get("uploadId"); uploadID != "" {
		s.listParts(w, r, bucket, key, uploadID)
		return
	}
	node, err := s.lookupObject(r.Context(), bucket, key)
	if err != nil {
		writeObjectError(w, r, err)
		return
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady {
		writeS3Error(w, r, http.StatusNotFound, codeNoSuchKey, "key does not exist")
		return
	}
	contentType := node.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", etag(node))
	w.Header().Set("Last-Modified", node.UpdatedAt.UTC().Format(http.TimeFormat))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("X-Amz-Storage-Class", "STANDARD")
	for _, header := range []string{"Content-Type", "Content-Disposition", "Content-Language", "Content-Encoding", "Cache-Control", "Expires"} {
		if value := r.URL.Query().Get("response-" + strings.ToLower(header)); value != "" {
			w.Header().Set(header, value)
		}
	}

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
	if q.Has("uploadId") || q.Has("partNumber") {
		if q.Get("uploadId") == "" || q.Get("partNumber") == "" {
			writeS3Error(w, r, http.StatusBadRequest, codeInvalidRequest, "uploadId and partNumber are required together")
			return
		}
		s.uploadPart(w, r, bucket, key, q.Get("uploadId"), q.Get("partNumber"))
		return
	}
	if r.Header.Get("X-Amz-Copy-Source") != "" {
		s.copyObject(w, r, bucket, key)
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
		if _, err := bucketNodeOf(r.Context(), s.svc, bucket); err != nil {
			writeS3Error(w, r, http.StatusNotFound, codeNoSuchBucket, "bucket does not exist")
			return
		}
		if err := validateObjectKey(key); err != nil {
			writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
			return
		}
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
		if _, err := s.multipart.getForObject(uploadID, bucket, key); err != nil {
			writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
			return
		}
		s.multipart.abort(uploadID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.deleteKey(r.Context(), bucket, key); err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) uploadPart(w http.ResponseWriter, r *http.Request, bucket, key, uploadID, partNoRaw string) {
	if _, err := s.multipart.getForObject(uploadID, bucket, key); err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
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
	up, err := s.multipart.getForObject(uploadID, bucket, key)
	if err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
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
	if up.ContentType != "" {
		contentType = up.ContentType
	}
	node, err := s.storeObject(r.Context(), bucket, key, contentType, reader)
	if err != nil {
		writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
		return
	}
	s.multipart.abort(uploadID)
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
	if err := validateObjectKey(key); err != nil {
		return domain.Node{}, err
	}
	folderPath, name := splitKey(key)
	bNode, err := bucketNodeOf(ctx, s.svc, bucket)
	if err != nil {
		return domain.Node{}, err
	}
	folder, err := s.ensureFolderPath(ctx, bNode.ID, folderPath)
	if err != nil {
		return domain.Node{}, err
	}
	if existing, err := s.svc.Nodes.FindChildByName(ctx, folder.ID, name); err == nil {
		if existing.Type != domain.NodeFile {
			return domain.Node{}, domain.ErrConflict
		}
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Node{}, err
	}
	// Stage under a unique name so concurrent PUTs cannot expose partial objects.
	node, err := s.svc.StageObject(ctx, folder.ID, uuid.NewString(), contentType, reader)
	if err != nil {
		return domain.Node{}, err
	}
	if err := s.svc.Nodes.Replace(ctx, node.ID, name); err != nil {
		_ = s.svc.Delete(ctx, node.ID)
		return domain.Node{}, err
	}
	return s.svc.Nodes.Get(ctx, node.ID)
}

// shortcut: keys follow drive path/name rules; use a separate key index for flat S3 namespaces.
func validateObjectKey(key string) error {
	if key == "" || len(key) > 1024 || !utf8.ValidString(key) {
		return fmt.Errorf("%w: object key must be 1..1024 UTF-8 bytes", domain.ErrValidation)
	}
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		if segment == "" && i == len(segments)-1 {
			continue
		}
		if err := domain.ValidateNodeName(segment); err != nil {
			return err
		}
	}
	return nil
}
