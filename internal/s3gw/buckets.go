package s3gw

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vrc/nimbus/internal/app"
	"github.com/vrc/nimbus/internal/domain"
)

// BucketInfo is a bucket as seen by S3 clients, with drive-side metadata.
type BucketInfo struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ObjectInfo is one stored object keyed by its full S3 key.
type ObjectInfo struct {
	Key      string    `json:"key"`
	Size     int64     `json:"size"`
	MimeType string    `json:"mime_type"`
	Modified time.Time `json:"modified"`
}

// ListBuckets returns every top-level drive folder, since the gateway serves
// each of them as a bucket.
func ListBuckets(ctx context.Context, svc *app.Services) ([]BucketInfo, error) {
	nodes, err := svc.List(ctx, rootID)
	if err != nil {
		return nil, err
	}
	out := make([]BucketInfo, 0, len(nodes))
	for _, n := range nodes {
		if n.Type != domain.NodeFolder {
			continue
		}
		out = append(out, BucketInfo{Name: n.Name, CreatedAt: n.CreatedAt})
	}
	return out, nil
}

// CreateBucket creates a top-level folder under S3 bucket naming rules.
func CreateBucket(ctx context.Context, svc *app.Services, name string) (BucketInfo, error) {
	name = strings.TrimSpace(name)
	if err := ValidateBucketName(name); err != nil {
		return BucketInfo{}, fmt.Errorf("%w: %v", domain.ErrValidation, err)
	}
	if _, err := svc.Nodes.FindChildByName(ctx, rootID, name); err == nil {
		return BucketInfo{}, fmt.Errorf("%w: bucket %q already exists", domain.ErrConflict, name)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return BucketInfo{}, err
	}
	node, err := svc.Mkdir(ctx, rootID, name)
	if err != nil {
		return BucketInfo{}, err
	}
	return BucketInfo{Name: node.Name, CreatedAt: node.CreatedAt}, nil
}

// ListBucketObjects lists every ready file under a bucket, keyed by full path.
// This is the "did my upload land" check for a bucket.
func ListBucketObjects(ctx context.Context, svc *app.Services, bucket string) ([]ObjectInfo, error) {
	bNode, err := bucketNodeOf(ctx, svc, bucket)
	if err != nil {
		return nil, err
	}
	var out []ObjectInfo
	if err := walkTree(ctx, svc, bNode.ID, "", func(key string, n domain.Node) {
		out = append(out, ObjectInfo{Key: key, Size: n.Size, MimeType: n.MimeType, Modified: n.UpdatedAt})
	}); err != nil {
		return nil, err
	}
	return out, nil
}
