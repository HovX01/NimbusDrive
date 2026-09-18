package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/vrc/nimbus/internal/domain"
)

type ctxKey int

const ctxKeyAPIAuth ctxKey = 1

func (s *Server) apiKeyOrSessionRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey != "" {
			if got := extractAPIKey(r); secureEqual(s.apiKey, got) {
				ok, err := s.svc.Authorized(r.Context())
				if err != nil || !ok {
					writeErr(w, fmt.Errorf("%w: telegram session required on server", domain.ErrNotAuthenticated))
					return
				}
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyAPIAuth, true)))
				return
			}
		}
		s.sessionRequired(next).ServeHTTP(w, r)
	})
}

func (s *Server) sessionRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			writeErr(w, domain.ErrUnauthorized)
			return
		}
		if _, err := s.svc.ParseToken(token); err != nil {
			writeErr(w, domain.ErrUnauthorized)
			return
		}
		ok, err := s.svc.Authorized(r.Context())
		if err != nil || !ok {
			writeErr(w, domain.ErrUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken reads JWT from Authorization or ?token= (for <video>/<audio> src).
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func extractAPIKey(r *http.Request) string {
	if k := strings.TrimSpace(r.Header.Get("X-Nimbus-Key")); k != "" {
		return k
	}
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

func apiKeyAuth(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyAPIAuth).(bool)
	return v
}

func wantsPublicURL(r *http.Request) bool {
	if apiKeyAuth(r.Context()) {
		return true
	}
	if q := r.URL.Query().Get("public"); q == "true" || q == "1" {
		return true
	}
	if r.FormValue("public") == "true" || r.FormValue("public") == "1" {
		return true
	}
	return false
}
