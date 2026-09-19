package s3gw

import (
	"context"
	"errors"
	"strings"

	"github.com/vrc/nimbus/internal/app"
	"github.com/vrc/nimbus/internal/domain"
)

func splitFirst(p string) (string, string) {
	if i := strings.Index(p, "/"); i >= 0 {
		return p[:i], p[i+1:]
	}
	return p, ""
}

// splitKey splits an object key into its folder path and file name.
func splitKey(key string) (string, string) {
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[:i], key[i+1:]
	}
	return "", key
}

func splitSegments(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

// splitPrefix turns a list prefix into the folder it resolves to plus the
// remaining literal name filter (only used with a delimiter).
func splitPrefix(prefix string) (folderKey, literal string) {
	if prefix == "" {
		return "", ""
	}
	if strings.HasSuffix(prefix, "/") {
		return strings.TrimSuffix(prefix, "/"), ""
	}
	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		return prefix[:i], prefix[i+1:]
	}
	return "", prefix
}

func joinKey(folder, name string) string {
	if folder == "" {
		return name
	}
	return folder + "/" + name
}

// bucketNodeOf resolves a bucket name to its top-level folder node.
func bucketNodeOf(ctx context.Context, svc *app.Services, bucket string) (domain.Node, error) {
	node, err := svc.Nodes.FindChildByName(ctx, rootID, bucket)
	if err != nil {
		return domain.Node{}, err
	}
	if node.Type != domain.NodeFolder {
		return domain.Node{}, domain.ErrNotFound
	}
	return node, nil
}

// resolveFolder walks an existing folder path, failing if any segment is missing.
func (s *Server) resolveFolder(ctx context.Context, startID, path string) (domain.Node, error) {
	node, err := s.svc.Nodes.Get(ctx, startID)
	if err != nil {
		return domain.Node{}, err
	}
	for _, seg := range splitSegments(path) {
		child, err := s.svc.Nodes.FindChildByName(ctx, node.ID, seg)
		if err != nil {
			return domain.Node{}, err
		}
		if child.Type != domain.NodeFolder {
			return domain.Node{}, domain.ErrNotFound
		}
		node = child
	}
	return node, nil
}

// ensureFolderPath creates missing folders like mkdir -p and returns the leaf.
func (s *Server) ensureFolderPath(ctx context.Context, startID, path string) (domain.Node, error) {
	node, err := s.svc.Nodes.Get(ctx, startID)
	if err != nil {
		return domain.Node{}, err
	}
	for _, seg := range splitSegments(path) {
		child, err := s.svc.Nodes.FindChildByName(ctx, node.ID, seg)
		switch {
		case err == nil:
			if child.Type != domain.NodeFolder {
				return domain.Node{}, errors.New("path segment is not a folder: " + seg)
			}
			node = child
		case errors.Is(err, domain.ErrNotFound):
			child, err = s.svc.Mkdir(ctx, node.ID, seg)
			if err != nil {
				return domain.Node{}, err
			}
			node = child
		default:
			return domain.Node{}, err
		}
	}
	return node, nil
}

// lookupObject resolves a full object key to its node (file or folder).
func (s *Server) lookupObject(ctx context.Context, bucket, key string) (domain.Node, error) {
	bNode, err := bucketNodeOf(ctx, s.svc, bucket)
	if err != nil {
		return domain.Node{}, err
	}
	folderPath, name := splitKey(key)
	folder, err := s.resolveFolder(ctx, bNode.ID, folderPath)
	if err != nil {
		return domain.Node{}, err
	}
	if name == "" {
		return folder, nil
	}
	return s.svc.Nodes.FindChildByName(ctx, folder.ID, name)
}

// walkTree visits every ready file under nodeID, reporting full keys.
func walkTree(ctx context.Context, svc *app.Services, nodeID, keyPrefix string, fn func(key string, n domain.Node)) error {
	children, err := svc.List(ctx, nodeID)
	if err != nil {
		return err
	}
	for _, c := range children {
		key := joinKey(keyPrefix, c.Name)
		if c.Type == domain.NodeFolder {
			if err := walkTree(ctx, svc, c.ID, key, fn); err != nil {
				return err
			}
			continue
		}
		if c.Status != domain.StatusReady {
			continue
		}
		fn(key, c)
	}
	return nil
}
