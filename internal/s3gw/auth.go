package s3gw

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	errNoAuth        = errors.New("missing AWS Signature Version 4 authorization header")
	errMalformedAuth = errors.New("malformed authorization header")
	errInvalidKey    = errors.New("invalid access key id")
	errBadSignature  = errors.New("signature does not match")
	errStaleDate     = errors.New("request timestamp missing, invalid, or expired")
)

// verifySigV4 authenticates an AWS Signature Version 4 request against the
// configured static keypair. The payload hash declared by the client
// (x-amz-content-sha256, or UNSIGNED-PAYLOAD / STREAMING markers) is trusted:
// the signature commits to it, so a forged hash cannot be signed without the key.
func verifySigV4(r *http.Request, accessKey, secretKey string) error {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return errMalformedAuth
	}
	header := r.Header.Get("Authorization")
	presigned := header == "" && q.Get("X-Amz-Algorithm") == "AWS4-HMAC-SHA256"
	var credential, signedHeaders, signature string
	amzDate := r.Header.Get("X-Amz-Date")
	if presigned {
		for _, name := range []string{"X-Amz-Algorithm", "X-Amz-Credential", "X-Amz-SignedHeaders", "X-Amz-Signature", "X-Amz-Date", "X-Amz-Expires"} {
			if len(q[name]) != 1 {
				return errMalformedAuth
			}
		}
		credential, signedHeaders, signature = q.Get("X-Amz-Credential"), q.Get("X-Amz-SignedHeaders"), q.Get("X-Amz-Signature")
		amzDate = q.Get("X-Amz-Date")
		q.Del("X-Amz-Signature")
	} else {
		if !strings.HasPrefix(header, "AWS4-HMAC-SHA256 ") {
			return errNoAuth
		}
		for _, part := range strings.Split(strings.TrimPrefix(header, "AWS4-HMAC-SHA256 "), ",") {
			p := strings.TrimSpace(part)
			switch {
			case strings.HasPrefix(p, "Credential="):
				credential = strings.TrimPrefix(p, "Credential=")
			case strings.HasPrefix(p, "SignedHeaders="):
				signedHeaders = strings.TrimPrefix(p, "SignedHeaders=")
			case strings.HasPrefix(p, "Signature="):
				signature = strings.TrimPrefix(p, "Signature=")
			}
		}
	}
	if credential == "" || signedHeaders == "" || signature == "" {
		return errMalformedAuth
	}
	cred := strings.Split(credential, "/")
	if len(cred) != 5 || cred[2] == "" || cred[4] != "aws4_request" || cred[3] != "s3" {
		return errMalformedAuth
	}
	reqAccessKey, dateStamp, region := cred[0], cred[1], cred[2]
	if subtle.ConstantTimeCompare([]byte(reqAccessKey), []byte(accessKey)) != 1 {
		return errInvalidKey
	}
	// The credential scope in the string-to-sign excludes the access key prefix.
	scope := strings.Join(cred[1:], "/")

	if amzDate == "" {
		if date, err := http.ParseTime(r.Header.Get("Date")); err == nil && !presigned {
			amzDate = date.UTC().Format("20060102T150405Z")
		}
	}
	signedAt, err := time.Parse("20060102T150405Z", amzDate)
	if err != nil || dateStamp != signedAt.Format("20060102") {
		return errStaleDate
	}
	lifetime := 15 * time.Minute
	if presigned {
		seconds, err := strconv.ParseInt(q.Get("X-Amz-Expires"), 10, 64)
		if err != nil || seconds < 1 || seconds > 604800 {
			return errStaleDate
		}
		lifetime = time.Duration(seconds) * time.Second
	}
	now := time.Now()
	if signedAt.After(now.Add(15*time.Minute)) || now.After(signedAt.Add(lifetime)) {
		return errStaleDate
	}
	names := strings.Split(signedHeaders, ";")
	hasHost := false
	for i, name := range names {
		if name == "" || name != strings.ToLower(name) || (i > 0 && names[i-1] >= name) {
			return errMalformedAuth
		}
		if name == "host" {
			hasHost = true
		} else if len(r.Header.Values(name)) == 0 && !(name == "content-length" && r.ContentLength > 0) {
			return errMalformedAuth
		}
	}
	if !hasHost {
		return errMalformedAuth
	}
	decodedSignature, err := hex.DecodeString(signature)
	if err != nil || len(decodedSignature) != sha256.Size {
		return errMalformedAuth
	}
	hash := r.Header.Get("X-Amz-Content-Sha256")
	if hash == "" {
		hash = "UNSIGNED-PAYLOAD"
		if !presigned {
			hash, err = hashRequestBody(r)
			if err != nil {
				return err
			}
		}
	}

	var cr strings.Builder
	cr.WriteString(canonicalQueryString(q))
	cr.WriteByte('\n')
	for _, name := range names {
		cr.WriteString(name)
		cr.WriteByte(':')
		cr.WriteString(canonicalHeaderValue(r, name))
		cr.WriteByte('\n')
	}
	cr.WriteByte('\n')
	cr.WriteString(signedHeaders)
	cr.WriteByte('\n')
	hashes := []string{hash}
	if !presigned && r.Header.Get("X-Amz-Content-Sha256") == "" {
		hashes = append(hashes, "UNSIGNED-PAYLOAD")
	}
	// S3 clients differ on escaping reserved path characters. Both variants
	// must still authenticate the complete request with the configured secret.
	paths := []string{canonicalURI(r)}
	if escaped := r.URL.EscapedPath(); escaped != "" && escaped != paths[0] {
		paths = append(paths, escaped)
	}
	key := signingKey(secretKey, dateStamp, region, "s3")
	for _, path := range paths {
		for _, hash := range hashes {
			canonical := r.Method + "\n" + path + "\n" + cr.String() + hash
			stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sha256Sum([]byte(canonical)))
			if subtle.ConstantTimeCompare(hmacSHA256(key, []byte(stringToSign)), decodedSignature) == 1 {
				return nil
			}
		}
	}
	return errBadSignature
}

