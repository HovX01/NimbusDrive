package telegram

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gotd/td/tg"
	"github.com/vrc/nimbus/internal/domain"
)

// ListBots returns bots the signed-in user already has dialogs with (started chats).
// It does not search the whole Telegram network — only bots you've opened before.
func (c *Client) ListBots(ctx context.Context) ([]domain.TelegramBot, error) {
	if ok, err := c.IsAuthorized(ctx); err != nil {
		return nil, err
	} else if !ok {
		return nil, domain.ErrNotAuthenticated
	}
	api, err := c.waitAPI(ctx)
	if err != nil {
		return nil, err
	}

	bots := make(map[int64]*tg.User)
	var (
		offsetDate int
		offsetID   int
		offsetPeer tg.InputPeerClass = &tg.InputPeerEmpty{}
	)
	for page := 0; page < 20; page++ {
		raw, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetDate: offsetDate,
			OffsetID:   offsetID,
			OffsetPeer: offsetPeer,
			Limit:      100,
		})
		if err != nil {
			return nil, err
		}
		users, dialogs, nextDate, nextID, nextPeer, _ := dialogPage(raw)
		for _, u := range users {
			user, ok := u.(*tg.User)
			if !ok || user == nil || !user.Bot || user.Deleted {
				continue
			}
			bots[user.ID] = user
		}
		c.cacheContactUsers(usersFromClass(users))
		if len(dialogs) == 0 || nextPeer == nil {
			break
		}
		if nextDate == offsetDate && nextID == offsetID {
			break
		}
		offsetDate, offsetID, offsetPeer = nextDate, nextID, nextPeer
		if _, ok := offsetPeer.(*tg.InputPeerEmpty); ok && page > 0 {
			break
		}
	}

	out := make([]domain.TelegramBot, 0, len(bots))
	for _, u := range bots {
		out = append(out, userToBot(u))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].DisplayName) < strings.ToLower(out[j].DisplayName)
	})
	return out, nil
}

func (c *Client) IsBot(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, fmt.Errorf("%w: user id required", domain.ErrValidation)
	}
	c.contactsMu.Lock()
	if user, ok := c.contactByID[userID]; ok {
		c.contactsMu.Unlock()
		return user.Bot, nil
	}
	c.contactsMu.Unlock()

	// Force a bot dialog scan so we populate cache for known bots.
	bots, err := c.ListBots(ctx)
	if err != nil {
		return false, err
	}
	for _, b := range bots {
		if b.ID == userID {
			return true, nil
		}
	}
	c.contactsMu.Lock()
	user, ok := c.contactByID[userID]
	c.contactsMu.Unlock()
	if ok {
		return user.Bot, nil
	}
	return false, nil
}

func userToBot(u *tg.User) domain.TelegramBot {
	display := strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
	if display == "" {
		display = u.Username
	}
	if display == "" {
		display = fmt.Sprintf("Bot %d", u.ID)
	}
	_, hasPhoto := u.GetPhoto()
	if hasPhoto {
		if _, ok := u.Photo.(*tg.UserProfilePhoto); !ok {
			hasPhoto = false
		}
	}
	return domain.TelegramBot{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: display,
		HasAvatar:   hasPhoto,
	}
}

func dialogPage(raw tg.MessagesDialogsClass) (users []tg.UserClass, dialogs []tg.DialogClass, offsetDate, offsetID int, offsetPeer tg.InputPeerClass, count int) {
	offsetPeer = &tg.InputPeerEmpty{}
	var chats []tg.ChatClass
	switch d := raw.(type) {
	case *tg.MessagesDialogs:
		users = d.Users
		chats = d.Chats
		dialogs = d.Dialogs
		count = len(d.Dialogs)
	case *tg.MessagesDialogsSlice:
		users = d.Users
		chats = d.Chats
		dialogs = d.Dialogs
		count = d.Count
	default:
		return nil, nil, 0, 0, offsetPeer, 0
	}
	if len(dialogs) == 0 {
		return users, dialogs, 0, 0, offsetPeer, count
	}
	last := dialogs[len(dialogs)-1]
	peer := dialogPeer(last, users, chats)
	msgID, date := dialogTopMessage(last, raw)
	return users, dialogs, date, msgID, peer, count
}

func dialogPeer(d tg.DialogClass, users []tg.UserClass, chats []tg.ChatClass) tg.InputPeerClass {
	var peer tg.PeerClass
	switch v := d.(type) {
	case *tg.Dialog:
		peer = v.Peer
	case *tg.DialogFolder:
		peer = v.Peer
	default:
		return &tg.InputPeerEmpty{}
	}
	return inputPeerFromPeer(peer, users, chats)
}

func dialogTopMessage(d tg.DialogClass, raw tg.MessagesDialogsClass) (msgID, date int) {
	var topID int
	switch v := d.(type) {
	case *tg.Dialog:
		topID = v.TopMessage
	case *tg.DialogFolder:
		topID = v.TopMessage
	}
	var messages []tg.MessageClass
	switch r := raw.(type) {
	case *tg.MessagesDialogs:
		messages = r.Messages
	case *tg.MessagesDialogsSlice:
		messages = r.Messages
	}
	for _, m := range messages {
		msg, ok := m.(*tg.Message)
		if ok && msg.ID == topID {
			return msg.ID, msg.Date
		}
	}
	return topID, 0
}

func inputPeerFromPeer(p tg.PeerClass, users []tg.UserClass, chats []tg.ChatClass) tg.InputPeerClass {
	switch v := p.(type) {
	case *tg.PeerUser:
		for _, u := range users {
			user, ok := u.(*tg.User)
			if ok && user.ID == v.UserID {
				return &tg.InputPeerUser{UserID: user.ID, AccessHash: user.AccessHash}
			}
		}
		return &tg.InputPeerUser{UserID: v.UserID}
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: v.ChatID}
	case *tg.PeerChannel:
		for _, c := range chats {
			ch, ok := c.(*tg.Channel)
			if ok && ch.ID == v.ChannelID {
				return &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}
			}
		}
		return &tg.InputPeerChannel{ChannelID: v.ChannelID}
	default:
		return &tg.InputPeerEmpty{}
	}
}
