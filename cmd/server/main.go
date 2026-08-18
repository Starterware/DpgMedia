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

	"github.com/mikael/dpgmedia/internal/api"
	"github.com/mikael/dpgmedia/internal/config"
	"github.com/mikael/dpgmedia/internal/logging"
	"github.com/mikael/dpgmedia/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	logger := logging.New(cfg.LogConfigOptions())
	slog.SetDefault(logger)

	if err := run(cfg, logger); err != nil {
		logger.Error("server stopped with error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(cfg *config.Config, logger *slog.Logger) error {
	messages, err := openStore(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := messages.Close(); err != nil {
			logger.Warn("failed to close message store", slog.Any("error", err))
		}
	}()

	handler := api.NewRouter(logger, api.Options{
		MaxBodyBytes: cfg.Server.MaxBodyBytes,
		Store:        messages,
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	srv.ErrorLog = slog.NewLogLogger(logger.With(slog.String("component", "http")).Handler(), slog.LevelWarn)

	errc := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.Any("config", cfg))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
			return
		}
		errc <- nil
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections",
			slog.Duration("timeout", cfg.Server.ShutdownTimeout))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
		return fmt.Errorf("http server shutdown failed: %w", err)
	}

	if err := <-errc; err != nil {
		return fmt.Errorf("http server failed: %w", err)
	}

	logger.Info("server stopped cleanly")
	return nil
}

func openStore(cfg *config.Config) (store.MessageStore, error) {
	switch driver := cfg.StoreDriver(); driver {
	case store.DriverLocal:
		return store.OpenLocal(store.LocalOptions{
			Path: cfg.Store.Path,
			TTL:  cfg.Store.TTL,
		})
	default:
		return nil, fmt.Errorf("unsupported store driver %q", driver)
	}
}
