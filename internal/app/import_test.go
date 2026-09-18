package app

import (
	"testing"
)

func TestValidateImportURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw   string
		ok    bool
		scheme string
	}{
		{"https://example.com/photo.jpg", true, "https"},
		{"http://cdn.example.com/a.png", true, "http"},
		{"ftp://example.com/x", false, ""},
		{"https://localhost/secret", false, ""},
		{"https://127.0.0.1/x", false, ""},
		{"https://10.0.0.1/x", false, ""},
		{"https://192.168.1.1/x", false, ""},
		{"https://169.254.169.254/meta", false, ""},
		{"not-a-url", false, ""},
	}
	for _, tc := range cases {
		u, err := validateImportURL(tc.raw)
		if tc.ok && err != nil {
			t.Fatalf("%q: unexpected error: %v", tc.raw, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%q: expected error", tc.raw)
		}
		if tc.ok && u.Scheme != tc.scheme {
			t.Fatalf("%q: scheme=%s want %s", tc.raw, u.Scheme, tc.scheme)
		}
	}
}
