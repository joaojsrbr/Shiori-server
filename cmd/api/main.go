// Shiori server — main entry point.
//
// Usage:
//
//	shiori-server serve [flags]
//	shiori-server version
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/go-chi/chi/v5"

	"github.com/joaojsr/shiori-server/api/openapi"
	"github.com/joaojsr/shiori-server/internal/ai"
	"github.com/joaojsr/shiori-server/internal/buildinfo"
	"github.com/joaojsr/shiori-server/internal/extraction"
	"github.com/joaojsr/shiori-server/internal/extraction/nuextract"
	"github.com/joaojsr/shiori-server/internal/jobs"
	"github.com/joaojsr/shiori-server/internal/library"
	libpostgres "github.com/joaojsr/shiori-server/internal/library/postgres"
	libsqlite "github.com/joaojsr/shiori-server/internal/library/sqlite"
	"github.com/joaojsr/shiori-server/internal/platform/ai/lmstudio"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
	"github.com/joaojsr/shiori-server/internal/platform/browser/chromedp"
	"github.com/joaojsr/shiori-server/internal/platform/config"
	"github.com/joaojsr/shiori-server/internal/platform/database"
	"github.com/joaojsr/shiori-server/internal/platform/database/postgres"
	"github.com/joaojsr/shiori-server/internal/platform/database/sqlite"
	"github.com/joaojsr/shiori-server/internal/platform/httpserver"
	"github.com/joaojsr/shiori-server/internal/platform/logging"
	"github.com/joaojsr/shiori-server/internal/platform/queue"
	"github.com/joaojsr/shiori-server/internal/platform/queue/sqlitequeue"
	"github.com/joaojsr/shiori-server/internal/platform/queue/valkeyqueue"
	"github.com/joaojsr/shiori-server/internal/platform/storage"
	"github.com/joaojsr/shiori-server/internal/platform/storage/localfs"
	"github.com/joaojsr/shiori-server/internal/platform/storage/s3fs"
	"github.com/joaojsr/shiori-server/internal/worker"
	"github.com/redis/go-redis/v9"
)

func main() {
	if len(os.Args) == 1 {
		// Default arguments if run directly without CLI flags
		os.Args = append(os.Args, "serve", "--profile", "portable", "--data-dir", "./data", "--port", "8080", "--log-level", "debug")
	}

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: shiori-server <command> [flags]")
		fmt.Fprintln(os.Stderr, "commands: serve, version")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Println(buildinfo.String())
		os.Exit(0)

	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		fmt.Fprintln(os.Stderr, "commands: serve, version")
		os.Exit(1)
	}
}

