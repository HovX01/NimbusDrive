package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vrc/nimbus/internal/domain"
)

type SocialOAuthConfig struct {
	PublicBaseURL   string
	TikTokClientKey string
	TikTokSecret    string
	MetaAppID       string
	MetaAppSecret   string
}

type socialProviderConfig struct {
	Provider      domain.SocialProvider
	ClientID      string
	ClientSecret  string
	AuthURL       string
	TokenURL      string
	Scopes        []string
	PublicBaseURL string
}

type socialOAuthState struct {
	Provider domain.SocialProvider
	Expires  time.Time
}

var socialOAuthProviders map[domain.SocialProvider]socialProviderConfig

func (s *Services) ConfigureSocialOAuth(cfg SocialOAuthConfig) {
	base := strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
	s.socialBaseURL = base
	socialOAuthProviders = map[domain.SocialProvider]socialProviderConfig{
		domain.SocialTikTok: {
			Provider:     domain.SocialTikTok,
			ClientID:     cfg.TikTokClientKey,
			ClientSecret: cfg.TikTokSecret,
			AuthURL:      "https://www.tiktok.com/v2/auth/authorize/",
			TokenURL:     "https://open.tiktokapis.com/v2/oauth/token/",
			Scopes:       []string{"user.info.basic", "video.list"},
		},
		domain.SocialInstagram: {
			Provider:     domain.SocialInstagram,
			ClientID:     cfg.MetaAppID,
			ClientSecret: cfg.MetaAppSecret,
			AuthURL:      "https://www.instagram.com/oauth/authorize",
			TokenURL:     "https://api.instagram.com/oauth/access_token",
			Scopes:       []string{"instagram_business_basic"},
		},
		domain.SocialFacebook: {
			Provider:     domain.SocialFacebook,
			ClientID:     cfg.MetaAppID,
			ClientSecret: cfg.MetaAppSecret,
			AuthURL:      "https://www.facebook.com/v21.0/dialog/oauth",
			TokenURL:     "https://graph.facebook.com/v21.0/oauth/access_token",
			Scopes:       []string{"public_profile", "pages_show_list", "pages_read_engagement", "instagram_basic"},
		},
	}
}

