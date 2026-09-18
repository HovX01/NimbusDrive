package httpserver

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/vrc/nimbus/internal/domain"
)

func (s *Server) listSocialConnections(w http.ResponseWriter, r *http.Request) {
	items, err := s.svc.ListSocialConnections(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) listSocialOAuthConfigs(w http.ResponseWriter, r *http.Request) {
	items, err := s.svc.ListSocialOAuthConfigs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) configureSocialOAuthProvider(w http.ResponseWriter, r *http.Request) {
	provider := domain.SocialProvider(chi.URLParam(r, "provider"))
	var body struct {
		ClientID      string `json:"client_id"`
		ClientSecret  string `json:"client_secret"`
		PublicBaseURL string `json:"public_base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	cfg, err := s.svc.ConfigureSocialOAuthProvider(r.Context(), domain.SocialOAuthAppConfig{
		Provider:      provider,
		ClientID:      body.ClientID,
		ClientSecret:  body.ClientSecret,
		PublicBaseURL: body.PublicBaseURL,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) addSocialCookieConnection(w http.ResponseWriter, r *http.Request) {
	provider := domain.SocialProvider(chi.URLParam(r, "provider"))
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	file, _, err := r.FormFile("cookies")
	if err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	defer file.Close()
	c, err := s.svc.AddSocialCookieConnection(r.Context(), provider, r.FormValue("display_name"), file)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) startSocialOAuth(w http.ResponseWriter, r *http.Request) {
	provider := domain.SocialProvider(chi.URLParam(r, "provider"))
	u, err := s.svc.StartSocialOAuth(provider, publicBase(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	if r.URL.Query().Get("redirect") == "1" || r.URL.Query().Get("redirect") == "true" {
		http.Redirect(w, r, u, http.StatusFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": u})
}

func (s *Server) socialOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := domain.SocialProvider(chi.URLParam(r, "provider"))
	if msg := r.URL.Query().Get("error_description"); msg != "" {
		http.Redirect(w, r, "/?social="+string(provider)+"&status=error&message="+url.QueryEscape(msg), http.StatusFound)
		return
	}
	if errCode := r.URL.Query().Get("error"); errCode != "" {
		http.Redirect(w, r, "/?social="+string(provider)+"&status=error&message="+url.QueryEscape(errCode), http.StatusFound)
		return
	}
	if _, err := s.svc.FinishSocialOAuth(r.Context(), provider, r.URL.Query().Get("code"), r.URL.Query().Get("state"), publicBase(r)); err != nil {
		http.Redirect(w, r, "/?social="+string(provider)+"&status=error&message="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/?social="+string(provider)+"&status=connected", http.StatusFound)
}

func (s *Server) patchSocialConnection(w http.ResponseWriter, r *http.Request) {
	provider := domain.SocialProvider(chi.URLParam(r, "provider"))
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Enabled == nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	c, err := s.svc.SetSocialConnectionEnabled(r.Context(), provider, *body.Enabled)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) setActiveSocialConnection(w http.ResponseWriter, r *http.Request) {
	provider := domain.SocialProvider(chi.URLParam(r, "provider"))
	c, err := s.svc.SetActiveSocialConnection(r.Context(), provider, chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) deleteSocialConnection(w http.ResponseWriter, r *http.Request) {
	provider := domain.SocialProvider(chi.URLParam(r, "provider"))
	if err := s.svc.DeleteSocialConnection(r.Context(), provider, chi.URLParam(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}
