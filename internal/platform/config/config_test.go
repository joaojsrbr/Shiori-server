package config_test

import (
	"os"
	"testing"

	"github.com/joaojsr/shiori-server/internal/platform/config"
)

func TestDefaults(t *testing.T) {
	cfg := config.Defaults()

	if cfg.Profile != config.ProfilePortable {
		t.Errorf("default profile = %q, want %q", cfg.Profile, config.ProfilePortable)
	}
	if cfg.Server.Port != 9180 {
		t.Errorf("default port = %d, want 9180", cfg.Server.Port)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("default log level = %q, want %q", cfg.Log.Level, "info")
	}
	if cfg.Log.Format != "pretty" {
		t.Errorf("default log format = %q, want %q", cfg.Log.Format, "pretty")
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("SHIORI_PROFILE", "docker")
	t.Setenv("SHIORI_PORT", "8080")
	t.Setenv("SHIORI_LOG_LEVEL", "debug")
	t.Setenv("SHIORI_LOG_FORMAT", "text")

	cfg, err := config.Load(config.Flags{})
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Profile != config.ProfileDocker {
		t.Errorf("profile = %q, want %q", cfg.Profile, config.ProfileDocker)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("log level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.Format != "text" {
		t.Errorf("log format = %q, want %q", cfg.Log.Format, "text")
	}
}

func TestFlagsOverrideEnv(t *testing.T) {
	t.Setenv("SHIORI_PROFILE", "docker")
	t.Setenv("SHIORI_PORT", "8080")

	flags := config.Flags{
		Profile: "portable",
		Port:    9999,
	}

	cfg, err := config.Load(flags)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Profile != config.ProfilePortable {
		t.Errorf("profile = %q, want %q", cfg.Profile, config.ProfilePortable)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("port = %d, want 9999", cfg.Server.Port)
	}
}

func TestValidateInvalidProfile(t *testing.T) {
	cfg := config.Defaults()
	cfg.Profile = "invalid"

	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with invalid profile")
	}
}

func TestValidateInvalidPort(t *testing.T) {
	cfg := config.Defaults()
	cfg.DataDir = t.TempDir()
	cfg.Server.Port = 0

	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with port 0")
	}
}

func TestValidateInvalidLogLevel(t *testing.T) {
	cfg := config.Defaults()
	cfg.DataDir = t.TempDir()
	cfg.Log.Level = "verbose"

	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with invalid log level")
	}
}

func TestValidateValid(t *testing.T) {
	cfg := config.Defaults()
	cfg.DataDir = t.TempDir()

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

func TestDataDirFromExecutable(t *testing.T) {
	// Ensure no env var overrides
	os.Unsetenv("SHIORI_DATA_DIR")

	cfg, err := config.Load(config.Flags{})
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.DataDir == "" {
		t.Error("DataDir should be resolved from executable, got empty")
	}
}

func TestDataDirFromFlag(t *testing.T) {
	dir := t.TempDir()

	cfg, err := config.Load(config.Flags{DataDir: dir})
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.DataDir != dir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, dir)
	}
}

func TestServerAddr(t *testing.T) {
	cfg := config.Defaults()
	want := "127.0.0.1:9180"
	if got := cfg.Server.Addr(); got != want {
		t.Errorf("Addr() = %q, want %q", got, want)
	}
}
