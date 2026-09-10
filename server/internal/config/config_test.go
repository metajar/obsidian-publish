package config

import (
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != DefaultAddr || cfg.DBPath != DefaultDBPath {
		t.Errorf("defaults not applied: %+v", cfg)
	}
}

func TestFlagsOverrideEnv(t *testing.T) {
	t.Setenv(EnvAddr, ":9999")
	t.Setenv(EnvDBPath, "/from/env.db")
	cfg, err := Load([]string{"-addr", ":1234"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":1234" {
		t.Errorf("Addr = %q, want flag value :1234", cfg.Addr)
	}
	if cfg.DBPath != "/from/env.db" {
		t.Errorf("DBPath = %q, want env value /from/env.db", cfg.DBPath)
	}
}

func TestEnvOverridesDefault(t *testing.T) {
	t.Setenv(EnvBaseURL, "https://notes.example.com/")
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseURL != "https://notes.example.com" {
		t.Errorf("BaseURL = %q, want trailing slash trimmed", cfg.BaseURL)
	}
}

func TestUnexpectedArgument(t *testing.T) {
	if _, err := Load([]string{"stray"}); err == nil || !strings.Contains(err.Error(), "stray") {
		t.Errorf("Load(stray) err = %v, want error mentioning the argument", err)
	}
}

func TestAssetsDir(t *testing.T) {
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AssetsDir != DefaultAssetsDir {
		t.Errorf("AssetsDir default = %q, want %q", cfg.AssetsDir, DefaultAssetsDir)
	}

	t.Setenv(EnvAssetsDir, "/from/env/assets")
	cfg, err = Load([]string{"-assets-dir", "/from/flag/assets"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AssetsDir != "/from/flag/assets" {
		t.Errorf("AssetsDir = %q, want flag value /from/flag/assets", cfg.AssetsDir)
	}

	cfg, err = Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AssetsDir != "/from/env/assets" {
		t.Errorf("AssetsDir = %q, want env value /from/env/assets", cfg.AssetsDir)
	}
}
