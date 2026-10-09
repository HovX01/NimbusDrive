package s3gw

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	minio "github.com/minio/minio-go/v7"
)

func TestGatewayRusticRecovery(t *testing.T) {
	binary := os.Getenv("NIMBUS_TEST_RUSTIC")
	if binary == "" {
		t.Skip("set NIMBUS_TEST_RUSTIC to a Rustic binary to run backup/replication/restore/prune")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_, ts, _ := newTestServer(t)
	client := minioClient(t, ts)
	if err := client.MakeBucket(ctx, "arcane", minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	profile := filepath.Join(work, "isolated")
	if err := os.WriteFile(profile+".toml", []byte("[repository]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(remote bool, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, append([]string{"-P", profile, "--no-progress", "--no-cache", "--log-level", "warn"}, args...)...)
		cmd.Dir = work
		for _, env := range os.Environ() {
			if !strings.HasPrefix(env, "RUSTIC_") && !strings.HasPrefix(env, "AWS_") {
				cmd.Env = append(cmd.Env, env)
			}
		}
		cmd.Env = append(cmd.Env, "RUSTIC_PASSWORD=nimbus-rustic-test-password", "RUSTIC_REPOSITORY="+filepath.Join(work, "local-repository"))
		if remote {
			cmd.Env = append(cmd.Env,
				"RUSTIC_REPOSITORY=opendal:s3", "RUSTIC_REPO_OPT_BUCKET=arcane",
				"RUSTIC_REPO_OPT_ROOT=/arcane-system-recovery", "RUSTIC_REPO_OPT_ENDPOINT="+ts.URL,
				"RUSTIC_REPO_OPT_REGION=us-east-1", "RUSTIC_REPO_OPT_ENABLE_VIRTUAL_HOST_STYLE=false",
				"AWS_ACCESS_KEY_ID="+testAccessKey, "AWS_SECRET_ACCESS_KEY="+testSecretKey,
				"AWS_REGION=us-east-1", "AWS_EC2_METADATA_DISABLED=true",
			)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("rustic remote=%t %v: %v\n%s", remote, args, err, out)
		}
		t.Logf("rustic remote=%t %v: passed", remote, args)
	}
	fixture := filepath.Join(work, "source")
	if err := os.Mkdir(fixture, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 12<<20)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "recovery.bin"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	run(false, "init")
	run(false, "backup", "--as-path", "/", fixture)
	replica := filepath.Join(work, "replica")
	run(false, "restore", "latest", replica)
	run(true, "init")
	run(true, "backup", "--as-path", "/", replica)
	run(true, "snapshots", "--json")
	run(true, "check", "--read-data")
	destination := filepath.Join(work, "restored")
	run(true, "restore", "latest", destination)
	restored, err := os.ReadFile(filepath.Join(destination, "recovery.bin"))
	if err != nil || !bytes.Equal(restored, payload) {
		t.Fatalf("recovery bytes differ: %d bytes, %v", len(restored), err)
	}
	run(true, "forget", "--prune", "latest")
	run(true, "prune", "--instant-delete", "--max-unused", "0")
	run(true, "check", "--read-data")
	if _, err := client.StatObject(ctx, "arcane", "arcane-system-recovery/keys/", minio.StatObjectOptions{}); err != nil {
		t.Fatalf("prune removed repository directory marker: %v", err)
	}
}
