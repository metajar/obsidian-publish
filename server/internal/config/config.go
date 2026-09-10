// Package config resolves server configuration from flags with environment
// variable fallbacks. Flag > env > default.
package config

import (
	"flag"
	"fmt"
	"os"
)

// Env var names.
const (
	EnvAddr       = "OBSPUB_ADDR"
	EnvDBPath     = "OBSPUB_DB"
	EnvTokenFile  = "OBSPUB_TOKEN_FILE"
	EnvSecretFile = "OBSPUB_SECRET_FILE"
	EnvBaseURL    = "OBSPUB_BASE_URL"
)

// Defaults.
const (
	DefaultAddr       = ":8080"
	DefaultDBPath     = "data/obsidian-publish.db"
	DefaultTokenFile  = "data/api-token"
	DefaultSecretFile = "data/session-secret"
)

// Config is the resolved server configuration.
type Config struct {
	// Addr is the listen address (host:port).
	Addr string
	// DBPath is the SQLite database file path.
	DBPath string
	// TokenFile holds the API bearer token (base64url, 32 bytes).
	TokenFile string
	// SecretFile holds the session HMAC secret (base64url, 32 bytes).
	SecretFile string
	// BaseURL is the public base URL of the server (e.g.
	// https://notes.example.com) used to build live page URLs in publish
	// responses. Empty means derive from each request's Host header.
	BaseURL string
}

// Load parses flags from args with env fallbacks. Exits-on-error behavior is
// left to the caller (flag.ExitOnError handles usage errors).
func Load(args []string) (*Config, error) {
	fs := flag.NewFlagSet("obsidian-publish-server", flag.ContinueOnError)
	cfg := &Config{}
	fs.StringVar(&cfg.Addr, "addr", envOr(EnvAddr, DefaultAddr), "listen address (env "+EnvAddr+")")
	fs.StringVar(&cfg.DBPath, "db", envOr(EnvDBPath, DefaultDBPath), "SQLite database file path (env "+EnvDBPath+")")
	fs.StringVar(&cfg.TokenFile, "token-file", envOr(EnvTokenFile, DefaultTokenFile), "API token file path (env "+EnvTokenFile+")")
	fs.StringVar(&cfg.SecretFile, "secret-file", envOr(EnvSecretFile, DefaultSecretFile), "session secret file path (env "+EnvSecretFile+")")
	fs.StringVar(&cfg.BaseURL, "base-url", envOr(EnvBaseURL, ""), "public base URL for live page links, e.g. https://notes.example.com (env "+EnvBaseURL+")")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected argument: %q", fs.Arg(0))
	}
	if cfg.BaseURL != "" {
		cfg.BaseURL = trimTrailingSlash(cfg.BaseURL)
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