func runServe(args []string) error {
	// Parse flags
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var flags config.Flags
	fs.StringVar(&flags.Profile, "profile", "", "infrastructure profile (portable, docker)")
	fs.StringVar(&flags.DataDir, "data-dir", "", "base data directory")
	fs.StringVar(&flags.Host, "host", "", "listen host")
	fs.IntVar(&flags.Port, "port", 0, "listen port")
	fs.StringVar(&flags.LogLevel, "log-level", "", "log level (debug, info, warn, error)")
	fs.StringVar(&flags.LogFormat, "log-format", "", "log format (json, text)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// Load and validate config
	cfg, err := config.Load(flags)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Setup logging
	logger := logging.Setup(os.Stderr, cfg.Log.Level, cfg.Log.Format)
	logger.Info("starting shiori-server",
		"version", buildinfo.Version,
		"commit", buildinfo.Commit,
		"profile", cfg.Profile,
		"data_dir", cfg.DataDir,
	)

	// Initialize infrastructure providers
	var (
		dbProvider      database.Provider
		queueProvider   queue.Provider
		storageProvider storage.Provider
		browserProvider browser.Provider
		mediaRepo       library.MediaRepository
	)

	switch cfg.Profile {
	case config.ProfilePortable:
		// Ensure data directory exists
		dataFolder := filepath.Join(cfg.DataDir, "data")
		os.MkdirAll(dataFolder, 0755)

		// Database
		dbPath := filepath.Join(dataFolder, "shiori.db")
		dbProvider, err = sqlite.New(dbPath)
		if err != nil {
			return fmt.Errorf("initializing sqlite database: %w", err)
		}
		defer dbProvider.Close()

		// Run migrations
		if err := dbProvider.Migrate(context.Background()); err != nil {
			return fmt.Errorf("running migrations: %w", err)
		}

		// Queue
		queueProvider = sqlitequeue.New(dbProvider.DB())

		// Storage
		storageDir := filepath.Join(cfg.DataDir, "storage")
		storageProvider, err = localfs.New(storageDir)
		if err != nil {
			return fmt.Errorf("initializing localfs: %w", err)
		}

		// 4. Browser
		browserProvider = chromedp.New(filepath.Join(dataFolder, "browser-profiles"))

		// 5. Library Repositories
		mediaRepo = libsqlite.NewRepository(dbProvider.DB())

	case config.ProfileDocker:
		// 1. PostgreSQL Database
		dbProvider, err = postgres.New(cfg.Postgres.URL)
		if err != nil {
			return fmt.Errorf("initializing postgres database: %w", err)
		}
		defer dbProvider.Close()

		// Note: Migrations would ideally run via a separate job or init container.
		// For simplicity, we skip automatic migrations in this snippet,
		// but they can be triggered by calling postgres.Migrate.

		// 2. Valkey Queue
		rdb := redis.NewClient(&redis.Options{
			Addr: cfg.Valkey.Addr,
		})
		vQueue := valkeyqueue.New(rdb, "shiori:jobs", "shiori_workers")
		if err := vQueue.EnsureGroup(context.Background()); err != nil {
			logger.Warn("failed to create valkey consumer group (might exist)", "error", err)
		}
		queueProvider = vQueue

		// 3. MinIO Storage
		storageProvider, err = s3fs.New(
			cfg.MinIO.Endpoint,
			cfg.MinIO.AccessKey,
			cfg.MinIO.SecretKey,
			cfg.MinIO.Bucket,
			cfg.MinIO.UseSSL,
		)
		if err != nil {
			return fmt.Errorf("initializing minio storage: %w", err)
		}

		// 4. Browser (Worker later)
		// For now, no browser capability in API container.
		// Playwright worker will handle browser tasks.

		// 5. Library Repositories
		mediaRepo = libpostgres.NewRepository(dbProvider.DB())

	default:
		return fmt.Errorf("unknown profile: %s", cfg.Profile)
	}

	// Create HTTP server
	srv := httpserver.New(cfg.Server.Addr(), logger)
	srv.SetTimeouts(cfg.Server.ReadTimeout, cfg.Server.WriteTimeout, cfg.Server.IdleTimeout)

	// Register Challenge Manager
	challengeManager := browser.NewChallengeManager()
	challengeHandler := jobs.NewChallengeHandler(browserProvider, challengeManager)

	// Register API routes
	libHandler := library.NewHandler(mediaRepo)
	aiHandler := ai.NewHandler(cfg.AI)
	jobsHandler := jobs.NewHandler(queueProvider)

	srv.Router().Route("/api/v1", func(r chi.Router) {
		r.Get("/openapi.yaml", openapi.Handler())
		r.Get("/capabilities", handleCapabilities(cfg, dbProvider, queueProvider, storageProvider, browserProvider))

		libHandler.RegisterRoutes(r)
		aiHandler.RegisterRoutes(r)
		jobsHandler.RegisterRoutes(r)
		challengeHandler.RegisterRoutes(r)
	})

	// Debug-only endpoints: run extraction synchronously and return AI output.
	// Only registered when log level is "debug".
	if cfg.Log.Level == "debug" && browserProvider != nil {
		lmClient := lmstudio.NewClient(cfg.AI.LMStudioBaseURL, cfg.AI.Token)
		debugExtProvider, err := nuextract.New(lmClient, cfg.AI.ModelDefault, cfg.AI.TemplatePath, cfg.AI.MaxContentBytes)
		if err != nil {
			logger.Error("failed to init debug ai provider", "err", err)
		} else {
			srv.Router().Route("/api/v1/debug", func(r chi.Router) {
				r.Post("/extract", jobs.HandleDebugExtract(browserProvider, debugExtProvider, mediaRepo, challengeManager))
			})
			logger.Warn("debug endpoints enabled", "path", "/api/v1/debug/extract")
		}
	}

	// Mark ready after initialization is complete.
	// In the future, migrations and other init steps happen before this.
	srv.MarkReady()

	// 6. Background Worker Pool
	workerPool := worker.New(queueProvider, logger, 3) // Concurrency 3

	// Setup Extraction Provider
	var extProvider extraction.Provider
	if cfg.AI.LMStudioBaseURL != "" {
		lmClient := lmstudio.NewClient(cfg.AI.LMStudioBaseURL, cfg.AI.Token)
		extProvider, err = nuextract.New(lmClient, cfg.AI.ModelDefault, cfg.AI.TemplatePath, cfg.AI.MaxContentBytes)
		if err != nil {
			logger.Error("failed to init ai provider", "err", err)
			extProvider = nil
		}
	}

	extractHandler := jobs.NewExtractHandler(browserProvider, extProvider, mediaRepo, challengeManager)
	workerPool.Register("extract_media", extractHandler)

	// Graceful shutdown context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start worker pool in background
	workerDone := make(chan struct{})
	go func() {
		workerPool.Start(ctx)
		close(workerDone)
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		logger.Info("received shutdown signal")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	// Wait for background workers to finish their current jobs before closing connections (like DB)
	<-workerDone

	logger.Info("server stopped gracefully")
	return nil
}

// handleCapabilities returns the current server capabilities.
func handleCapabilities(cfg config.Config, db database.Provider, q queue.Provider, st storage.Provider, bp browser.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type capability struct {
			Name      string `json:"name"`
			Available bool   `json:"available"`
		}
		caps := struct {
			Profile      string       `json:"profile"`
			Capabilities []capability `json:"capabilities"`
		}{
			Profile: string(cfg.Profile),
			Capabilities: []capability{
				{Name: "database", Available: db != nil},
				{Name: "queue", Available: q != nil},
				{Name: "storage", Available: st != nil},
				{Name: "browser", Available: bp != nil && bp.IsAvailable()},
				{Name: "ai_extract", Available: cfg.AI.LMStudioBaseURL != ""},
			},
		}
		httpserver.RespondJSON(w, http.StatusOK, caps)
	}
}
