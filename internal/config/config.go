// Package config manages the runtime configuration created by the first-run
// installer: database connection settings and the session signing secret.
// The config lives as JSON inside the data directory, WordPress-style — the
// app boots into setup mode until it exists.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
)

const fileName = "config.json"

// Database holds Postgres connection settings.
type Database struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Name     string `json:"name"`
	SSLMode  string `json:"sslmode"`
}

// DSN renders the settings as a postgres:// connection URL.
func (d Database) DSN() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(d.User, d.Password),
		Host:   d.Host + ":" + d.Port,
		Path:   "/" + d.Name,
	}
	q := url.Values{}
	if d.SSLMode != "" {
		q.Set("sslmode", d.SSLMode)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// Config is the persisted runtime configuration.
type Config struct {
	Database      Database `json:"database"`
	SessionSecret string   `json:"session_secret"`
}

// FromEnv returns database defaults from the environment, used to pre-fill
// the installer form. Every field can still be edited during setup.
func FromEnv() Database {
	get := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}
	return Database{
		Host:     get("DB_HOST", "localhost"),
		Port:     get("DB_PORT", "5432"),
		User:     get("DB_USER", ""),
		Password: get("DB_PASSWORD", ""),
		Name:     get("DB_NAME", "documents"),
		SSLMode:  get("DB_SSLMODE", "disable"),
	}
}

func path(dataDir string) string {
	return filepath.Join(dataDir, fileName)
}

// Load reads the config from the data directory. It returns (nil, nil) when
// the app has not been installed yet.
func Load(dataDir string) (*Config, error) {
	data, err := os.ReadFile(path(dataDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if cfg.SessionSecret == "" {
		return nil, errors.New("config is missing session_secret")
	}
	return &cfg, nil
}

// Save writes the config atomically into the data directory with a fresh
// session secret if one is not set. File mode is 0600 — it contains the
// database password.
func Save(dataDir string, cfg *Config) error {
	if cfg.SessionSecret == "" {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return fmt.Errorf("generate session secret: %w", err)
		}
		cfg.SessionSecret = hex.EncodeToString(secret)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	tmp := path(dataDir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path(dataDir)); err != nil {
		return fmt.Errorf("commit config: %w", err)
	}
	return nil
}
