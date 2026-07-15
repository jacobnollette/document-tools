// Command server runs the document tools API and web interface. Until the
// first-run installer has been completed it serves the setup wizard; after
// that (or immediately, when config already exists) it serves the full app.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"document-tools/internal/api"
	"document-tools/internal/auth"
	"document-tools/internal/config"
	"document-tools/internal/db"
	"document-tools/internal/ocr"
	"document-tools/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		os.Exit(1)
	}
}

// switchableHandler lets the installer swap the setup handler for the full
// application without restarting the process.
type switchableHandler struct {
	current atomic.Pointer[http.Handler]
}

func (s *switchableHandler) Set(h http.Handler) { s.current.Store(&h) }

func (s *switchableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	(*s.current.Load()).ServeHTTP(w, r)
}

func run() error {
	var (
		addr        = flag.String("addr", envOr("LISTEN_ADDR", ":8080"), "listen address")
		dataDir     = flag.String("data", envOr("DATA_DIR", "./data"), "directory for config and document files")
		webDist     = flag.String("web", envOr("WEB_DIST", ""), "directory of the built web app (empty = API only)")
		concurrency = flag.Int("ocr-workers", 2, "number of concurrent OCR workers")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	engine := &ocr.Pipeline{}
	if missing := engine.MissingTools(); len(missing) > 0 {
		logger.Warn("missing external tools; some processing will fail", "tools", missing)
	}

	var worker *ocr.Worker
	handler := &switchableHandler{}

	// startApp brings up the full application on an open database connection.
	startApp := func(cfg *config.Config, conn *sql.DB) error {
		st, err := store.New(conn, *dataDir)
		if err != nil {
			return err
		}
		authSvc := auth.New(conn, []byte(cfg.SessionSecret))

		worker = ocr.NewWorker(st, engine, logger, *concurrency)
		worker.Start(ctx)
		if err := worker.Recover(ctx); err != nil {
			logger.Error("recover pending documents", "error", err)
		}

		handler.Set(api.New(st, worker, authSvc, logger, *webDist).Handler())
		return nil
	}

	cfg, err := config.Load(*dataDir)
	if err != nil {
		return err
	}
	if cfg == nil {
		logger.Info("no configuration found — serving the setup wizard")
		handler.Set(api.NewSetup(*dataDir, logger, *webDist, startApp).Handler())
	} else {
		conn, err := db.Open(ctx, cfg.Database.DSN())
		if err != nil {
			return err
		}
		defer conn.Close()
		if err := db.Migrate(ctx, conn); err != nil {
			return fmt.Errorf("apply migrations: %w", err)
		}
		if err := startApp(cfg, conn); err != nil {
			return err
		}
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", *addr, "data", *dataDir)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if worker != nil {
		worker.Wait()
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
