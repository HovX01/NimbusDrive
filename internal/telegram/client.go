package telegram

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/vrc/nimbus/internal/domain"
)

const storageChannelTitle = "Nimbus Storage"
const storageChannelAbout = "nimbus-cloud-storage"

// Client owns a single long-lived MTProto connection (Telegram-Drive runner pattern).
type Client struct {
	apiID   int
	apiHash string
	dataDir string

	mu       sync.Mutex
	parent   context.Context
	client   *telegram.Client
	cancel   context.CancelFunc
	ready    chan struct{}
	runErr   error
	api      *tg.Client
	self     *tg.User
	channel  *tg.Channel
	authFlow pendingAuth

	contactsMu   sync.Mutex
	contactUsers []tg.UserClass
	contactByID  map[int64]*tg.User
}

type pendingAuth struct {
	phone         string
	phoneCodeHash string
}

func New(apiID int, apiHash, dataDir string) *Client {
	c := &Client{
		apiID:   apiID,
		apiHash: strings.TrimSpace(apiHash),
		dataDir: dataDir,
		ready:   make(chan struct{}),
	}
	// Prefer saved UI credentials over empty env.
	if id, hash, ok := c.loadStoredCreds(); ok {
		if c.apiID == 0 {
			c.apiID = id
		}
		if c.apiHash == "" {
			c.apiHash = hash
		}
	}
	return c
}

func (c *Client) sessionPath() string {
	return filepath.Join(c.dataDir, "telegram.session")
}

// Start boots the gotd client in the background and waits until the RPC layer is ready.
// No-op until api_id / api_hash are configured (via env or UI setup).
func (c *Client) Start(parent context.Context) error {
	c.mu.Lock()
	if c.apiID == 0 || c.apiHash == "" {
		c.mu.Unlock()
		return nil
	}
	if c.client != nil {
		c.mu.Unlock()
		return nil
	}
	c.parent = parent
	c.mu.Unlock()
	return c.startLocked(parent)
}

// Configure saves Telegram API credentials (like Telegram-Drive first-run) and starts the client.
func (c *Client) Configure(ctx context.Context, apiID int, apiHash string) error {
	apiHash = strings.TrimSpace(apiHash)
	if apiID <= 0 || apiHash == "" {
		return fmt.Errorf("%w: api_id and api_hash are required", domain.ErrValidation)
	}
	if err := c.saveCreds(apiID, apiHash); err != nil {
		return err
	}

	c.Stop()

	c.mu.Lock()
	c.apiID = apiID
	c.apiHash = apiHash
	c.client = nil
	c.api = nil
	c.self = nil
	c.channel = nil
	c.ready = make(chan struct{})
	parent := c.parent
	if parent == nil {
		parent = ctx
	}
	c.mu.Unlock()

	return c.startLocked(parent)
}

func (c *Client) startLocked(parent context.Context) error {
	c.mu.Lock()
	if c.apiID == 0 || c.apiHash == "" {
		c.mu.Unlock()
		return domain.ErrNotConfigured
	}
	if c.client != nil {
		c.mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	c.ready = make(chan struct{})
	storage := &session.FileStorage{Path: c.sessionPath()}

	client := telegram.NewClient(c.apiID, c.apiHash, telegram.Options{
		SessionStorage: storage,
	})
	c.client = client
	c.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		err := client.Run(ctx, func(ctx context.Context) error {
			api := client.API()
			c.mu.Lock()
			c.api = api
			c.mu.Unlock()
			close(c.ready)

			status, err := client.Auth().Status(ctx)
			if err == nil && status.Authorized {
				if err := c.cacheSelf(ctx); err != nil {
					return err
				}
			}
			<-ctx.Done()
			return ctx.Err()
		})
		c.mu.Lock()
		c.runErr = err
		c.mu.Unlock()
		errCh <- err
	}()

	select {
	case <-c.ready:
		return nil
	case err := <-errCh:
		return err
	case <-parent.Done():
		return parent.Err()
	}
}

func (c *Client) requireStarted(ctx context.Context) error {
	if !c.IsConfigured() {
		return domain.ErrNotConfigured
	}
	c.mu.Lock()
	started := c.client != nil
	parent := c.parent
	c.mu.Unlock()
	if started {
		return nil
	}
	if parent == nil {
		parent = ctx
	}
	return c.startLocked(parent)
}