func (s *Services) ListSocialConnections(ctx context.Context) ([]domain.SocialConnection, error) {
	items := seededSocialProviders()
	if s.Social == nil {
		return items, nil
	}
	stored, err := s.Social.ListSocialConnections(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[domain.SocialProvider]bool, len(stored))
	out := make([]domain.SocialConnection, 0, len(items)+len(stored))
	for _, c := range stored {
		seen[c.Provider] = true
		out = append(out, c)
	}
	for _, c := range items {
		if !seen[c.Provider] {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *Services) ListSocialOAuthConfigs(ctx context.Context) ([]domain.SocialOAuthAppConfig, error) {
	items := seededSocialOAuthConfigs()
	env := socialOAuthProviders
	for i := range items {
		if cfg, ok := env[items[i].Provider]; ok && cfg.ClientID != "" && cfg.ClientSecret != "" {
			items[i].ClientID = cfg.ClientID
			items[i].ClientSecret = maskedSecret(cfg.ClientSecret)
			items[i].PublicBaseURL = s.socialBaseURL
			items[i].Configured = true
		}
	}
	if s.SocialApp == nil {
		return items, nil
	}
	stored, err := s.SocialApp.ListSocialOAuthConfigs(ctx)
	if err != nil {
		return nil, err
	}
	byProvider := make(map[domain.SocialProvider]domain.SocialOAuthAppConfig, len(stored))
	for _, c := range stored {
		c.ClientSecret = maskedSecret(c.ClientSecret)
		byProvider[c.Provider] = c
	}
	for i := range items {
		if c, ok := byProvider[items[i].Provider]; ok {
			items[i] = c
		}
	}
	return items, nil
}

func (s *Services) ConfigureSocialOAuthProvider(ctx context.Context, c domain.SocialOAuthAppConfig) (domain.SocialOAuthAppConfig, error) {
	if s.SocialApp == nil {
		return domain.SocialOAuthAppConfig{}, domain.ErrNotConfigured
	}
	if _, err := s.socialProvider(c.Provider); err != nil {
		return domain.SocialOAuthAppConfig{}, err
	}
	c.ClientID = strings.TrimSpace(c.ClientID)
	c.ClientSecret = strings.TrimSpace(c.ClientSecret)
	c.PublicBaseURL = strings.TrimRight(strings.TrimSpace(c.PublicBaseURL), "/")
	if c.ClientSecret == "" && s.SocialApp != nil {
		if existing, err := s.SocialApp.GetSocialOAuthConfig(ctx, c.Provider); err == nil {
			c.ClientSecret = existing.ClientSecret
		}
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return domain.SocialOAuthAppConfig{}, fmt.Errorf("%w: app id/key and secret are required", domain.ErrProviderConfig)
	}
	saved, err := s.SocialApp.UpsertSocialOAuthConfig(ctx, c)
	if err != nil {
		return domain.SocialOAuthAppConfig{}, err
	}
	saved.ClientSecret = maskedSecret(saved.ClientSecret)
	return saved, nil
}

func (s *Services) StartSocialOAuth(provider domain.SocialProvider, publicBase string) (string, error) {
	cfg, err := s.configuredSocialProvider(context.Background(), provider)
	if err != nil {
		return "", err
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return "", fmt.Errorf("%w: %s OAuth credentials are missing", domain.ErrProviderConfig, provider)
	}
	state, err := randomState()
	if err != nil {
		return "", err
	}
	s.socialMu.Lock()
	if s.socialStates == nil {
		s.socialStates = map[string]socialOAuthState{}
	}
	s.socialStates[state] = socialOAuthState{Provider: provider, Expires: time.Now().Add(10 * time.Minute)}
	s.socialMu.Unlock()

	q := url.Values{}
	switch provider {
	case domain.SocialTikTok:
		q.Set("client_key", cfg.ClientID)
		q.Set("disable_auto_auth", "1")
	case domain.SocialFacebook:
		q.Set("client_id", cfg.ClientID)
		q.Set("auth_type", "rerequest")
	default:
		q.Set("client_id", cfg.ClientID)
	}
	redirectBase := publicBase
	if cfg.publicBaseURL() != "" {
		redirectBase = cfg.publicBaseURL()
	}
	q.Set("redirect_uri", s.socialRedirectURI(provider, redirectBase))
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(cfg.Scopes, ","))
	q.Set("state", state)
	return cfg.AuthURL + "?" + q.Encode(), nil
}

func (s *Services) FinishSocialOAuth(ctx context.Context, provider domain.SocialProvider, code, state, publicBase string) (domain.SocialConnection, error) {
	if strings.TrimSpace(code) == "" {
		return domain.SocialConnection{}, fmt.Errorf("%w: oauth code required", domain.ErrValidation)
	}
	if !s.consumeSocialState(provider, state) {
		return domain.SocialConnection{}, fmt.Errorf("%w: invalid oauth state", domain.ErrUnauthorized)
	}
	cfg, err := s.configuredSocialProvider(ctx, provider)
	if err != nil {
		return domain.SocialConnection{}, err
	}
	redirectBase := publicBase
	if cfg.publicBaseURL() != "" {
		redirectBase = cfg.publicBaseURL()
	}
	tok, err := exchangeSocialToken(ctx, cfg, code, s.socialRedirectURI(provider, redirectBase))
	if err != nil {
		return domain.SocialConnection{}, err
	}
	if s.Social == nil {
		return domain.SocialConnection{}, domain.ErrNotConfigured
	}
	accountID := tok.AccountID()
	accountKey := accountID
	if accountKey == "" {
		accountKey = tokenAccountKey(provider, tok.AccessToken, tok.RefreshToken)
	}
	profile := s.lookupSocialProfile(ctx, cfg, tok.AccessToken, accountID)
	return s.Social.UpsertSocialConnection(ctx, domain.SocialConnectionSecret{
		SocialConnection: domain.SocialConnection{
			Provider:    provider,
			AccountID:   accountID,
			AccountKey:  accountKey,
			DisplayName: profile.DisplayName,
			Username:    profile.Username,
			Scopes:      tok.Scopes,
			Connected:   true,
			Enabled:     true,
			Active:      true,
			ExpiresAt:   tok.ExpiresAt,
		},
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
	})
}

func (s *Services) SetSocialConnectionEnabled(ctx context.Context, provider domain.SocialProvider, enabled bool) (domain.SocialConnection, error) {
	if s.Social == nil {
		return domain.SocialConnection{}, domain.ErrNotConfigured
	}
	if _, err := s.socialProvider(provider); err != nil {
		return domain.SocialConnection{}, err
	}
	return s.Social.SetSocialConnectionEnabled(ctx, provider, enabled)
}

func (s *Services) SetActiveSocialConnection(ctx context.Context, provider domain.SocialProvider, id string) (domain.SocialConnection, error) {
	if s.Social == nil {
		return domain.SocialConnection{}, domain.ErrNotConfigured
	}
	if _, err := s.socialProvider(provider); err != nil {
		return domain.SocialConnection{}, err
	}
	if strings.TrimSpace(id) == "" {
		return domain.SocialConnection{}, fmt.Errorf("%w: connection id required", domain.ErrValidation)
	}
	return s.Social.SetActiveSocialConnection(ctx, provider, id)
}

func (s *Services) AddSocialCookieConnection(ctx context.Context, provider domain.SocialProvider, displayName string, r io.Reader) (domain.SocialConnection, error) {
	if s.Social == nil {
		return domain.SocialConnection{}, domain.ErrNotConfigured
	}
	if _, err := s.socialProvider(provider); err != nil {
		return domain.SocialConnection{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = providerLabel(provider) + " browser session"
	}
	if strings.TrimSpace(s.DataDir) == "" {
		return domain.SocialConnection{}, domain.ErrNotConfigured
	}
	dir := filepath.Join(s.DataDir, "social-cookies")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return domain.SocialConnection{}, err
	}
	id := randomID()
	path := filepath.Join(dir, string(provider)+"-"+id+".txt")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return domain.SocialConnection{}, err
	}
	h := sha256.New()
	written, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, 2<<20))
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(path)
		return domain.SocialConnection{}, err
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return domain.SocialConnection{}, closeErr
	}
	if written < 20 {
		_ = os.Remove(path)
		return domain.SocialConnection{}, fmt.Errorf("%w: cookies file is empty", domain.ErrValidation)
	}
	accountKey := "cookies:" + hex.EncodeToString(h.Sum(nil))
	return s.Social.UpsertSocialConnection(ctx, domain.SocialConnectionSecret{
		SocialConnection: domain.SocialConnection{
			ID:          id,
			Provider:    provider,
			AccountKey:  accountKey,
			AuthType:    "cookies",
			DisplayName: displayName,
			Connected:   true,
			Enabled:     true,
			Active:      true,
		},
		CookiePath: path,
	})
}

