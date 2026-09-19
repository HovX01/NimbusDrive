package s3gw

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	minio "github.com/minio/minio-go/v7"
	miniocreds "github.com/minio/minio-go/v7/pkg/credentials"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/vrc/nimbus/internal/app"
	"github.com/vrc/nimbus/internal/store/sqlite"
)

const (
	testAccessKey = "nimbusaccesskey"
	testSecretKey = "nimbustopsecretkeyforunittests"
)

// fakeBlobs is an in-memory domain.BlobStore so tests never touch Telegram.
type fakeBlobs struct {
	mu     sync.Mutex
	msgs   map[int64]map[int][]byte
	nextID int
}

func newFakeBlobs() *fakeBlobs {
	return &fakeBlobs{msgs: map[int64]map[int][]byte{1: {}}}
}

func (b *fakeBlobs) EnsureStorageChannel(_ context.Context) (int64, error) { return 1, nil }

func (b *fakeBlobs) UploadPart(_ context.Context, channelID int64, _ int, _ string, r io.Reader, _ int64) (int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	store, ok := b.msgs[channelID]
	if !ok {
		store = map[int][]byte{}
		b.msgs[channelID] = store
	}
	store[b.nextID] = data
	return b.nextID, nil
}

func (b *fakeBlobs) DownloadPart(_ context.Context, channelID int64, messageID int, w io.Writer) error {
	b.mu.Lock()
	data := b.msgs[channelID][messageID]
	b.mu.Unlock()
	if data == nil {
		return fmt.Errorf("missing part %d", messageID)
	}
	_, err := w.Write(data)
	return err
}

func (b *fakeBlobs) DeleteMessages(_ context.Context, channelID int64, ids []int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, id := range ids {
		delete(b.msgs[channelID], id)
	}
	return nil
}

func newTestServer(t *testing.T) (*app.Services, *httptest.Server, *sqlite.Store) {
	t.Helper()
	dataDir := t.TempDir()
	store, err := sqlite.Open("sqlite://" + filepath.Join(dataDir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := store.EnsureRoot(context.Background()); err != nil {
		t.Fatalf("ensure root: %v", err)
	}
	svc := &app.Services{
		Nodes:     store,
		Parts:     store,
		Blobs:     newFakeBlobs(),
		DataDir:   dataDir,
		ChunkSize: 8 << 20,
	}
	srv := New(svc, Config{Enabled: true, AccessKey: testAccessKey, SecretKey: testSecretKey, Region: "us-east-1"}, dataDir)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
		_ = store.Close()
	})
	return svc, ts, store
}

func minioClient(t *testing.T, ts *httptest.Server) *minio.Client {
	t.Helper()
	client, err := minio.New(strings.TrimPrefix(ts.URL, "http://"), &minio.Options{
		Creds:           miniocreds.NewStaticV4(testAccessKey, testSecretKey, ""),
		Secure:          false,
		Region:          "us-east-1",
		BucketLookup:    minio.BucketLookupPath,
		TrailingHeaders: true,
	})
	if err != nil {
		t.Fatalf("minio client: %v", err)
	}
	return client
}

func testData(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(i*7 + 3)
	}
	return out
}

