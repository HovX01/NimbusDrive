package domain

import (
	"context"
	"io"
)

// NodeRepository persists drive metadata.
type NodeRepository interface {
	EnsureRoot(ctx context.Context) (Node, error)
	Get(ctx context.Context, id string) (Node, error)
	ListChildren(ctx context.Context, parentID string) ([]Node, error)
	ListReady(ctx context.Context, limit int) ([]Node, error)
	Create(ctx context.Context, node Node) (Node, error)
	UpdateStatus(ctx context.Context, id string, status NodeStatus) error
	UpdateSize(ctx context.Context, id string, size int64) error
	Rename(ctx context.Context, id, name string) error
	Move(ctx context.Context, id, parentID string) error
	SoftDelete(ctx context.Context, id string) error
	GetDeleted(ctx context.Context, id string) (Node, error)
	ListTrashRoots(ctx context.Context, limit int) ([]Node, error)
	ListDeletedChildren(ctx context.Context, parentID string) ([]Node, error)
	HardDelete(ctx context.Context, id string) error
	FindChildByName(ctx context.Context, parentID, name string) (Node, error)
	FindByContentHash(ctx context.Context, hash string) (Node, error)
	SetContentHash(ctx context.Context, nodeID, hash string) error
	Search(ctx context.Context, query string, limit int) ([]SearchHit, error)
}

// PartRepository persists Telegram message mappings for file bytes.
type PartRepository interface {
	ReplaceParts(ctx context.Context, fileID string, parts []FilePart) error
	ListByFile(ctx context.Context, fileID string) ([]FilePart, error)
	DeleteByFile(ctx context.Context, fileID string) error
}

// BlobStore is the object storage port (Telegram channel behind the scenes).
type BlobStore interface {
	EnsureStorageChannel(ctx context.Context) (channelID int64, err error)
	UploadPart(ctx context.Context, channelID int64, partNo int, name string, r io.Reader, size int64) (messageID int, err error)
	DownloadPart(ctx context.Context, channelID int64, messageID int, w io.Writer) error
	DeleteMessages(ctx context.Context, channelID int64, messageIDs []int) error
}

// ShareRepository persists public share links.
type ShareRepository interface {
	CreateShare(ctx context.Context, link ShareLink) (ShareLink, error)
	GetByToken(ctx context.Context, token string) (ShareLink, error)
	ListByNode(ctx context.Context, nodeID string) ([]ShareLink, error)
	Revoke(ctx context.Context, id string) error
	DeleteByNode(ctx context.Context, nodeID string) error
	IncrementDownload(ctx context.Context, id string) error
}

// TelegramAuth handles MTProto user login lifecycle.
type TelegramAuth interface {
	SendCode(ctx context.Context, phone string) (phoneCodeHash string, err error)
	SignIn(ctx context.Context, phone, code, phoneCodeHash, password string) (UserProfile, error)
	CurrentUser(ctx context.Context) (UserProfile, error)
	DownloadAvatar(ctx context.Context, w io.Writer) error
	Logout(ctx context.Context) error
	IsAuthorized(ctx context.Context) (bool, error)
}

// TelegramSetup configures api_id / api_hash from the UI (Telegram-Drive style).
type TelegramSetup interface {
	IsConfigured() bool
	Configure(ctx context.Context, apiID int, apiHash string) error
}

// TelegramMessenger lists contacts and sends files via Telegram DMs.
type TelegramMessenger interface {
	ListContacts(ctx context.Context, query string) ([]TelegramContact, error)
	ListBots(ctx context.Context) ([]TelegramBot, error)
	IsBot(ctx context.Context, userID int64) (bool, error)
	DownloadContactAvatar(ctx context.Context, userID int64, w io.Writer) error
	SendFileToUser(ctx context.Context, userID int64, filename, mimeType string, r io.Reader, size int64) error
}

// BotGrantRepository stores which bots the Nimbus user has allowed.
type BotGrantRepository interface {
	ListBotGrants(ctx context.Context) (map[int64]bool, error)
	SetBotAllowed(ctx context.Context, botID int64, allowed bool) error
	IsBotAllowed(ctx context.Context, botID int64) (bool, error)
}

// DataRepository stores JSON documents in named collections.
type DataRepository interface {
	Insert(ctx context.Context, collection string, data map[string]any) (DataRow, error)
	GetRow(ctx context.Context, collection, id string) (DataRow, error)
	Query(ctx context.Context, collection string, q DataQuery) ([]DataRow, error)
	Update(ctx context.Context, collection, id string, patch map[string]any) (DataRow, error)
	Delete(ctx context.Context, collection, id string) error
	ListCollections(ctx context.Context) ([]string, error)
}

// EditProjectRepository persists video editor timelines.
type EditProjectRepository interface {
	CreateProject(ctx context.Context, p EditProject) (EditProject, error)
	GetProject(ctx context.Context, id string) (EditProject, error)
	GetProjectBySource(ctx context.Context, sourceNodeID string) (EditProject, error)
	ListProjects(ctx context.Context) ([]EditProject, error)
	UpdateTimeline(ctx context.Context, id string, timelineJSON string) error
	DeleteProject(ctx context.Context, id string) error
}

// SocialConnectionRepository persists OAuth tokens for connected media providers.
type SocialConnectionRepository interface {
	ListSocialConnections(ctx context.Context) ([]SocialConnection, error)
	GetActiveSocialConnection(ctx context.Context, provider SocialProvider) (SocialConnectionSecret, error)
	UpsertSocialConnection(ctx context.Context, c SocialConnectionSecret) (SocialConnection, error)
	SetSocialConnectionEnabled(ctx context.Context, provider SocialProvider, enabled bool) (SocialConnection, error)
	SetActiveSocialConnection(ctx context.Context, provider SocialProvider, id string) (SocialConnection, error)
	DeleteSocialConnection(ctx context.Context, provider SocialProvider, id string) error
}

// SocialOAuthConfigRepository persists provider app credentials configured from Settings.
type SocialOAuthConfigRepository interface {
	ListSocialOAuthConfigs(ctx context.Context) ([]SocialOAuthAppConfig, error)
	GetSocialOAuthConfig(ctx context.Context, provider SocialProvider) (SocialOAuthAppConfig, error)
	UpsertSocialOAuthConfig(ctx context.Context, c SocialOAuthAppConfig) (SocialOAuthAppConfig, error)
}
