package s3gw

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

var (
	errNoAuth        = errors.New("missing AWS Signature Version 4 authorization header")
	errMalformedAuth = errors.New("malformed authorization header")
	errInvalidKey    = errors.New("invalid access key id")
	errBadSignature  = errors.New("signature does not match")
	errStaleDate     = errors.New("request timestamp missing")
)

// verifySigV4 authenticates an AWS Signature Version 4 request against the
// configured static keypair. The payload hash declared by the client
// (x-amz-content-sha256, or UNSIGNED-PAYLOAD / STREAMING markers) is trusted:
// the signature commits to it, so a forged hash cannot be signed without the key.
func verifySigV4(r *http.Request, accessKey, secretKey string) error {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "AWS4-HMAC-SHA256 ") {
		return errNoAuth
	}
	var credential, signedHeaders, signature string
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
	if credential == "" || signedHeaders == "" || signature == "" {
		return errMalformedAuth
	}
	cred := strings.Split(credential, "/")
	if len(cred) != 5 || cred[4] != "aws4_request" || cred[3] != "s3" {
		return errMalformedAuth
	}
	reqAccessKey, dateStamp, region := cred[0], cred[1], cred[2]
	if subtle.ConstantTimeCompare([]byte(reqAccessKey), []byte(accessKey)) != 1 {
		return errInvalidKey
	}
	// The credential scope in the string-to-sign excludes the access key prefix.
	scope := strings.Join(cred[1:], "/")

	amzDate := r.Header.Get("X-Amz-Date")
	if amzDate == "" {
		amzDate = r.Header.Get("Date")
	}
	if amzDate == "" {
		return errStaleDate
	}

	var cr strings.Builder
	cr.WriteString(r.Method)
	cr.WriteByte('\n')
	cr.WriteString(canonicalURI(r))
	cr.WriteByte('\n')
	cr.WriteString(canonicalQueryString(r.URL.Query()))
	cr.WriteByte('\n')
	names := strings.Split(signedHeaders, ";")
	for _, name := range names {
		cr.WriteString(name)
		cr.WriteByte(':')
		cr.WriteString(canonicalHeaderValue(r, name))
		cr.WriteByte('\n')
	}
	cr.WriteByte('\n')
	cr.WriteString(signedHeaders)
	cr.WriteByte('\n')
	cr.WriteString(payloadHash(r))

	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sha256Sum([]byte(cr.String())))
	got := hex.EncodeToString(hmacSHA256(signingKey(secretKey, dateStamp, region, "s3"), []byte(stringToSign)))
	if subtle.ConstantTimeCompare([]byte(got), []byte(signature)) != 1 {
		return errBadSignature
	}
	return nil
}

func canonicalURI(r *http.Request) string {
	if escaped := r.URL.EscapedPath(); escaped != "" {
		return escaped
	}
	return "/"
}

// canonicalQueryString sorts parameters and URI-encodes them per RFC 3986.
func canonicalQueryString(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		for _, v := range q[k] {
			if b.Len() > 0 {
				b.WriteByte('&')
			}
			b.WriteString(uriEncode(k))
			b.WriteByte('=')
			b.WriteString(uriEncode(v))
		}
	}
	return b.String()
}

func canonicalHeaderValue(r *http.Request, name string) string {
	if strings.EqualFold(name, "host") {
		return strings.Join(strings.Fields(r.Host), " ")
	}
	values := r.Header.Values(name)
	for i := range values {
		values[i] = strings.Join(strings.Fields(values[i]), " ")
	}
	return strings.Join(values, ",")
}

func payloadHash(r *http.Request) string {
	if h := r.Header.Get("X-Amz-Content-Sha256"); h != "" {
		if strings.EqualFold(h, "UNSIGNED-PAYLOAD") {
			return "UNSIGNED-PAYLOAD"
		}
		return h
	}
	return "UNSIGNED-PAYLOAD"
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