func (s *Services) DeleteSocialConnection(ctx context.Context, provider domain.SocialProvider, id string) error {
	if s.Social == nil {
		return domain.ErrNotConfigured
	}
	if _, err := s.socialProvider(provider); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: connection id required", domain.ErrValidation)
	}
	return s.Social.DeleteSocialConnection(ctx, provider, id)
}

func (s *Services) socialBearerForService(ctx context.Context, service string) string {
	if s.Social == nil {
		return ""
	}
	provider := domain.SocialProvider(service)
	if _, err := s.socialProvider(provider); err != nil {
		return ""
	}
	c, err := s.Social.GetActiveSocialConnection(ctx, provider)
	if err != nil || !c.Enabled || c.AccessToken == "" {
		return ""
	}
	if c.ExpiresAt != nil && time.Now().After(*c.ExpiresAt) {
		return ""
	}
	return c.AccessToken
}

func (s *Services) socialCookiesForService(ctx context.Context, service string) string {
	if s.Social == nil {
		return ""
	}
	provider := domain.SocialProvider(service)
	if _, err := s.socialProvider(provider); err != nil {
		return ""
	}
	c, err := s.Social.GetActiveSocialConnection(ctx, provider)
	if err != nil || !c.Enabled || c.CookiePath == "" {
		return ""
	}
	return c.CookiePath
}

