package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

		"github.com/vrc/nimbus/internal/app"
	"github.com/vrc/nimbus/internal/backup"
	"github.com/vrc/nimbus/internal/config"
	"github.com/vrc/nimbus/internal/domain"
	httpserver "github.com/vrc/nimbus/internal/http"
	"github.com/vrc/nimbus/internal/s3gw"
	"github.com/vrc/nimbus/internal/store/sqlite"
	"github.com/vrc/nimbus/internal/telegram"
)

func main() {
	log.Println("nimbus starting…")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	store, err := sqlite.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer store.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

		tg := telegram.New(cfg.TelegramAPIID, cfg.TelegramAPIHash, cfg.DataDir)

	var blobs domain.BlobStore = tg
	if cfg.TelegramBotAPIURL != "" {
		blobs = telegram.NewBotAPI(cfg.TelegramBotAPIURL, cfg.TelegramBotToken, cfg.TelegramStorageChatID)
		log.Println("storage backend: Telegram Bot API")
	} else {
		log.Println("storage backend: MTProto")
	}

	go func() {
		log.Println("connecting to Telegram…")
		if err := tg.Start(ctx); err != nil {
			log.Printf("telegram: %v (API stays up; sign in again from the web UI)", err)
			return
		}
		if tg.IsConfigured() {
			log.Println("telegram connected")
		} else {
			log.Println("telegram api credentials missing — enter them in the web UI")
		}
	}()

	if cfg.Public {
		log.Println("public mode: Telegram sign-in only (no access key)")
	} else {
		log.Printf("access key required: X-Nimbus-Access (see %s)", filepath.Join(cfg.DataDir, "access.secret"))
	}
	log.Printf("storage API key: X-Nimbus-Key (see %s)", filepath.Join(cfg.DataDir, "api.key"))

	svc := &app.Services{
		Nodes:         store,
		Parts:         store,
		Shares:        store,
		Data:          store,
		Edits:         store,
		BotGrants:     store,
		Social:        store,
		SocialApp:     store,
				Blobs:         blobs,
		TG:            tg,
		Messenger:     tg,
		Setup:         tg,
		JWTSecret:     []byte(cfg.JWTSecret),
		JWTTTL:        time.Duration(cfg.JWTTTLHours) * time.Hour,
		ChunkSize:     cfg.ChunkSize,
		DataDir:       cfg.DataDir,
UploadWorkers: 3,
	}
	backupSvc := backup.NewService(cfg.DataDir, store, svc, store)
	svc.Backup = backupSvc
	svc.ConfigureSocialOAuth(app.SocialOAuthConfig{
		PublicBaseURL:   cfg.PublicBaseURL,
		TikTokClientKey: cfg.TikTokClientKey,
		TikTokSecret:    cfg.TikTokSecret,
		MetaAppID:       cfg.MetaAppID,
		MetaAppSecret:   cfg.MetaAppSecret,
	})
	if _, err := store.EnsureRoot(ctx); err != nil {
		log.Fatalf("root: %v", err)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpserver.New(svc, cfg.CORSOrigins, cfg.AccessSecret, cfg.APIKey, cfg.Public, cfg.WebDir, cfg.S3Port).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	var s3srv *http.Server
	s3cfg, err := s3gw.Load(cfg.DataDir, cfg.S3Enabled, cfg.S3Region)
	if err != nil {
		log.Fatalf("s3 gateway: %v", err)
	}
	if s3cfg.Enabled {
		s3srv = &http.Server{
			Addr:              ":" + cfg.S3Port,
			Handler:           s3gw.New(svc, s3cfg, cfg.DataDir).Handler(),
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			log.Printf("s3 gateway listening on http://localhost:%s (region %s, access key %s)", cfg.S3Port, s3cfg.Region, s3cfg.AccessKey)
			if err := s3srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("s3 gateway: %v", err)
			}
		}()
	} else {
		log.Printf("s3 gateway disabled (NIMBUS_S3_ENABLED=true to enable, access key %s)", s3cfg.AccessKey)
	}

	go func() {
		log.Printf("nimbus listening on http://localhost:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	tg.Stop()
	_ = srv.Shutdown(shutdownCtx)
	if s3srv != nil {
		_ = s3srv.Shutdown(shutdownCtx)
	}
	log.Println("nimbus stopped")
}
