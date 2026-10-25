package config

import "testing"

func TestLoadChunkDefaultsAndCap(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("CHUNK_SIZE", "")
	t.Setenv("UPLOAD_WORKERS", "")
	t.Setenv("TELEGRAM_BOT_API_URL", "")
	t.Setenv("TELEGRAM_BOT_CHUNK_SIZE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ChunkSize != 8<<20 {
		t.Fatalf("default chunk size = %d, want %d", cfg.ChunkSize, 8<<20)
	}
	if cfg.UploadWorkers != 3 {
		t.Fatalf("default workers = %d, want 3", cfg.UploadWorkers)
	}
}

func TestLoadCapsChunkSize(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("CHUNK_SIZE", "20971520")
	t.Setenv("UPLOAD_WORKERS", "2")
	t.Setenv("TELEGRAM_BOT_API_URL", "")
	t.Setenv("TELEGRAM_BOT_CHUNK_SIZE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ChunkSize != 8<<20 {
		t.Fatalf("chunk size = %d, want capped %d", cfg.ChunkSize, 8<<20)
	}
	if cfg.UploadWorkers != 2 {
		t.Fatalf("workers = %d, want 2", cfg.UploadWorkers)
	}
}
