package s3gw

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	minio "github.com/minio/minio-go/v7"
	"github.com/vrc/nimbus/internal/domain"
)

func awsClient(ts *httptest.Server) *awss3.Client {
	return awss3.NewFromConfig(aws.Config{
		Region: "us-east-1", Credentials: awscreds.NewStaticCredentialsProvider(testAccessKey, testSecretKey, ""),
	}, func(o *awss3.Options) {
		o.BaseEndpoint = &ts.URL
		o.UsePathStyle = true
		o.RetryMaxAttempts = 1
	})
}

func TestGatewayEmptyObjectsAndDirectoryMarkers(t *testing.T) {
	ctx := context.Background()
	svc, ts, _ := newTestServer(t)
	client := minioClient(t, ts)
	if err := client.MakeBucket(ctx, "arcane", minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, body string }{
		{"arcane-system-recovery/keys/", ""},
		{"arcane-system-recovery/empty", ""},
		{"arcane-system-recovery/nonempty/", "marker contents"},
		{"arcane-system-recovery/keys/key-id", "encrypted key"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			if _, err := client.PutObject(ctx, "arcane", tc.key, strings.NewReader(tc.body), int64(len(tc.body)), minio.PutObjectOptions{}); err != nil {
				t.Fatal(err)
			}
			info, err := client.StatObject(ctx, "arcane", tc.key, minio.StatObjectOptions{})
			if err != nil || info.Size != int64(len(tc.body)) {
				t.Fatalf("HEAD: %+v, %v", info, err)
			}
			obj, err := client.GetObject(ctx, "arcane", tc.key, minio.GetObjectOptions{})
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(obj)
			obj.Close()
			if err != nil || string(body) != tc.body {
				t.Fatalf("GET: %q, %v", body, err)
			}
		})
	}
	if _, err := client.StatObject(ctx, "arcane", "arcane-system-recovery/keys", minio.StatObjectOptions{}); err == nil {
		t.Fatal("trailing slash aliased to key without slash")
	}
	objects, err := ListBucketObjects(ctx, svc, "arcane")
	if err != nil || len(objects) != 4 {
		t.Fatalf("management listing: %+v, %v", objects, err)
	}
	if err := client.RemoveObject(ctx, "arcane", "arcane-system-recovery/keys/", minio.RemoveObjectOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.StatObject(ctx, "arcane", "arcane-system-recovery/keys/", minio.StatObjectOptions{}); err == nil {
		t.Fatal("deleted marker still exists")
	}
	if _, err := client.StatObject(ctx, "arcane", "arcane-system-recovery/keys/key-id", minio.StatObjectOptions{}); err != nil {
		t.Fatalf("marker deletion removed child: %v", err)
	}
	if err := client.RemoveObject(ctx, "arcane", "arcane-system-recovery/keys", minio.RemoveObjectOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.StatObject(ctx, "arcane", "arcane-system-recovery/keys/key-id", minio.StatObjectOptions{}); err != nil {
		t.Fatalf("implicit folder deletion removed child: %v", err)
	}
	root, err := svc.Nodes.FindChildByName(ctx, rootID, "arcane")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Upload(ctx, root.ID, "ui-empty", "text/plain", strings.NewReader(""), 0); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("drive upload validation changed: %v", err)
	}
	if _, err := awsClient(ts).GetObject(ctx, &awss3.GetObjectInput{
		Bucket: ptr("arcane"), Key: ptr("arcane-system-recovery/empty"), Range: ptr("bytes=0-0"),
	}); err == nil {
		t.Fatal("range on empty object succeeded")
	}
}