func (s *Services) socialProvider(provider domain.SocialProvider) (socialProviderConfig, error) {
	cfg, ok := socialOAuthProviders[provider]
	if !ok {
		return socialProviderConfig{}, fmt.Errorf("%w: unsupported social provider", domain.ErrValidation)
	}
	return cfg, nil
}

func (s *Services) configuredSocialProvider(ctx context.Context, provider domain.SocialProvider) (socialProviderConfig, error) {
	cfg, err := s.socialProvider(provider)
	if err != nil {
		return socialProviderConfig{}, err
	}
	if s.SocialApp == nil {
		return cfg, nil
	}
	stored, err := s.SocialApp.GetSocialOAuthConfig(ctx, provider)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return cfg, nil
		}
		return socialProviderConfig{}, err
	}
	if stored.ClientID != "" {
		cfg.ClientID = stored.ClientID
	}
	if stored.ClientSecret != "" {
		cfg.ClientSecret = stored.ClientSecret
	}
	if stored.PublicBaseURL != "" {
		cfg.PublicBaseURL = stored.PublicBaseURL
	}
	return cfg, nil
}

func (s *Services) socialRedirectURI(provider domain.SocialProvider, publicBase string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	if s.socialBaseURL != "" {
		base = s.socialBaseURL
	}
	return base + "/api/v1/social/oauth/" + string(provider) + "/callback"
}

func (c socialProviderConfig) publicBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(c.PublicBaseURL), "/")
}

func (s *Services) consumeSocialState(provider domain.SocialProvider, state string) bool {
	s.socialMu.Lock()
	defer s.socialMu.Unlock()
	st, ok := s.socialStates[state]
	if ok {
		delete(s.socialStates, state)
	}
	return ok && st.Provider == provider && time.Now().Before(st.Expires)
}

type socialTokenResponse struct {
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
	OpenID       string     `json:"open_id"`
	UserID       string     `json:"user_id"`
	Scope        string     `json:"scope"`
	Scopes       []string   `json:"scopes"`
	ExpiresIn    int        `json:"expires_in"`
	ExpiresAt    *time.Time `json:"-"`
}

func (t socialTokenResponse) AccountID() string {
	if t.OpenID != "" {
		return t.OpenID
	}
	return t.UserID
}

func exchangeSocialToken(ctx context.Context, cfg socialProviderConfig, code, redirectURI string) (socialTokenResponse, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", redirectURI)
	switch cfg.Provider {
	case domain.SocialTikTok:
		form.Set("client_key", cfg.ClientID)
		form.Set("client_secret", cfg.ClientSecret)
	default:
		form.Set("client_id", cfg.ClientID)
		form.Set("client_secret", cfg.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return socialTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return socialTokenResponse{}, err
	}
	defer res.Body.Close()
	var out socialTokenResponse
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return socialTokenResponse{}, err
	}
	if res.StatusCode >= 400 {
		if msg, _ := body["error_description"].(string); msg != "" {
			return socialTokenResponse{}, fmt.Errorf("oauth token exchange failed: %s", msg)
		}
		return socialTokenResponse{}, fmt.Errorf("oauth token exchange failed: http %d", res.StatusCode)
	}
	b, _ := json.Marshal(body)
	_ = json.Unmarshal(b, &out)
	if out.AccessToken == "" {
		return socialTokenResponse{}, fmt.Errorf("oauth token exchange returned no access token")
	}
	if out.Scope != "" && len(out.Scopes) == 0 {
		out.Scopes = splitOAuthScopes(out.Scope)
	}
	if len(out.Scopes) == 0 {
		out.Scopes = cfg.Scopes
	}
	if out.ExpiresIn > 0 {
		t := time.Now().UTC().Add(time.Duration(out.ExpiresIn) * time.Second)
		out.ExpiresAt = &t
	}
	return out, nil
}

