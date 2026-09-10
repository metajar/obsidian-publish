// Command server runs the Obsidian Publish HTTP server: a bearer-token
// authenticated /api/* plane for the plugin and a public reader plane for
// published pages.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"obsidian-publish/server/internal/assets"
	"obsidian-publish/server/internal/config"
	"obsidian-publish/server/internal/httpapi"
	"obsidian-publish/server/internal/ratelimit"
	"obsidian-publish/server/internal/session"
	"obsidian-publish/server/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Ensure the data directory for db/token/secret exists.
	for _, p := range []string{cfg.DBPath, cfg.TokenFile, cfg.SecretFile} {
		if dir := filepath.Dir(p); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return fmt.Errorf("create data dir %s: %w", dir, err)
			}
		}
	}

	// Session secret: generated at first run, persisted. Rotating it (delete
	// the file and restart) invalidates all outstanding page sessions.
	secretStr, created, err := loadOrCreateSecret(cfg.SecretFile)
	if err != nil {
		return fmt.Errorf("session secret: %w", err)
	}
	if created {
		log.Info("session secret generated", "path", cfg.SecretFile)
	}

	// API token: 32 random bytes, persisted. Generated only when the file is
	// absent — delete the file and restart to rotate. Printed once, below,
	// only at generation time.
	token, created, err := loadOrCreateSecret(cfg.TokenFile)
	if err != nil {
		return fmt.Errorf("api token: %w", err)
	}
	if created {
		fmt.Println("==========================================================")
		fmt.Println(" API token generated (shown once — paste it into the")
		fmt.Println(" Obsidian plugin settings and store it somewhere safe):")
		fmt.Println()
		fmt.Println("   " + token)
		fmt.Println()
		fmt.Println(" To rotate: delete " + cfg.TokenFile + " and restart.")
		fmt.Println("==========================================================")
	}

	pages, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer pages.Close()

	assetStore, err := assets.Open(cfg.AssetsDir)
	if err != nil {
		return err
	}

	sessions, err := session.NewManager([]byte(secretStr), time.Hour)
	if err != nil {
		return err
	}
	limiter := ratelimit.New(5, time.Minute) // S9: 5 password attempts / min / IP+route

	e, _ := httpapi.New(httpapi.Deps{
		Store:    pages,
		Token:    token,
		Sessions: sessions,
		Limiter:  limiter,
		BaseURL:  cfg.BaseURL,
		Assets:   assetStore,
		Log:      log,
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           e,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Info("listening", "addr", cfg.Addr, "db", cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

// loadOrCreateSecret reads a base64url 32-byte secret from path, creating a
// new random one (mode 0600) if the file does not exist. created reports
// whether a new secret was written. Deleting the file and restarting rotates
// the value.
func loadOrCreateSecret(path string) (value string, created bool, err error) {
	if b, rerr := os.ReadFile(path); rerr == nil {
		v := strings.TrimSpace(string(b))
		if v == "" {
			return "", false, fmt.Errorf("%s is empty", path)
		}
		return v, false, nil
	} else if !errors.Is(rerr, os.ErrNotExist) {
		return "", false, rerr
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", false, fmt.Errorf("generate: %w", err)
	}
	v := base64.RawURLEncoding.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(v+"\n"), 0o600); err != nil {
		return "", false, fmt.Errorf("write: %w", err)
	}
	return v, true, nil
}
