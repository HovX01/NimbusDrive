package domain

import (
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrConflict         = errors.New("conflict")
	ErrValidation       = errors.New("validation")
	ErrNotAuthenticated = errors.New("telegram not authenticated")
	ErrTwoFARequired    = errors.New("two-factor password required")
	ErrNotConfigured    = errors.New("telegram api credentials not configured")
	ErrProviderConfig   = errors.New("provider credentials not configured")
)

type NodeType string

const (
	NodeFile   NodeType = "file"
	NodeFolder NodeType = "folder"
)

type NodeStatus string

const (
	StatusReady   NodeStatus = "ready"
	StatusPending NodeStatus = "pending"
	StatusDeleted NodeStatus = "deleted"
)

// Node is a drive entry. Folders use ParentID for hierarchy; Telegram has no real tree.
type Node struct {
	ID        string     `json:"id"`
	ParentID  *string    `json:"parent_id,omitempty"`
	Name      string     `json:"name"`
	Type      NodeType   `json:"type"`
	MimeType  string     `json:"mime_type"`
	Size      int64      `json:"size"`
	Status    NodeStatus `json:"status"`
	ChannelID int64      `json:"channel_id,omitempty"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	// Duplicate is set on upload/fetch when content already exists (not stored).
	Duplicate bool `json:"duplicate,omitempty"`
}

// PathCrumb is one segment of a drive breadcrumb.
type PathCrumb struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SearchHit is a ranked FTS5 match with a path from My Drive.
type SearchHit struct {
	Node
	Path []PathCrumb `json:"path"`
	Rank float64     `json:"rank"`
}

// FilePart maps one byte range of a file to one Telegram message/document.
type FilePart struct {
	ID        string
	FileID    string
	PartNo    int
	MessageID int
	Size      int64
	ChannelID int64
}

type UserProfile struct {
	TelegramID int64  `json:"telegram_id"`
	Username   string `json:"username"`
	FirstName  string `json:"first_name"`
	Phone      string `json:"phone"`
	HasAvatar  bool   `json:"has_avatar"`
}

// TelegramContact is a Telegram user from the signed-in account's contact list.
type TelegramContact struct {
	ID          int64  `json:"id"`
	Username    string `json:"username,omitempty"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name,omitempty"`
	DisplayName string `json:"display_name"`
	HasAvatar   bool   `json:"has_avatar"`
}

// TelegramBot is a bot the signed-in user has already opened in Telegram.
// Allowed is false until the user opts in (Telegram-style consent).
type TelegramBot struct {
	ID          int64  `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name"`
	HasAvatar   bool   `json:"has_avatar"`
	Allowed     bool   `json:"allowed"`
}

type AuthSession struct {
	Phone         string
	PhoneCodeHash string
}

// ShareLink is a revocable public download token for a file.
type ShareLink struct {
	ID            string     `json:"id"`
	Token         string     `json:"token"`
	NodeID        string     `json:"node_id"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	DownloadCount int        `json:"download_count"`
}

type SocialProvider string

const (
	SocialTikTok    SocialProvider = "tiktok"
	SocialInstagram SocialProvider = "instagram"
	SocialFacebook  SocialProvider = "facebook"
)

type SocialConnection struct {
	ID          string         `json:"id,omitempty"`
	Provider    SocialProvider `json:"provider"`
	AccountID   string         `json:"account_id,omitempty"`
	AccountKey  string         `json:"-"`
	AuthType    string         `json:"auth_type,omitempty"`
	DisplayName string         `json:"display_name,omitempty"`
	Username    string         `json:"username,omitempty"`
	Scopes      []string       `json:"scopes,omitempty"`
	Connected   bool           `json:"connected"`
	Enabled     bool           `json:"enabled"`
	Active      bool           `json:"active"`
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type SocialConnectionSecret struct {
	SocialConnection
	AccessToken  string
	RefreshToken string
	CookiePath   string
}

type SocialOAuthAppConfig struct {
	Provider      SocialProvider `json:"provider"`
	ClientID      string         `json:"client_id,omitempty"`
	ClientSecret  string         `json:"client_secret,omitempty"`
	PublicBaseURL string         `json:"public_base_url,omitempty"`
	Configured    bool           `json:"configured"`
	UpdatedAt     time.Time      `json:"updated_at"`
}
