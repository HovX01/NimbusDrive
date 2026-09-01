package telegram

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/vrc/nimbus/internal/domain"
)

func (c *Client) ListContacts(ctx context.Context, query string) ([]domain.TelegramContact, error) {
	if ok, err := c.IsAuthorized(ctx); err != nil {
		return nil, err
	} else if !ok {
		return nil, domain.ErrNotAuthenticated
	}
	api, err := c.waitAPI(ctx)
	if err != nil {
		return nil, err
	}

	query = strings.TrimSpace(query)
	var users []tg.UserClass
	if len(query) >= 3 {
		found, err := api.ContactsSearch(ctx, &tg.ContactsSearchRequest{Q: query, Limit: 50})
		if err != nil {
			return nil, err
		}
		users = found.Users
	} else {
		users, err = c.fetchContactUsers(ctx, api)
		if err != nil {
			return nil, err
		}
	}

	out := make([]domain.TelegramContact, 0, len(users))
	seen := make(map[int64]struct{}, len(users))
	cache := make(map[int64]*tg.User, len(users))
	for _, u := range users {
		user, ok := u.(*tg.User)
		if !ok || user == nil {
			continue
		}
		if user.Bot || user.Deleted {
			continue
		}
		if _, dup := seen[user.ID]; dup {
			continue
		}
		seen[user.ID] = struct{}{}
		cache[user.ID] = user
		contact := userToContact(user)
		if query != "" && len(query) < 3 && !contactMatches(contact, query) {
			continue
		}
		out = append(out, contact)
	}
	c.cacheContactUsers(cache)

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].DisplayName) < strings.ToLower(out[j].DisplayName)
	})
	return out, nil
}

func (c *Client) SendFileToUser(ctx context.Context, userID int64, filename, mimeType string, r io.Reader, size int64) error {
	if ok, err := c.IsAuthorized(ctx); err != nil {
		return err
	} else if !ok {
		return domain.ErrNotAuthenticated
	}
	user, err := c.contactUser(ctx, userID)
	if err != nil {
		return err
	}
	api, err := c.waitAPI(ctx)
	if err != nil {
		return err
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	up := uploader.NewUploader(api)
	file, err := up.Upload(ctx, uploader.NewUpload(filename, r, size))
	if err != nil {
		return err
	}
	_, err = api.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer: &tg.InputPeerUser{UserID: user.ID, AccessHash: user.AccessHash},
		Media: &tg.InputMediaUploadedDocument{
			File:     file,
			MimeType: mimeType,
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeFilename{FileName: filename},
			},
		},
		RandomID: newRandomID(),
	})
	return err
}

func (c *Client) fetchContactUsers(ctx context.Context, api *tg.Client) ([]tg.UserClass, error) {
	raw, err := api.ContactsGetContacts(ctx, 0)
	if err != nil {
		return nil, err
	}
	switch v := raw.(type) {
	case *tg.ContactsContacts:
		c.contactsMu.Lock()
		c.contactUsers = append([]tg.UserClass(nil), v.Users...)
		c.contactsMu.Unlock()
		c.cacheContactUsers(usersFromClass(v.Users))
		return v.Users, nil
	case *tg.ContactsContactsNotModified:
		c.contactsMu.Lock()
		users := append([]tg.UserClass(nil), c.contactUsers...)
		c.contactsMu.Unlock()
		if len(users) == 0 {
			return nil, fmt.Errorf("contacts not loaded")
		}
		return users, nil
	default:
		return nil, fmt.Errorf("unexpected contacts response %T", raw)
	}
}

func (c *Client) contactUser(ctx context.Context, userID int64) (*tg.User, error) {
	c.contactsMu.Lock()
	if user, ok := c.contactByID[userID]; ok {
		c.contactsMu.Unlock()
		return user, nil
	}
	c.contactsMu.Unlock()

	api, err := c.waitAPI(ctx)
	if err != nil {
		return nil, err
	}
	users, err := c.fetchContactUsers(ctx, api)
	if err != nil {
		return nil, err
	}
	c.cacheContactUsers(usersFromClass(users))
	c.contactsMu.Lock()
	user, ok := c.contactByID[userID]
	c.contactsMu.Unlock()
	if ok {
		return user, nil
	}
	return nil, fmt.Errorf("%w: contact not found", domain.ErrNotFound)
}

func usersFromClass(users []tg.UserClass) map[int64]*tg.User {
	out := make(map[int64]*tg.User, len(users))
	for _, u := range users {
		user, ok := u.(*tg.User)
		if ok && user != nil {
			out[user.ID] = user
		}
	}
	return out
}

func (c *Client) cacheContactUsers(users map[int64]*tg.User) {
	if len(users) == 0 {
		return
	}
	c.contactsMu.Lock()
	if c.contactByID == nil {
		c.contactByID = make(map[int64]*tg.User, len(users))
	}
	for id, user := range users {
		c.contactByID[id] = user
	}
	c.contactsMu.Unlock()
}

func (c *Client) DownloadContactAvatar(ctx context.Context, userID int64, w io.Writer) error {
	user, err := c.contactUser(ctx, userID)
	if err != nil {
		return err
	}
	photoClass, ok := user.GetPhoto()
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
		Peer:    &tg.InputPeerUser{UserID: user.ID, AccessHash: user.AccessHash},
		PhotoID: up.PhotoID,
	}
	_, err = downloader.NewDownloader().Download(api, loc).Stream(ctx, w)
	return err
}

func userToContact(u *tg.User) domain.TelegramContact {
	display := strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
	if display == "" {
		display = u.Username
	}
	if display == "" {
		display = fmt.Sprintf("User %d", u.ID)
	}
	_, hasPhoto := u.GetPhoto()
	if hasPhoto {
		if _, ok := u.Photo.(*tg.UserProfilePhoto); !ok {
			hasPhoto = false
		}
	}
	return domain.TelegramContact{
		ID:          u.ID,
		Username:    u.Username,
		FirstName:   u.FirstName,
		LastName:    u.LastName,
		DisplayName: display,
		HasAvatar:   hasPhoto,
	}
}

func contactMatches(c domain.TelegramContact, query string) bool {
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(c.DisplayName), q) {
		return true
	}
	if c.Username != "" && strings.Contains(strings.ToLower(c.Username), q) {
		return true
	}
	return strings.Contains(strings.ToLower(c.FirstName), q) || strings.Contains(strings.ToLower(c.LastName), q)
}