func TestGatewayEmptyObjectNeedsNoTelegram(t *testing.T) {
	ctx := context.Background()
	svc, ts, _ := newTestServer(t)
	client := minioClient(t, ts)
	if err := client.MakeBucket(ctx, "offline", minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	svc.Blobs = nil
	if _, err := client.PutObject(ctx, "offline", "keys/", strings.NewReader(""), 0, minio.PutObjectOptions{}); err != nil {
		t.Fatalf("empty marker called Telegram: %v", err)
	}
}

func TestGatewayStagedUploadsAreHidden(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestServer(t)
	if _, err := CreateBucket(ctx, svc, "hidden"); err != nil {
		t.Fatal(err)
	}
	bucket, err := bucketNodeOf(ctx, svc, "hidden")
	if err != nil {
		t.Fatal(err)
	}
	node, err := svc.StageObject(ctx, bucket.ID, "staged", "text/plain", strings.NewReader("data"))
	if err != nil || node.Status != domain.StatusPending {
		t.Fatalf("stage: %+v, %v", node, err)
	}
	objects, err := ListBucketObjects(ctx, svc, "hidden")
	if err != nil || len(objects) != 0 {
		t.Fatalf("unpublished upload listed: %+v, %v", objects, err)
	}
	if err := svc.Nodes.Replace(ctx, node.ID, "published"); err != nil {
		t.Fatal(err)
	}
	objects, err = ListBucketObjects(ctx, svc, "hidden")
	if err != nil || len(objects) != 1 || objects[0].Key != "published" {
		t.Fatalf("published upload missing: %+v, %v", objects, err)
	}
}

type pausedListing struct {
	domain.NodeRepository
	bucketID         string
	entered, release chan struct{}
	once             sync.Once
}

func (p *pausedListing) ListChildren(ctx context.Context, id string) ([]domain.Node, error) {
	children, err := p.NodeRepository.ListChildren(ctx, id)
	if id == p.bucketID {
		p.once.Do(func() { close(p.entered); <-p.release })
	}
	return children, err
}

func TestGatewayBucketDeletionExcludesConcurrentPut(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	svc, ts, _ := newTestServer(t)
	client := awsClient(ts)
	bucket := ptr("racing")
	if _, err := client.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: bucket}); err != nil {
		t.Fatal(err)
	}
	node, err := bucketNodeOf(ctx, svc, *bucket)
	if err != nil {
		t.Fatal(err)
	}
	pause := &pausedListing{NodeRepository: svc.Nodes, bucketID: node.ID, entered: make(chan struct{}), release: make(chan struct{})}
	svc.Nodes = pause
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(pause.release) }) }
	t.Cleanup(release)
	deleted := make(chan error, 1)
	go func() {
		_, err := client.DeleteBucket(ctx, &awss3.DeleteBucketInput{Bucket: bucket})
		deleted <- err
	}()
	select {
	case <-pause.entered:
	case <-ctx.Done():
		t.Fatal("delete did not reach emptiness check")
	}
	put := make(chan error, 1)
	go func() {
		_, err := client.PutObject(ctx, &awss3.PutObjectInput{Bucket: bucket, Key: ptr("must-not-disappear"), Body: strings.NewReader("data")})
		put <- err
	}()
	select {
	case err := <-put:
		t.Fatalf("PUT entered bucket removal window: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	release()
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	if err := <-put; err == nil {
		t.Fatal("PUT succeeded into deleted bucket")
	}
}

func TestGatewayUnsupportedSubresourcesKeepData(t *testing.T) {
	ctx := context.Background()
	_, ts, _ := newTestServer(t)
	client := awsClient(ts)
	bucket, key := ptr("subresources"), ptr("saved")
	if _, err := client.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: bucket}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutObject(ctx, &awss3.PutObjectInput{Bucket: bucket, Key: key, Body: strings.NewReader("keep")}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutObjectAcl(ctx, &awss3.PutObjectAclInput{Bucket: bucket, Key: key, ACL: types.ObjectCannedACLPrivate}); err == nil || !strings.Contains(err.Error(), "NotImplemented") {
		t.Fatalf("PUT ACL: %v", err)
	}
	if _, err := client.DeleteObjectTagging(ctx, &awss3.DeleteObjectTaggingInput{Bucket: bucket, Key: key}); err == nil || !strings.Contains(err.Error(), "NotImplemented") {
		t.Fatalf("DELETE tagging: %v", err)
	}
	if _, err := client.DeleteBucketLifecycle(ctx, &awss3.DeleteBucketLifecycleInput{Bucket: bucket}); err == nil || !strings.Contains(err.Error(), "NotImplemented") {
		t.Fatalf("DELETE lifecycle: %v", err)
	}
	obj, err := client.GetObject(ctx, &awss3.GetObjectInput{Bucket: bucket, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(obj.Body)
	obj.Body.Close()
	if err != nil || string(body) != "keep" {
		t.Fatalf("subresource request changed object: %q, %v", body, err)
	}
}

func TestObjectKeyValidation(t *testing.T) {
	for _, key := range []string{"keys/", "nested/file", "a +ក.txt"} {
		if err := validateObjectKey(key); err != nil {
			t.Fatalf("valid key %q: %v", key, err)
		}
	}
	for _, key := range []string{"", "/file", "folder//file", "a/../b", "a\\b", string([]byte{0xff}), strings.Repeat("x/", 600)} {
		if err := validateObjectKey(key); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("invalid key %q: %v", key, err)
		}
	}
}

func TestGatewayListPagination(t *testing.T) {
	ctx := context.Background()
	_, ts, _ := newTestServer(t)
	client := awsClient(ts)
	if _, err := client.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: ptr("pages")}); err != nil {
		t.Fatal(err)
	}
	keys := []string{"group/a", "group/b", "root.txt", "z ?+.txt", "zz.txt", "ក.txt"}
	for _, key := range keys {
		if _, err := client.PutObject(ctx, &awss3.PutObjectInput{Bucket: ptr("pages"), Key: ptr(key), Body: strings.NewReader(key)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, delimiter := range []string{"", "/", "r"} {
		var token *string
		seen := map[string]bool{}
		for page := 0; ; page++ {
			if page > 10 {
				t.Fatal("pagination failed to advance")
			}
			out, err := client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{
				Bucket: ptr("pages"), MaxKeys: aws.Int32(1), Delimiter: ptr(delimiter), ContinuationToken: token,
				EncodingType: types.EncodingTypeUrl,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Contents)+len(out.CommonPrefixes) != 1 || aws.ToInt32(out.KeyCount) != 1 {
				t.Fatalf("max-keys ignored: %+v", out)
			}
			for _, obj := range out.Contents {
				key := aws.ToString(obj.Key)
				if seen[key] {
					t.Fatalf("duplicate key %q", key)
				}
				seen[key] = true
			}
			for _, prefix := range out.CommonPrefixes {
				key := aws.ToString(prefix.Prefix)
				if seen[key] {
					t.Fatalf("duplicate prefix %q", key)
				}
				seen[key] = true
			}
			if !aws.ToBool(out.IsTruncated) {
				break
			}
			token = out.NextContinuationToken
			if aws.ToString(token) == "" {
				t.Fatal("truncated page missing continuation token")
			}
		}
		if delimiter == "" && len(seen) != len(keys) {
			t.Fatalf("lost keys: %+v", seen)
		}
	}
	out, err := client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{Bucket: ptr("pages"), StartAfter: ptr("root.txt"), MaxKeys: aws.Int32(0)})
	if err != nil || len(out.Contents) != 0 || len(out.CommonPrefixes) != 0 || aws.ToBool(out.IsTruncated) {
		t.Fatalf("max-keys=0: %+v, %v", out, err)
	}
	out, err = client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{Bucket: ptr("pages"), StartAfter: ptr("root.txt")})
	if err != nil || len(out.Contents) != 3 {
		t.Fatalf("start-after: %+v, %v", out, err)
	}
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, errors.New("upload interrupted") }

type failedReplace struct{ domain.NodeRepository }

func (failedReplace) Replace(context.Context, string, string) error {
	return errors.New("publish interrupted")
}

func TestGatewayFailedOverwriteKeepsObject(t *testing.T) {
	ctx := context.Background()
	svc, ts, _ := newTestServer(t)
	client := minioClient(t, ts)
	if err := client.MakeBucket(ctx, "atomic", minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	server := New(svc, Config{}, svc.DataDir)
	if _, err := server.storeObject(ctx, "atomic", "config", "text/plain", strings.NewReader("keep me")); err != nil {
		t.Fatal(err)
	}
	if _, err := server.storeObject(ctx, "atomic", "config", "text/plain", failedReader{}); err == nil {
		t.Fatal("broken upload succeeded")
	}
	svc.ChunkSize = 4
	if _, err := server.storeObject(ctx, "atomic", "config", "text/plain", io.MultiReader(strings.NewReader("partial replacement"), failedReader{})); err == nil {
		t.Fatal("interrupted streaming upload succeeded")
	}
	blobs := svc.Blobs.(*fakeBlobs)
	blobs.mu.Lock()
	remaining := len(blobs.msgs[1])
	blobs.mu.Unlock()
	if remaining != 1 {
		t.Fatalf("failed upload left %d blob parts, want original part only", remaining)
	}
	node, err := server.lookupObject(ctx, "atomic", "config")
	if err != nil {
		t.Fatalf("old object lost: %v", err)
	}
	var body bytes.Buffer
	if _, err := svc.Download(ctx, node.ID, &body); err != nil || body.String() != "keep me" {
		t.Fatalf("old body changed: %q, %v", body.String(), err)
	}
}

func TestGatewayCopyAndBulkDelete(t *testing.T) {
	ctx := context.Background()
	_, ts, _ := newTestServer(t)
	client := awsClient(ts)
	if _, err := client.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: ptr("operations")}); err != nil {
		t.Fatal(err)
	}
	key := "a +ក.txt"
	if _, err := client.PutObject(ctx, &awss3.PutObjectInput{Bucket: ptr("operations"), Key: ptr(key), Body: strings.NewReader("copied body")}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CopyObject(ctx, &awss3.CopyObjectInput{Bucket: ptr("operations"), Key: ptr("copy"), CopySource: ptr("operations/a%20%2B%E1%9E%80.txt")}); err != nil {
		t.Fatal(err)
	}
	obj, err := client.GetObject(ctx, &awss3.GetObjectInput{Bucket: ptr("operations"), Key: ptr("copy")})
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(obj.Body)
	obj.Body.Close()
	if err != nil || string(body) != "copied body" {
		t.Fatalf("copy: %q, %v", body, err)
	}
	if _, err := client.DeleteBucket(ctx, &awss3.DeleteBucketInput{Bucket: ptr("operations")}); err == nil {
		t.Fatal("nonempty bucket deleted")
	}
	result, err := client.DeleteObjects(ctx, &awss3.DeleteObjectsInput{Bucket: ptr("operations"), Delete: &types.Delete{
		Objects: []types.ObjectIdentifier{{Key: ptr(key)}, {Key: ptr("copy")}, {Key: ptr("missing")}},
	}})
	if err != nil || len(result.Errors) != 0 || len(result.Deleted) != 3 {
		t.Fatalf("bulk delete: %+v, %v", result, err)
	}
	if _, err := client.DeleteBucket(ctx, &awss3.DeleteBucketInput{Bucket: ptr("operations")}); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayMultipartCompatibility(t *testing.T) {
	ctx := context.Background()
	svc, ts, _ := newTestServer(t)
	client := awsClient(ts)
	bucket, key := ptr("parts"), ptr("backup.bin")
	if _, err := client.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: bucket}); err != nil {
		t.Fatal(err)
	}
	up, err := client.CreateMultipartUpload(ctx, &awss3.CreateMultipartUploadInput{Bucket: bucket, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.UploadPart(ctx, &awss3.UploadPartInput{Bucket: bucket, Key: ptr("other-key"), UploadId: up.UploadId, PartNumber: aws.Int32(1), Body: strings.NewReader("wrong")}); err == nil {
		t.Fatal("part accepted for wrong object")
	}
	var parts []types.CompletedPart
	for i, body := range [][]byte{testData(5 << 20), []byte("tail")} {
		out, err := client.UploadPart(ctx, &awss3.UploadPartInput{Bucket: bucket, Key: key, UploadId: up.UploadId, PartNumber: aws.Int32(int32(i + 1)), Body: bytes.NewReader(body)})
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, types.CompletedPart{PartNumber: aws.Int32(int32(i + 1)), ETag: out.ETag})
	}
	listed, err := client.ListParts(ctx, &awss3.ListPartsInput{Bucket: bucket, Key: key, UploadId: up.UploadId, MaxParts: aws.Int32(1)})
	if err != nil || len(listed.Parts) != 1 || !aws.ToBool(listed.IsTruncated) || aws.ToString(listed.NextPartNumberMarker) != "1" {
		t.Fatalf("list parts: %+v, %v", listed, err)
	}
	uploads, err := client.ListMultipartUploads(ctx, &awss3.ListMultipartUploadsInput{Bucket: bucket})
	if err != nil || len(uploads.Uploads) != 1 || aws.ToString(uploads.Uploads[0].UploadId) != aws.ToString(up.UploadId) {
		t.Fatalf("list uploads: %+v, %v", uploads, err)
	}
	complete := &awss3.CompleteMultipartUploadInput{Bucket: bucket, Key: key, UploadId: up.UploadId, MultipartUpload: &types.CompletedMultipartUpload{Parts: []types.CompletedPart{parts[1], parts[0]}}}
	if _, err := client.CompleteMultipartUpload(ctx, complete); err == nil {
		t.Fatal("unordered completion accepted")
	}
	complete.MultipartUpload.Parts = parts
	repo := svc.Nodes
	svc.Nodes = failedReplace{repo}
	if _, err := client.CompleteMultipartUpload(ctx, complete); err == nil {
		t.Fatal("broken publish succeeded")
	}
	svc.Nodes = repo
	if _, err := client.CompleteMultipartUpload(ctx, complete); err != nil {
		t.Fatalf("completion retry with quoted ETags: %v", err)
	}
	obj, err := client.GetObject(ctx, &awss3.GetObjectInput{Bucket: bucket, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(obj.Body)
	obj.Body.Close()
	if err != nil || !bytes.Equal(body, append(testData(5<<20), []byte("tail")...)) {
		t.Fatalf("multipart body corrupted: %d bytes, %v", len(body), err)
	}
	up, err = client.CreateMultipartUpload(ctx, &awss3.CreateMultipartUploadInput{Bucket: bucket, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.AbortMultipartUpload(ctx, &awss3.AbortMultipartUploadInput{Bucket: bucket, Key: ptr("other-key"), UploadId: up.UploadId}); err == nil {
		t.Fatal("abort accepted for wrong object")
	}
	if _, err := client.AbortMultipartUpload(ctx, &awss3.AbortMultipartUploadInput{Bucket: bucket, Key: key, UploadId: up.UploadId}); err != nil {
		t.Fatal(err)
	}
	uploads, err = client.ListMultipartUploads(ctx, &awss3.ListMultipartUploadsInput{Bucket: bucket})
	if err != nil || len(uploads.Uploads) != 0 {
		t.Fatalf("abort left upload: %+v, %v", uploads, err)
	}
}
