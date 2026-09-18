package app

import (
	"context"
	"fmt"

	"github.com/vrc/nimbus/internal/domain"
)

func (s *Services) ListBots(ctx context.Context) ([]domain.TelegramBot, error) {
	if s.Messenger == nil {
		return nil, domain.ErrNotConfigured
	}
	bots, err := s.Messenger.ListBots(ctx)
	if err != nil {
		return nil, err
	}
	grants := map[int64]bool{}
	if s.BotGrants != nil {
		grants, err = s.BotGrants.ListBotGrants(ctx)
		if err != nil {
			return nil, err
		}
	}
	for i := range bots {
		bots[i].Allowed = grants[bots[i].ID]
	}
	return bots, nil
}

func (s *Services) SetBotAllowed(ctx context.Context, botID int64, allowed bool) (domain.TelegramBot, error) {
	if s.Messenger == nil || s.BotGrants == nil {
		return domain.TelegramBot{}, domain.ErrNotConfigured
	}
	if botID <= 0 {
		return domain.TelegramBot{}, fmt.Errorf("%w: bot id required", domain.ErrValidation)
	}
	bots, err := s.Messenger.ListBots(ctx)
	if err != nil {
		return domain.TelegramBot{}, err
	}
	var found *domain.TelegramBot
	for i := range bots {
		if bots[i].ID == botID {
			found = &bots[i]
			break
		}
	}
	if found == nil {
		return domain.TelegramBot{}, fmt.Errorf("%w: bot not in your Telegram chats — open it in Telegram first", domain.ErrNotFound)
	}
	if err := s.BotGrants.SetBotAllowed(ctx, botID, allowed); err != nil {
		return domain.TelegramBot{}, err
	}
	found.Allowed = allowed
	return *found, nil
}

func (s *Services) requireBotAllowed(ctx context.Context, userID int64) error {
	if s.Messenger == nil {
		return nil
	}
	isBot, err := s.Messenger.IsBot(ctx, userID)
	if err != nil {
		return err
	}
	if !isBot {
		return nil
	}
	if s.BotGrants == nil {
		return fmt.Errorf("%w: bot access not configured", domain.ErrUnauthorized)
	}
	ok, err := s.BotGrants.IsBotAllowed(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: this bot is blocked — allow it in Settings → Telegram bots", domain.ErrUnauthorized)
	}
	return nil
}
