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
	"github.com/vrc/nimbus/internal/config"
	httpserver "github.com/vrc/nimbus/internal/http"
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
		Blobs:         tg,
		TG:            tg,
		Messenger:     tg,
		Setup:         tg,
		JWTSecret:     []byte(cfg.JWTSecret),
		JWTTTL:        time.Duration(cfg.JWTTTLHours) * time.Hour,
		ChunkSize:     cfg.ChunkSize,
		DataDir:       cfg.DataDir,
		UploadWorkers: 3,
	}
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
		Handler:           httpserver.New(svc, cfg.CORSOrigins, cfg.AccessSecret, cfg.APIKey, cfg.Public, cfg.WebDir).Router(),
		ReadHeaderTimeout: 10 * time.Second,
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
	log.Println("nimbus stopped")
}
