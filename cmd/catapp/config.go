//go:build darwin || windows || catapp_headless

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// appConfig is the launcher's own persisted settings — deliberately separate
// from the catway's ~/.config/cats/config.yaml. It records which mode the
// window opens in and, for remote mode, where to point the webview, so a user's
// choice in the connect form survives relaunches. It lives in the platform
// app-data dir (see appDataDir) as app.json.
type appConfig struct {
	// Mode is "local" (supervise the in-bundle daemons) or "remote" (thin client
	// to a catway URL). An empty value falls back to the build-time defaultMode.
	Mode   string       `json:"mode"`
	Remote remoteTarget `json:"remote"`
}

// remoteTarget is the catway a thin client connects to: a relay host
// (https://<home-id>.relay.herdr.dev) or a direct LAN/VPN address. Only URL is
// load-bearing; Label is a friendly name for any future UI.
type remoteTarget struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

// appConfigFile is the launcher settings filename inside appDataDir.
const appConfigFile = "app.json"

// loadAppConfig reads app.json, falling back to the build-time defaultMode on a
// first run or any read/parse problem — the launcher must always resolve to a
// usable mode, never fail to open. A malformed file is logged, not fatal.
func loadAppConfig() appConfig {
	cfg := appConfig{Mode: defaultMode}
	dir, err := appDataDir()
	if err != nil {
		log.Printf("app data dir unavailable, using build defaults: %v", err)
		return cfg
	}
	return loadAppConfigFile(filepath.Join(dir, appConfigFile), defaultMode)
}

func loadAppConfigFile(path, fallbackMode string) appConfig {
	cfg := appConfig{Mode: fallbackMode}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) { // a missing file is the normal first-run case
			log.Printf("read %s, using build defaults: %v", path, err)
		}
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("%s is malformed, using build defaults: %v", path, err)
		return appConfig{Mode: fallbackMode}
	}
	if cfg.Mode == "" {
		cfg.Mode = fallbackMode
	}
	return cfg
}

// saveAppConfig persists cfg to app.json (0600 in a 0700 dir — it can hold a
// remote URL that is nobody else's business). Parent dirs are created as needed.
func saveAppConfig(cfg appConfig) error {
	dir, err := appDataDir()
	if err != nil {
		return err
	}
	return saveAppConfigFile(filepath.Join(dir, appConfigFile), cfg)
}

func saveAppConfigFile(path string, cfg appConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create app data dir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal app.json: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