func canonicalURI(r *http.Request) string {
	if r.URL.Path != "" {
		return strings.ReplaceAll(uriEncode(r.URL.Path), "%2F", "/")
	}
	return "/"
}

// canonicalQueryString sorts parameters and URI-encodes them per RFC 3986.
func canonicalQueryString(q url.Values) string {
	var pairs []string
	for k := range q {
		for _, v := range q[k] {
			pairs = append(pairs, uriEncode(k)+"="+uriEncode(v))
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		ki, vi, _ := strings.Cut(pairs[i], "=")
		kj, vj, _ := strings.Cut(pairs[j], "=")
		if ki != kj {
			return ki < kj
		}
		return vi < vj
	})
	return strings.Join(pairs, "&")
}

func canonicalHeaderValue(r *http.Request, name string) string {
	if strings.EqualFold(name, "host") {
		return strings.Join(strings.Fields(r.Host), " ")
	}
	if name == "content-length" && len(r.Header.Values(name)) == 0 && r.ContentLength > 0 {
		return strconv.FormatInt(r.ContentLength, 10)
	}
	values := append([]string(nil), r.Header.Values(name)...)
	for i := range values {
		values[i] = strings.Join(strings.Fields(values[i]), " ")
	}
	return strings.Join(values, ",")
}

func hashRequestBody(r *http.Request) (string, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return hex.EncodeToString(sha256Sum(nil)), nil
	}
	// ponytail: missing-hash clients spool to disk; send x-amz-content-sha256 for streaming uploads.
	f, err := os.CreateTemp("", "nimbus-s3-body-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), r.Body)
	r.Body.Close()
	if err == nil {
		_, err = f.Seek(0, io.SeekStart)
	}
	if err != nil {
		f.Close()
		return "", err
	}
	r.Body = f
	return hex.EncodeToString(h.Sum(nil)), nil
}

func signingKey(secretKey, dateStamp, region, service string) []byte {
	k := []byte("AWS4" + secretKey)
	k = hmacSHA256(k, []byte(dateStamp))
	k = hmacSHA256(k, []byte(region))
	k = hmacSHA256(k, []byte(service))
	return hmacSHA256(k, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Sum(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

// uriEncode percent-encodes all bytes except RFC 3986 unreserved characters.
func uriEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