func TestGatewayRoundTrip(t *testing.T) {
	ctx := context.Background()
	_, ts, _ := newTestServer(t)
	client := minioClient(t, ts)

	if err := client.MakeBucket(ctx, "testbucket", minio.MakeBucketOptions{}); err != nil {
		t.Fatalf("make bucket: %v", err)
	}
	buckets, err := client.ListBuckets(ctx)
	if err != nil {
		t.Fatalf("list buckets: %v", err)
	}
	if len(buckets) != 1 || buckets[0].Name != "testbucket" {
		t.Fatalf("unexpected buckets: %+v", buckets)
	}

	// PUT / GET / HEAD
	const body = "hello world"
	if _, err := client.PutObject(ctx, "testbucket", "hello.txt", strings.NewReader(body), int64(len(body)), minio.PutObjectOptions{ContentType: "text/plain"}); err != nil {
		t.Fatalf("put object: %v", err)
	}
	obj, err := client.GetObject(ctx, "testbucket", "hello.txt", minio.GetObjectOptions{})
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	got, _ := io.ReadAll(obj)
	obj.Close()
	if string(got) != body {
		t.Fatalf("get object = %q, want %q", got, body)
	}
	info, err := client.StatObject(ctx, "testbucket", "hello.txt", minio.StatObjectOptions{})
	if err != nil {
		t.Fatalf("stat object: %v", err)
	}
	if info.Size != int64(len(body)) {
		t.Fatalf("stat size = %d, want %d", info.Size, len(body))
	}

	// Range request
	rangeOpts := minio.GetObjectOptions{}
	if err := rangeOpts.SetRange(0, 4); err != nil {
		t.Fatalf("set range: %v", err)
	}
	obj, _ = client.GetObject(ctx, "testbucket", "hello.txt", rangeOpts)
	got, _ = io.ReadAll(obj)
	obj.Close()
	if string(got) != "hello" {
		t.Fatalf("range get = %q, want %q", got, "hello")
	}

	// Nested keys create folders implicitly.
	nested := "photos/2024/cat.txt"
	if _, err := client.PutObject(ctx, "testbucket", nested, strings.NewReader("meow"), 4, minio.PutObjectOptions{}); err != nil {
		t.Fatalf("put nested: %v", err)
	}
	obj, _ = client.GetObject(ctx, "testbucket", nested, minio.GetObjectOptions{})
	got, _ = io.ReadAll(obj)
	obj.Close()
	if string(got) != "meow" {
		t.Fatalf("nested get = %q", got)
	}

	// Delimited listing groups the nested folder under a common prefix.
	var listing []minio.ObjectInfo
	for obj := range client.ListObjects(ctx, "testbucket", minio.ListObjectsOptions{Prefix: "photos/"}) {
		if obj.Err != nil {
			t.Fatalf("list objects: %v", obj.Err)
		}
		listing = append(listing, obj)
	}
	if len(listing) != 1 || !strings.HasSuffix(listing[0].Key, "/") {
		t.Fatalf("delimited listing = %+v", listing)
	}

	// Recursive listing flattens keys.
	keys := map[string]bool{}
	for obj := range client.ListObjects(ctx, "testbucket", minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			t.Fatalf("list recursive: %v", obj.Err)
		}
		keys[obj.Key] = true
	}
	if !keys["hello.txt"] || !keys[nested] {
		t.Fatalf("recursive listing = %+v", keys)
	}

	// PUT overwrites in place instead of forking a "name (1)" copy.
	if _, err := client.PutObject(ctx, "testbucket", "hello.txt", strings.NewReader("replaced"), 8, minio.PutObjectOptions{}); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	obj, _ = client.GetObject(ctx, "testbucket", "hello.txt", minio.GetObjectOptions{})
	got, _ = io.ReadAll(obj)
	obj.Close()
	if string(got) != "replaced" {
		t.Fatalf("after overwrite = %q, want %q", got, "replaced")
	}

	if err := client.RemoveObject(ctx, "testbucket", "hello.txt", minio.RemoveObjectOptions{}); err != nil {
		t.Fatalf("remove object: %v", err)
	}
	if _, err := client.StatObject(ctx, "testbucket", "hello.txt", minio.StatObjectOptions{}); err == nil {
		t.Fatalf("stat after delete: expected error")
	}
}

func TestGatewayMultipartUpload(t *testing.T) {
	ctx := context.Background()
	_, ts, _ := newTestServer(t)
	client := minioClient(t, ts)
	if err := client.MakeBucket(ctx, "bigbucket", minio.MakeBucketOptions{}); err != nil {
		t.Fatalf("make bucket: %v", err)
	}

	// 12 MiB forces minio-go down the multipart path (parts of 5/5/2 MiB).
	payload := testData(12 << 20)
	info, err := client.PutObject(ctx, "bigbucket", "big.bin", bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
		PartSize:    5 << 20,
	})
	if err != nil {
		t.Fatalf("multipart put: %v", err)
	}
	if info.Size != int64(len(payload)) {
		t.Fatalf("multipart size = %d, want %d", info.Size, len(payload))
	}

	obj, err := client.GetObject(ctx, "bigbucket", "big.bin", minio.GetObjectOptions{})
	if err != nil {
		t.Fatalf("get multipart: %v", err)
	}
	got, err := io.ReadAll(obj)
	obj.Close()
	if err != nil {
		t.Fatalf("read multipart: %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("multipart round trip length = %d, want %d", len(got), len(payload))
	}
	for i := range payload {
		if got[i] != payload[i] {
			t.Fatalf("multipart byte mismatch at %d", i)
		}
	}

	// Identical content under a second key must still create that key
	// (the gateway bypasses the drive's content-hash dedup).
	if _, err := client.PutObject(ctx, "bigbucket", "big-copy.bin", bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{PartSize: 5 << 20}); err != nil {
		t.Fatalf("dedup put: %v", err)
	}
	if _, err := client.StatObject(ctx, "bigbucket", "big-copy.bin", minio.StatObjectOptions{}); err != nil {
		t.Fatalf("stat dedup copy: %v", err)
	}
}