func (c *Client) Stop() {
	c.mu.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.client = nil
	c.api = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *Client) waitAPI(ctx context.Context) (*tg.Client, error) {
	select {
	case <-c.ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.api == nil {
		if c.runErr != nil {
			return nil, c.runErr
		}
		return nil, fmt.Errorf("telegram api not ready")
	}
	return c.api, nil
}

func (c *Client) cacheSelf(ctx context.Context) error {
	api, err := c.waitAPI(ctx)
	if err != nil {
		return err
	}
	users, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return domain.ErrNotAuthenticated
	}
	u, ok := users[0].(*tg.User)
	if !ok {
		return fmt.Errorf("unexpected self type")
	}
	c.mu.Lock()
	c.self = u
	c.mu.Unlock()
	return nil
}

func (c *Client) IsAuthorized(ctx context.Context) (bool, error) {
	if !c.IsConfigured() {
		return false, nil
	}
	if err := c.requireStarted(ctx); err != nil {
		return false, err
	}
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()
	if client == nil {
		return false, nil
	}
	status, err := client.Auth().Status(ctx)
	if err != nil {
		return false, err
	}
	return status.Authorized, nil
}

func (c *Client) SendCode(ctx context.Context, phone string) (string, error) {
	if err := c.requireStarted(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()
	if client == nil {
		return "", domain.ErrNotConfigured
	}
	sent, err := client.Auth().SendCode(ctx, phone, auth.SendCodeOptions{})
	if err != nil {
		return "", err
	}
	sc, ok := sent.(*tg.AuthSentCode)
	if !ok {
		return "", fmt.Errorf("unexpected sent code type %T", sent)
	}
	hash := sc.PhoneCodeHash
	c.mu.Lock()
	c.authFlow = pendingAuth{phone: phone, phoneCodeHash: hash}
	c.mu.Unlock()
	return hash, nil
}

func (c *Client) SignIn(ctx context.Context, phone, code, phoneCodeHash, password string) (domain.UserProfile, error) {
	if err := c.requireStarted(ctx); err != nil {
		return domain.UserProfile{}, err
	}
	c.mu.Lock()
	client := c.client
	if phone == "" {
		phone = c.authFlow.phone
	}
	if phoneCodeHash == "" {
		phoneCodeHash = c.authFlow.phoneCodeHash
	}
	c.mu.Unlock()
	if client == nil {
		return domain.UserProfile{}, domain.ErrNotConfigured
	}

	_, err := client.Auth().SignIn(ctx, phone, code, phoneCodeHash)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordAuthNeeded) {
			if password == "" {
				return domain.UserProfile{}, domain.ErrTwoFARequired
			}
			if _, err := client.Auth().Password(ctx, password); err != nil {
				return domain.UserProfile{}, err
			}
		} else {
			return domain.UserProfile{}, err
		}
	}
	if err := c.cacheSelf(ctx); err != nil {
		return domain.UserProfile{}, err
	}
	return c.CurrentUser(ctx)
}

func (c *Client) CurrentUser(ctx context.Context) (domain.UserProfile, error) {
	c.mu.Lock()
	self := c.self
	c.mu.Unlock()
	if self == nil {
		if err := c.cacheSelf(ctx); err != nil {
			return domain.UserProfile{}, err
		}
		c.mu.Lock()
		self = c.self
		c.mu.Unlock()
	}
	if self == nil {
		return domain.UserProfile{}, domain.ErrNotAuthenticated
	}
	_, hasPhoto := self.GetPhoto()
	if hasPhoto {
		if _, ok := self.Photo.(*tg.UserProfilePhoto); !ok {
			hasPhoto = false
		}
	}
	return domain.UserProfile{
		TelegramID: self.ID,
		Username:   self.Username,
		FirstName:  self.FirstName,
		Phone:      self.Phone,
		HasAvatar:  hasPhoto,
	}, nil
}

