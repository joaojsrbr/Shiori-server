// Package config provides typed configuration for the Shiori server.
//
// Configuration is loaded from environment variables and command-line flags.
// Environment variables use the SHIORI_ prefix. Flags take precedence over
// environment variables, which take precedence over defaults.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Profile identifies the infrastructure profile.
type Profile string

const (
	ProfilePortable Profile = "portable"
	ProfileDocker   Profile = "docker"
)

// Config holds all server configuration.
type Config struct {
	Profile Profile
	DataDir string
	Server  ServerConfig
	Log     LogConfig

	Postgres PostgresConfig
	Valkey   ValkeyConfig
	MinIO    MinIOConfig
	AI       AIConfig
}

type AIConfig struct {
	LMStudioBaseURL string
	Token           string
	ModelTiny       string
	ModelDefault    string
	ModelQuality    string
	TemplatePath    string
}

type PostgresConfig struct {
	URL string
}

type ValkeyConfig struct {
	Addr string
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string // debug, info, warn, error
	Format string // json, text
}

// Addr returns the listen address as host:port.
func (s ServerConfig) Addr() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

// Defaults returns configuration with sensible defaults for portable profile.
func Defaults() Config {
	return Config{
		Profile: ProfilePortable,
		Server: ServerConfig{
			Host:            "127.0.0.1",
			Port:            9180,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    60 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 15 * time.Second,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
		Postgres: PostgresConfig{
			URL: "postgres://shiori:shiori@localhost:5432/shiori?sslmode=disable",
		},
		Valkey: ValkeyConfig{
			Addr: "localhost:6379",
		},
		MinIO: MinIOConfig{
			Endpoint:  "localhost:9000",
			AccessKey: "shiori",
			SecretKey: "shiori-secret",
			Bucket:    "shiori",
			UseSSL:    false,
		},
		AI: AIConfig{
			LMStudioBaseURL: "http://127.0.0.1:1234",
			ModelTiny:       "nuextract-1.5-tiny",
			ModelDefault:    "nuextract3@q4_k_m",
			ModelQuality:    "nuextract3@q5_k_m",
			TemplatePath:    "config/nuextract_templates.json",
		},
	}
}

// Load reads configuration from environment variables and applies flags.
// It does not validate the configuration; call Validate() after Load().
func Load(flags Flags) (Config, error) {
	// Try loading .env file from the config directory.
	// We ignore the error because the file is optional.
	_ = godotenv.Load("config/.env")

	cfg := Defaults()

	// Profile
	if v := envOr("SHIORI_PROFILE", ""); v != "" {
		cfg.Profile = Profile(strings.ToLower(v))
	}
	if flags.Profile != "" {
		cfg.Profile = Profile(strings.ToLower(flags.Profile))
	}

	// Data directory
	if v := envOr("SHIORI_DATA_DIR", ""); v != "" {
		cfg.DataDir = v
	}
	if flags.DataDir != "" {
		cfg.DataDir = flags.DataDir
	}

	// If data dir is still empty, resolve relative to executable.
	if cfg.DataDir == "" {
		exePath, err := os.Executable()
		if err != nil {
			return cfg, fmt.Errorf("resolving executable path: %w", err)
		}
		cfg.DataDir = filepath.Dir(exePath)
	}

	// Resolve to absolute path.
	absDir, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return cfg, fmt.Errorf("resolving absolute data dir: %w", err)
	}
	cfg.DataDir = absDir

	// Server
	if v := envOr("SHIORI_HOST", ""); v != "" {
		cfg.Server.Host = v
	}
	if flags.Host != "" {
		cfg.Server.Host = flags.Host
	}

	if v := envInt("SHIORI_PORT", 0); v != 0 {
		cfg.Server.Port = v
	}
	if flags.Port != 0 {
		cfg.Server.Port = flags.Port
	}

	// Log
	if v := envOr("SHIORI_LOG_LEVEL", ""); v != "" {
		cfg.Log.Level = strings.ToLower(v)
	}
	// AI Overrides
	if v := envOr("SHIORI_LMSTUDIO_BASE_URL", ""); v != "" {
		cfg.AI.LMStudioBaseURL = v
	}
	if v := envOr("SHIORI_LMSTUDIO_API_TOKEN", ""); v != "" {
		cfg.AI.Token = v
	}
	if v := envOr("SHIORI_MODEL_EXTRACT_TINY", ""); v != "" {
		cfg.AI.ModelTiny = v
	}
	if v := envOr("SHIORI_MODEL_EXTRACT_DEFAULT", ""); v != "" {
		cfg.AI.ModelDefault = v
	}
	if v := envOr("SHIORI_MODEL_EXTRACT_QUALITY", ""); v != "" {
		cfg.AI.ModelQuality = v
	}

	if flags.LogLevel != "" {
		cfg.Log.Level = strings.ToLower(flags.LogLevel)
	}

	if v := envOr("SHIORI_LOG_FORMAT", ""); v != "" {
		cfg.Log.Format = strings.ToLower(v)
	}
	if flags.LogFormat != "" {
		cfg.Log.Format = strings.ToLower(flags.LogFormat)
	}

	// Docker Profile Overrides
	if v := envOr("SHIORI_POSTGRES_URL", ""); v != "" {
		cfg.Postgres.URL = v
	}
	if v := envOr("SHIORI_VALKEY_ADDR", ""); v != "" {
		cfg.Valkey.Addr = v
	}
	if v := envOr("SHIORI_MINIO_ENDPOINT", ""); v != "" {
		cfg.MinIO.Endpoint = v
	}
	if v := envOr("SHIORI_MINIO_ACCESS_KEY", ""); v != "" {
		cfg.MinIO.AccessKey = v
	}
	if v := envOr("SHIORI_MINIO_SECRET_KEY", ""); v != "" {
		cfg.MinIO.SecretKey = v
	}
	if v := envOr("SHIORI_MINIO_BUCKET", ""); v != "" {
		cfg.MinIO.Bucket = v
	}
	if v := envOr("SHIORI_MINIO_USE_SSL", ""); v != "" {
		cfg.MinIO.UseSSL = v == "true"
	}

	return cfg, nil
}

// Validate checks the configuration for errors.
func (c Config) Validate() error {
	var errs []error

	switch c.Profile {
	case ProfilePortable, ProfileDocker:
		// valid
	default:
		errs = append(errs, fmt.Errorf("invalid profile %q: must be 'portable' or 'docker'", c.Profile))
	}

	if c.DataDir == "" {
		errs = append(errs, errors.New("data directory must not be empty"))
	}

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errs = append(errs, fmt.Errorf("invalid port %d: must be 1-65535", c.Server.Port))
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Log.Level] {
		errs = append(errs, fmt.Errorf("invalid log level %q: must be debug, info, warn, or error", c.Log.Level))
	}

	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[c.Log.Format] {
		errs = append(errs, fmt.Errorf("invalid log format %q: must be json or text", c.Log.Format))
	}

	return errors.Join(errs...)
}

// Flags holds command-line flag values parsed by the caller.
type Flags struct {
	Profile   string
	DataDir   string
	Host      string
	Port      int
	LogLevel  string
	LogFormat string
}

// --- helpers ---

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
