package s3gw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

func TestSigV4ClientCompatibility(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	creds := aws.Credentials{AccessKeyID: "compat-access", SecretAccessKey: "compat-secret"}
	signer := v4.NewSigner(func(o *v4.SignerOptions) { o.DisableURIPathEscaping = true })
	for _, tc := range []struct {
		name, method, path, body, hash string
		presigned, hashHeader          bool
	}{
		{"hashed GET", "GET", "/bucket/file", "", "", false, true},
		{"headerless GET", "GET", "/bucket/file", "", "", false, false},
		{"headerless PUT", "PUT", "/bucket/file", "upload body", "", false, false},
		{"unsigned headerless GET", "GET", "/bucket/file", "", "UNSIGNED-PAYLOAD", false, false},
		{"unsigned PUT", "PUT", "/bucket/file", "upload body", "UNSIGNED-PAYLOAD", false, true},
		{"reserved path", "GET", "/bucket/a+b:@$", "", "", false, true},
		{"encoded path", "GET", "/bucket/a%2Bb%20%E1%9E%80%252F//x", "", "", false, true},
		{"lowercase escape", "GET", "/bucket/a%2fb", "", "", false, true},
		{"repeated query", "GET", "/bucket?q=z&q=a", "", "", false, true},
		{"query key prefix", "GET", "/bucket?a=x&a-=y", "", "", false, true},
		{"presigned GET", "GET", "/bucket/file", "", "UNSIGNED-PAYLOAD", true, false},
		{"presigned reserved path", "GET", "/bucket/a+b:@$", "", "UNSIGNED-PAYLOAD", true, false},
		{"presigned PUT", "PUT", "/bucket/file", "upload body", "UNSIGNED-PAYLOAD", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := http.NewRequest(tc.method, "http://example.test"+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			hash := tc.hash
			if hash == "" {
				sum := sha256.Sum256([]byte(tc.body))
				hash = hex.EncodeToString(sum[:])
			}
			if tc.hashHeader {
				r.Header.Set("X-Amz-Content-Sha256", hash)
			}
			if tc.presigned {
				q := r.URL.Query()
				q.Set("X-Amz-Expires", "300")
				r.URL.RawQuery = q.Encode()
				uri, _, err := signer.PresignHTTP(context.Background(), creds, r, hash, "s3", "custom-region", time.Now())
				if err != nil {
					t.Fatal(err)
				}
				r.URL, err = url.Parse(uri)
				if err != nil {
					t.Fatal(err)
				}
			} else if err := signer.SignHTTP(context.Background(), creds, r, hash, "s3", "custom-region", time.Now()); err != nil {
				t.Fatal(err)
			}
			if tc.name == "repeated query" {
				r.URL.RawQuery = "q=z&q=a"
			}
			if err := verifySigV4(r, creds.AccessKeyID, creds.SecretAccessKey); err != nil {
				t.Fatal(err)
			}
			if r.Body != nil {
				defer r.Body.Close()
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != tc.body {
					t.Fatalf("body after verification: %q, %v", body, err)
				}
			}
			r.Host = "different.test"
			if err := verifySigV4(r, creds.AccessKeyID, creds.SecretAccessKey); err != errBadSignature {
				t.Fatalf("tampered host accepted: %v", err)
			}
			if r.Body != nil {
				r.Body.Close()
			}
		})
	}
	if files, err := os.ReadDir(tempDir); err != nil || len(files) != 0 {
		t.Fatalf("payload hash left temporary files: %v, %v", files, err)
	}
}

func TestSigV4RejectsInvalidRequests(t *testing.T) {
	creds := aws.Credentials{AccessKeyID: "compat-access", SecretAccessKey: "compat-secret"}
	signer := v4.NewSigner()
	for _, tc := range []struct {
		name, expires string
		age           time.Duration
		presigned     bool
	}{
		{"stale header", "", -16 * time.Minute, false},
		{"future header", "", 16 * time.Minute, false},
		{"expired URL", "60", -2 * time.Minute, true},
		{"zero expiry", "0", 0, true},
		{"excessive expiry", "604801", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "http://example.test/bucket/file", nil)
			if tc.presigned {
				r.URL.RawQuery = "X-Amz-Expires=" + tc.expires
				uri, _, err := signer.PresignHTTP(context.Background(), creds, r, "UNSIGNED-PAYLOAD", "s3", "us-east-1", time.Now().Add(tc.age))
				if err != nil {
					t.Fatal(err)
				}
				r.URL, _ = url.Parse(uri)
			} else if err := signer.SignHTTP(context.Background(), creds, r, "UNSIGNED-PAYLOAD", "s3", "us-east-1", time.Now().Add(tc.age)); err != nil {
				t.Fatal(err)
			}
			if err := verifySigV4(r, creds.AccessKeyID, creds.SecretAccessKey); err != errStaleDate {
				t.Fatalf("got %v, want expired timestamp", err)
			}
		})
	}
	r, _ := http.NewRequest("GET", "http://example.test/bucket/file?X-Amz-Date=x&X-Amz-Date=y&X-Amz-Algorithm=AWS4-HMAC-SHA256", nil)
	if err := verifySigV4(r, creds.AccessKeyID, creds.SecretAccessKey); err != errMalformedAuth {
		t.Fatalf("duplicate auth fields accepted: %v", err)
	}
}
