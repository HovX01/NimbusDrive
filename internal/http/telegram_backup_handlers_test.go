package httpserver

import (
	"net/http/httptest"
	"testing"
)

func TestIsLoopbackBackupRequest(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       bool
	}{
		{name: "ipv4 loopback", remoteAddr: "127.0.0.1:1234", want: true},
		{name: "ipv6 loopback", remoteAddr: "[::1]:1234", want: true},
		{name: "private address", remoteAddr: "10.0.0.5:1234", want: false},
		{name: "public address", remoteAddr: "203.0.113.7:1234", want: false},
		{name: "loopback behind proxy", remoteAddr: "127.0.0.1:1234", headers: map[string]string{"X-Forwarded-For": "203.0.113.7"}, want: false},
		{name: "loopback with real ip", remoteAddr: "127.0.0.1:1234", headers: map[string]string{"X-Real-IP": "203.0.113.7"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1/backups/telegram/local", nil)
			r.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			if got := isLoopbackBackupRequest(r); got != tt.want {
				t.Fatalf("isLoopbackBackupRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}
