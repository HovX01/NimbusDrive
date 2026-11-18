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

func TestLoadHonorsChunkSizeAndCapsOversize(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("UPLOAD_WORKERS", "2")
	t.Setenv("TELEGRAM_BOT_API_URL", "")
	t.Setenv("TELEGRAM_BOT_CHUNK_SIZE", "")

	for _, tc := range []struct {
		value string
		want  int
	}{
		{"20971520", 20 << 20},
		{"52428800", 50 << 20},
		{"209715200", 50 << 20},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("CHUNK_SIZE", tc.value)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ChunkSize != tc.want || cfg.UploadWorkers != 2 {
				t.Fatalf("chunk size=%d workers=%d, want chunk size=%d workers=2", cfg.ChunkSize, cfg.UploadWorkers, tc.want)
			}
		})
	}
}

func TestLoadHonors50MiBBotChunkOverride(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("CHUNK_SIZE", "20971520")
	t.Setenv("TELEGRAM_BOT_API_URL", "http://localhost:8081")
	t.Setenv("TELEGRAM_BOT_CHUNK_SIZE", "52428800")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ChunkSize != 50<<20 {
		t.Fatalf("bot chunk size = %d, want %d", cfg.ChunkSize, 50<<20)
	}
}