func (c *Client) DownloadAvatar(ctx context.Context, w io.Writer) error {
	if err := c.cacheSelf(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	self := c.self
	c.mu.Unlock()
	if self == nil {
		return domain.ErrNotAuthenticated
	}
	photoClass, ok := self.GetPhoto()
	if !ok {
		return domain.ErrNotFound
	}
	up, ok := photoClass.(*tg.UserProfilePhoto)
	if !ok {
		return domain.ErrNotFound
	}
	api, err := c.waitAPI(ctx)
	if err != nil {
		return err
	}
	loc := &tg.InputPeerPhotoFileLocation{
		Peer:    &tg.InputPeerSelf{},
		PhotoID: up.PhotoID,
	}
	_, err = downloader.NewDownloader().Download(api, loc).Stream(ctx, w)
	return err
}

func (c *Client) Logout(ctx context.Context) error {
	// Soft logout: keep telegram.session so restart / next visit does not need a new OTP.
	// Full Telegram disconnect is AuthDisconnect.
	c.mu.Lock()
	c.self = nil
	c.channel = nil
	c.mu.Unlock()
	return nil
}

// AuthDisconnect signs out of Telegram and deletes the saved session (requires OTP next time).
func (c *Client) AuthDisconnect(ctx context.Context) error {
	api, err := c.waitAPI(ctx)
	if err != nil {
		return err
	}
	_, err = api.AuthLogOut(ctx)
	c.mu.Lock()
	c.self = nil
	c.channel = nil
	c.mu.Unlock()
	_ = os.Remove(c.sessionPath())
	return err
}

func (c *Client) EnsureStorageChannel(ctx context.Context) (int64, error) {
	if ok, err := c.IsAuthorized(ctx); err != nil {
		return 0, err
	} else if !ok {
		return 0, domain.ErrNotAuthenticated
	}
	api, err := c.waitAPI(ctx)
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
	if c.channel != nil {
		id := c.channel.ID
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	dialogs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      100,
	})
	if err != nil {
		return 0, err
	}
	if ch := findNimbusChannel(dialogs); ch != nil {
		c.mu.Lock()
		c.channel = ch
		c.mu.Unlock()
		return ch.ID, nil
	}

	u, err := api.ChannelsCreateChannel(ctx, &tg.ChannelsCreateChannelRequest{
		Title:     storageChannelTitle,
		About:     storageChannelAbout,
		Broadcast: true,
	})
	if err != nil {
		return 0, err
	}
	ch, err := channelFromUpdates(u)
	if err != nil {
		return 0, err
	}
	c.mu.Lock()
	c.channel = ch
	c.mu.Unlock()
	return ch.ID, nil
}

func (c *Client) UploadPart(ctx context.Context, channelID int64, partNo int, name string, r io.Reader, size int64) (int, error) {
	api, err := c.waitAPI(ctx)
	if err != nil {
		return 0, err
	}
	ch, err := c.inputChannel(ctx, channelID)
	if err != nil {
		return 0, err
	}
	up := uploader.NewUploader(api)
	file, err := up.Upload(ctx, uploader.NewUpload(fmt.Sprintf("%s.part%04d", name, partNo), r, size))
	if err != nil {
		return 0, err
	}
	updates, err := api.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer: &tg.InputPeerChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
		Media: &tg.InputMediaUploadedDocument{
			File:     file,
			MimeType: "application/octet-stream",
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeFilename{FileName: fmt.Sprintf("%s.part%04d", name, partNo)},
			},
		},
		Message:  fmt.Sprintf("nimbus-part:%d", partNo),
		RandomID: newRandomID(),
	})
	if err != nil {
		return 0, err
	}
	return messageIDFromUpdates(updates)
}

func newRandomID() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return int64(binary.LittleEndian.Uint64(b[:]) & 0x7fffffffffffffff)
}

func (c *Client) DownloadPart(ctx context.Context, channelID int64, messageID int, w io.Writer) error {
	api, err := c.waitAPI(ctx)
	if err != nil {
		return err
	}
	ch, err := c.inputChannel(ctx, channelID)
	if err != nil {
		return err
	}
	msgs, err := api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: ch,
		ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: messageID}},
	})
	if err != nil {
		return err
	}
	doc, err := documentFromMessages(msgs)
	if err != nil {
		return err
	}
	_, err = downloader.NewDownloader().Download(api, doc.AsInputDocumentFileLocation()).Stream(ctx, w)
	return err
}

