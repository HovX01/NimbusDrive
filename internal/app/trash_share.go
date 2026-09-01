package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/vrc/nimbus/internal/domain"
)

// MoveToTrash soft-deletes a node and its descendants (blobs kept until purge).
func (s *Services) MoveToTrash(ctx context.Context, id string) error {
	node, err := s.Nodes.Get(ctx, id)
	if err != nil {
		return err
	}
	if node.ID == "root" {
		return fmt.Errorf("%w: cannot delete root", domain.ErrValidation)
	}
	if node.Type == domain.NodeFolder {
		children, err := s.Nodes.ListChildren(ctx, node.ID)
		if err != nil {
			return err
		}
		for _, c := range children {
			if err := s.MoveToTrash(ctx, c.ID); err != nil {
				return err
			}
		}
	}
	return s.Nodes.SoftDelete(ctx, id)
}

func (s *Services) ListTrash(ctx context.Context) ([]domain.Node, error) {
	return s.Nodes.ListTrashRoots(ctx, 200)
}

func (s *Services) Purge(ctx context.Context, id string) error {
	node, err := s.Nodes.GetDeleted(ctx, id)
	if err != nil {
		return err
	}
	if node.Type == domain.NodeFolder {
		children, err := s.Nodes.ListDeletedChildren(ctx, id)
		if err != nil {
			return err
		}
		for _, c := range children {
			if err := s.Purge(ctx, c.ID); err != nil {
				return err
			}
		}
		return s.hardDeleteNode(ctx, id)
	}
	return s.purgeFile(ctx, id)
}

func (s *Services) EmptyTrash(ctx context.Context) error {
	items, err := s.Nodes.ListTrashRoots(ctx, 500)
	if err != nil {
		return err
	}
	for _, n := range items {
		if err := s.Purge(ctx, n.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Services) purgeFile(ctx context.Context, id string) error {
	parts, err := s.Parts.ListByFile(ctx, id)
	if err != nil {
		return err
	}
	ids := make([]int, 0, len(parts))
	var channelID int64
	for _, p := range parts {
		ids = append(ids, p.MessageID)
		channelID = p.ChannelID
	}
	if len(ids) > 0 {
		_ = s.Blobs.DeleteMessages(ctx, channelID, ids)
	}
	_ = s.Parts.DeleteByFile(ctx, id)
	s.removeThumb(id)
	return s.hardDeleteNode(ctx, id)
}

func (s *Services) hardDeleteNode(ctx context.Context, id string) error {
	if s.Shares != nil {
		if err := s.Shares.DeleteByNode(ctx, id); err != nil {
			return err
		}
	}
	return s.Nodes.HardDelete(ctx, id)
}

func newShareToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type ShareInfo struct {
	Link domain.ShareLink `json:"link"`
	URL  string           `json:"url"`
}

func (s *Services) CreateShare(ctx context.Context, nodeID string, publicBase string) (ShareInfo, error) {
	if s.Shares == nil {
		return ShareInfo{}, domain.ErrNotConfigured
	}
	node, err := s.Nodes.Get(ctx, nodeID)
	if err != nil {
		return ShareInfo{}, err
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady {
		return ShareInfo{}, fmt.Errorf("%w: only ready files can be shared", domain.ErrValidation)
	}
	token, err := newShareToken()
	if err != nil {
		return ShareInfo{}, err
	}
	link, err := s.Shares.CreateShare(ctx, domain.ShareLink{
		ID:     uuid.NewString(),
		Token:  token,
		NodeID: nodeID,
	})
	if err != nil {
		return ShareInfo{}, err
	}
	url := fmt.Sprintf("%s/api/v1/share/%s/download", stringsTrimRight(publicBase), token)
	return ShareInfo{Link: link, URL: url}, nil
}

func (s *Services) ListShares(ctx context.Context, nodeID string, publicBase string) ([]ShareInfo, error) {
	if s.Shares == nil {
		return nil, domain.ErrNotConfigured
	}
	if _, err := s.Nodes.Get(ctx, nodeID); err != nil {
		return nil, err
	}
	links, err := s.Shares.ListByNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	out := make([]ShareInfo, 0, len(links))
	base := stringsTrimRight(publicBase)
	for _, l := range links {
		out = append(out, ShareInfo{
			Link: l,
			URL:  fmt.Sprintf("%s/api/v1/share/%s/download", base, l.Token),
		})
	}
	return out, nil
}

func (s *Services) RevokeShare(ctx context.Context, shareID string) error {
	if s.Shares == nil {
		return domain.ErrNotConfigured
	}
	return s.Shares.Revoke(ctx, shareID)
}

func (s *Services) GetSharedFile(ctx context.Context, token string) (domain.Node, domain.ShareLink, error) {
	if s.Shares == nil {
		return domain.Node{}, domain.ShareLink{}, domain.ErrNotFound
	}
	link, err := s.Shares.GetByToken(ctx, token)
	if err != nil {
		return domain.Node{}, domain.ShareLink{}, err
	}
	if link.RevokedAt != nil {
		return domain.Node{}, domain.ShareLink{}, domain.ErrNotFound
	}
	if link.ExpiresAt != nil && time.Now().After(*link.ExpiresAt) {
		return domain.Node{}, domain.ShareLink{}, domain.ErrNotFound
	}
	node, err := s.Nodes.Get(ctx, link.NodeID)
	if err != nil {
		return domain.Node{}, domain.ShareLink{}, err
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady {
		return domain.Node{}, domain.ShareLink{}, domain.ErrNotFound
	}
	return node, link, nil
}

func (s *Services) IncrementShareDownload(ctx context.Context, shareID string) error {
	if s.Shares == nil {
		return nil
	}
	return s.Shares.IncrementDownload(ctx, shareID)
}

func stringsTrimRight(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '/' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}