func seededSocialProviders() []domain.SocialConnection {
	now := time.Now().UTC()
	return []domain.SocialConnection{
		{Provider: domain.SocialTikTok, Connected: false, Enabled: false, UpdatedAt: now},
		{Provider: domain.SocialInstagram, Connected: false, Enabled: false, UpdatedAt: now},
		{Provider: domain.SocialFacebook, Connected: false, Enabled: false, UpdatedAt: now},
	}
}

func providerLabel(provider domain.SocialProvider) string {
	switch provider {
	case domain.SocialTikTok:
		return "TikTok"
	case domain.SocialInstagram:
		return "Instagram"
	case domain.SocialFacebook:
		return "Facebook"
	default:
		return string(provider)
	}
}

func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func seededSocialOAuthConfigs() []domain.SocialOAuthAppConfig {
	now := time.Now().UTC()
	return []domain.SocialOAuthAppConfig{
		{Provider: domain.SocialTikTok, Configured: false, UpdatedAt: now},
		{Provider: domain.SocialInstagram, Configured: false, UpdatedAt: now},
		{Provider: domain.SocialFacebook, Configured: false, UpdatedAt: now},
	}
}

func maskedSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if len(secret) <= 6 {
		return "******"
	}
	return secret[:2] + strings.Repeat("*", len(secret)-6) + secret[len(secret)-4:]
}

type socialProfile struct {
	DisplayName string
	Username    string
}

func (s *Services) lookupSocialProfile(ctx context.Context, cfg socialProviderConfig, token, accountID string) socialProfile {
	switch cfg.Provider {
	case domain.SocialFacebook:
		return lookupGraphProfile(ctx, "https://graph.facebook.com/v21.0/me?fields=id,name", token)
	case domain.SocialInstagram:
		return lookupGraphProfile(ctx, "https://graph.instagram.com/me?fields=id,username,account_type", token)
	case domain.SocialTikTok:
		return lookupTikTokProfile(ctx, token)
	default:
		if accountID != "" {
			return socialProfile{DisplayName: accountID}
		}
		return socialProfile{}
	}
}

func lookupGraphProfile(ctx context.Context, endpoint, token string) socialProfile {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return socialProfile{}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return socialProfile{}
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return socialProfile{}
	}
	var body struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return socialProfile{}
	}
	if body.Name == "" {
		body.Name = body.Username
	}
	return socialProfile{DisplayName: body.Name, Username: body.Username}
}

func lookupTikTokProfile(ctx context.Context, token string) socialProfile {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://open.tiktokapis.com/v2/user/info/?fields=open_id,union_id,avatar_url,display_name", nil)
	if err != nil {
		return socialProfile{}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return socialProfile{}
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return socialProfile{}
	}
	var body struct {
		Data struct {
			User struct {
				DisplayName string `json:"display_name"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return socialProfile{}
	}
	return socialProfile{DisplayName: body.Data.User.DisplayName}
}

func tokenAccountKey(provider domain.SocialProvider, accessToken, refreshToken string) string {
	raw := string(provider) + ":" + accessToken + ":" + refreshToken
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func randomState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func splitOAuthScopes(s string) []string {
	f := func(r rune) bool { return r == ',' || r == ' ' }
	parts := strings.FieldsFunc(s, f)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
