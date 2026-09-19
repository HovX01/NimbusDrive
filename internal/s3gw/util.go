package s3gw

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/vrc/nimbus/internal/domain"
)

func etag(n domain.Node) string { return `"` + n.ID + `"` }

func intQueryDefault(v string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return def
	}
	return n
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func sortObjects(o []s3Object) {
	sort.Slice(o, func(i, j int) bool { return o[i].Key < o[j].Key })
}

func sortCommon(c []s3CommonPrefix) {
	sort.Slice(c, func(i, j int) bool { return c[i].Prefix < c[j].Prefix })
}

func filterAfter(o []s3Object, marker string) []s3Object {
	out := o[:0]
	for _, x := range o {
		if x.Key > marker {
			out = append(out, x)
		}
	}
	return out
}

func filterCommonAfter(c []s3CommonPrefix, marker string) []s3CommonPrefix {
	out := c[:0]
	for _, x := range c {
		if x.Prefix > marker {
			out = append(out, x)
		}
	}
	return out
}

func decodeXMLBody(r *http.Request, v any) error {
	defer func() { _, _ = io.Copy(io.Discard, r.Body) }()
	return xml.NewDecoder(r.Body).Decode(v)
}

// writeObjectError maps a lookup failure to its S3 shape.
func writeObjectError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		writeS3Error(w, r, http.StatusNotFound, codeNoSuchKey, "key does not exist")
		return
	}
	writeS3Error(w, r, errorStatus(err), errorCode(err), err.Error())
}

func errorStatus(err error) int {
	switch {
	case errors.Is(err, errUnknownUpload):
		return http.StatusNotFound
	case errors.Is(err, errInvalidPart), errors.Is(err, errMalformedXML), errors.Is(err, domain.ErrValidation):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, errUnknownUpload):
		return codeNoSuchUpload
	case errors.Is(err, errInvalidPart):
		return codeInvalidPart
	case errors.Is(err, errMalformedXML):
		return codeMalformedXML
	case errors.Is(err, domain.ErrValidation):
		if strings.Contains(err.Error(), "too large") {
			return codeEntityTooLarge
		}
		return codeInvalidRequest
	case errors.Is(err, domain.ErrNotFound):
		return codeNoSuchKey
	case errors.Is(err, domain.ErrConflict):
		return codeBucketAlreadyOwned
	}
	return codeInternalError
}

func downloadContentType(mimeType, name string) string {
	if mimeType != "" && mimeType != "application/octet-stream" {
		return mimeType
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".mp3":
		return "audio/mpeg"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	}
	if mimeType != "" {
		return mimeType
	}
	return "application/octet-stream"
}

// parseObjectRange handles a single "bytes=start-end" request; ok=false means
// the whole object was requested.
func parseObjectRange(header string, size int64) (start, end int64, ok bool, err error) {
	if header == "" || size <= 0 {
		return 0, 0, false, nil
	}
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false, errors.New("range must use bytes=")
	}
	spec := strings.TrimPrefix(header, "bytes=")
	if strings.Contains(spec, ",") {
		return 0, 0, false, errors.New("multi-range not supported")
	}
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false, errors.New("malformed range")
	}
	if parts[0] == "" {
		suffix, convErr := strconv.ParseInt(parts[1], 10, 64)
		if convErr != nil || suffix <= 0 {
			return 0, 0, false, errors.New("malformed suffix range")
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, true, nil
	}
	start, convErr := strconv.ParseInt(parts[0], 10, 64)
	if convErr != nil || start < 0 {
		return 0, 0, false, errors.New("malformed range start")
	}
	if parts[1] == "" {
		end = size - 1
	} else if end, convErr = strconv.ParseInt(parts[1], 10, 64); convErr != nil {
		return 0, 0, false, errors.New("malformed range end")
	}
	if end >= size {
		end = size - 1
	}
	if start > end {
		return 0, 0, false, errors.New("invalid range")
	}
	return start, end, true, nil
}

// validateBucketName enforces the S3 bucket naming rules clients depend on.
func validateBucketName(name string) error {
	if len(name) < 3 || len(name) > 63 {
		return fmt.Errorf("bucket name must be 3-63 characters")
	}
	dotOrDigit := true
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '.', c == '-':
		default:
			return fmt.Errorf("bucket names may only contain lowercase letters, numbers, dots and hyphens")
		}
		if !(c == '.' || (c >= '0' && c <= '9')) {
			dotOrDigit = false
		}
	}
	first, last := name[0], name[len(name)-1]
	if !((first >= 'a' && first <= 'z') || (first >= '0' && first <= '9')) {
		return fmt.Errorf("bucket name must start with a letter or number")
	}
	if !((last >= 'a' && last <= 'z') || (last >= '0' && last <= '9')) {
		return fmt.Errorf("bucket name must end with a letter or number")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("bucket name may not contain consecutive dots")
	}
	if dotOrDigit {
		return fmt.Errorf("bucket name may not be formatted as an IP address")
	}
	return nil
}
