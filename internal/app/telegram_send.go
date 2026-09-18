package app

import (
	"context"
	"fmt"
	"io"

	"github.com/vrc/nimbus/internal/domain"
)

const telegramMaxSendBytes = 2 * 1024 * 1024 * 1024

func (s *Services) ListContacts(ctx context.Context, query string) ([]domain.TelegramContact, error) {
	if s.Messenger == nil {
		return nil, domain.ErrNotConfigured
	}
	return s.Messenger.ListContacts(ctx, query)
}

func (s *Services) ContactAvatar(ctx context.Context, userID int64, w io.Writer) error {
	if s.Messenger == nil {
		return domain.ErrNotConfigured
	}
	if userID <= 0 {
		return fmt.Errorf("%w: user id required", domain.ErrValidation)
	}
	return s.Messenger.DownloadContactAvatar(ctx, userID, w)
}

func (s *Services) SendToTelegram(ctx context.Context, fileID string, userID int64) error {
	if s.Messenger == nil {
		return domain.ErrNotConfigured
	}
	if userID <= 0 {
		return fmt.Errorf("%w: user_id required", domain.ErrValidation)
	}
	if err := s.requireBotAllowed(ctx, userID); err != nil {
		return err
	}

	node, err := s.Nodes.Get(ctx, fileID)
	if err != nil {
		return err
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady {
		return fmt.Errorf("%w: only ready files can be sent", domain.ErrValidation)
	}
	if node.Size > telegramMaxSendBytes {
		return fmt.Errorf("%w: file exceeds Telegram's 2 GB limit", domain.ErrValidation)
	}

	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		_, err := s.Download(ctx, fileID, pw)
		_ = pw.CloseWithError(err)
		errCh <- err
	}()

	sendErr := s.Messenger.SendFileToUser(ctx, userID, node.Name, node.MimeType, pr, node.Size)
	dlErr := <-errCh
	if sendErr != nil {
		return sendErr
	}
	return dlErr
}
