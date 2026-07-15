// Command server runs the document tools API and web interface.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"document-tools/internal/api"
	"document-tools/internal/ocr"
	"document-tools/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		addr        = flag.String("addr", envOr("LISTEN_ADDR", ":8080"), "listen address")
		dataDir     = flag.String("data", envOr("DATA_DIR", "./data"), "directory for document storage")
		webDist     = flag.String("web", envOr("WEB_DIST", ""), "directory of the built web app (empty = API only)")
		concurrency = flag.Int("ocr-workers", 2, "number of concurrent OCR workers")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	st, err := store.New(*dataDir)
	if err != nil {
		return err
	}

	engine := &ocr.Tesseract{}
	if !engine.Available() {
		logger.Warn("tesseract binary not found; uploads will fail OCR until it is installed")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	worker := ocr.NewWorker(st, engine, logger, *concurrency)
	worker.Start(ctx)
	if err := worker.Recover(); err != nil {
		logger.Error("recover pending documents", "error", err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.New(st, worker, logger, *webDist).Handler(),
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
	worker.Wait()
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