// TestGatewayUnsignedPayload covers the aws-sdk-go-v2 default signing mode
// (payload not hashed), exercised through a second independent client library.
func TestGatewayUnsignedPayload(t *testing.T) {
	ctx := context.Background()
	_, ts, _ := newTestServer(t)

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(awscreds.NewStaticCredentialsProvider(testAccessKey, testSecretKey, "")),
	)
	if err != nil {
		t.Fatalf("aws config: %v", err)
	}
	client := awss3.NewFromConfig(cfg, func(o *awss3.Options) {
		o.BaseEndpoint = &ts.URL
		o.UsePathStyle = true
	})

	if _, err := client.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: ptr("awsbucket")}); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	if _, err := client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:      ptr("awsbucket"),
		Key:         ptr("unsigned.txt"),
		Body:        strings.NewReader("unsigned payload body"),
		ContentType: ptr("text/plain"),
	}); err != nil {
		t.Fatalf("put unsigned: %v", err)
	}
	out, err := client.GetObject(ctx, &awss3.GetObjectInput{Bucket: ptr("awsbucket"), Key: ptr("unsigned.txt")})
	if err != nil {
		t.Fatalf("get unsigned: %v", err)
	}
	got, _ := io.ReadAll(out.Body)
	out.Body.Close()
	if string(got) != "unsigned payload body" {
		t.Fatalf("unsigned get = %q", got)
	}
}

func ptr(s string) *string { return &s }

// TestGatewayStreamsUploads verifies the gateway never buffers a whole file to
// disk (the old 2GB ceiling came from that buffering) and still records the
// content hash for the streamed bytes.
func TestGatewayStreamsUploads(t *testing.T) {
	ctx := context.Background()
	svc, ts, store := newTestServer(t)
	client := minioClient(t, ts)
	if err := client.MakeBucket(ctx, "streambucket", minio.MakeBucketOptions{}); err != nil {
		t.Fatalf("make bucket: %v", err)
	}

	// Spans several 8 MiB storage parts, so this leaves the single-part path.
	payload := testData(20 << 20)
	if _, err := client.PutObject(ctx, "streambucket", "stream.bin", bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{}); err != nil {
		t.Fatalf("put: %v", err)
	}

	if matches, _ := filepath.Glob(filepath.Join(svc.DataDir, "upload-tmp", "*")); len(matches) > 0 {
		t.Fatalf("streaming upload left temp files behind: %v", matches)
	}

	sum := sha256.Sum256(payload)
	if _, err := store.FindByContentHash(ctx, hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("content hash not recorded for streamed upload: %v", err)
	}

	obj, _ := client.GetObject(ctx, "streambucket", "stream.bin", minio.GetObjectOptions{})
	got, _ := io.ReadAll(obj)
	obj.Close()
	if len(got) != len(payload) {
		t.Fatalf("streamed round trip = %d bytes, want %d", len(got), len(payload))
	}
}

func TestAWSChunkReader(t *testing.T) {
	// Two 4-byte chunks plus a terminating zero chunk, matching the framing
	// AWS clients emit for streaming uploads.
	raw := "4;chunk-signature=abc1\r\nABCD\r\n" +
		"4;chunk-signature=abc2\r\nEFGH\r\n" +
		"0;chunk-signature=abc3\r\n\r\n"
	r := newAWSChunkReader(strings.NewReader(raw))
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read chunked: %v", err)
	}
	if string(got) != "ABCDEFGH" {
		t.Fatalf("chunked body = %q, want %q", got, "ABCDEFGH")
	}
}

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}