func (c *Client) DeleteMessages(ctx context.Context, channelID int64, messageIDs []int) error {
	if len(messageIDs) == 0 {
		return nil
	}
	api, err := c.waitAPI(ctx)
	if err != nil {
		return err
	}
	ch, err := c.inputChannel(ctx, channelID)
	if err != nil {
		return err
	}
	ids := make([]int, len(messageIDs))
	copy(ids, messageIDs)
	_, err = api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
		Channel: ch,
		ID:      ids,
	})
	return err
}

func (c *Client) inputChannel(ctx context.Context, channelID int64) (*tg.InputChannel, error) {
	c.mu.Lock()
	ch := c.channel
	c.mu.Unlock()
	if ch != nil && ch.ID == channelID {
		return &tg.InputChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}, nil
	}
	// Refresh by ensuring storage channel (single-channel MVP).
	id, err := c.EnsureStorageChannel(ctx)
	if err != nil {
		return nil, err
	}
	if id != channelID {
		return nil, fmt.Errorf("unknown channel %d", channelID)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return &tg.InputChannel{ChannelID: c.channel.ID, AccessHash: c.channel.AccessHash}, nil
}

func findNimbusChannel(dialogs tg.MessagesDialogsClass) *tg.Channel {
	var chats []tg.ChatClass
	switch d := dialogs.(type) {
	case *tg.MessagesDialogs:
		chats = d.Chats
	case *tg.MessagesDialogsSlice:
		chats = d.Chats
	default:
		return nil
	}
	for _, chat := range chats {
		ch, ok := chat.(*tg.Channel)
		if !ok {
			continue
		}
		if ch.Title == storageChannelTitle {
			return ch
		}
	}
	return nil
}

func channelFromUpdates(u tg.UpdatesClass) (*tg.Channel, error) {
	var chats []tg.ChatClass
	switch v := u.(type) {
	case *tg.Updates:
		chats = v.Chats
	case *tg.UpdatesCombined:
		chats = v.Chats
	default:
		return nil, fmt.Errorf("unexpected updates type %T", u)
	}
	for _, chat := range chats {
		if ch, ok := chat.(*tg.Channel); ok {
			return ch, nil
		}
	}
	return nil, fmt.Errorf("channel missing from create response")
}

func messageIDFromUpdates(u tg.UpdatesClass) (int, error) {
	var updates []tg.UpdateClass
	switch v := u.(type) {
	case *tg.Updates:
		updates = v.Updates
	case *tg.UpdatesCombined:
		updates = v.Updates
	case *tg.UpdateShortSentMessage:
		return v.ID, nil
	default:
		return 0, fmt.Errorf("message id not found in send response (%T)", u)
	}
	for _, up := range updates {
		switch m := up.(type) {
		case *tg.UpdateNewChannelMessage:
			if msg, ok := m.Message.(*tg.Message); ok {
				return msg.ID, nil
			}
		case *tg.UpdateNewMessage:
			if msg, ok := m.Message.(*tg.Message); ok {
				return msg.ID, nil
			}
		}
	}
	return 0, fmt.Errorf("message id not found in send response")
}

func documentFromMessages(msgs tg.MessagesMessagesClass) (*tg.Document, error) {
	var list []tg.MessageClass
	switch m := msgs.(type) {
	case *tg.MessagesChannelMessages:
		list = m.Messages
	case *tg.MessagesMessages:
		list = m.Messages
	case *tg.MessagesMessagesSlice:
		list = m.Messages
	default:
		return nil, fmt.Errorf("unexpected messages type %T", msgs)
	}
	if len(list) == 0 {
		return nil, domain.ErrNotFound
	}
	msg, ok := list[0].(*tg.Message)
	if !ok {
		return nil, domain.ErrNotFound
	}
	media, ok := msg.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil, fmt.Errorf("message has no document")
	}
	doc, ok := media.Document.(*tg.Document)
	if !ok {
		return nil, fmt.Errorf("empty document")
	}
	return doc, nil
}

var (
	_ domain.BlobStore        = (*Client)(nil)
	_ domain.TelegramAuth     = (*Client)(nil)
	_ domain.TelegramMessenger = (*Client)(nil)
)
